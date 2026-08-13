package replication

import (
	"context"
	"testing"

	"github.com/super-phenix/superphenix/internal/superphenix-controller/pkg/config"

	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const testNamespace = "prj-example"

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := replicationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}
	return scheme
}

func setDynamicClient(t *testing.T, objects ...runtime.Object) {
	t.Helper()
	orig := config.DynamicClientSet
	config.DynamicClientSet = dynamicfake.NewSimpleDynamicClient(newTestScheme(t), objects...)
	t.Cleanup(func() { config.DynamicClientSet = orig })
}

func newVR(className string) *replicationv1alpha1.VolumeReplication {
	return &replicationv1alpha1.VolumeReplication{
		TypeMeta:   metav1.TypeMeta{APIVersion: replicationv1alpha1.GroupVersion.String(), Kind: "VolumeReplication"},
		ObjectMeta: metav1.ObjectMeta{Name: "vr-disk-1", Namespace: testNamespace},
		Spec: replicationv1alpha1.VolumeReplicationSpec{
			VolumeReplicationClass: className,
		},
		Status: replicationv1alpha1.VolumeReplicationStatus{
			State: replicationv1alpha1.PrimaryState,
		},
	}
}

func newVRC() *replicationv1alpha1.VolumeReplicationClass {
	return &replicationv1alpha1.VolumeReplicationClass{
		TypeMeta:   metav1.TypeMeta{APIVersion: replicationv1alpha1.GroupVersion.String(), Kind: "VolumeReplicationClass"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "vrc-example",
			Labels: map[string]string{ClassSelectorLabel: "daily"},
		},
		Spec: replicationv1alpha1.VolumeReplicationClassSpec{
			Provisioner: "rbd.csi.example.org",
			Parameters:  map[string]string{"schedulingInterval": "5m"},
		},
	}
}

func TestGetReplication(t *testing.T) {
	tests := []struct {
		name         string
		objects      []runtime.Object
		vrName       string
		wantErr      bool
		wantNotFound bool
		wantClass    bool
	}{
		{
			name:      "vr and class",
			objects:   []runtime.Object{newVR("vrc-example"), newVRC()},
			vrName:    "vr-disk-1",
			wantClass: true,
		},
		{
			name:    "class missing",
			objects: []runtime.Object{newVR("vrc-example")},
			vrName:  "vr-disk-1",
		},
		{
			name:         "vr missing",
			objects:      []runtime.Object{},
			vrName:       "vr-disk-1",
			wantErr:      true,
			wantNotFound: true,
		},
		{
			name:    "vr without class name",
			objects: []runtime.Object{newVR("")},
			vrName:  "vr-disk-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDynamicClient(t, tt.objects...)

			got, err := GetReplication(context.Background(), testNamespace, tt.vrName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantNotFound && !apierrors.IsNotFound(err) {
					t.Fatalf("expected NotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Name != "vr-disk-1" || got.State != "Primary" {
				t.Errorf("unexpected view: %+v", got)
			}
			if !tt.wantClass {
				if got.Class != nil {
					t.Errorf("expected nil class, got %+v", got.Class)
				}
				return
			}
			if got.Class == nil {
				t.Fatal("expected class, got nil")
			}
			if got.Class.Name != "daily" {
				t.Errorf("unexpected class name %q", got.Class.Name)
			}
			if got.Class.SchedulingInterval != "5m" {
				t.Errorf("expected schedulingInterval 5m, got %+v", got.Class)
			}
		})
	}
}

func TestGetReplicationUnconvertible(t *testing.T) {
	// lastSyncBytes must be an integer, a string breaks typed conversion
	broken := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": replicationv1alpha1.GroupVersion.String(),
		"kind":       "VolumeReplication",
		"metadata": map[string]interface{}{
			"name":      "vr-disk-1",
			"namespace": testNamespace,
		},
		"status": map[string]interface{}{
			"lastSyncBytes": "not-a-number",
		},
	}}
	setDynamicClient(t, broken)

	if _, err := GetReplication(context.Background(), testNamespace, "vr-disk-1"); err == nil {
		t.Fatal("expected conversion error, got nil")
	}
}
