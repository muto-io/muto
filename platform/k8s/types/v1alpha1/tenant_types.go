package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TenantSpec   `json:"spec,omitempty"`
	Status            TenantStatus `json:"status,omitempty"`
}

type TenantSpec struct {
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
	// +kubebuilder:validation:Enum=shared;dedicated
	IsolationTier string        `json:"isolationTier"`
	MessageBus    TenantBusSpec `json:"messageBus,omitempty"`
}

type TenantBusSpec struct {
	// +kubebuilder:validation:Enum=nats;kafka
	Type      string `json:"type"`
	Dedicated bool   `json:"dedicated,omitempty"`
}

type TenantStatus struct {
	Ready bool `json:"ready,omitempty"`
}

// +kubebuilder:object:root=true

type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
