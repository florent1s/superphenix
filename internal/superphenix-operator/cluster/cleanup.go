package cluster

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

func (r *Reconciler) cleanupCluster(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	log := logf.FromContext(ctx)
	log.Info("Cleaning up external resources for Cluster", "Name", cluster.Name)

	// Check for root application
	rootApp, err := r.fetchApplication(ctx, cluster.Name)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check root application: %w", err)
	}

	// Check for sub-applications
	subApps, err := r.listSubApplications(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to list sub-applications: %w", err)
	}

	if rootApp != nil || len(subApps) > 0 {
		var appNames []string
		if rootApp != nil {
			appNames = append(appNames, rootApp.GetName())
		}
		for _, app := range subApps {
			appNames = append(appNames, app.GetName())
		}

		log.Info("Waiting for ArgoCD applications to be deleted", "cluster", cluster.Name, "applications", appNames)
		return fmt.Errorf("waiting for %d ArgoCD applications to be deleted", len(appNames))
	}

	log.Info("All ArgoCD applications deleted for cluster", "Name", cluster.Name)
	// Remove the cluster entry from the shared ConfigMap
	if err := r.removeClusterFromConfigMap(ctx, cluster); err != nil {
		return fmt.Errorf("failed to remove cluster from configmap: %w", err)
	}
	return nil
}
