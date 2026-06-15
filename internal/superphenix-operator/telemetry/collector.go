package telemetry

import (
	"context"
	"regexp"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

// labelValueRe mirrors the upstream label value pattern. Values that
// would be rejected by the server are dropped at collection time so the
// rest of the report can still go through.
var labelValueRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// Collector builds a Report from the operator's view of the world by
// listing Cluster CRs from the local cache. It never contacts remote
// clusters.
type Collector struct {
	Client          client.Reader
	OperatorVersion string

	// ManagementVersion is the currently deployed superphenix-management
	// chart version on this cluster. Empty when this operator does not run
	// on a management cluster, in which case no component_info metric for
	// the management chart is emitted.
	ManagementVersion string
}

// Collect returns the current Report. It always includes operator_info;
// AZ-related metrics are included only when at least one Cluster CR is
// readable.
func (c *Collector) Collect(ctx context.Context) (Report, error) {
	report := Report{SchemaVersion: SchemaVersion}

	report.Metrics = append(report.Metrics, Metric{
		Name:   MetricOperatorInfo,
		Kind:   KindGauge,
		Value:  1,
		Labels: map[string]string{"version": sanitizeVersion(c.OperatorVersion)},
	})

	if c.ManagementVersion != "" {
		report.Metrics = append(report.Metrics, Metric{
			Name:  MetricComponentInfo,
			Kind:  KindGauge,
			Value: 1,
			Labels: map[string]string{
				"name":    "superphenix-management",
				"version": sanitizeVersion(c.ManagementVersion),
			},
		})
	}

	clusters := &operatorv1alpha1.ClusterList{}
	if err := c.Client.List(ctx, clusters); err != nil {
		return report, err
	}

	report.Metrics = append(report.Metrics, Metric{
		Name:  MetricAZCount,
		Kind:  KindGauge,
		Value: float64(len(clusters.Items)),
	})

	for i := range clusters.Items {
		cl := &clusters.Items[i]
		report.Metrics = append(report.Metrics, Metric{
			Name:  MetricAZInfo,
			Kind:  KindGauge,
			Value: 1,
			Labels: map[string]string{
				"topology": topologyLabel(cl.Spec.DeploymentTopology),
				"type":     typeLabel(cl.Spec.DeploymentTopology, cl.Spec.Type),
				"version":  sanitizeVersion(cl.Status.CurrentVersion),
			},
		})
	}

	if len(report.Metrics) > MaxMetricsPerReport {
		report.Metrics = report.Metrics[:MaxMetricsPerReport]
	}

	return report, nil
}

func topologyLabel(t operatorv1alpha1.DeploymentTopology) string {
	switch t {
	case operatorv1alpha1.DeploymentTopologyHyperconverged:
		return TopologyHyperconverged
	case operatorv1alpha1.DeploymentTopologyDecoupled:
		return TopologyDecoupled
	default:
		return TopologyHyperconverged
	}
}

func typeLabel(topo operatorv1alpha1.DeploymentTopology, t *operatorv1alpha1.ClusterType) string {
	if topo == operatorv1alpha1.DeploymentTopologyHyperconverged || t == nil {
		return TypeNone
	}
	switch *t {
	case operatorv1alpha1.ClusterTypeStorage:
		return TypeStorage
	case operatorv1alpha1.ClusterTypeVirtualization:
		return TypeVirtualization
	default:
		return TypeNone
	}
}

// sanitizeVersion coerces a version string into something the server's
// label value regex will accept. Empty or invalid inputs collapse to
// "unknown" so the report still validates.
func sanitizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	if len(v) > MaxLabelValueLen {
		v = v[:MaxLabelValueLen]
	}
	if !labelValueRe.MatchString(v) {
		return "unknown"
	}
	return v
}
