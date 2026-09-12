# Agent Lifecycle: State Machine and Job Transitions

An Agent Job goes through a well-defined lifecycle from creation to completion. Understanding the state machine is essential for monitoring, debugging, and building reliable multi-agent workflows.

## Job State Machine

Every agent job follows this state machine:

```
                        ┌─────────────┐
                        │   Pending   │
                        └──────┬──────┘
                               │
                        ┌──────▼──────┐
                   ┌───►│  Scheduled  │◄───┐
                   │    └──────┬──────┘    │
                   │           │          │
                   │    ┌──────▼──────┐   │ (Retry)
                   │    │   Running   │   │
                   │    └──────┬──────┘   │
                   │           │          │
                   │    ┌──────▼────────┐ │
                   └────┤   Completed   │─┘
                        └───────────────┘
                        
       Alternative paths:
                   ┌──────────────────────┐
                   │      Cancelled       │
                   └──────────────────────┘
                   
                   ┌──────────────────────┐
                   │       Failed         │
                   └──────────────────────┘
```

### State Definitions

#### Pending
**Initial state** — Job created, waiting for scheduler to process.

- Job CRD/API request received
- Job validated (syntax, resource limits, image availability)
- Job stored in persistent storage (etcd for K8s, database for CF)
- Scheduler notified of new job
- No execution has started yet

Transitions:
- -> **Scheduled**: Scheduler accepted job and allocated resources
- -> **Cancelled**: User cancelled before scheduling
- -> **Failed**: Validation failed (e.g., invalid image, missing fields)

#### Scheduled
**Transition state** — Resources allocated, platform-specific execution initiated.

- Platform adapter accepted the job
- Pod created (K8s) or Task queued (CF)
- Agent container image pulled (if not cached)
- Resources reserved but execution may not have started
- Job ready to transition to Running when platform starts execution

Transitions:
- -> **Running**: Platform started executing the job
- -> **Failed**: Platform rejected execution (insufficient resources, image not found)
- -> **Cancelled**: User cancelled during scheduling

#### Running
**Active state** — Agent container executing.

- Container started (entrypoint running)
- Agent processing input
- Agent may emit logs and metrics
- Agent may publish to message bus
- Still consuming CPU/memory

Transitions:
- -> **Completed**: Agent exited successfully (exit code 0)
- -> **Failed**: Agent exited with error (exit code non-zero) or timed out
- -> **Scheduled**: Retrying (if retry policy allows)
- -> **Cancelled**: User cancelled during execution

#### Completed
**Terminal state** — Agent finished successfully.

- Container exited with code 0
- All resources released (CPU, memory, I/O)
- Results available (logs, output files, etc.)
- Job recorded in history for audit trail
- No further transitions

**Final state** — Processing complete.

#### Failed
**Terminal state** — Agent execution failed.

- Container exited with non-zero code, timeout, or platform error
- Failure reason recorded:
  - `ContainerExited(exitCode)` — Non-zero exit
  - `Timeout` — Exceeded timeout threshold
  - `OutOfMemory` — Killed due to memory limit
  - `ImagePullError` — Could not pull container image
  - `SchedulingError` — Platform couldn't schedule job
- Resources released
- Failure details available in job status

Transitions (if retry policy allows):
- -> **Scheduled**: Will retry execution
- **Final state** — If retries exhausted, job terminates

#### Cancelled
**Terminal state** — Job terminated by user or system.

- User explicitly cancelled via API
- System cancelled (e.g., tenant deleted)
- Container killed (SIGTERM, then SIGKILL)
- Resources cleaned up
- May have partial results depending on when cancelled

**Final state** — Job stopped before completion.

## Detailed State Transitions

### Pending -> Scheduled

**Trigger:** Scheduler processes job and allocates resources

```
Reconciliation Loop:
1. Detect job in Pending state
2. Run validation:
   - Check image exists (or assume exists)
   - Check tenant exists and has resources
   - Check job spec is valid
3. Call PlatformAdapter.CreateJob()
   - K8s: Create Pod spec, submit to API server
   - CF: Queue task with Cloud Controller
4. If successful:
   - Update job.Status.State = Scheduled
   - Record job.Status.JobID (from platform)
   - Update job.Status.ScheduledTime
5. If failed:
   - Update job.Status.State = Failed
   - Record failure reason
```

### Scheduled -> Running

**Trigger:** Platform reports execution started

```
Event from platform:
K8s: Pod phase changes to Running
CF: Task state changes to RUNNING

Reconciliation Loop:
1. Watch event notifies of state change
2. Job status updated:
   - job.Status.State = Running
   - job.Status.StartTime = now
3. Begin collecting logs
```

### Running -> Completed

**Trigger:** Platform reports successful execution

```
Event from platform:
K8s: Pod phase changes to Succeeded
CF: Task state changes to SUCCEEDED

Reconciliation Loop:
1. Watch event notifies of completion
2. Job status updated:
   - job.Status.State = Completed
   - job.Status.EndTime = now
   - job.Status.ExitCode = 0
3. Collect final logs
4. Clean up resources (optional, K8s GC handles)
5. Mark job as terminal
```

### Running -> Failed

**Trigger:** Platform reports execution failed

