package version

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ManagementSuperphenixName is the ArgoCD Application name for the superphenix-management chart.
	ManagementSuperphenixName = "superphenix-management"
)

var (
	// MinManagementVersionBeforeUpgrade is the minimum version the management must be in before upgrade.
	MinManagementVersionBeforeUpgrade = "0.0.0"

	// MinClusterVersion is the minimum version of the cluster supported by the operator.
	MinClusterVersion = "0.0.0"
)

// IsManagementUpgradeSupported checks if upgrading management from current to target version is supported.
// Upgrades are supported if the current version is at least MinManagementVersionBeforeUpgrade.
// Special versions like "0.0.0-latest" or empty versions bypass the check.
func IsManagementUpgradeSupported(current, target string) error {
	if current == "" || current == "0.0.0-latest" || target == "0.0.0-latest" {
		return nil
	}

	constraintString := ">= " + MinManagementVersionBeforeUpgrade
	return checkConstraint(current, target, constraintString, "management")
}

// IsClusterUpgradeSupported checks if upgrading a cluster from current to target is supported.
// Upgrades are supported if the current version is at least MinClusterVersion.
// Special versions like "0.0.0-latest" or empty versions bypass the check.
func IsClusterUpgradeSupported(current, target string) error {
	if current == "" || target == "" || current == target {
		return nil
	}

	// Special case for latest
	if target == "0.0.0-latest" {
		return nil
	}

	constraintString := ">= " + MinClusterVersion
	return checkConstraint(current, target, constraintString, "cluster")
}

// IsClusterCompatibleWithManagement checks if a cluster version is supported by a management version.
// A cluster is considered compatible if its version is at least MinClusterVersion.
// Empty management version bypasses the check.
func IsClusterCompatibleWithManagement(clusterVersion, managementVersion string) error {
	if managementVersion == "" {
		return nil
	}

	constraintString := ">= " + MinClusterVersion

	if clusterVersion == "" || clusterVersion == "0.0.0-latest" {
		if clusterVersion == "0.0.0-latest" {
			return checkConstraint("0.0.0", managementVersion, constraintString, "cluster compatibility")
		}
		return nil
	}

	return checkConstraint(clusterVersion, managementVersion, constraintString, "cluster compatibility")
}

// GetCurrentManagementVersion attempts to retrieve the current version of the management chart
// by looking at the existing ArgoCD Application in the given namespace.
// Returns an empty string if the application is not found or the version cannot be determined.
func GetCurrentManagementVersion(ctx context.Context, c client.Reader, operatorNamespace string) (string, error) {
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	err := c.Get(ctx, types.NamespacedName{Name: ManagementSuperphenixName, Namespace: operatorNamespace}, app)
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

func checkConstraint(current, target, constraintString, scope string) error {
	v, err := semver.NewVersion(current)
	if err != nil {
		return fmt.Errorf("invalid %s version %q: %w", scope, current, err)
	}

	constraint, err := semver.NewConstraint(constraintString)
	if err != nil {
		return fmt.Errorf("invalid semver constraint for %s version %s: %w", scope, target, err)
	}

	if !constraint.Check(v) {
		if scope == "cluster compatibility" {
			return fmt.Errorf("cluster version %s is not supported by management version %s (required: %s)",
				current, target, constraintString)
		}
		return fmt.Errorf("%s upgrade from %s to %s is not supported (must satisfy: %s)",
			scope, current, target, constraintString)
	}

	return nil
}
