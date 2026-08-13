package replication

import (
	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// VolumeReplicationNameAnnotation is set by csi-addons on replicated PVCs and
// points at the VolumeReplication resource. It is whitelisted in internal/utils/filter.go.
const VolumeReplicationNameAnnotation = "replication.storage.openshift.io/volume-replication-name"

// ClassSelectorLabel carries the user-facing class identifier, set by the spx-rook-connection chart.
const ClassSelectorLabel = "replication.superphenix.net/classSelector"

var (
	VolumeReplicationGVR = schema.GroupVersionResource{
		Group:    replicationv1alpha1.GroupVersion.Group,
		Version:  replicationv1alpha1.GroupVersion.Version,
		Resource: "volumereplications",
	}

	// VolumeReplicationClassGVR is cluster scoped
	VolumeReplicationClassGVR = schema.GroupVersionResource{
		Group:    replicationv1alpha1.GroupVersion.Group,
		Version:  replicationv1alpha1.GroupVersion.Version,
		Resource: "volumereplicationclasses",
	}
)
