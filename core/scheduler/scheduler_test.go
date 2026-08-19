package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
)

type mockAdapter struct {
	mu         sync.Mutex
	spawned    []string
	terminated []string
}

func (m *mockAdapter) SpawnAgent(_ context.Context, spec *agent.Spec) (string, error) {
	id := "agent-" + spec.TenantRef
	m.mu.Lock()
	m.spawned = append(m.spawned, id)
	m.mu.Unlock()
	return id, nil
}
func (m *mockAdapter) TerminateAgent(_ context.Context, id string) error {
	m.mu.Lock()
	m.terminated = append(m.terminated, id)
	m.mu.Unlock()
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
	mu          sync.Mutex
	watchCalled bool
}

func (m *blockingAdapter) SpawnAgent(_ context.Context, spec *agent.Spec) (string, error) {
	return "agent-" + spec.TenantRef, nil
}
func (m *blockingAdapter) TerminateAgent(_ context.Context, _ string) error { return nil }
func (m *blockingAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	m.mu.Lock()
	m.watchCalled = true
	m.mu.Unlock()
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

// TestScheduleDuplicateJobIDRejected verifies that a second Schedule() call
// with a jobID already in flight is rejected instead of overwriting the
// jobRecord out from under the first call's watchJob goroutine.
func TestScheduleDuplicateJobIDRejected(t *testing.T) {
	adapter := &blockingAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)
	job1 := &agent.Job{
		ID:       "job-dup",
		TenantID: "acme",
		Spec: agent.Spec{
			TenantRef: "acme",
			Agents:    []agent.AgentRole{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	if err := sched.Schedule(context.Background(), job1); err != nil {
		t.Fatalf("first schedule: unexpected error: %v", err)
	}

	job2 := &agent.Job{
		ID:       "job-dup",
		TenantID: "acme",
		Spec: agent.Spec{
			TenantRef: "acme",
			Agents:    []agent.AgentRole{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	err := sched.Schedule(context.Background(), job2)
	if err == nil {
		t.Fatal("expected error scheduling duplicate jobID, got nil")
	}

	// The original job record must be untouched.
	st, statusErr := sched.Status(context.Background(), "job-dup")
	if statusErr != nil {
		t.Fatalf("status: unexpected error: %v", statusErr)
	}
	if st.Phase != agent.PhaseRunning {
		t.Errorf("expected original job to remain PhaseRunning, got %v", st.Phase)
	}
}

// TestScheduleConcurrentDifferentJobIDs verifies concurrent Schedule() calls
// with distinct jobIDs all succeed without racing on s.jobs.
func TestScheduleConcurrentDifferentJobIDs(t *testing.T) {
	adapter := &mockAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			job := &agent.Job{
				ID:       "job-concurrent-" + string(rune('a'+i)),
				TenantID: "acme",
				Spec: agent.Spec{
					TenantRef: "acme",
					Agents:    []agent.AgentRole{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
				},
			}
			errs[i] = sched.Schedule(context.Background(), job)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("schedule %d: unexpected error: %v", i, err)
		}
	}

	jobs, err := sched.ListActive(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != n {
		t.Errorf("expected %d active jobs, got %d", n, len(jobs))
	}
}

// TestScheduleConcurrentSameJobID verifies that when many goroutines race to
// schedule the same jobID, exactly one succeeds and the rest are rejected —
// this is the regression test for the double-schedule race.
func TestScheduleConcurrentSameJobID(t *testing.T) {
	adapter := &blockingAdapter{}
	sched := scheduler.NewDefaultScheduler(adapter)

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			job := &agent.Job{
				ID:       "job-race",
				TenantID: "acme",
				Spec: agent.Spec{
					TenantRef: "acme",
					Agents:    []agent.AgentRole{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
				},
			}
			errs[i] = sched.Schedule(context.Background(), job)
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("expected exactly 1 successful schedule, got %d", successes)
	}

	jobs, err := sched.ListActive(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("expected exactly 1 active job, got %d", len(jobs))
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
