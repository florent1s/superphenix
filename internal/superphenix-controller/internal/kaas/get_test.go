package kaas

import (
	"context"
	"testing"

	"github.com/super-phenix/superphenix/internal/superphenix-controller/internal/models/view"
	"github.com/super-phenix/superphenix/internal/superphenix-controller/pkg/config"

	spxId "github.com/super-phenix/superphenix/pkg/superphenix-id"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	testNamespace = "spx-22222222-2222-2222-2222-222222222222"
	testClusterId = "spx-33333333-3333-3333-3333-333333333333"
)

func setFakeDynamicClient(t *testing.T, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	t.Helper()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			clustersGVR:                 "ClusterList",
			machineDeploymentsGVR:       "MachineDeploymentList",
			kubevirtMachineTemplatesGVR: "KubevirtMachineTemplateList",
		},
		objects...,
	)
	old := config.DynamicClientSet
	config.DynamicClientSet = client
	t.Cleanup(func() { config.DynamicClientSet = old })
	return client
}

func newCluster() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(clustersGVR.GroupVersion().String())
	obj.SetKind("Cluster")
	obj.SetName(testClusterId)
	obj.SetNamespace(testNamespace)
	obj.SetLabels(map[string]string{spxId.SpxLabelProjectID: testNamespace})
	return obj
}

func newMachineDeployment(name, clusterName string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(machineDeploymentsGVR.GroupVersion().String())
	obj.SetKind("MachineDeployment")
	obj.SetName(name)
	obj.SetNamespace(testNamespace)
	obj.SetLabels(map[string]string{ClusterLabelKey: clusterName})
	_ = unstructured.SetNestedField(obj.Object, name, "spec", "template", "spec", "infrastructureRef", "name")
	return obj
}

func newKubevirtMachineTemplate(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(kubevirtMachineTemplatesGVR.GroupVersion().String())
	obj.SetKind("KubevirtMachineTemplate")
	obj.SetName(name)
	obj.SetNamespace(testNamespace)
	return obj
}

func TestGetCluster(t *testing.T) {
	tests := []struct {
		name            string
		namespace       string
		eid             string
		objects         []runtime.Object
		wantErr         bool
		wantNotFound    bool
		wantName        string
		wantDeployments int
	}{
		{
			name:      "no namespace",
			namespace: "",
			eid:       testClusterId,
			wantErr:   true,
		},
		{
			// The cluster CR is never created when the gitops sync fails. Answering with an empty
			// cluster and no error made the API blank out the whole product response.
			name:         "missing cluster is reported as not found",
			namespace:    testNamespace,
			eid:          testClusterId,
			wantErr:      true,
			wantNotFound: true,
		},
		{
			name:      "cluster owned by another project is not found",
			namespace: testNamespace,
			eid:       testClusterId,
			objects: []runtime.Object{func() runtime.Object {
				cluster := newCluster()
				cluster.SetLabels(map[string]string{spxId.SpxLabelProjectID: "spx-other-project"})
				return cluster
			}()},
			wantErr:      true,
			wantNotFound: true,
		},
		{
			name:            "cluster without machine deployment",
			namespace:       testNamespace,
			eid:             testClusterId,
			objects:         []runtime.Object{newCluster()},
			wantName:        testClusterId,
			wantDeployments: 0,
		},
		{
			name:      "cluster with machine deployments",
			namespace: testNamespace,
			eid:       testClusterId,
			objects: []runtime.Object{
				newCluster(),
				newMachineDeployment("workers", testClusterId),
				newKubevirtMachineTemplate("workers"),
			},
			wantName:        testClusterId,
			wantDeployments: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setFakeDynamicClient(t, tt.objects...)

			cluster, err := GetCluster(context.Background(), tt.namespace, tt.eid)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got none")
				}
				if tt.wantNotFound && !apierrors.IsNotFound(err) {
					t.Fatalf("expected a NotFound error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cluster.Cluster.Name != tt.wantName {
				t.Errorf("expected cluster %q, got %q", tt.wantName, cluster.Cluster.Name)
			}
			if len(cluster.MachineDeployments) != tt.wantDeployments {
				t.Errorf("expected %d machine deployments, got %d", tt.wantDeployments, len(cluster.MachineDeployments))
			}
		})
	}
}

// TestGetClusterMachineDeploymentNotFound checks that a cluster is still returned when its machine
// deployments cannot be listed: the cluster itself exists and must stay manageable.
func TestGetClusterMachineDeploymentNotFound(t *testing.T) {
	client := setFakeDynamicClient(t, newCluster())
	client.PrependReactor("list", "machinedeployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(machineDeploymentsGVR.GroupResource(), "")
	})

	cluster, err := GetCluster(context.Background(), testNamespace, testClusterId)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cluster.Cluster.Name != testClusterId {
		t.Errorf("expected cluster %q, got %q", testClusterId, cluster.Cluster.Name)
	}
	if len(cluster.MachineDeployments) != 0 {
		t.Errorf("expected no machine deployment, got %d", len(cluster.MachineDeployments))
	}
}

// TestClusterChartAndKubeVersion checks the chart label and kube version reach the KaaS view
// through both the get and list paths.
func TestClusterChartAndKubeVersion(t *testing.T) {
	withChart := func(chart string) *unstructured.Unstructured {
		cluster := newCluster()
		labels := cluster.GetLabels()
		labels[HelmChartLabelKey] = chart
		cluster.SetLabels(labels)
		return cluster
	}
	withVersion := func(version string) *unstructured.Unstructured {
		md := newMachineDeployment("workers", testClusterId)
		_ = unstructured.SetNestedField(md.Object, version, "spec", "template", "spec", "version")
		return md
	}

	tests := []struct {
		name            string
		objects         []runtime.Object
		wantChart       string
		wantKubeVersion string
	}{
		{
			name:            "chart label and machine deployment",
			objects:         []runtime.Object{withChart("sfs-kaas-0.3.8"), withVersion("v1.33.4"), newKubevirtMachineTemplate("workers")},
			wantChart:       "sfs-kaas-0.3.8",
			wantKubeVersion: "v1.33.4",
		},
		{
			name:            "no chart label",
			objects:         []runtime.Object{newCluster(), withVersion("v1.33.4"), newKubevirtMachineTemplate("workers")},
			wantChart:       "",
			wantKubeVersion: "v1.33.4",
		},
		{
			name:            "no machine deployment",
			objects:         []runtime.Object{withChart("sfs-kaas-0.3.8")},
			wantChart:       "sfs-kaas-0.3.8",
			wantKubeVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setFakeDynamicClient(t, tt.objects...)

			got, err := GetCluster(context.Background(), testNamespace, testClusterId)
			if err != nil {
				t.Fatalf("GetCluster() error = %v", err)
			}
			listed, err := ListCluster(context.Background(), testNamespace)
			if err != nil || len(listed) != 1 {
				t.Fatalf("ListCluster() = %d clusters, error = %v", len(listed), err)
			}

			for path, cluster := range map[string]view.Cluster{"get": got, "list": listed[0]} {
				res := view.KaaSToResource(cluster)
				if res.Chart != tt.wantChart || res.KubeVersion != tt.wantKubeVersion {
					t.Errorf("%s: chart = %q, kubeVersion = %q, want %q, %q",
						path, res.Chart, res.KubeVersion, tt.wantChart, tt.wantKubeVersion)
				}
			}
		})
	}
}
