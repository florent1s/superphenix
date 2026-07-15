package vmClusterPreference

import (
	"context"
	"superphenix-controller/internal/models/view"
	k8s "superphenix-controller/pkg/config"
	logger "utils/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetVMClusterPreference(ctx context.Context, name string) (view.VirtualMachinePreferenceView, error) {
	log := logger.GetLogger(ctx)

	pref, err := k8s.VirtClient.VirtualMachineClusterPreference().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Err(err).Str("name", name).Msg("Error getting VM Cluster Preference")
		return view.VirtualMachinePreferenceView{}, err
	}

	return view.VMClusterPreferenceToView(pref.Name, pref.Spec), nil
}
