package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConditionTypeConnected represents the status of the connection to the remote cluster.
	ConditionTypeConnected = "Connected"
	// ConditionTypeUnreachable represents the status when the remote cluster is unreachable.
	ConditionTypeUnreachable = "Unreachable"

	// ReasonConnectionSuccess is used when the connection to the remote cluster is successful.
	ReasonConnectionSuccess = "ConnectionSuccess"
	// ReasonConnectionFailed is used when the connection to the remote cluster fails.
	ReasonConnectionFailed = "ConnectionFailed"
	// ReasonSecretNotFound is used when the secret containing the connection credentials is not found.
	ReasonSecretNotFound = "SecretNotFound"
	// ReasonInvalidSecret is used when the secret containing the connection credentials is invalid.
	ReasonInvalidSecret = "InvalidSecret"
	// ReasonConnectionConfigError is used when the connection configuration is invalid or missing.
	ReasonConnectionConfigError = "ConnectionConfigError"

	// ReasonInvalidVersion is used when the requested version change is invalid.
	ReasonInvalidVersion = "InvalidVersion"
)

// DeploymentMode defines whether the cluster is hyperconverged or decoupled.
// +kubebuilder:validation:Enum=Hyperconverged;Decoupled
type DeploymentMode string

const (
	// DeploymentModeHyperconverged - Storage and virtualization run on the same cluster.
	DeploymentModeHyperconverged DeploymentMode = "Hyperconverged"

	// DeploymentModeDecoupled - Storage and virtualization run on separate clusters.
	DeploymentModeDecoupled DeploymentMode = "Decoupled"
)

// ClusterType defines the type of cluster when in Decoupled mode.
// +kubebuilder:validation:Enum=Storage;Virtualization
type ClusterType string

const (
	// ClusterTypeStorage - Dedicated storage cluster.
	ClusterTypeStorage ClusterType = "Storage"

	// ClusterTypeVirtualization - Dedicated virtualization/hypervisor cluster.
	ClusterTypeVirtualization ClusterType = "Virtualization"
)

// ClusterSpec defines the desired state of Cluster.
type ClusterSpec struct {
	// DeploymentMode defines whether the cluster is hyperconverged or decoupled.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Hyperconverged;Decoupled
	DeploymentMode DeploymentMode `json:"deploymentMode,omitempty"`

	// Type specifies the cluster type (Storage or Virtualization) when DeploymentMode is Decoupled.
	// This field can only be set when DeploymentMode is Decoupled and is ignored otherwise.
	// +optional
	Type *ClusterType `json:"type,omitempty"`

	// Region is the geographic region where this cluster is located.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// AvailabilityZone is the availability zone identifier within the region.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	AvailabilityZone string `json:"availabilityZone"`

	// NetworkConfiguration is a YAML dict of unknown values that will be passed to the network configuration chart.
	// +optional
	NetworkConfiguration *apiextensionsv1.JSON `json:"networkConfiguration,omitempty"`

	// SystemConfiguration is a YAML dict of unknown values that will be passed to the system configuration chart.
	// +optional
	SystemConfiguration *apiextensionsv1.JSON `json:"systemConfiguration,omitempty"`

	// Version is the Superphenix version for this cluster.
	// It must follow semantic versioning.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^v?([0-9]+)(\.[0-9]+)?(\.[0-9]+)?(-([0-9A-Za-z\-.]+))?(\+([0-9A-Za-z\-.]+))?$`
	Version string `json:"version"`

	// Connection defines the parameters to connect to the remote cluster.
	// +kubebuilder:validation:Required
	Connection *ClusterConnectionSpec `json:"connection"`
}

// ConnectionMode defines how the operator connects to the cluster.
// +kubebuilder:validation:Enum=Remote;Local
type ConnectionMode string

const (
	// ConnectionModeRemote - Connect to a remote cluster via URL and Secret.
	ConnectionModeRemote ConnectionMode = "Remote"

	// ConnectionModeLocal - Connect to the local cluster (the one where the operator is running).
	ConnectionModeLocal ConnectionMode = "Local"
)

// ClusterConnectionSpec defines the parameters to connect to the remote cluster.
type ClusterConnectionSpec struct {
	// Mode specifies the connection mode (Remote or Local).
	// +kubebuilder:validation:Required
	// +kubebuilder:default=Local
	Mode ConnectionMode `json:"mode"`

	// URL is the address of the remote cluster API server.
	// This is required when Mode is Remote and ignored when Mode is Local.
	// +optional
	URL string `json:"url,omitempty"`

	// SecretRef is a reference to a secret containing the connection credentials.
	// This is required when Mode is Remote and ignored when Mode is Local.
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// SecretReference defines a reference to a Secret.
type SecretReference struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret. If empty, the namespace of the Cluster is used.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ClusterStatus defines the observed state of Cluster.
type ClusterStatus struct {
	// Phase represents the current phase of the cluster lifecycle.
	// +kubebuilder:validation:Enum=Deployed;Deploying;OutOfSync;Error
	// +optional
	Phase string `json:"phase,omitempty"`

	// CurrentVersion is the actual Superphenix version currently running on the cluster.
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

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
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Cluster represents a Superphenix cluster deployment.
// A cluster is an availability zone (AZ) where the full Superphenix stack is deployed.
type Cluster struct {
	metav1.TypeMeta `json:",inline"`

	// Metadata is a standard object metadata.
	// +required
	metav1.ObjectMeta `json:"metadata"`

	// Spec defines the desired state of Cluster.
	// +required
	Spec ClusterSpec `json:"spec"`

	// Status defines the observed state of Cluster.
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
