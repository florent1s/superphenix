package kubevirt

import (
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	httpError "github.com/super-phenix/superphenix/pkg/utils/error"
	logger "github.com/super-phenix/superphenix/pkg/utils/log"

	"github.com/super-phenix/superphenix/internal/superphenix-controller/pkg/api/utils"
	"github.com/super-phenix/superphenix/internal/superphenix-controller/pkg/config"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "kubevirt.io/client-go/kubevirt/typed/core/v1"
)

const (
	writeWait         = 10 * time.Second
	pongWait          = 60 * time.Second
	pingPeriod        = (pongWait * 9) / 10
	serialMaxMsgSize  = 8192
	serialTimeout     = 5
	serialReadBufSize = 4096
)

var serialUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// wsReader is an interface for reading websocket messages directly.
// The KubeVirt AsConn() wraps the websocket with a binaryReader that only
// handles BinaryMessage frames. Serial consoles send TextMessage frames,
// so we need direct access to ReadMessage() on the underlying websocket.
type wsReader interface {
	ReadMessage() (messageType int, p []byte, err error)
}

// serialConsoleWS bridges a browser WebSocket to a KubeVirt serial console.
// It reads from the KubeVirt websocket using ReadMessage() directly (via the
// promoted method on the net.Conn returned by AsConn()) to handle both text
// and binary frames. KubeVirt serial console sends text frames, which the
// default AsConn().Read() silently discards.
// Binary messages from the browser (e.g. resize events) are skipped since
// serial consoles do not support terminal resize.
//
//	@Summary		Websocket to Serial
//	@Description	Websocket to Serial Console of an instance
//	@Tags			v1, Instance
//	@Accept			json
//	@Produce		plain
//	@Param			orgId		path	string	true	"Organization ID"
//	@Param			projectId	path	string	true	"Project ID"
//	@Param			effectiveId	path	string	true	"Instance Effective ID"
//	@Success		101
//	@Failure		400
//	@Failure		404
//	@Router			/{orgId}/{projectId}/instance/{effectiveId}/serial [get]
func serialConsoleWS(w http.ResponseWriter, r *http.Request, namespace, name string) {
	log.Info().Str("namespace", namespace).Str("name", name).Msg("[serial] Opening serial console connection")

	stream, err := config.VirtClient.VirtualMachineInstance(namespace).SerialConsole(name, &v1.SerialConsoleOptions{
		ConnectionTimeout: time.Duration(serialTimeout) * time.Minute,
	})
	if err != nil {
		log.Error().Err(err).Msg("[serial] Failed to open serial console")
		http.Error(w, "Failed to open serial console", http.StatusInternalServerError)
		return
	}
	log.Info().Msg("[serial] KubeVirt serial console stream obtained")

	serialConn := stream.AsConn()
	defer serialConn.Close()

	// The net.Conn from AsConn() embeds *websocket.Conn, so ReadMessage() is
	// promoted. We need it because the default Read() only handles binary
	// frames, but serial console sends text frames.
	kvWSReader, ok := serialConn.(wsReader)
	if !ok {
		log.Error().Msg("[serial] KubeVirt conn does not support ReadMessage(); falling back will not work for text frames")
		http.Error(w, "Incompatible KubeVirt stream", http.StatusInternalServerError)
		return
	}

	browserWS, err := serialUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("[serial] Failed to upgrade browser WebSocket")
		return
	}
	defer browserWS.Close()
	log.Info().Msg("[serial] Browser WebSocket upgraded successfully")

	browserWS.SetReadLimit(serialMaxMsgSize)
	browserWS.SetReadDeadline(time.Now().Add(pongWait))
	browserWS.SetPongHandler(func(string) error {
		browserWS.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	var wsMu sync.Mutex
	done := make(chan struct{})
	var closeOnce sync.Once
	closeDone := func() { closeOnce.Do(func() { close(done) }) }

	// Ping routine — keeps the browser WebSocket alive
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				wsMu.Lock()
				err := browserWS.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait))
				wsMu.Unlock()
				if err != nil {
					log.Warn().Err(err).Msg("[serial] Ping failed")
					closeDone()
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Browser → KubeVirt: read from browser WS, write to serial conn
	go func() {
		defer func() {
			closeDone()
			serialConn.Close()
		}()
		log.Info().Msg("[serial] Browser→KubeVirt goroutine started")
		for {
			msgType, data, err := browserWS.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, io.EOF) {
					log.Info().Msg("[serial] Browser WebSocket closed normally")
				} else {
					log.Warn().Err(err).Msg("[serial] Browser WebSocket read error")
				}
				return
			}

			// Skip binary messages (resize events) — serial consoles do not support resize
			if msgType != websocket.TextMessage {
				log.Debug().Int("type", msgType).Msg("[serial] Skipping non-text browser message (e.g. resize)")
				continue
			}

			if _, err := serialConn.Write(data); err != nil {
				log.Error().Err(err).Msg("[serial] Failed to write to KubeVirt serial conn")
				return
			}
		}
	}()

	// KubeVirt → Browser: read from KubeVirt websocket, write to browser WS.
	// We use ReadMessage() directly instead of Read() because the KubeVirt
	// serial console sends text frames, and the binaryReader inside AsConn()
	// silently discards non-binary frames.
	log.Info().Msg("[serial] KubeVirt→Browser loop started")
	for {
		msgType, data, err := kvWSReader.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, io.EOF) {
				log.Info().Msg("[serial] KubeVirt stream closed")
			} else {
				log.Warn().Err(err).Msg("[serial] KubeVirt read error")
			}
			closeDone()
			return
		}

		if msgType == websocket.CloseMessage {
			log.Info().Msg("[serial] KubeVirt sent close frame")
			closeDone()
			return
		}

		if len(data) == 0 {
			continue
		}

		wsMu.Lock()
		browserWS.SetWriteDeadline(time.Now().Add(writeWait))
		err = browserWS.WriteMessage(msgType, data)
		wsMu.Unlock()
		if err != nil {
			log.Error().Err(err).Msg("[serial] Failed to write to browser WebSocket")
			closeDone()
			return
		}
	}
}

func SerialEndpoint(router chi.Router) {
	router.Get(baseVMEndpoint+"/{effectiveId}/serial", func(w http.ResponseWriter, r *http.Request) {
		l := logger.GetLogger(r.Context())
		namespace := utils.GetRequestNamespace(r)
		effectiveId := chi.URLParam(r, "effectiveId")
		if effectiveId == "" {
			l.Error().Ctx(r.Context()).Msg("no Resource Effective Id provided")
			httpError.Http(w, r, http.StatusBadRequest).Msg("no Resource Effective Id provided")
			return
		}

		_, err := config.VirtClient.VirtualMachineInstance(namespace).Get(r.Context(), effectiveId, k8smetav1.GetOptions{})
		if err != nil {
			l.Err(err).Msg("Failed to find the vmi")
			httpError.Http(w, r, http.StatusNotFound).Msg(http.StatusText(http.StatusNotFound))
			return
		}
		l.Info().Msgf("Asking serial console connection from : %s for [%s, %s]", r.URL.Path, namespace, effectiveId)
		serialConsoleWS(w, r, namespace, effectiveId)
	})
}
