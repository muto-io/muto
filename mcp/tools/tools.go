package tools

import (
	"context"
	"fmt"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/core/tenant"
)

type Handlers struct {
	sched scheduler.Scheduler
}

func NewHandlers(sched scheduler.Scheduler) *Handlers {
	return &Handlers{sched: sched}
}

func (h *Handlers) ScheduleAgentJob(ctx context.Context, jobID, tenantID, image, triggerSource string, ttl int32) error {
	topic := tenant.TopicPrefix(tenantID) + jobID
	job := &agent.Job{
		ID:       jobID,
		TenantID: tenantID,
		Spec: agent.Spec{
			TenantRef: tenantID,
			Trigger:   agent.Trigger{Type: agent.TriggerEvent, Source: triggerSource},
			Agents: []agent.AgentRole{
				{Role: "worker", Image: image, MaxReplicas: 1},
			},
			MessageBus:         agent.MessageBusConfig{Topic: topic},
			TTLAfterCompletion: ttl,
		},
	}
	return h.sched.Schedule(ctx, job)
}

func (h *Handlers) GetJobStatus(ctx context.Context, jobID string) (*agent.Status, error) {
	st, err := h.sched.Status(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get status: %w", err)
	}
	return st, nil
}

func (h *Handlers) CancelJob(ctx context.Context, jobID string) error {
	return h.sched.Cancel(ctx, jobID)
}

func (h *Handlers) ListActiveAgents(ctx context.Context, tenantID string) ([]*agent.Job, error) {
	return h.sched.ListActive(ctx, tenantID)
}
