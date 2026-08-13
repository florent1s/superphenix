package admission

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/super-phenix/superphenix/internal/superphenix-controller/pkg/config"
	v1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMutateVolumeSnapshot(t *testing.T) {
	// Mock PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptrTo("test-storage-class"),
		},
	}

	// Setup fake client
	config.K8sClient = fake.NewSimpleClientset(pvc)

	// Mock VolumeSnapshot in AdmissionRequest
	vs := v1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Spec: v1.VolumeSnapshotSpec{
			Source: v1.VolumeSnapshotSource{
				PersistentVolumeClaimName: ptrTo("test-pvc"),
			},
		},
	}
	vsRaw, _ := json.Marshal(vs)

	admissionReview := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			UID: "test-uid",
			Kind: metav1.GroupVersionKind{
				Group:   "snapshot.storage.k8s.io",
				Version: "v1",
				Kind:    "VolumeSnapshot",
			},
			Operation: admissionv1.Create,
			Namespace: "default",
			Object: runtime.RawExtension{
				Raw: vsRaw,
			},
		},
	}
	body, _ := json.Marshal(admissionReview)

	req := httptest.NewRequest("POST", "/mutate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	MutateVolumeSnapshot(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response admissionv1.AdmissionReview
	json.NewDecoder(w.Body).Decode(&response)

	if !response.Response.Allowed {
		t.Errorf("Expected admission to be allowed")
	}

	if response.Response.PatchType == nil || *response.Response.PatchType != admissionv1.PatchTypeJSONPatch {
		t.Errorf("Expected patch type %s, got %v", admissionv1.PatchTypeJSONPatch, response.Response.PatchType)
	}

	var patch []map[string]any
	json.Unmarshal(response.Response.Patch, &patch)

	if len(patch) != 1 {
		t.Errorf("Expected 1 patch operation, got %d", len(patch))
	}

	if patch[0]["op"] != "add" || patch[0]["path"] != "/spec/volumeSnapshotClassName" || patch[0]["value"] != "test-storage-class" {
		t.Errorf("Unexpected patch: %v", patch)
	}
}

func ptrTo[T any](v T) *T {
	return &v
}
