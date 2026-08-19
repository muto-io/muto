package tools_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/mcp/tools"
)

type mockScheduler struct {
	scheduled []*agent.Job
}

func (m *mockScheduler) Schedule(_ context.Context, job *agent.Job) error {
	m.scheduled = append(m.scheduled, job)
	return nil
}
func (m *mockScheduler) Cancel(_ context.Context, _ string) error { return nil }
func (m *mockScheduler) Status(_ context.Context, jobID string) (*agent.Status, error) {
	return &agent.Status{Phase: agent.PhaseRunning}, nil
}
func (m *mockScheduler) ListActive(_ context.Context, _ string) ([]*agent.Job, error) {
	return []*agent.Job{{ID: "j1", TenantID: "acme"}}, nil
}

var _ scheduler.Scheduler = (*mockScheduler)(nil)

func TestHandleScheduleAgentJob(t *testing.T) {
	sched := &mockScheduler{}
	h := tools.NewHandlers(sched)

	err := h.ScheduleAgentJob(context.Background(), "test-job", "acme", "worker:latest", "nats://tasks", 300)
	if err != nil {
		t.Fatal(err)
	}
	if len(sched.scheduled) != 1 {
		t.Errorf("expected 1 scheduled job, got %d", len(sched.scheduled))
	}
}

func TestHandleGetJobStatus(t *testing.T) {
	sched := &mockScheduler{}
	h := tools.NewHandlers(sched)

	status, err := h.GetJobStatus(context.Background(), "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agent.PhaseRunning {
		t.Errorf("expected Running, got %s", status.Phase)
	}
}

func TestHandleListActiveAgents(t *testing.T) {
	sched := &mockScheduler{}
	h := tools.NewHandlers(sched)

	jobs, err := h.ListActiveAgents(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(jobs))
	}
}
