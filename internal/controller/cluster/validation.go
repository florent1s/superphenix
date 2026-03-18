package cluster

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

var (
	// supportedPreviousVersions maps the target version to a semver constraint for the previous version.
	// For example, to upgrade to "1.1.0", the current version might need to be ">= 1.0.0".
	supportedPreviousVersions = map[string]string{
		"1.1.0": ">= 1.0.0",
		"1.2.0": ">= 1.1.0",
		"2.0.0": ">= 1.2.0",
	}
)

// validate runs all the validation logic for the cluster.
func (r *Reconciler) validate(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	log := logf.FromContext(ctx)

	// Validate version upgrade/downgrade
	if err := r.validateUpgradePath(ctx, cluster); err != nil {
		log.Error(err, "Validation failed")
		r.updateStatusWithPhase(ctx, cluster, "Ready", metav1.ConditionFalse, operatorv1alpha1.ReasonInvalidVersion, err.Error(), "Error")
		return err
	}

	return nil
}

// validateUpgradePath ensures the upgrade path is possible and safe.
func (r *Reconciler) validateUpgradePath(ctx context.Context, cluster *operatorv1alpha1.Cluster) error {
	specVersion := cluster.Spec.Version
	statusVersion := cluster.Status.CurrentVersion

	// If the live cluster is already at the correct version, do not do anything
	if specVersion == statusVersion {
		return nil
	}

	// If no version is specified, or the running cluster isn't installed yet (no version), we can't validate.
	if specVersion == "" || statusVersion == "" {
		return nil
	}

	// Make sure the new version is a valid semver
	newVersion, err := semver.NewVersion(specVersion)
	if err != nil {
		return fmt.Errorf("invalid version format in spec (%q): %w", specVersion, err)
	}

	// Make sure the old version is a valid semver
	oldVersion, err := semver.NewVersion(statusVersion)
	if err != nil {
		return fmt.Errorf("invalid version format in status (%q): %w", statusVersion, err)
	}

	// Check if we have a specific constraint for this target version
	constraintString, ok := supportedPreviousVersions[newVersion.String()]
	if !ok {
		return fmt.Errorf("upgrade to %s is not supported (no entry in supported versions table)", specVersion)
	}

	constraint, err := semver.NewConstraint(constraintString)
	if err != nil {
		return fmt.Errorf("invalid semver constraint for version %s: %w", specVersion, err)
	}

	if !constraint.Check(oldVersion) {
		return fmt.Errorf("upgrade to %s is not supported from version %s (must satisfy: %s)", specVersion, cluster.Status.CurrentVersion, constraintString)
	}

	return nil
}
