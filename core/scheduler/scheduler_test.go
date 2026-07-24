package scheduler_test

import (
	"context"
	"testing"
	"time"

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

// completingAdapter returns a channel that immediately sends EventCompleted.
type completingAdapter struct {
	spawned    []string
	terminated []string
}

func (m *completingAdapter) SpawnAgent(_ context.Context, spec *agent.Spec) (string, error) {
	id := "agent-" + spec.TenantRef
	m.spawned = append(m.spawned, id)
	return id, nil
}
func (m *completingAdapter) TerminateAgent(_ context.Context, id string) error {
	m.terminated = append(m.terminated, id)
	return nil
}
func (m *completingAdapter) WatchAgent(_ context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 1)
	ch <- agent.Event{AgentID: agentID, Type: agent.EventCompleted}
	close(ch)
	return ch, nil
}

// failingAdapter returns a channel that immediately sends EventFailed.
type failingAdapter struct{ spawned []string }

func (m *failingAdapter) SpawnAgent(_ context.Context, spec *agent.Spec) (string, error) {
	id := "agent-" + spec.TenantRef
	m.spawned = append(m.spawned, id)
	return id, nil
}
func (m *failingAdapter) TerminateAgent(_ context.Context, _ string) error { return nil }
func (m *failingAdapter) WatchAgent(_ context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 1)
	ch <- agent.Event{AgentID: agentID, Type: agent.EventFailed}
	close(ch)
	return ch, nil
}

// blockingAdapter returns a channel that blocks until ctx is cancelled.
type blockingAdapter struct {
	watchCalled bool
}

func (m *blockingAdapter) SpawnAgent(_ context.Context, spec *agent.Spec) (string, error) {
	return "agent-" + spec.TenantRef, nil
}
func (m *blockingAdapter) TerminateAgent(_ context.Context, _ string) error { return nil }
func (m *blockingAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	m.watchCalled = true
	ch := make(chan agent.Event)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

func TestScheduleWatchesAgents(t *testing.T) {
	adapter := &completingAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)
	job := &agent.Job{
		ID:       "job-watch",
		TenantID: "acme",
		Spec: agent.Spec{
			TenantRef: "acme",
			Agents:    []agent.AgentRole{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	if err := sched.Schedule(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	var st *agent.Status
	for i := 0; i < 50; i++ {
		st, _ = sched.Status(context.Background(), "job-watch")
		if st != nil && st.Phase == agent.PhaseSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st == nil || st.Phase != agent.PhaseSucceeded {
		t.Errorf("expected PhaseSucceeded, got %v", st)
	}
}

func TestScheduleAgentFailure(t *testing.T) {
	adapter := &failingAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)
	job := &agent.Job{
		ID:       "job-fail",
		TenantID: "acme",
		Spec: agent.Spec{
			TenantRef: "acme",
			Agents:    []agent.AgentRole{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	if err := sched.Schedule(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	var st *agent.Status
	for i := 0; i < 50; i++ {
		st, _ = sched.Status(context.Background(), "job-fail")
		if st != nil && st.Phase == agent.PhaseFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st == nil || st.Phase != agent.PhaseFailed {
		t.Errorf("expected PhaseFailed, got %v", st)
	}
}

func TestCancelStopsWatch(t *testing.T) {
	adapter := &blockingAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)
	job := &agent.Job{
		ID:       "job-cancel-watch",
		TenantID: "acme",
		Spec: agent.Spec{
			TenantRef: "acme",
			Agents:    []agent.AgentRole{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	if err := sched.Schedule(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !adapter.watchCalled {
		t.Error("expected WatchAgent to be called after Schedule")
	}
	if err := sched.Cancel(context.Background(), "job-cancel-watch"); err != nil {
		t.Fatal(err)
	}
	st, _ := sched.Status(context.Background(), "job-cancel-watch")
	if st == nil || st.Phase != agent.PhaseTerminating {
		t.Errorf("expected PhaseTerminating, got %v", st)
	}
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
	if st.Phase != agent.PhaseRunning && st.Phase != agent.PhaseSucceeded {
		t.Errorf("expected Running or Succeeded, got %s", st.Phase)
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