```
Event from platform:
K8s: Pod phase changes to Failed or Unknown
     Container exits with non-zero code
CF: Task state changes to FAILED
    Exit code available in task metadata

Reconciliation Loop:
1. Watch event notifies of failure
2. Analyze failure reason:
   - Container exit code
   - Platform error message
   - Resource constraints
   - Timeout
3. Job status updated:
   - job.Status.State = Failed
   - job.Status.FailureReason = specific cause
   - job.Status.EndTime = now
4. Check retry policy:
   - If retries remaining and retryable:
     - job.Status.State = Scheduled
     - job.Status.RetryCount++
     - Submit retry (goto Scheduled)
   - Else:
     - Job terminates in Failed state
```

### Running -> Cancelled

**Trigger:** User requests cancellation

```
User action:
kubectl delete agentjob/job-name  (K8s)
cf cancel-task job-id             (CF API)

Reconciliation Loop:
1. Delete request observed
2. Verify permission (RBAC, tenant ownership)
3. Send termination signal:
   - K8s: Delete Pod (triggers graceful shutdown)
   - CF: Cancel task
4. Wait for graceful period (default 30s, configurable)
5. Force kill if needed:
   - K8s: kubelet kills container after grace period
   - CF: Task marked as FAILED
6. Update job status:
   - job.Status.State = Cancelled
   - job.Status.EndTime = now
   - job.Status.CancelledBy = username
```

### Scheduled/Running -> Scheduled (Retry)

**Trigger:** Failure with retry policy enabled

```
Retry Policy:
apiVersion: muto.io/v1
kind: AgentJob
spec:
  retryPolicy:
    maxRetries: 3
    backoffSeconds: 5
    backoffMultiplier: 2  # exponential: 5s, 10s, 20s

Reconciliation Loop (on failure):
1. Check job.Status.RetryCount < spec.RetryPolicy.MaxRetries
2. Calculate backoff:
   backoff = baseDelay * (multiplier ^ attemptNumber)
3. Schedule retry after backoff
4. Clear any failed Pod/Task
5. Set job.Status.State = Scheduled
6. Increment job.Status.RetryCount
7. Next reconciliation will try again
```

## Lifecycle Events

Each state transition is recorded as an event:

```
Event: JobPending
  Time: 2026-09-03T10:30:00Z
  Reason: JobCreated
  Message: "Agent job created and ready for scheduling"

Event: JobScheduled
  Time: 2026-09-03T10:30:05Z
  Reason: SchedulerAccepted
  Message: "Job scheduled on Kubernetes, Pod: default/job-name-0"

Event: JobRunning
  Time: 2026-09-03T10:30:10Z
  Reason: PodRunning
  Message: "Agent container started successfully"

Event: JobCompleted
  Time: 2026-09-03T10:35:20Z
  Reason: Succeeded
  Message: "Agent exited with code 0"
```

Applications can watch these events:

```bash
# Kubernetes
kubectl get events --sort-by=.metadata.creationTimestamp
kubectl describe agentjob job-name

# Watch for state changes
kubectl get agentjob job-name -w
```

## Monitoring Job Lifecycle

### Status Queries

Get current job status:

```bash
# Kubernetes
kubectl get agentjob job-name -o jsonpath='{.status.state}'
# Output: Completed

kubectl get agentjob job-name -o json | jq '.status'
# Output:
# {
#   "state": "Completed",
#   "jobId": "k8s-default-job-name-123",
#   "scheduledTime": "2026-09-03T10:30:05Z",
#   "startTime": "2026-09-03T10:30:10Z",
#   "endTime": "2026-09-03T10:35:20Z",
#   "exitCode": 0
# }
```

### Observability

Job lifecycle transitions are recorded in the `AgentJob` status (`kubectl get agentjobs -w`). The operator exposes controller-runtime's reconcile metrics for the `agentjob` controller:

```
# AgentJob reconciliations by result
controller_runtime_reconcile_total{controller="agentjob",result="success"} 1
controller_runtime_reconcile_total{controller="agentjob",result="requeue_after"} 6

# AgentJob reconcile duration
controller_runtime_reconcile_time_seconds_count{controller="agentjob"} 7
```

Job-level metrics (jobs by end state, job duration, running jobs per tenant) and structured lifecycle log events are planned in [#79](https://github.com/muto-io/muto/issues/79).

## Common Patterns

### Waiting for Completion

Applications can wait for job completion by:
- Polling the job status periodically
- Watching for status change events
- Using webhooks to be notified of state changes

The most efficient approach is to watch events or subscribe to webhooks rather than polling the API continuously.

### Handling Retries

For transient failures, let Muto retry automatically:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: flaky-network-job
spec:
  agents:
  - name: api-caller
    image: myorg/api-caller:v1
  retryPolicy:
    maxRetries: 3
    backoffSeconds: 5
    backoffMultiplier: 2
    retryableExitCodes: [1, 2]  # Retry on these codes
```

## Timeout and Cancellation

### Timeout

Jobs have a maximum execution time:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
spec:
  agents:
  - name: long-runner
    image: myorg/runner:v1
  timeout: 1h  # Kill if running longer than 1 hour
```

If job exceeds timeout:
1. Platform sends SIGTERM to container
2. Container gets `terminationGracePeriodSeconds` to shut down gracefully (default 30s)
3. If not terminated, SIGKILL is sent
4. Job transitions to Failed with reason `Timeout`

### Graceful Shutdown

Agents should handle SIGTERM:

---

## Next Steps

- **[Reconcilers](./reconcilers.md)** — How reconciliation drives job lifecycle
- **[Platform Design](./platform-design.md)** — Platform-specific implementations
- **[Concepts (Job States)](../getting-started/concepts.md#job-states)** — Return to concepts
