package version

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ManagementAppName is the ArgoCD Application name for the management stack.
	ManagementAppName = "management"
)

var (
	// ClusterLabel is the label used to identify the cluster in ArgoCD.
	ClusterLabel = "operator.superphenix.net/clusterName"

	// ManagedLabel identifies Applications managed by the Superphenix operator
	// (root cluster Apps, superphenix-system child Apps, and management Apps).
	// It is used to scope the Application informer.
	ManagedLabel = "operator.superphenix.net/managed"

	// RootApplicationLabel is the label used to identify root applications in ArgoCD.
	RootApplicationLabel = "operator.superphenix.net/root"
)

var (
	// MinOperatorVersion is the minimum version the operator must be in before upgrade.
	MinOperatorVersion = "0.0.0"

	// MinClusterVersion is the minimum version of the cluster supported by the operator.
	MinClusterVersion = "0.0.0"

	// MaxClusterVersion is the ceiling for supported cluster versions (exclusive).
	MaxClusterVersion = "999.999.999"

	// OperatorVersion is the version of the running operator. Overridden at build time with -ldflags.
	OperatorVersion = "dev"
)

// IsOperatorUpgradeSupported checks if upgrading the operator from current to target version is supported.
//
// Validation process:
// 1. Skip check if current version is empty or "0.0.0" (new installation).
// 2. Skip check if target version is "0.0.0" (usually representing 'latest' or 'dev').
// 3. Verify that the current version satisfies the minimum version requirement (MinOperatorVersion).
func IsOperatorUpgradeSupported(current, target string) error {
	if current == "" || current == "0.0.0" || target == "0.0.0" {
		return nil
	}

	constraint := ">= " + MinOperatorVersion
	ok, err := checkConstraint(current, constraint)
	if err != nil {
		return fmt.Errorf("management upgrade from %s to %s is not supported: %w", current, target, err)
	}
	if !ok {
		return fmt.Errorf("management upgrade from %s to %s is not supported (must satisfy: %s)",
			current, target, constraint)
	}
	return nil
}

// IsClusterUpgradeSupported checks if upgrading a cluster from current to target is supported.
//
// Validation process:
// 1. Skip check if versions are identical or empty.
// 2. Skip check if the current/target version is "0.0.0".
// 3. Verify that both current and target versions are within the supported range [MinClusterVersion, MaxClusterVersion[.
func IsClusterUpgradeSupported(current, target string) error {
	if current == "" || target == "" || current == target {
		return nil
	}

	if current == "0.0.0" || target == "0.0.0" {
		return nil
	}

	constraint := fmt.Sprintf(">= %s, < %s", MinClusterVersion, MaxClusterVersion)

	// Validate current version is still supported
	ok, err := checkConstraint(current, constraint)
	if err != nil {
		return fmt.Errorf("cluster upgrade from %s to %s is not supported: %w", current, target, err)
	}
	if !ok {
		return fmt.Errorf("cluster upgrade from %s to %s is not supported: current version %s is outside supported range (%s)",
			current, target, current, constraint)
	}

	// Validate target version is supported
	ok, err = checkConstraint(target, constraint)
	if err != nil {
		return fmt.Errorf("cluster upgrade from %s to %s is not supported: %w", current, target, err)
	}
	if !ok {
		return fmt.Errorf("cluster upgrade from %s to %s is not supported: target version %s is outside supported range (%s)",
			current, target, target, constraint)
	}

	return nil
}

// IsClusterCompatibleWithOperator checks if a cluster version is supported by an operator version.
//
// Validation process:
// 1. Skip check if management version/cluster version is empty (unknown compatibility).
// 2. Skip check if management version/cluster version is "0.0.0".
// 3. Verify that the cluster version is within the supported range [MinClusterVersion, MaxClusterVersion[.
func IsClusterCompatibleWithOperator(clusterVersion, managementVersion string) error {
	if managementVersion == "" || managementVersion == "0.0.0" || clusterVersion == "" || clusterVersion == "0.0.0" {
		return nil
	}

	constraint := fmt.Sprintf(">= %s, < %s", MinClusterVersion, MaxClusterVersion)
	ok, err := checkConstraint(clusterVersion, constraint)
	if err != nil {
		return fmt.Errorf("cluster compatibility check failed: %w", err)
	}

	if !ok {
		return fmt.Errorf("cluster version %s is not supported by management version %s (required: %s)",
			clusterVersion, managementVersion, constraint)
	}

	return nil
}

// GetCurrentManagementVersion attempts to retrieve the current version of the management chart
// by looking at the existing ArgoCD Application in the given namespace.
// Returns an empty string if the application is not found or the version cannot be determined.
func GetCurrentManagementVersion(ctx context.Context, c client.Reader, operatorNamespace string) (string, error) {
	return GetApplicationVersion(ctx, c, ManagementAppName, operatorNamespace)
}

// GetCurrentClusterVersion attempts to retrieve the current version of the cluster
// by looking at its root ArgoCD Application.
// Returns an empty string if the application is not found or the version cannot be determined.
func GetCurrentClusterVersion(ctx context.Context, c client.Reader, clusterName, operatorNamespace string) (string, error) {
	return GetApplicationVersion(ctx, c, clusterName, operatorNamespace)
}

// GetApplicationVersion attempts to retrieve the version of a given ArgoCD Application.
func GetApplicationVersion(ctx context.Context, c client.Reader, name, namespace string) (string, error) {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, app)
	if err != nil {
		if errors.IsNotFound(err) || meta.IsNoMatchError(err) {
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

// checkConstraint parses a version string and checks it against a semver constraint.
// It returns true if the version satisfies the constraint, and an error if parsing fails.
func checkConstraint(versionString, constraintString string) (bool, error) {
	v, err := semver.NewVersion(versionString)
	if err != nil {
		return false, fmt.Errorf("invalid version %q: %w", versionString, err)
	}

	constraint, err := semver.NewConstraint(constraintString)
	if err != nil {
		return false, fmt.Errorf("invalid constraint %q: %w", constraintString, err)
	}

	return constraint.Check(v), nil
}
