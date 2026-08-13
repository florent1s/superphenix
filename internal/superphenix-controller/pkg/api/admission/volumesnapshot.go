package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/super-phenix/superphenix/internal/superphenix-controller/pkg/config"
	logger "github.com/super-phenix/superphenix/pkg/utils/log"

	v1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MutateVolumeSnapshot handles the mutation of VolumeSnapshot resources.
// It sets the VolumeSnapshotClassName to match the StorageClassName of the referenced PVC.
func MutateVolumeSnapshot(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	var admissionReview admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&admissionReview); err != nil {
		log.Err(err).Msg("Failed to decode AdmissionReview")
		http.Error(w, "Failed to decode AdmissionReview", http.StatusBadRequest)
		return
	}

	request := admissionReview.Request
	if request == nil {
		log.Error().Msg("AdmissionReview request is nil")
		http.Error(w, "AdmissionReview request is nil", http.StatusBadRequest)
		return
	}

	// Default response: allowed
	response := &admissionv1.AdmissionResponse{
		UID:     request.UID,
		Allowed: true,
	}

	if request.Kind.Kind == "VolumeSnapshot" && request.Operation == admissionv1.Create {
		handleVolumeSnapshotMutation(r.Context(), request, response)
	}

	admissionReview.Response = response
	admissionReview.Request = nil // Optional: clear request to reduce response size

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(admissionReview); err != nil {
		log.Err(err).Msg("Failed to encode AdmissionReview response")
	}
}

func handleVolumeSnapshotMutation(ctx context.Context, request *admissionv1.AdmissionRequest, response *admissionv1.AdmissionResponse) {
	log := logger.GetLogger(ctx)

	var vs v1.VolumeSnapshot
	if err := json.Unmarshal(request.Object.Raw, &vs); err != nil {
		log.Err(err).Msg("Failed to unmarshal VolumeSnapshot")
		response.Result = &metav1.Status{
			Message: fmt.Sprintf("Failed to unmarshal VolumeSnapshot: %v", err),
		}
		return
	}

	if vs.Spec.VolumeSnapshotClassName != nil && *vs.Spec.VolumeSnapshotClassName != "" {
		return
	}

	if vs.Spec.Source.PersistentVolumeClaimName == nil {
		return
	}

	pvcName := *vs.Spec.Source.PersistentVolumeClaimName
	namespace := request.Namespace

	pvc, err := config.K8sClient.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		log.Err(err).Str("namespace", namespace).Str("pvc", pvcName).Msg("Failed to get PVC")
		// We don't block the creation if we can't find the PVC, just log it.
		return
	}

	if pvc.Spec.StorageClassName == nil {
		return
	}

	scName := *pvc.Spec.StorageClassName
	log.Info().Str("namespace", namespace).Str("vs", vs.Name).Str("pvc", pvcName).Str("storageClass", scName).Msg("Setting VolumeSnapshotClassName")

	patch := []struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}{
		{
			Op:    "add",
			Path:  "/spec/volumeSnapshotClassName",
			Value: scName,
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		log.Err(err).Msg("Failed to marshal patch")
		return
	}

	pt := admissionv1.PatchTypeJSONPatch
	response.PatchType = &pt
	response.Patch = patchBytes
}
