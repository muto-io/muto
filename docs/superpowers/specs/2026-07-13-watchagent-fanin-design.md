# WatchAgent Fan-In Wiring Design Spec

**Date:** 2026-07-13
**Status:** Approved

---

## Summary

Wire the `WatchAgent` channel into `DefaultScheduler` so that agent completion events drive in-memory phase transitions (`Running → Succeeded/Failed`). Currently `Schedule` calls `SpawnAgent` but never calls `WatchAgent`, leaving `Status()` stuck at `Running` permanently. This fix makes `muto:get_job_status` return correct terminal phases via the MCP server.

---

## Problem

`DefaultScheduler.Schedule` calls `SpawnAgent` (creates the agent) but never calls `WatchAgent` (opens the completion channel). `jobRecord` has no `cancelWatch` field. The K8s reconciler drives `AgentJob` CRD status correctly in-cluster, but the in-memory `DefaultScheduler` state — which `muto:get_job_status` reads — stays at `PhaseRunning` forever.

---

## Changes

### 1. `jobRecord` — add `cancelWatch`

```go
type jobRecord struct {
    job         *agent.Job
    agentIDs    []string
    cancelWatch context.CancelFunc // stops all WatchAgent goroutines for this job
}
```

Watch channels are not stored — they are handed off to `watchJob` immediately and never read outside it.

### 2. `DefaultScheduler.Schedule` — open watch channels

After all `SpawnAgent` calls succeed:

```go
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
```

**Why `context.Background()` not the request ctx:** The request `ctx` is the MCP handler's context, cancelled when the HTTP response is sent — much shorter than job lifetime. Watch goroutines must outlive the request.

### 3. `DefaultScheduler.watchJob` — fan-in goroutine

New method. Drains all per-agent channels concurrently via an internal `results` channel. When `completed + failed == total`, transitions the job's in-memory phase:

```go
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
        ev := <-results
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

### 4. `DefaultScheduler.Cancel` — stop watch goroutines

After `TerminateAgent` calls and before the phase transition, add:

```go
rec.cancelWatch()
```

This stops all `WatchAgent` goroutines for the cancelled job immediately.

---

## Files Changed

```
core/scheduler/scheduler.go       # jobRecord + Schedule + watchJob + Cancel
core/scheduler/scheduler_test.go  # 3 new tests
```

---

## Tests

Three new tests in `core/scheduler/scheduler_test.go`:

### TestScheduleWatchesAgents
Mock adapter's `WatchAgent` returns a channel that immediately sends `EventCompleted`. After `Schedule`, spin-wait until `Status` returns `PhaseSucceeded`. Confirms fan-in drives the phase transition.

### TestScheduleAgentFailure
Mock adapter's `WatchAgent` returns a channel that immediately sends `EventFailed`. Assert `Status` eventually returns `PhaseFailed`.

### TestCancelStopsWatch
Mock adapter's `WatchAgent` returns a channel backed by a goroutine blocked on a `done` channel. After `Cancel`, assert the `done` channel is closed (i.e. `cancelWatch` was called). Confirms cancel propagates to watch goroutines.

---

## Invariants

- `watchJob` goroutine holds no locks while waiting on channels — only acquires `s.mu` at the very end to write the phase.
- If the job is deleted from `s.jobs` before `watchJob` finishes (e.g. by a concurrent cleanup), the `!ok` guard silently exits — no panic.
- `cancelWatch` is always set before the goroutine is launched — no nil-pointer risk.
- `context.Background()` for `watchCtx` is intentional: job lifecycle is longer than any single request context.
