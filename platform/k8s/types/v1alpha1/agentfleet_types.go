package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type AgentFleet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AgentFleetSpec   `json:"spec,omitempty"`
	Status            AgentFleetStatus `json:"status,omitempty"`
}

type AgentFleetSpec struct {
	TenantRef string   `json:"tenantRef"`
	JobRefs   []string `json:"jobRefs,omitempty"`
}

type AgentFleetStatus struct {
	TotalJobs     int32 `json:"totalJobs,omitempty"`
	RunningJobs   int32 `json:"runningJobs,omitempty"`
	CompletedJobs int32 `json:"completedJobs,omitempty"`
}

// +kubebuilder:object:root=true

type AgentFleetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentFleet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentFleet{}, &AgentFleetList{})
}
