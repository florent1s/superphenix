package cluster

import (
	"context"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

func (r *Reconciler) cleanupCluster(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	log := logf.FromContext(ctx)
	log.Info("Cleaning up external resources for Cluster", "Name", cluster.Name)

	return nil
}
