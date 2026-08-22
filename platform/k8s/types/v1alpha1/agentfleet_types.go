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
	// TenantRef is the name of the Tenant that owns this fleet.
	TenantRef string `json:"tenantRef"`
	// JobRefs lists the names of AgentJob resources that belong to this fleet.
	// Fleet-level operations (e.g. cancel) apply to all listed jobs.
	JobRefs []string `json:"jobRefs,omitempty"`
}

type AgentFleetStatus struct {
	// TotalJobs is the total number of AgentJobs in this fleet.
	TotalJobs int32 `json:"totalJobs,omitempty"`
	// RunningJobs is the number of AgentJobs currently in Running phase.
	RunningJobs int32 `json:"runningJobs,omitempty"`
	// CompletedJobs is the number of AgentJobs in Succeeded or Failed phase.
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
