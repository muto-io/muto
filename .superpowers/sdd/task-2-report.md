# Task 2 Report: Fix Lock Contention in Cancel()

**Status:** DONE_WITH_CONCERNS (see "Re-verification" section below — concern is
about a *later, unrelated* task, not this one)

## Summary

Replaced the `Cancel()` method in `core/scheduler/scheduler.go` so that
`TerminateAgent` I/O calls happen outside the scheduler's mutex. The lock is
now acquired twice: once briefly to snapshot the fields needed for I/O
(`agentIDs`, `cancelWatch`), and once more afterward to perform the state
transition. After re-acquiring the lock, the job record is re-fetched from
`s.jobs` to guard against the record having been removed while the lock was
released (in which case `Cancel` now returns `nil` instead of erroring).

One deviation from the literal snippet in the task: the snippet included an
unused local `job := rec.job` copy. The editor/linter auto-removed this dead
assignment immediately after the edit was applied (confirmed via `go build`
succeeding with no errors), since it was never referenced later in the
function. The removal does not change behavior — `rec.job` is used directly
after the second lock acquisition. Final code:

```go
func (s *DefaultScheduler) Cancel(ctx context.Context, jobID string) error {
	s.mu.Lock()
	rec, ok := s.jobs[jobID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("job %q not found", jobID)
	}
	// Copy fields needed for I/O, then release lock
	agentIDs := rec.agentIDs
	cancelWatch := rec.cancelWatch
	s.mu.Unlock()

	// Perform I/O outside the lock
	for _, id := range agentIDs {
		if err := s.adapter.TerminateAgent(ctx, id); err != nil {
			return fmt.Errorf("terminate agent %s: %w", id, err)
		}
	}

	// Re-acquire lock only for state transition
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify job still exists and re-check state
	rec, ok = s.jobs[jobID]
	if !ok {
		return nil // Already removed
	}

	cancelWatch()
	next, err := Transition(rec.job.Status.Phase, EventCancelled)
	if err != nil {
		return err
	}
	rec.job.Status.Phase = next
	return nil
}
```

## Test Output

```
$ go test -v ./core/scheduler/ -race
=== RUN   TestScheduleWatchesAgents
--- PASS: TestScheduleWatchesAgents (0.01s)
=== RUN   TestScheduleAgentFailure
--- PASS: TestScheduleAgentFailure (0.01s)
=== RUN   TestCancelStopsWatch
--- PASS: TestCancelStopsWatch (0.00s)
=== RUN   TestScheduleCreatesJob
--- PASS: TestScheduleCreatesJob (0.00s)
=== RUN   TestCancelJob
--- PASS: TestCancelJob (0.00s)
=== RUN   TestScheduleDuplicateJobIDRejected
--- PASS: TestScheduleDuplicateJobIDRejected (0.00s)
=== RUN   TestScheduleConcurrentDifferentJobIDs
--- PASS: TestScheduleConcurrentDifferentJobIDs (0.00s)
=== RUN   TestScheduleConcurrentSameJobID
--- PASS: TestScheduleConcurrentSameJobID (0.00s)
=== RUN   TestListActive
--- PASS: TestListActive (0.00s)
=== RUN   TestTransitionPendingToRunning
--- PASS: TestTransitionPendingToRunning (0.00s)
=== RUN   TestTransitionRunningToSucceeded
--- PASS: TestTransitionRunningToSucceeded (0.00s)
=== RUN   TestTransitionRunningToFailed
--- PASS: TestTransitionRunningToFailed (0.00s)
=== RUN   TestTransitionToTerminating
--- PASS: TestTransitionToTerminating (0.00s)
=== RUN   TestInvalidTransition
--- PASS: TestInvalidTransition (0.00s)
PASS
ok  	github.com/muto-io/muto/core/scheduler	1.032s
```

All 14 tests passed, including under `-race`.

## Commit

- **Hash:** `1875df0a020eb774a80a5cc589d16997d52ba038`
- **Message:** `fix(scheduler): move I/O outside lock in Cancel() to prevent contention`
- **Files changed:** `core/scheduler/scheduler.go` (20 insertions, 4 deletions)

## Notes

- `go` was not on `PATH` in this environment; used
  `/home/zpascal/go-sdk/go1.26.2/bin/go` directly.
- Working tree is otherwise clean except for pre-existing untracked
  `.superpowers/` and `docs/superpowers/` directories, unrelated to this task.

## Re-verification (second pass, after later commits landed)

This file was picked up by a second, concurrently-running execution of Task 2
after the above had already been implemented and committed as `1875df0` by a
parallel agent instance. During that second pass:

1. Confirmed `core/scheduler/scheduler.go` at current `HEAD` already contains
   exactly the Cancel() implementation described above (byte-for-byte match,
   including the dropped unused `job` variable). `git status` shows no
   uncommitted changes to this file — nothing left to commit for Task 2.
