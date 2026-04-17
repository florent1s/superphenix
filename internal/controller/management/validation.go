package management

import (
	"context"
	"fmt"

	"github.com/super-phenix/superphenix/api/operator/v1alpha1"
	"github.com/super-phenix/superphenix/internal/controller/version"
)

// validateManagementUpgrade verifies that the management chart can be upgraded to the target version
// and that all existing clusters are compatible with this new version.
func (r *Reconciler) validateManagementUpgrade(ctx context.Context, targetVersion string) error {
	if err := r.validateManagementUpgradePath(ctx, targetVersion); err != nil {
		return err
	}
	return r.validateClustersCompatibility(ctx, targetVersion)
}

// validateManagementUpgradePath ensures that the upgrade from the current version to the target version is supported.
func (r *Reconciler) validateManagementUpgradePath(ctx context.Context, targetVersion string) error {
	// Try to get the current version from the existing ArgoCD Application.
	currentVersion, err := version.GetCurrentManagementVersion(ctx, r, r.OperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to get current management version: %w", err)
	}

	return version.IsManagementUpgradeSupported(currentVersion, targetVersion)
}

// validateClustersCompatibility ensures all Cluster resources are at a version supported by the target management version.
func (r *Reconciler) validateClustersCompatibility(ctx context.Context, managementVersion string) error {
	clusterList := &v1alpha1.ClusterList{}
	if err := r.List(ctx, clusterList); err != nil {
		return fmt.Errorf("failed to list clusters for compatibility check: %w", err)
	}

	for _, cluster := range clusterList.Items {
		clusterVersionStr := cluster.Status.CurrentVersion
		if clusterVersionStr == "" {
			// Fall back to spec version when the cluster has not been deployed yet.
			clusterVersionStr = cluster.Spec.Version
		}

		if err := version.IsClusterCompatibleWithManagement(clusterVersionStr, managementVersion); err != nil {
			return fmt.Errorf("cluster %s: %w", cluster.Name, err)
		}
	}

	return nil
}
