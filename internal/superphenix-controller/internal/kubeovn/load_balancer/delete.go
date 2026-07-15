package loadBalancer

import (
	"context"
	"superphenix-controller/internal/utils"
	k8s "superphenix-controller/pkg/config"
	logger "utils/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func DeleteLoadBalancer(ctx context.Context, namespace, name string) error {
	log := logger.GetLogger(ctx)

	lbToDelete, err := k8s.KubeOvnClient.KubeovnV1().SwitchLBRules().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Err(err).Str("name", name).Msg("Failed to get load balancer")
		return err
	}

	if err := utils.CheckProjectLabel(lbToDelete, namespace); err != nil {
		log.Warn().Str("namespace", namespace).Str("name", name).Msg("LoadBalancer access denied")
		return err
	}

	if err := utils.IsEditAllowed(lbToDelete.GetLabels()); err != nil {
		log.Error().Err(err).Str("name", name).Msg("Edit not allowed on this resource")
		return err
	}

	err = k8s.KubeOvnClient.KubeovnV1().SwitchLBRules().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		log.Err(err).Str("name", name).Msg("Failed to remove load balancer")
		return err
	}

	return nil
}
