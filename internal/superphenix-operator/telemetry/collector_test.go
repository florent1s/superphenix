package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1alpha1 "github.com/super-phenix/superphenix/api/operator/v1alpha1"
)

func TestCollector_Collect(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1alpha1.SchemeBuilder.AddToScheme(scheme)

	cluster1 := &operatorv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-1"},
		Spec: operatorv1alpha1.ClusterSpec{
			Region:             "us-east-1",
			AvailabilityZone:   "us-east-1a",
			DeploymentTopology: operatorv1alpha1.DeploymentTopologyHyperconverged,
		},
		Status: operatorv1alpha1.ClusterStatus{
			CurrentVersion: "v1.2.3",
			NodeCount:      3,
		},
	}
	cluster2 := &operatorv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-2"},
		Spec: operatorv1alpha1.ClusterSpec{
			Region:             "us-east-1",
			AvailabilityZone:   "us-east-1b",
			DeploymentTopology: operatorv1alpha1.DeploymentTopologyDecoupled,
			Type:               ptr(operatorv1alpha1.ClusterTypeStorage),
		},
		Status: operatorv1alpha1.ClusterStatus{
			CurrentVersion: "v1.2.4",
			NodeCount:      5,
		},
	}
	cluster3 := &operatorv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-3"},
		Spec: operatorv1alpha1.ClusterSpec{
			Region:             "eu-west-1",
			AvailabilityZone:   "eu-west-1a",
			DeploymentTopology: operatorv1alpha1.DeploymentTopologyHyperconverged,
		},
		Status: operatorv1alpha1.ClusterStatus{
			CurrentVersion: "v1.2.3",
		},
	}
	cluster4 := &operatorv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-4"},
		Spec: operatorv1alpha1.ClusterSpec{
			Region:             "us-east-1",
			AvailabilityZone:   "us-east-1b", // Same as cluster 2
			DeploymentTopology: operatorv1alpha1.DeploymentTopologyHyperconverged,
		},
	}

	c := &Collector{
		Client:            fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(cluster1, cluster2, cluster3, cluster4).Build(),
		OperatorVersion:   "v1.0.0",
		ManagementVersion: "v2.0.0",
		ArgoCDVersion:     "v9.7.0",
	}

	report, err := c.Collect(context.Background())
	require.NoError(t, err)

	assert.Equal(t, SchemaVersion, report.SchemaVersion)

	// Check operator_info
	found := false
	for _, m := range report.Metrics {
		if m.Name == MetricOperatorInfo {
			assert.Equal(t, "v1.0.0", m.Labels["version"])
			found = true
		}
	}
	assert.True(t, found, "operator_info missing")

	// Check region_count
	found = false
	for _, m := range report.Metrics {
		if m.Name == MetricRegionCount {
			assert.Equal(t, float64(2), m.Value)
			found = true
		}
	}
	assert.True(t, found, "region_count missing")

	// Check az_count
	azCounts := make(map[string]float64)
	for _, m := range report.Metrics {
		if m.Name == MetricAZCount {
			azCounts[m.Labels["region"]] = m.Value
		}
	}
	assert.Equal(t, 2, len(azCounts))
	assert.Equal(t, float64(2), azCounts[anonymize("us-east-1")])
	assert.Equal(t, float64(1), azCounts[anonymize("eu-west-1")])

	// Check cluster_info
	clustersFound := 0
	for _, m := range report.Metrics {
		if m.Name == MetricClusterInfo {
			clustersFound++
			switch m.Labels["cluster"] {
			case anonymize("cluster-1"):
				assert.Equal(t, "hyperconverged", m.Labels["topology"])
				assert.Equal(t, "none", m.Labels["type"])
				assert.Equal(t, "v1.2.3", m.Labels["version"])
			case anonymize("cluster-2"):
				assert.Equal(t, "decoupled", m.Labels["topology"])
				assert.Equal(t, "storage", m.Labels["type"])
				assert.Equal(t, "v1.2.4", m.Labels["version"])
			}
		}
	}
	assert.Equal(t, 4, clustersFound)

	// Check node_count
	nodeCountsFound := 0
	for _, m := range report.Metrics {
		if m.Name == MetricNodeCount {
			nodeCountsFound++
			switch m.Labels["cluster"] {
			case anonymize("cluster-1"):
				assert.Equal(t, float64(3), m.Value)
			case anonymize("cluster-2"):
				assert.Equal(t, float64(5), m.Value)
			default:
				t.Errorf("unexpected node_count for cluster %s", m.Labels["cluster"])
			}
		}
	}
	assert.Equal(t, 2, nodeCountsFound)

	// Check component_info for superphenix-system
	systemComponentsFound := 0
	for _, m := range report.Metrics {
		if m.Name == MetricComponentInfo && m.Labels["name"] == "superphenix-system" {
			systemComponentsFound++
			switch m.Labels["cluster"] {
			case anonymize("cluster-1"):
				assert.Equal(t, "v1.2.3", m.Labels["version"])
			case anonymize("cluster-2"):
				assert.Equal(t, "v1.2.4", m.Labels["version"])
			case anonymize("cluster-3"):
				assert.Equal(t, "v1.2.3", m.Labels["version"])
			case anonymize("cluster-4"):
				assert.Equal(t, "unknown", m.Labels["version"])
			default:
				t.Errorf("unexpected superphenix-system for cluster %s", m.Labels["cluster"])
			}
		}
	}
	assert.Equal(t, 4, systemComponentsFound)

	// Check management component info
	mgmtFound := false
	for _, m := range report.Metrics {
		if m.Name == MetricComponentInfo && m.Labels["name"] == "superphenix-management" {
			mgmtFound = true
			assert.Equal(t, "v2.0.0", m.Labels["version"])
			_, clusterPresent := m.Labels["cluster"]
			assert.False(t, clusterPresent, "cluster label should not be present for superphenix-management")
		}
	}
	assert.True(t, mgmtFound, "superphenix-management component info missing")

	// Check argocd component info
	argocdFound := false
	for _, m := range report.Metrics {
		if m.Name == MetricComponentInfo && m.Labels["name"] == "argocd" {
			argocdFound = true
			assert.Equal(t, "v9.7.0", m.Labels["version"])
			_, clusterPresent := m.Labels["cluster"]
			assert.False(t, clusterPresent, "cluster label should not be present for argocd")
		}
	}
	assert.True(t, argocdFound, "argocd component info missing")
}

func ptr[T any](v T) *T {
	return &v
}
