package kubevirt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/super-phenix/superphenix/internal/superphenix-controller/internal/gc/resources/testhelper"
	"github.com/super-phenix/superphenix/internal/superphenix-controller/internal/informers"
	"github.com/super-phenix/superphenix/internal/superphenix-controller/internal/replication"
	"github.com/super-phenix/superphenix/internal/superphenix-controller/pkg/config"

	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"kubevirt.io/client-go/kubecli"
)

func TestGetDiskReplication(t *testing.T) {
	const (
		projectId = "example"
		namespace = "prj-example"
		eid       = "disk-eid-1"
		vrName    = "vr-disk-1"
	)

	newPVC := func(annotations map[string]string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: k8smetav1.ObjectMeta{
				Name:        eid,
				Namespace:   namespace,
				Annotations: annotations,
			},
		}
	}

	newVR := func() *replicationv1alpha1.VolumeReplication {
		return &replicationv1alpha1.VolumeReplication{
			TypeMeta:   k8smetav1.TypeMeta{APIVersion: replicationv1alpha1.GroupVersion.String(), Kind: "VolumeReplication"},
			ObjectMeta: k8smetav1.ObjectMeta{Name: vrName, Namespace: namespace},
			Status: replicationv1alpha1.VolumeReplicationStatus{
				State: replicationv1alpha1.PrimaryState,
			},
		}
	}

	replicatedAnnotations := map[string]string{
		replication.VolumeReplicationNameAnnotation: vrName,
	}

	tests := []struct {
		name            string
		pvc             *corev1.PersistentVolumeClaim
		dynamicObjects  []runtime.Object
		wantReplication bool
		wantState       string
	}{
		{
			name: "pvc without annotation",
			pvc:  newPVC(nil),
		},
		{
			name:            "pvc with annotation and vr present",
			pvc:             newPVC(replicatedAnnotations),
			dynamicObjects:  []runtime.Object{newVR()},
			wantReplication: true,
			wantState:       "Primary",
		},
		{
			name: "pvc with annotation but vr fetch fails",
			pvc:  newPVC(replicatedAnnotations),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origPrefix := config.Global.SpxPrefix
			config.Global.SpxPrefix = "prj"
			t.Cleanup(func() { config.Global.SpxPrefix = origPrefix })

			origK8s := config.K8sClient
			config.K8sClient = fake.NewClientset(tt.pvc)
			t.Cleanup(func() { config.K8sClient = origK8s })

			ctrl := gomock.NewController(t)
			mockVirt := kubecli.NewMockKubevirtClient(ctrl)
			mockVirt.EXPECT().CdiClient().Return(testhelper.NewFakeCdiClientset()).AnyTimes()
			origVirt := config.VirtClient
			config.VirtClient = mockVirt
			t.Cleanup(func() { config.VirtClient = origVirt })

			// empty vm indexer, disk is not mounted
			indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
				"namespace": cache.MetaNamespaceIndexFunc,
			})
			origWatcher, hadWatcher := informers.WatcherSet[informers.VirtualMachine]
			informers.WatcherSet[informers.VirtualMachine] = informers.Watcher{Indexer: indexer}
			t.Cleanup(func() {
				if hadWatcher {
					informers.WatcherSet[informers.VirtualMachine] = origWatcher
				} else {
					delete(informers.WatcherSet, informers.VirtualMachine)
				}
			})

			scheme := runtime.NewScheme()
			if err := replicationv1alpha1.AddToScheme(scheme); err != nil {
				t.Fatalf("failed to build scheme: %v", err)
			}
			origDyn := config.DynamicClientSet
			config.DynamicClientSet = dynamicfake.NewSimpleDynamicClient(scheme, tt.dynamicObjects...)
			t.Cleanup(func() { config.DynamicClientSet = origDyn })

			r := reqWithParams(http.MethodGet, "/disk/"+eid, map[string]string{
				"projectId":   projectId,
				"effectiveId": eid,
			})
			w := httptest.NewRecorder()

			getDiskByEffectiveId(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var body map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid response json: %v", err)
			}

			rep, ok := body["replication"]
			if !tt.wantReplication {
				if ok {
					t.Fatalf("expected no replication field, got %v", rep)
				}
				return
			}
			if !ok {
				t.Fatal("expected replication field, got none")
			}
			state := rep.(map[string]interface{})["state"]
			if state != tt.wantState {
				t.Errorf("expected state %q, got %v", tt.wantState, state)
			}
		})
	}
}
