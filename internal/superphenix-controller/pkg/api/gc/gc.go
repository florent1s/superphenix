package gc

import (
	"context"
	"fmt"
	"net/http"
	"superphenix-controller/internal/gc/alerting"
	"superphenix-controller/internal/gc/resources/k8s/namespace"
	"superphenix-controller/internal/gc/resources/k8s/netpol"
	"superphenix-controller/internal/gc/resources/k8s/pvc"
	"superphenix-controller/internal/gc/resources/k8s/ssh"
	"superphenix-controller/internal/gc/resources/kubeovn/eip"
	"superphenix-controller/internal/gc/resources/kubeovn/loadbalancer"
	"superphenix-controller/internal/gc/resources/kubeovn/subnet"
	"superphenix-controller/internal/gc/resources/kubeovn/vpc"
	"superphenix-controller/internal/gc/resources/kubevirt/datavolume"
	"superphenix-controller/internal/gc/resources/kubevirt/vm"
	"superphenix-controller/internal/gc/resources/kubevirt/vmSnapshot"
	"superphenix-controller/internal/gc/resources/kubevirt/volumeSnapshot"
	"superphenix-controller/internal/gc/utils"
	apiUtils "superphenix-controller/pkg/api/utils"
	"superphenix-controller/pkg/config"
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
//	@Description	Mark all resources inside a project for deletion
//	@Tags			v1, GC
//	@Produce		json
//	@Param			orgId		path	string	true	"Organization ID"
//	@Param			projectId	path	string	true	"Project ID"
//	@Success		200
//	@Failure		500
//	@Router			/{orgId}/{projectId}/mark [get]
//	@Security		Bearer[OrganizationProjectManagement]
func MarkForDeletion(w http.ResponseWriter, r *http.Request) {
	ns := apiUtils.GetRequestNamespace(r)
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
		&vm.Cleaner{ResourceType: "VirtualMachine", Logger: logger},
		&vmSnapshot.Cleaner{ResourceType: "VirtualMachineSnapshot", Logger: logger},
		&eip.Cleaner{ResourceType: "EIP (FIP + SNAT + DNAT)", Logger: logger},
		&ssh.Cleaner{ResourceType: "SSH Key", Logger: logger},
		&loadbalancer.Cleaner{ResourceType: "Load Balancer", Logger: logger},
		&netpol.Cleaner{ResourceType: "Network Policy", Logger: logger},
		&subnet.Cleaner{ResourceType: "Subnet (NAD + NatGW)", Logger: logger},
		&datavolume.Cleaner{ResourceType: "DataVolume", Logger: logger},
		&volumeSnapshot.Cleaner{ResourceType: "Volume Snapshot", Logger: logger},
		&pvc.Cleaner{ResourceType: "PVC", Logger: logger},
		&vpc.Cleaner{ResourceType: "VPC", Logger: logger},
		&namespace.Cleaner{ResourceType: "Namespace", Logger: logger},
	}

	// Wait Group handling error, when a error is raised, cancel the context
	errWg, ctxWg := errgroup.WithContext(ctx)

	for _, cleaner := range cleaners {
		errWg.Go(func() error {
			if err := cleaner.Mark(ctxWg, namespaceValue, deletionTimestampStr); err != nil {
				logger.Error().Err(err).Msg(cleaner.ErrorMessage())
				cleaner.RaiseListingFailureAlert(ctxWg, alerting.ProcessLabeling)
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
