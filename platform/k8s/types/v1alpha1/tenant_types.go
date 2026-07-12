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
	// Namespace is the Kubernetes namespace where agent pods for this tenant run.
	// The TenantReconciler creates this namespace if it does not exist.
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
	// IsolationTier controls how strictly this tenant is isolated from others.
	// shared: tenant shares the controller's message bus infrastructure.
	// dedicated: tenant gets its own message bus instance (NATS or Kafka StatefulSet).
	// +kubebuilder:validation:Enum=shared;dedicated
	IsolationTier string        `json:"isolationTier"`
	// MessageBus configures which message bus implementation and mode to use.
	MessageBus    TenantBusSpec `json:"messageBus,omitempty"`
}

type TenantBusSpec struct {
	// Type selects the message bus implementation.
	// nats: NATS JetStream — lightweight, low-latency, suited for simple agent tasks.
	// kafka: Apache Kafka — high-throughput, durable, suited for complex pipelines.
	// +kubebuilder:validation:Enum=nats;kafka
	Type string `json:"type"`
	// Dedicated provisions a per-tenant message bus instance in the tenant namespace.
	// Only applicable when the parent Tenant has isolationTier: dedicated.
	Dedicated bool `json:"dedicated,omitempty"`
}

type TenantStatus struct {
	// Ready is true once the tenant namespace and RBAC have been provisioned
	// and the message bus (if dedicated) is running.
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
