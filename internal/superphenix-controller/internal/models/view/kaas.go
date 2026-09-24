package view

import (
	spxId "github.com/super-phenix/superphenix/pkg/superphenix-id"

	"sigs.k8s.io/cluster-api/api/core/v1beta2"
)

type Cluster struct {
	Cluster            v1beta2.Cluster     `json:"cluster,omitempty"`
	MachineDeployments []MachineDeployment `json:"machineDeployments"`
	// Chart is the helm.sh/chart label of the Cluster.
	Chart string `json:"-"`
}

type MachineDeployment struct {
	MachineDeployment v1beta2.MachineDeployment         `json:"machineDeployment"`
	MachineTemplate   KubevirtMachineTemplateSimplified `json:"machineTemplate,omitempty"`
}

type KubevirtMachineTemplateSimplified struct {
	ObjectMeta          `json:"metadata,omitempty"`
	DataVolumeTemplates []DataVolumeTemplate `json:"dataVolumeTemplates"`
	MachineTemplate     MachineTemplate      `json:"machineTemplate"`
}

type DataVolumeTemplate struct {
	RegistryUrl string `json:"registryUrl"`
	StorageSize string `json:"storageSize"`
	VolumeMode  string `json:"volumeMode"`
}

type MachineTemplate struct {
	CPU        MachineTemplateCPU        `json:"cpu"`
	Memory     MachineTemplateMemory     `json:"memory"`
	Preference MachineTemplatePreference `json:"preference"`
}

type MachineTemplateCPU struct {
	Cores   int `json:"cores"`
	Sockets int `json:"sockets"`
	Threads int `json:"threads"`
}

type MachineTemplateMemory struct {
	Guest string `json:"guest"`
}

type MachineTemplatePreference struct {
	Name string `json:"name"`
}

func KaaSToResource(cluster Cluster) KaaS {
	return KaaS{
		Resource: Resource{
			ID:          cluster.Cluster.Labels[spxId.SpxLabelResourceLocalID],
			EId:         cluster.Cluster.Name,
			ProductName: cluster.Cluster.Labels[spxId.SpxLabelResourceName],
			Gitops:      cluster.Cluster.Labels[spxId.SpxLabelGitops],
		},
		Chart:       cluster.Chart,
		KubeVersion: kubeVersion(cluster.MachineDeployments),
		Cluster:     cluster,
	}
}

// kubeVersion returns the kube version of the cluster's first machine deployment.
func kubeVersion(mds []MachineDeployment) string {
	if len(mds) == 0 {
		return ""
	}
	return mds[0].MachineDeployment.Spec.Template.Spec.Version
}
