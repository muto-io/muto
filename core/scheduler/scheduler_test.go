package scheduler_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
)

type mockAdapter struct {
	spawned    []string
	terminated []string
}

func (m *mockAdapter) SpawnAgent(_ context.Context, spec *agent.Spec) (string, error) {
	id := "agent-" + spec.TenantRef
	m.spawned = append(m.spawned, id)
	return id, nil
}
func (m *mockAdapter) TerminateAgent(_ context.Context, id string) error {
	m.terminated = append(m.terminated, id)
	return nil
}
func (m *mockAdapter) WatchAgent(_ context.Context, _ string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event)
	close(ch)
	return ch, nil
}

func TestScheduleCreatesJob(t *testing.T) {
	adapter := &mockAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)
	job := &agent.Job{
		ID:       "job-1",
		TenantID: "acme",
		Spec: agent.Spec{
			TenantRef: "acme",
			Agents:    []agent.AgentRole{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	if err := sched.Schedule(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	st, err := sched.Status(context.Background(), "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase != agent.PhaseRunning {
		t.Errorf("expected Running, got %s", st.Phase)
	}
}

func TestCancelJob(t *testing.T) {
	adapter := &mockAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)
	job := &agent.Job{ID: "job-2", TenantID: "acme", Spec: agent.Spec{TenantRef: "acme", Agents: []agent.AgentRole{{Role: "w", Image: "i", MaxReplicas: 1}}}}
	_ = sched.Schedule(context.Background(), job)
	if err := sched.Cancel(context.Background(), "job-2"); err != nil {
		t.Fatal(err)
	}
	st, _ := sched.Status(context.Background(), "job-2")
	if st.Phase != agent.PhaseTerminating {
		t.Errorf("expected Terminating, got %s", st.Phase)
	}
}

func TestListActive(t *testing.T) {
	adapter := &mockAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)
	for _, id := range []string{"j1", "j2"} {
		_ = sched.Schedule(context.Background(), &agent.Job{ID: id, TenantID: "acme", Spec: agent.Spec{TenantRef: "acme", Agents: []agent.AgentRole{{Role: "w", Image: "i", MaxReplicas: 1}}}})
	}
	jobs, err := sched.ListActive(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}