2. In between, two more commits landed on this branch from other parallel
   task executions:
   - `c5f72b7` — "fix: implement 6 remaining code review fixes" (Tasks 3–8),
     which added `delete(s.jobs, jobID)` to `watchJob()` when a job reaches a
     terminal phase, plus a `Transition()`-based phase-set in `Schedule()`.
   - `fb05d3f` — "fix: critical mutex deadlock and phase initialization in
     Schedule()", which fixed a deadlock the `c5f72b7` change had introduced
     (a `return` after `s.mu.Lock()` with no matching `s.mu.Unlock()` on the
     `Transition()` error path), and initializes `job.Status.Phase` to
     `PhasePending` when unset.
3. Re-ran the verification commands against current `HEAD` (`fb05d3f`):

   ```
   $ go test -v ./core/scheduler/ -run TestCancel -timeout 60s
   === RUN   TestCancelStopsWatch
   --- PASS: TestCancelStopsWatch (0.00s)
   === RUN   TestCancelJob
   --- PASS: TestCancelJob (0.00s)
   PASS
   ok  	github.com/muto-io/muto/core/scheduler	0.001s
   ```

   ```
   $ go test -v ./core/scheduler/ -race -timeout 120s
   === RUN   TestScheduleWatchesAgents
       scheduler_test.go:121: expected PhaseSucceeded, got <nil>
   --- FAIL: TestScheduleWatchesAgents (0.51s)
   === RUN   TestScheduleAgentFailure
       scheduler_test.go:148: expected PhaseFailed, got <nil>
   --- FAIL: TestScheduleAgentFailure (0.51s)
   === RUN   TestCancelStopsWatch
   --- PASS: TestCancelStopsWatch (0.00s)
   === RUN   TestScheduleCreatesJob
   --- PASS: TestScheduleCreatesJob (0.00s)
   === RUN   TestCancelJob
   --- PASS: TestCancelJob (0.00s)
   === RUN   TestScheduleDuplicateJobIDRejected
   --- PASS: TestScheduleDuplicateJobIDRejected (0.00s)
   === RUN   TestScheduleConcurrentDifferentJobIDs
   --- PASS: TestScheduleConcurrentDifferentJobIDs (0.00s)
   === RUN   TestScheduleConcurrentSameJobID
   --- PASS: TestScheduleConcurrentSameJobID (0.00s)
   === RUN   TestListActive
   --- PASS: TestListActive (0.00s)
   === RUN   TestTransitionPendingToRunning
   --- PASS: TestTransitionPendingToRunning (0.00s)
   === RUN   TestTransitionRunningToSucceeded
   --- PASS: TestTransitionRunningToSucceeded (0.00s)
   === RUN   TestTransitionRunningToFailed
   --- PASS: TestTransitionRunningToFailed (0.00s)
   === RUN   TestTransitionToTerminating
   --- PASS: TestTransitionToTerminating (0.00s)
   === RUN   TestInvalidTransition
   --- PASS: TestInvalidTransition (0.00s)
   FAIL
   FAIL	github.com/muto-io/muto/core/scheduler	1.030s
   ```

4. **Root cause of the 2 failures — not Task 2:** both `TestCancelStopsWatch`
   and `TestCancelJob` (the tests that exercise Cancel()) still pass, and the
   race detector reports zero data races anywhere in the package. The two
   failures are in tests that never call `Cancel()`. They fail because
   `watchJob()` (changed by the `c5f72b7` "Task 3" commit) now does
   `delete(s.jobs, jobID)` in the same critical section where it sets the
   terminal phase. `completingAdapter`/`failingAdapter` complete
   near-instantly, so by the time the test's polling loop calls `Status()`,
   the record is frequently already gone from the map — `Status()` returns
   `(nil, err)` for a job that legitimately finished, instead of the terminal
   phase. This is a design gap in Task 3's cleanup logic (deleting
   immediately on terminal phase races with any caller trying to observe that
   terminal phase via `Status()`), not a regression caused by this task's
   Cancel() change.
5. Flagged this to the coordinator (`main`) via `SendMessage` mid-task,
   along with an earlier transient deadlock (see below) that has since been
   fixed by `fb05d3f`.

**Earlier transient issue (self-resolved):** partway through this second
pass, a read of `scheduler.go` showed the `c5f72b7` change already applied
but *before* `fb05d3f` landed — at that point `Schedule()` would deadlock
`s.mu` on the `Transition()` error path for any job without
`Status.Phase == PhasePending` pre-set (i.e. every test job). This was
reported to the coordinator; `fb05d3f` (committed independently, not by this
pass) fixed it before further action was needed.

**Conclusion:** Task 2 itself is correctly implemented, committed
(`1875df0`), and verified — Cancel()'s I/O runs outside the lock, the lock is
re-acquired only for the state transition, and `-race` finds no data races.
No further commit was needed from this pass since the working tree already
matched the required implementation exactly. The two currently-failing tests
in the full `-race` run are an out-of-scope regression from Task 3's map
cleanup and should be tracked/fixed separately (e.g. `Status()` should still
report the last known terminal phase for a short grace period, or `ListActive`
/`Status` callers need a different contract for "job finished and was
reaped").
