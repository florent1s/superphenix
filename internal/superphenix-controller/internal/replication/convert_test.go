package replication

import (
	"testing"
	"time"

	"github.com/super-phenix/superphenix/internal/superphenix-controller/internal/models/view"

	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVolumeReplicationToView(t *testing.T) {
	syncTime := metav1.NewTime(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC))
	completionTime := metav1.NewTime(time.Date(2026, 7, 1, 10, 0, 12, 0, time.UTC))
	syncBytes := int64(1572864)

	tests := []struct {
		name string
		vr   replicationv1alpha1.VolumeReplication
		want view.ReplicationView
	}{
		{
			name: "full status",
			vr: replicationv1alpha1.VolumeReplication{
				ObjectMeta: metav1.ObjectMeta{Name: "vr-disk-1"},
				Status: replicationv1alpha1.VolumeReplicationStatus{
					State:              replicationv1alpha1.PrimaryState,
					Message:            "volume is marked primary",
					LastSyncTime:       &syncTime,
					LastSyncDuration:   &metav1.Duration{Duration: 90 * time.Second},
					LastSyncBytes:      &syncBytes,
					LastCompletionTime: &completionTime,
				},
			},
			want: view.ReplicationView{
				Name:               "vr-disk-1",
				State:              "Primary",
				Message:            "volume is marked primary",
				LastSyncTime:       &syncTime,
				LastSyncDuration:   "1m30s",
				LastSyncBytes:      1572864,
				LastCompletionTime: &completionTime,
			},
		},
		{
			name: "empty status",
			vr: replicationv1alpha1.VolumeReplication{
				ObjectMeta: metav1.ObjectMeta{Name: "vr-disk-1"},
			},
			want: view.ReplicationView{Name: "vr-disk-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := volumeReplicationToView(tt.vr)
			if got.Name != tt.want.Name ||
				got.State != tt.want.State ||
				got.Message != tt.want.Message ||
				got.LastSyncDuration != tt.want.LastSyncDuration ||
				got.LastSyncBytes != tt.want.LastSyncBytes {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if !timeEqual(got.LastSyncTime, tt.want.LastSyncTime) {
				t.Errorf("lastSyncTime: got %v, want %v", got.LastSyncTime, tt.want.LastSyncTime)
			}
			if !timeEqual(got.LastCompletionTime, tt.want.LastCompletionTime) {
				t.Errorf("lastCompletionTime: got %v, want %v", got.LastCompletionTime, tt.want.LastCompletionTime)
			}
		})
	}
}

func TestVolumeReplicationClassToView(t *testing.T) {
	tests := []struct {
		name string
		vrc  replicationv1alpha1.VolumeReplicationClass
		want view.ReplicationClassView
	}{
		{
			name: "scheduling params kept, secret refs dropped",
			vrc: replicationv1alpha1.VolumeReplicationClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "vrc-example",
					Labels: map[string]string{ClassSelectorLabel: "daily"},
				},
				Spec: replicationv1alpha1.VolumeReplicationClassSpec{
					Provisioner: "rbd.csi.example.org",
					Parameters: map[string]string{
						"schedulingInterval":  "5m",
						"schedulingStartTime": "14:00:00-05:00",
						"mirroringMode":       "snapshot",
						"replication.storage.openshift.io/replication-secret-name":      "csi-secret",
						"replication.storage.openshift.io/replication-secret-namespace": "rook-ceph",
					},
				},
			},
			want: view.ReplicationClassView{
				Name:                "daily",
				Provisioner:         "rbd.csi.example.org",
				SchedulingInterval:  "5m",
				SchedulingStartTime: "14:00:00-05:00",
				MirroringMode:       "snapshot",
			},
		},
		{
			name: "no parameters, no selector label",
			vrc: replicationv1alpha1.VolumeReplicationClass{
				ObjectMeta: metav1.ObjectMeta{Name: "vrc-example"},
				Spec: replicationv1alpha1.VolumeReplicationClassSpec{
					Provisioner: "rbd.csi.example.org",
				},
			},
			want: view.ReplicationClassView{
				Provisioner: "rbd.csi.example.org",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := volumeReplicationClassToView(tt.vrc)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func timeEqual(a, b *metav1.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(b)
}
