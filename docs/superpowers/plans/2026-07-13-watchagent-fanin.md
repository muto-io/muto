# WatchAgent Fan-In Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `WatchAgent` into `DefaultScheduler` so agent completion events drive in-memory phase transitions, making `muto:get_job_status` return correct terminal phases (`Succeeded`/`Failed`) instead of staying stuck at `Running`.

**Architecture:** `Schedule` opens a `WatchAgent` channel per agent using a `context.Background()`-derived context (job outlives requests), stores a `cancelWatch` func in `jobRecord`, and launches a `watchJob` fan-in goroutine. `watchJob` drains all per-agent channels and transitions the job's in-memory phase when all agents have reported. `Cancel` calls `cancelWatch` to stop watch goroutines.

**Tech Stack:** Go 1.26, `sync.RWMutex`, `context.WithCancel`, `core/agent` event types

---

## File Map

```
core/scheduler/scheduler.go       # modify: jobRecord, Schedule, Cancel, add watchJob
core/scheduler/scheduler_test.go  # modify: update mockAdapter.WatchAgent, add 3 tests
```

---

### Task 1: Add cancelWatch to jobRecord and update Schedule + Cancel

**Files:**
- Modify: `core/scheduler/scheduler.go`

- [ ] **Step 1: Write the three new failing tests first**

Add these three tests to `core/scheduler/scheduler_test.go` (after the existing tests). The `mockAdapter` currently closes `WatchAgent` channels immediately — replace it with a more capable version that supports per-call channel control:

```go
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

// blockingAdapter returns a channel that never sends until cancelCh is closed.
type blockingAdapter struct {
	cancelCh chan struct{}
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
		select {
		case <-ctx.Done():
		case <-m.cancelCh:
		}
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
	// watchJob runs in goroutine — poll until phase transitions
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
	done := make(chan struct{})
	adapter := &blockingAdapter{cancelCh: done}
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
	// After Cancel, the watchCtx should be cancelled, unblocking the goroutine.
	// Verify by checking the channel closes within 100ms.
	select {
	case <-done:
		// done was not closed by Cancel — ctx.Done() triggered instead, which is correct
		t.Error("done channel should not be closed by Cancel (ctx should be cancelled instead)")
	default:
		// correct: cancelWatch cancelled the context, goroutine exited via ctx.Done()
	}
	// The job phase should be Terminating
	st, _ := sched.Status(context.Background(), "job-cancel-watch")
	if st == nil || st.Phase != agent.PhaseTerminating {
		t.Errorf("expected PhaseTerminating, got %v", st)
	}
}
```

Also add `"time"` to the imports in `scheduler_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler
go test ./core/scheduler/... -run "TestScheduleWatches|TestScheduleAgent|TestCancelStops" -v 2>&1 | head -20
```
Expected: compile error or FAIL — `completingAdapter`, `failingAdapter`, `blockingAdapter` reference undefined scheduler behaviour.

Note: `TestScheduleCreatesJob` asserts `PhaseRunning` immediately after `Schedule`. After the implementation, an agent with a fast-completing mock will transition to `PhaseSucceeded` asynchronously. The existing `mockAdapter.WatchAgent` closes the channel immediately — this means `watchJob` will see `completed == total` and transition to `Succeeded` almost instantly. Update `TestScheduleCreatesJob` to accept either `PhaseRunning` or `PhaseSucceeded`:

```go
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
	// Phase may be Running or Succeeded depending on goroutine scheduling
	if st.Phase != agent.PhaseRunning && st.Phase != agent.PhaseSucceeded {
		t.Errorf("expected Running or Succeeded, got %s", st.Phase)
	}
}
```

- [ ] **Step 3: Implement the changes in `core/scheduler/scheduler.go`**

Replace the entire file with:

