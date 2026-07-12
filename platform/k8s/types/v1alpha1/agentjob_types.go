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
	// TenantRef is the name of the Tenant that owns this job.
	// The AgentJob must be created in the namespace defined by that Tenant.
	TenantRef string `json:"tenantRef"`
	// Trigger defines what initiates this agent job.
	Trigger TriggerSpec `json:"trigger"`
	// Agents lists the agent roles to spawn for this job.
	// At least one agent role is required.
	Agents []AgentRoleSpec `json:"agents"`
	// MessageBus configures the message bus topic for agent communication.
	MessageBus JobBusSpec `json:"messageBus,omitempty"`
	// TTLAfterCompletion is the number of seconds to wait after the job
	// reaches a terminal phase (Succeeded or Failed) before deleting it
	// and all associated pods. 0 means no automatic cleanup.
	// +kubebuilder:validation:Minimum=0
	TTLAfterCompletion int32 `json:"ttlAfterCompletion,omitempty"`
}

type TriggerSpec struct {
	// Type specifies how this job is triggered.
	// event: triggered by an incoming message on the source topic.
	// cron: triggered on a schedule (source is a cron expression).
	// manual: triggered explicitly via the API or MCP tool.
	// +kubebuilder:validation:Enum=event;cron;manual
	Type string `json:"type"`
	// Source is the trigger origin — a message bus topic URL for event triggers,
	// or a cron expression for cron triggers.
	Source string `json:"source,omitempty"`
}

type AgentRoleSpec struct {
	// Role is the functional name of this agent (e.g. "coordinator", "worker").
	// Used to label pods and derive runner app names on CF.
	Role string `json:"role"`
	// Image is the container image to run when deploying on Kubernetes.
	// Required when MUTO_PLATFORM=k8s.
	Image string `json:"image,omitempty"`
	// Command is the task command to run on the CF runner app.
	// Required when MUTO_PLATFORM=cf.
	Command string `json:"command,omitempty"`
	// MaxReplicas caps the number of concurrent instances for this agent role.
	// Defaults to 1 if unset.
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas,omitempty"`
}

type JobBusSpec struct {
	// Topic is the message bus topic prefix for agent-to-agent communication.
	// Automatically namespaced to tenant.<tenantRef>.<topic> at runtime.
	Topic string `json:"topic,omitempty"`
}

type AgentJobStatus struct {
	// Phase is the current lifecycle phase of the AgentJob.
	// Pending: job created, agents not yet spawned.
	// Running: agents are active.
	// Succeeded: all agents completed successfully.
	// Failed: one or more agents failed.
	// Terminating: cleanup in progress.
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Terminating
	Phase        string       `json:"phase,omitempty"`
	// ActiveAgents is the count of currently running agent instances.
	ActiveAgents int32        `json:"activeAgents,omitempty"`
	// StartedAt is the time the job transitioned to Running.
	StartedAt    *metav1.Time `json:"startedAt,omitempty"`
	// CompletedAt is the time the job reached a terminal phase.
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
