package management

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

var (
	// supportedManagementUpgradePaths maps the target version of superphenix-management to a semver constraint for the previous version.
	supportedManagementUpgradePaths = map[string]string{
		"0.0.0-latest": ">= 0.0.0",
		"1.0.0":        ">= 0.0.0",
		"1.1.0":        ">= 1.0.0",
	}

	// supportedClusterVersionsForManagement defines which Cluster versions are supported by a specific Management version.
	// This ensures that the management chart is compatible with all deployed clusters.
	supportedClusterVersionsForManagement = map[string]string{
		"0.0.0-latest": ">= 0.0.0",
		"1.0.0":        ">= 1.0.0",
		"1.1.0":        ">= 1.0.0",
	}
)

// validateManagementUpgrade verifies that the management chart can be upgraded to the target version
// and that all existing clusters are compatible with this new version.
func (r *Reconciler) validateManagementUpgrade(ctx context.Context, targetVersion string) error {
	// Validate the management upgrade path
	if err := r.validateManagementUpgradePath(ctx, targetVersion); err != nil {
		return err
	}

	// Validate compatibility with all deployed Cluster CRDs
	if err := r.validateClustersCompatibility(ctx, targetVersion); err != nil {
		return err
	}

	return nil
}

// validateManagementUpgradePath ensures that the upgrade from the current version to the target version is supported.
func (r *Reconciler) validateManagementUpgradePath(ctx context.Context, targetVersion string) error {
	constraintString, ok := supportedManagementUpgradePaths[targetVersion]
	if !ok {
		// If the target version is not in the map, we assume it's a new version without specific constraints yet,
		// or we don't want to block it.
		return nil
	}

	constraint, err := semver.NewConstraint(constraintString)
	if err != nil {
		return fmt.Errorf("invalid semver constraint for management version %s: %w", targetVersion, err)
	}

	// Try to get the current version from the existing ArgoCD Application.
	currentVersion, err := r.getCurrentManagementVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current management version: %w", err)
	}

	if currentVersion == "" {
		// No current version found, assuming first install.
		return nil
	}

	if currentVersion == "0.0.0-latest" || targetVersion == "0.0.0-latest" {
		// Skip validation for latest versions.
		return nil
	}

	cv, err := semver.NewVersion(currentVersion)
	if err != nil {
		return fmt.Errorf("current management version %q is invalid: %w", currentVersion, err)
	}

	if !constraint.Check(cv) {
		return fmt.Errorf("management upgrade from %s to %s is not supported (required: %s)",
			currentVersion, targetVersion, constraintString)
	}

	return nil
}

// getCurrentManagementVersion attempts to retrieve the current version of the management chart
// by looking at the existing ArgoCD Application.
func (r *Reconciler) getCurrentManagementVersion(ctx context.Context) (string, error) {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	err := r.Get(ctx, types.NamespacedName{Name: ManagementSuperphenixName, Namespace: r.OperatorNamespace}, app)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}

	// The version is stored in spec.source.targetRevision
	version, found, err := unstructured.NestedString(app.Object, "spec", "source", "targetRevision")
	if err != nil || !found {
		return "", nil
	}

	return version, nil
}

// validateClustersCompatibility ensures all Cluster resources are at a version supported by the target management version.
func (r *Reconciler) validateClustersCompatibility(ctx context.Context, managementVersion string) error {
	constraintString, ok := supportedClusterVersionsForManagement[managementVersion]
	if !ok {
		return fmt.Errorf("management version %s is not in the supported versions table", managementVersion)
	}

	constraint, err := semver.NewConstraint(constraintString)
	if err != nil {
		return fmt.Errorf("invalid semver constraint for management version %s: %w", managementVersion, err)
	}

	clusterList := &operatorv1alpha1.ClusterList{}
	if err := r.List(ctx, clusterList); err != nil {
		return fmt.Errorf("failed to list clusters for compatibility check: %w", err)
	}

	for _, cluster := range clusterList.Items {
		clusterVersionStr := cluster.Status.CurrentVersion
		if clusterVersionStr == "" {
			// Cluster not fully deployed yet, or doesn't have a version in status.
			// We check the spec version as a fallback.
			clusterVersionStr = cluster.Spec.Version
		}

		if clusterVersionStr == "" || clusterVersionStr == "0.0.0-latest" {
			// Skip validation for latest or unversioned clusters if constraint allows
			if clusterVersionStr == "0.0.0-latest" {
				v, _ := semver.NewVersion("0.0.0")
				if !constraint.Check(v) {
					return fmt.Errorf("cluster %s version %s is not supported by management version %s (required: %s)",
						cluster.Name, clusterVersionStr, managementVersion, constraintString)
				}
			}
			continue
		}

		cv, err := semver.NewVersion(clusterVersionStr)
		if err != nil {
			return fmt.Errorf("cluster %s has invalid version %q: %w", cluster.Name, clusterVersionStr, err)
		}

		if !constraint.Check(cv) {
			return fmt.Errorf("cluster %s version %s is not supported by management version %s (required: %s)",
				cluster.Name, clusterVersionStr, managementVersion, constraintString)
		}
	}

	return nil
}
