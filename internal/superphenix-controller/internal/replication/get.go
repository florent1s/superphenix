package replication

import (
	"context"

	"github.com/super-phenix/superphenix/internal/superphenix-controller/internal/models/view"
	k8s "github.com/super-phenix/superphenix/internal/superphenix-controller/pkg/config"
	logger "github.com/super-phenix/superphenix/pkg/utils/log"

	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GetReplication reads the VolumeReplication resource and its class. A missing
// or unreadable class is tolerated, the caller gets a partial view.
func GetReplication(ctx context.Context, namespace, vrName string) (*view.ReplicationView, error) {
	log := logger.GetLogger(ctx)

	unstructVR, err := k8s.DynamicClientSet.Resource(VolumeReplicationGVR).Namespace(namespace).Get(ctx, vrName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var vr replicationv1alpha1.VolumeReplication
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructVR.Object, &vr); err != nil {
		return nil, err
	}

	repView := volumeReplicationToView(vr)

	if className := vr.Spec.VolumeReplicationClass; className != "" {
		// class is cluster scoped
		unstructVRC, err := k8s.DynamicClientSet.Resource(VolumeReplicationClassGVR).Get(ctx, className, metav1.GetOptions{})
		if err != nil {
			log.Warn().Err(err).Str("class", className).Msg("volume replication class not readable, skipping class details")
		} else {
			var vrc replicationv1alpha1.VolumeReplicationClass
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructVRC.Object, &vrc); err != nil {
				log.Warn().Err(err).Str("class", className).Msg("volume replication class not convertible, skipping class details")
			} else {
				classView := volumeReplicationClassToView(vrc)
				repView.Class = &classView
			}
		}
	}

	return &repView, nil
}