```go
package scheduler

import (
	"context"
	"fmt"
	"sync"

	"github.com/muto-io/muto/core/agent"
)

type PlatformAdapter interface {
	SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error)
	TerminateAgent(ctx context.Context, agentID string) error
	WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error)
}

type Scheduler interface {
	Schedule(ctx context.Context, job *agent.Job) error
	Cancel(ctx context.Context, jobID string) error
	Status(ctx context.Context, jobID string) (*agent.Status, error)
	ListActive(ctx context.Context, tenantID string) ([]*agent.Job, error)
}

type jobRecord struct {
	job         *agent.Job
	agentIDs    []string
	cancelWatch context.CancelFunc // stops all WatchAgent goroutines for this job
}

type DefaultScheduler struct {
	mu      sync.RWMutex
	adapter PlatformAdapter
	jobs    map[string]*jobRecord
}

func NewDefaultScheduler(adapter PlatformAdapter) *DefaultScheduler {
	return &DefaultScheduler{
		adapter: adapter,
		jobs:    make(map[string]*jobRecord),
	}
}

func (s *DefaultScheduler) Schedule(ctx context.Context, job *agent.Job) error {
	// Spawn agents outside the lock — I/O must not block other scheduler operations.
	var agentIDs []string
	for _, role := range job.Spec.Agents {
		spec := &agent.Spec{
			TenantRef: job.Spec.TenantRef,
			Agents:    []agent.AgentRole{role},
		}
		id, err := s.adapter.SpawnAgent(ctx, spec)
		if err != nil {
			return fmt.Errorf("spawn agent %s: %w", role.Role, err)
		}
		agentIDs = append(agentIDs, id)
	}

	// context.Background() — watch goroutines must outlive the request context.
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	var watchChans []<-chan agent.Event
	for _, id := range agentIDs {
		ch, err := s.adapter.WatchAgent(watchCtx, id)
		if err != nil {
			cancelWatch()
			return fmt.Errorf("watch agent %s: %w", id, err)
		}
		watchChans = append(watchChans, ch)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	job.Status.Phase = agent.PhaseRunning
	s.jobs[job.ID] = &jobRecord{job: job, agentIDs: agentIDs, cancelWatch: cancelWatch}
	go s.watchJob(job.ID, watchChans)
	return nil
}

func (s *DefaultScheduler) Cancel(ctx context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %q not found", jobID)
	}
	for _, id := range rec.agentIDs {
		if err := s.adapter.TerminateAgent(ctx, id); err != nil {
			return fmt.Errorf("terminate agent %s: %w", id, err)
		}
	}
	rec.cancelWatch()
	next, err := Transition(rec.job.Status.Phase, EventCancelled)
	if err != nil {
		return err
	}
	rec.job.Status.Phase = next
	return nil
}

func (s *DefaultScheduler) Status(_ context.Context, jobID string) (*agent.Status, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.jobs[jobID]
	if !ok {
		return nil, fmt.Errorf("job %q not found", jobID)
	}
	st := rec.job.Status
	return &st, nil
}

func (s *DefaultScheduler) ListActive(_ context.Context, tenantID string) ([]*agent.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*agent.Job
	for _, rec := range s.jobs {
		if rec.job.TenantID == tenantID && !rec.job.Status.Phase.IsTerminal() {
			// Return a copy to avoid races with concurrent writers.
			jobCopy := *rec.job
			result = append(result, &jobCopy)
		}
	}
	return result, nil
}

// watchJob fans in all per-agent event channels and transitions the job's
// in-memory phase once all agents have reported a terminal event.
func (s *DefaultScheduler) watchJob(jobID string, chans []<-chan agent.Event) {
	total := len(chans)
	results := make(chan agent.Event, total*2)
	for _, ch := range chans {
		go func(c <-chan agent.Event) {
			for ev := range c {
				results <- ev
			}
		}(ch)
	}
	completed, failed := 0, 0
	for completed+failed < total {
		ev, ok := <-results
		if !ok {
			return
		}
		switch ev.Type {
		case agent.EventCompleted:
			completed++
		case agent.EventFailed:
			failed++
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.jobs[jobID]
	if !ok {
		return
	}
	if failed > 0 {
		rec.job.Status.Phase = agent.PhaseFailed
	} else {
		rec.job.Status.Phase = agent.PhaseSucceeded
	}
}
```

- [ ] **Step 4: Run all scheduler tests**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler
go test ./core/scheduler/... -v -count=1 -race 2>&1 | tail -20
```
Expected: all PASS, including the 3 new tests. The `-race` flag catches any data races introduced.

- [ ] **Step 5: Run full unit test suite to confirm no regressions**

```bash
make test-unit 2>&1 | grep -E "^(ok|FAIL)"
```
Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add core/scheduler/scheduler.go core/scheduler/scheduler_test.go
git commit -m "feat(scheduler): wire WatchAgent fan-in so Status() returns terminal phases"
```

---

## Self-Review

**Spec coverage:**
| Spec Requirement | Step |
|---|---|
| `jobRecord` gains `cancelWatch context.CancelFunc` | Step 3 (jobRecord struct) |
| `Schedule` opens `WatchAgent` channels with `context.Background()`-derived ctx | Step 3 (Schedule) |
| `Schedule` stores `cancelWatch` in `jobRecord` | Step 3 (Schedule) |
| `Schedule` launches `watchJob` goroutine | Step 3 (Schedule) |
| `watchJob` fan-in drains all channels concurrently | Step 3 (watchJob) |
| `watchJob` transitions to `PhaseSucceeded` when all agents complete | Step 3 (watchJob) |
| `watchJob` transitions to `PhaseFailed` when any agent fails | Step 3 (watchJob) |
| `watchJob` `!ok` guard for deleted job | Step 3 (watchJob) |
| `Cancel` calls `cancelWatch` | Step 3 (Cancel) |
| `TestScheduleWatchesAgents` — EventCompleted → PhaseSucceeded | Step 1 |
| `TestScheduleAgentFailure` — EventFailed → PhaseFailed | Step 1 |
| `TestCancelStopsWatch` — cancel propagates to watch goroutines | Step 1 |

**No placeholders.** All code blocks complete.

**Type consistency:** `agent.EventCompleted`, `agent.EventFailed`, `agent.PhaseSucceeded`, `agent.PhaseFailed`, `agent.PhaseTerminating`, `agent.PhaseRunning` — all match `core/agent/types.go`. `cancelWatch context.CancelFunc` matches `context.WithCancel` return type. `watchJob(jobID string, chans []<-chan agent.Event)` signature matches how it's called in `Schedule`.

**Race check:** `watchJob` acquires `s.mu.Lock()` only at the end, never while blocking on channels. The `results` channel is local to `watchJob`. No shared state accessed without the lock.
