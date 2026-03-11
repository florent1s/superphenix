package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConditionTypeConnected represents the status of the connection to the remote cluster
	ConditionTypeConnected = "Connected"
	// ConditionTypeUnreachable represents the status when the remote cluster is unreachable
	ConditionTypeUnreachable = "Unreachable"

	// ReasonConnectionSuccess is used when the connection to the remote cluster is successful
	ReasonConnectionSuccess = "ConnectionSuccess"
	// ReasonConnectionFailed is used when the connection to the remote cluster fails
	ReasonConnectionFailed = "ConnectionFailed"
	// ReasonSecretNotFound is used when the secret containing the connection credentials is not found
	ReasonSecretNotFound = "SecretNotFound"
	// ReasonInvalidSecret is used when the secret containing the connection credentials is invalid
	ReasonInvalidSecret = "InvalidSecret"
)

// ClusterConnectionSpec defines the desired state of ClusterConnection
type ClusterConnectionSpec struct {
	// URL is the address of the remote cluster API server
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// SecretRef is a reference to a secret containing the connection credentials
	// +kubebuilder:validation:Required
	SecretRef SecretReference `json:"secretRef"`
}

// SecretReference defines a reference to a Secret
type SecretReference struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret. If empty, the namespace of the ClusterConnection is used.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ClusterConnectionStatus defines the observed state of ClusterConnection
type ClusterConnectionStatus struct {
	// Conditions represent the current state of the ClusterConnection resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed ClusterConnection.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterConnection represents a connection to a remote cluster.
type ClusterConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterConnectionSpec   `json:"spec"`
	Status ClusterConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterConnectionList contains a list of ClusterConnection
type ClusterConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterConnection{}, &ClusterConnectionList{})
}
