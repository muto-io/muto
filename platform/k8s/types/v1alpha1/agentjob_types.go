package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="ActiveAgents",type=integer,JSONPath=".status.activeAgents"

type AgentJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AgentJobSpec   `json:"spec,omitempty"`
	Status            AgentJobStatus `json:"status,omitempty"`
}

type AgentJobSpec struct {
	TenantRef          string          `json:"tenantRef"`
	Trigger            TriggerSpec     `json:"trigger"`
	Agents             []AgentRoleSpec `json:"agents"`
	MessageBus         JobBusSpec      `json:"messageBus,omitempty"`
	TTLAfterCompletion int32           `json:"ttlAfterCompletion,omitempty"`
}

type TriggerSpec struct {
	// +kubebuilder:validation:Enum=event;cron;manual
	Type   string `json:"type"`
	Source string `json:"source,omitempty"`
}

type AgentRoleSpec struct {
	Role        string `json:"role"`
	Image       string `json:"image"`
	MaxReplicas int32  `json:"maxReplicas,omitempty"`
}

type JobBusSpec struct {
	Topic string `json:"topic,omitempty"`
}

type AgentJobStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Terminating
	Phase        string       `json:"phase,omitempty"`
	ActiveAgents int32        `json:"activeAgents,omitempty"`
	StartedAt    *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt  *metav1.Time `json:"completedAt,omitempty"`
}

// +kubebuilder:object:root=true

type AgentJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentJob{}, &AgentJobList{})
}
