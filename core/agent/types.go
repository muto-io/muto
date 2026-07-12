package agent

import "time"

type Phase string

const (
	PhasePending     Phase = "Pending"
	PhaseRunning     Phase = "Running"
	PhaseSucceeded   Phase = "Succeeded"
	PhaseFailed      Phase = "Failed"
	PhaseTerminating Phase = "Terminating"
)

func (p Phase) IsTerminal() bool {
	return p == PhaseSucceeded || p == PhaseFailed
}

type TriggerType string

const (
	TriggerEvent  TriggerType = "event"
	TriggerCron   TriggerType = "cron"
	TriggerManual TriggerType = "manual"
)

type AgentRole struct {
	Role        string
	Image       string  // K8s: container image ref
	Command     string  // CF: task command to run on runner app
	MaxReplicas int32
}

type Trigger struct {
	Type   TriggerType
	Source string
}

type MessageBusConfig struct {
	Topic string
}

type Spec struct {
	TenantRef          string
	Trigger            Trigger
	Agents             []AgentRole
	MessageBus         MessageBusConfig
	TTLAfterCompletion int32
}

type Status struct {
	Phase        Phase
	ActiveAgents int32
	StartedAt    *time.Time
	CompletedAt  *time.Time
}

type Job struct {
	ID       string
	TenantID string
	Spec     Spec
	Status   Status
}

type EventType string

const (
	EventStarted   EventType = "Started"
	EventCompleted EventType = "Completed"
	EventFailed    EventType = "Failed"
)

type Event struct {
	AgentID string
	Type    EventType
	Message string
}

type MsgHandler func(topic string, data []byte) error
