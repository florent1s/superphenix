package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeploymentMode defines whether the cluster is hyperconverged or decoupled
// +kubebuilder:validation:Enum=Hyperconverged;Decoupled
type DeploymentMode string

const (
	// Hyperconverged - Storage and virtualization run on the same cluster
	DeploymentModeHyperconverged DeploymentMode = "Hyperconverged"

	// Decoupled - Storage and virtualization run on separate clusters
	DeploymentModeDecoupled DeploymentMode = "Decoupled"
)

// ClusterType defines the type of cluster when in Decoupled mode
// +kubebuilder:validation:Enum=Storage;Virtualization
type ClusterType string

const (
	// ClusterTypeStorage - Dedicated storage cluster
	ClusterTypeStorage ClusterType = "Storage"

	// ClusterTypeVirtualization - Dedicated virtualization/hypervisor cluster
	ClusterTypeVirtualization ClusterType = "Virtualization"
)

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// Name is the human-readable name of the cluster
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// DeploymentMode defines whether the cluster is hyperconverged or decoupled
	// +kubebuilder:validation:Required
	DeploymentMode DeploymentMode `json:"deploymentMode"`

	// Type specifies the cluster type (Storage or Virtualization) when DeploymentMode is Decoupled.
	// This field is required when DeploymentMode is Decoupled and ignored when Hyperconverged.
	// +optional
	Type *ClusterType `json:"type,omitempty"`

	// Region is the geographic region where this cluster is located
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// AvailabilityZone is the availability zone identifier within the region
	// An AZ is a Kubernetes cluster where the full Superphenix stack is deployed
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	AvailabilityZone string `json:"availabilityZone"`

	// NetworkConfiguration is a YAML dict of unknown values that will be passed to the network configuration chart
	// +optional
	NetworkConfiguration *apiextensionsv1.JSON `json:"networkConfiguration,omitempty"`

	// SystemConfiguration is a YAML dict of unknown values that will be passed to the system configuration chart
	// +optional
	SystemConfiguration *apiextensionsv1.JSON `json:"systemConfiguration,omitempty"`
}

// ClusterStatus defines the observed state of Cluster.
type ClusterStatus struct {
	// Phase represents the current phase of the cluster lifecycle
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the current state of the Cluster resource.
	// Standard condition types include:
	// - "Ready": the cluster is fully operational
	// - "Progressing": the cluster is being provisioned or updated
	// - "Degraded": the cluster has encountered issues
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed Cluster.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.deploymentMode`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Region",type=string,JSONPath=`.spec.region`
// +kubebuilder:printcolumn:name="AZ",type=string,JSONPath=`.spec.availabilityZone`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Cluster represents a Superphenix cluster deployment.
// A cluster is an availability zone (AZ) where the full Superphenix stack is deployed.
type Cluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Cluster
	// +required
	Spec ClusterSpec `json:"spec"`

	// status defines the observed state of Cluster
	// +optional
	Status ClusterStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
