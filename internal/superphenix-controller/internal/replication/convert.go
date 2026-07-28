package replication

import (
	"github.com/super-phenix/superphenix/internal/superphenix-controller/internal/models/view"

	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
)

func volumeReplicationToView(vr replicationv1alpha1.VolumeReplication) view.ReplicationView {
	repView := view.ReplicationView{
		Name:               vr.Name,
		State:              string(vr.Status.State),
		Message:            vr.Status.Message,
		LastSyncTime:       vr.Status.LastSyncTime,
		LastCompletionTime: vr.Status.LastCompletionTime,
	}

	if vr.Status.LastSyncDuration != nil {
		repView.LastSyncDuration = vr.Status.LastSyncDuration.Duration.String()
	}
	if vr.Status.LastSyncBytes != nil {
		repView.LastSyncBytes = *vr.Status.LastSyncBytes
	}

	return repView
}

func volumeReplicationClassToView(vrc replicationv1alpha1.VolumeReplicationClass) view.ReplicationClassView {
	classView := view.ReplicationClassView{
		Name:        vrc.Labels[ClassSelectorLabel],
		Provisioner: vrc.Spec.Provisioner,
	}

	// only expose scheduling parameters, secret refs stay out
	classView.SchedulingInterval = vrc.Spec.Parameters["schedulingInterval"]
	classView.SchedulingStartTime = vrc.Spec.Parameters["schedulingStartTime"]
	classView.MirroringMode = vrc.Spec.Parameters["mirroringMode"]

	return classView
}
