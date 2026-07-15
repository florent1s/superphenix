package gc

import (
	"argo-controller/internal/gc/resources/appproject"
	"argo-controller/internal/gc/resources/argoapp"
	"argo-controller/internal/gc/utils"
	internalUtils "argo-controller/internal/utils"
	"argo-controller/pkg/config"
	"context"
	"fmt"
	"net/http"
	"time"
	httpError "utils/error"
	gcLog "utils/log"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

// MarkForDeletion
//
//	@Summary		Mark for deletion
//	@Description	Mark all argo resources inside a project for deletion
//	@Tags			v1, GC
//	@Produce		json
//	@Param			orgId		path	string	true	"Organization ID"
//	@Param			projectId	path	string	true	"Project ID"
//	@Success		200
//	@Failure		500
//	@Router			/v1/{orgId}/{projectId}/mark [get]
//	@Security		Bearer[]
func MarkForDeletion(w http.ResponseWriter, r *http.Request) {
	ns := internalUtils.GetRequestNamespace(r)
	if err := deletionLabeling(r.Context(), ns); err != nil {
		log.Err(err).Msg("Failed to mark all resources")
		httpError.Http(w, r, http.StatusInternalServerError).Msg("Failed to mark all resources")
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func deletionLabeling(ctx context.Context, namespaceValue string) error {
	processId := uuid.New().String()
	ctx = context.WithValue(ctx, gcLog.ProcessIdKey, fmt.Sprintf("marking-%s-%s", namespaceValue, processId))
	logger := gcLog.GetProcessLogger(ctx)

	deletionTimestamp := time.Now().Add(config.Global.GarbageCollection.Delay)
	deletionTimestampStr := deletionTimestamp.Format(utils.TimestampFormat)

	logger.Info().Msgf("Labeling started at : %s", time.Now().String())

	cleaners := []utils.Cleaner{
		&argoapp.Cleaner{ResourceType: "Argo Application", Logger: logger},
		&appproject.Cleaner{ResourceType: "App Project", Logger: logger},
	}

	errWg, ctxWg := errgroup.WithContext(ctx)

	for _, cleaner := range cleaners {
		errWg.Go(func() error {
			if err := cleaner.Mark(ctxWg, namespaceValue, deletionTimestampStr); err != nil {
				logger.Error().Err(err).Msg(cleaner.ErrorMessage())
				return fmt.Errorf("failed to mark item")
			}
			return nil
		})
	}

	err := errWg.Wait()

	if err != nil {
		logger.Err(err).Msg("Labeling failed")
		return err
	} else {
		logger.Info().Msgf("Labeling finished at : %s", time.Now().String())
	}
	return nil
}
