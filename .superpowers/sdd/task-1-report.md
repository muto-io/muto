# Task 1 Report: Fix Double-Schedule Race in Scheduler

**Status:** DONE

**Commit:** `7fc1b7c` — fix(scheduler): prevent double-schedule race by rejecting duplicate jobIDs

## What I found

The fix described in the task (reject duplicate jobIDs under the lock in
`Schedule()`) was **already present, uncommitted**, in the working tree when
I started — `git diff core/scheduler/scheduler.go` showed the check already
implemented, along with an uncommitted test file
(`core/scheduler/scheduler_test.go`) containing three new regression tests.
No `test/integration/scheduler/` directory exists in this repo (only
`core/scheduler/`), so the `TestSchedule` integration-test command in the
task instructions doesn't apply here; I ran the actual scheduler test suite
instead.

## What I implemented / verified

1. **Reviewed** the existing uncommitted diff in
   `core/scheduler/scheduler.go`. It matches the task's intent but goes a
   step further than the minimal snippet in the task: instead of returning
   the error while still holding the lock with agents/watchers already
   spawned, it unlocks first, then cleans up the orphaned watch context and
   terminates the already-spawned agents before returning the "already
   scheduled" error (lines 70–93). This avoids leaking agents/goroutines on
   a rejected duplicate.

2. **Ran the test suite with `-race`** and found two pre-existing data races
   — not in `scheduler.go`, but in the test doubles
   (`core/scheduler/scheduler_test.go`): `mockAdapter.spawned`/`terminated`
   slices and `blockingAdapter.watchCalled` were mutated without
   synchronization. These only surfaced because the new tests
   (`TestScheduleConcurrentDifferentJobIDs`,
   `TestScheduleConcurrentSameJobID`) share a single adapter instance across
   many goroutines. I added a `sync.Mutex` to both `mockAdapter` and
   `blockingAdapter` to guard their fields. This is a test-helper fix only;
   no production code was touched here.

3. **Re-ran `go build ./...`** — succeeds.

4. **Re-ran `go test -v ./core/scheduler/ -race`** — all 13 tests pass
   cleanly, no race warnings.

5. **Committed** `core/scheduler/scheduler.go` and
   `core/scheduler/scheduler_test.go` together (single commit `7fc1b7c`).
   Left `.superpowers/` and `docs/superpowers/` (untracked, unrelated to
   this task) out of the commit.

## Test output (actual)

```
$ go build ./...
(no output — success)

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

Before the mutex fix to the test doubles, `-race` failed with two
`WARNING: DATA RACE` reports in `mockAdapter.SpawnAgent` and
`blockingAdapter.WatchAgent` (both in test helper code, triggered by the new
concurrent tests sharing an adapter across goroutines). Those are resolved.

## Key regression tests added (already present in working tree, verified passing)

- `TestScheduleDuplicateJobIDRejected` — second `Schedule()` call with same
  jobID while first is still running is rejected; original job record and
  phase (`PhaseRunning`) are untouched.
- `TestScheduleConcurrentDifferentJobIDs` — 20 goroutines scheduling distinct
  jobIDs concurrently all succeed, `ListActive` reports all 20.
- `TestScheduleConcurrentSameJobID` — 20 goroutines racing to schedule the
  same jobID: exactly 1 succeeds, `ListActive` reports exactly 1 job. This is
  the direct regression test for the race described in the task.

## Concerns

- The task's step 3 referenced `go test -v ./test/integration/scheduler/
  -run TestSchedule`, but no such directory exists in this repo. I treated
  `core/scheduler/` as the authoritative test location and ran the full
  package there instead. Flagging this in case the coordinator expected a
  separate integration-test layer that doesn't yet exist.
- The scheduler.go fix and its tests were already staged in the working
  directory before I started (not authored by me in this session) — I
  verified, tested, hardened the test doubles, and committed them, but the
  core logic in `scheduler.go` predates my involvement. Worth confirming
  with whoever wrote it that the cleanup-outside-lock behavior (terminating
  agents on a rejected duplicate) is the intended semantics, since it's a
  slightly larger change than the minimal patch described in the task.
