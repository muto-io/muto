# Reconcilers: Control Loops and Reconciliation Pattern

Reconcilers are the core pattern in Muto. They continuously watch for desired state and make reality match it. This document explains how reconcilers work and describes the built-in reconcilers.

## The Reconciliation Pattern

Reconciliation is a control loop that implements the Kubernetes operator pattern:

```
┌──────────────────────────────────────────────────────┐
│ Reconciliation Loop (runs continuously)              │
└──────────────────────────────────────────────────────┘

WATCH Phase
  │
  ├─ Watch for events on resources (AgentJob, Tenant, etc.)
  ├─ When event detected, add resource to work queue
  │
  ▼

DEQUEUE Phase
  │
  ├─ Get next resource from work queue
  │
  ▼

RECONCILE Phase
  │
  ├─ Load resource spec (desired state)
  ├─ Load current status (actual state)
  ├─ Compare desired vs actual
  ├─ If drift detected:
  │   - Take corrective actions
  │   - Create/update/delete platform resources
  │   - Update resource status
  ├─ If no drift:
  │   - Do nothing (idempotent)
  │
  ▼

QUEUE RESULT Phase
  │
  ├─ If successful:
  │   - Dequeue work item
  ├─ If failed (retryable):
  │   - Requeue with exponential backoff
  │   - Try again after delay
  │ 
  └─ Goto WATCH Phase
```

## Reconciler Interface

<<<<<<< HEAD
All reconcilers implement a standard interface with two main methods:

- **Reconcile()** — Called when a resource needs reconciliation. Takes the resource identifier and returns a result indicating success/failure and retry behavior.
- **SetupWithManager()** — Registers the reconciler with the control manager so it watches the appropriate resources.

### Reconciliation Pattern in Practice

A typical reconciliation flow:

1. **Load desired state** — Fetch the job/tenant/resource definition
2. **Load actual state** — Get the current status from the platform
3. **Compare** — Check if actual matches desired
4. **Act** — If drift detected, take corrective actions (create/update/delete resources)
5. **Update status** — Record the new state in the resource
6. **Return result** — Indicate success or schedule retry if needed
=======
All reconcilers implement this interface:

```go
type Reconciler interface {
    // Reconcile is called when a resource needs reconciliation
    // req contains the resource name/namespace
    // Returns Result indicating success/failure and retry behavior
    Reconcile(ctx context.Context, req Request) Result
    
    // SetupWithManager registers this reconciler with the control manager
    SetupWithManager(mgr Manager) error
}

type Request struct {
    NamespacedName types.NamespacedName  // Namespace/Name of resource
}

type Result struct {
    Requeue      bool           // Requeue this work item?
    RequeueAfter time.Duration  // If requeue, wait this long
}
```

### Reconciliation Example

```go
type AgentJobReconciler struct {
    client       Client
    platformMgr  PlatformManager
    logger       Logger
}

func (r *AgentJobReconciler) Reconcile(ctx context.Context, req Request) Result {
    // Load the desired state
    job := &muto.AgentJob{}
    if err := r.client.Get(ctx, req.NamespacedName, job); err != nil {
        if errors.IsNotFound(err) {
            return Result{}  // Resource deleted, nothing to do
        }
        r.logger.Error(err, "failed to get AgentJob", "name", req.Name)
        return Result{Requeue: true, RequeueAfter: 5 * time.Second}
    }
    
    // Check if job is already terminal
    if job.Status.IsTerminal() {
        return Result{}  // Nothing to do, job is done
    }
    
    // Reconcile based on current state
    switch job.Status.State {
    case muto.Pending:
        return r.reconcilePending(ctx, job)
    case muto.Scheduled:
        return r.reconcileScheduled(ctx, job)
    case muto.Running:
        return r.reconcileRunning(ctx, job)
    default:
        return Result{}
    }
}

func (r *AgentJobReconciler) reconcilePending(ctx context.Context, job *muto.AgentJob) Result {
    // Validate job spec
    if err := r.validateJob(job); err != nil {
        job.Status.State = muto.Failed
        job.Status.FailureReason = err.Error()
        r.client.Status().Update(ctx, job)
        return Result{}  // Terminal state, no retry
    }
    
    // Try to schedule the job
    adapter := r.platformMgr.GetAdapter(job.Spec.Platform)
    jobID, err := adapter.CreateJob(ctx, job)
    if err != nil {
        r.logger.Error(err, "failed to create job")
        return Result{
            Requeue: true,
            RequeueAfter: 10 * time.Second,
        }  // Retry after delay
    }
    
    // Update status
    job.Status.State = muto.Scheduled
    job.Status.JobID = jobID
    job.Status.ScheduledTime = metav1.Now()
    if err := r.client.Status().Update(ctx, job); err != nil {
        r.logger.Error(err, "failed to update job status")
        return Result{Requeue: true}
    }
    
    return Result{}  // Success, no requeue
}
>>>>>>> 70f0a8b (docs: write architecture/reconcilers.md - reconciliation pattern and control loops)
```

## Key Properties of Reconcilers

### Idempotent

<<<<<<< HEAD
Running reconciliation twice with the same inputs produces the same result. If reconciliation runs again on an already-scheduled job, it detects that the Pod already exists and takes no additional action.

### Resilient to Transient Failures

If reconciliation fails, it's automatically retried with exponential backoff. For example, if the API server is temporarily unavailable, reconciliation returns "requeue after 5 seconds". On the next attempt, the API server is back and the job creation succeeds.
=======
Running reconciliation twice with the same inputs produces the same result:

```go
// First reconciliation
job.Status.State = Pending
r.Reconcile(ctx, req)
// job.Status.State = Scheduled, Pod created

// Second reconciliation (same resource, nothing changed)
r.Reconcile(ctx, req)
// job.Status.State still = Scheduled, Pod already exists (no duplicate created)
```

### Resilient to Transient Failures

If reconciliation fails, it's automatically retried:

```go
// Scenario: API server temporarily unavailable
adapter.CreateJob() returns "connection timeout"
return Result{Requeue: true, RequeueAfter: 5s}
// Muto retries after 5 seconds
// On next attempt, API server is back, job creation succeeds
```
>>>>>>> 70f0a8b (docs: write architecture/reconcilers.md - reconciliation pattern and control loops)

### Observable

Each reconciliation loop iteration is logged:

```json
{
  "timestamp": "2026-09-03T10:30:05Z",
  "level": "info",
  "component": "agentjob-reconciler",
  "action": "reconcile_start",
  "resource": "default/data-pipeline",
  "state": "Pending"
}
{
  "timestamp": "2026-09-03T10:30:05.523Z",
  "level": "info",
  "component": "agentjob-reconciler",
  "action": "reconcile_success",
  "resource": "default/data-pipeline",
  "new_state": "Scheduled",
  "duration_ms": 523
}
```

## Built-in Reconcilers

### 1. TenantReconciler

Ensures tenant namespaces/spaces exist and are properly configured.

**Watches:** Tenant CRDs

**Reconciliation logic:**
1. Load Tenant spec (desired configuration)
2. Check if namespace/space exists:
   - K8s: Check if namespace exists
   - CF: Check if space exists
3. If missing: Create it with proper configuration
4. If exists: Update configuration as needed
   - RBAC rules
   - Resource quotas
   - Network policies
   - Storage classes

**Example:**
```yaml
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: tenant-a
spec:
  platform: kubernetes
  kubernetesConfig:
    namespace: tenant-a
    resourceQuota:
      limits.cpu: 10
      limits.memory: 50Gi
    networkPolicy:
      allowIngressFrom:
      - namespaceSelector:
          matchLabels:
            name: muto-system
```

**TenantReconciler ensures:**
- Namespace `tenant-a` exists
- ResourceQuota created with limits
- NetworkPolicy configured to allow muto-system access
- RBAC ServiceAccount and Roles created

### 2. AgentJobReconciler

Ensures agent jobs are executed on the appropriate platform.

**Watches:** AgentJob CRDs

**Reconciliation logic:**
1. Load AgentJob spec (desired execution)
2. Check current job state:
   - **Pending**: Schedule on platform
   - **Scheduled**: Monitor for platform response
   - **Running**: Monitor for completion
   - **Terminal**: Do nothing (job complete)
3. For each state, take appropriate action:
   - **Pending**: Validate spec, call platform adapter
   - **Scheduled**: Check if platform started execution
   - **Running**: Check if platform reports completion

<<<<<<< HEAD
=======
**Pseudocode:**
```go
func (r *AgentJobReconciler) Reconcile(ctx, req) Result {
    job := loadJob(req)
    
    switch job.Status.State {
    case Pending:
        // Validate and schedule
        if err := validateJob(job); err != nil {
            job.Status.State = Failed
            job.Status.FailureReason = err.Error()
            updateStatus(job)
            return Result{}
        }
        
        adapter := selectAdapter(job.Spec.Platform)
        jobID, err := adapter.CreateJob(ctx, job)
        if err != nil {
            return Result{Requeue: true, RequeueAfter: 10s}
        }
        
        job.Status.State = Scheduled
        job.Status.JobID = jobID
        updateStatus(job)
        return Result{}
    
    case Scheduled:
        // Check if execution started
        adapter := selectAdapter(job.Spec.Platform)
        platformStatus, err := adapter.GetJobStatus(ctx, job.Status.JobID)
        if err != nil {
            return Result{Requeue: true}
        }
        
        if platformStatus.State == "Running" {
            job.Status.State = Running
            updateStatus(job)
        }
        return Result{}
    
    case Running:
        // Check if execution finished
        adapter := selectAdapter(job.Spec.Platform)
        platformStatus, err := adapter.GetJobStatus(ctx, job.Status.JobID)
        if err != nil {
            return Result{Requeue: true}
        }
        
        if platformStatus.State == "Completed" {
            job.Status.State = Completed
            updateStatus(job)
        } else if platformStatus.State == "Failed" {
            job.Status.State = Failed
            job.Status.FailureReason = platformStatus.Error
            
            // Check retry policy
            if canRetry(job) {
                job.Status.RetryCount++
                job.Status.State = Scheduled  // Will try again
            }
            updateStatus(job)
        }
        return Result{}
    }
}
```
>>>>>>> 70f0a8b (docs: write architecture/reconcilers.md - reconciliation pattern and control loops)

### 3. EventWatcherReconciler

Watches platform events and triggers reconciliation when state changes.

**Watches:** Platform-emitted events (K8s Events, CF Task state changes)

**Reconciliation logic:**
1. Listen for platform events
2. Correlate event to Muto resource:
   - K8s: Pod owned by AgentJob
   - CF: Task with Muto labels
3. Enqueue parent AgentJob for reconciliation
4. AgentJobReconciler will handle status update

**Example flow:**
```
Kubernetes Event: Pod phase changed to Running
         │
         ├─ EventWatcher observes event
         ├─ Check pod labels: muto.io/job=data-pipeline
         ├─ Load AgentJob: data-pipeline
         ├─ Add to reconciliation queue
         │
         ▼
AgentJobReconciler processes queued item
         │
         ├─ Load AgentJob: data-pipeline
         ├─ Check platform status (Pod is Running)
         ├─ Update job.Status.State = Running
```

### 4. RetryReconciler

Handles retries for failed jobs.

**Watches:** AgentJobs in Failed state with retry policy

**Reconciliation logic:**
1. Load failed AgentJob
2. Check if retry allowed:
   - RetryCount < MaxRetries?
   - Enough time passed for backoff?
   - Exit code is retryable?
3. If retry allowed:
   - Reset state to Pending
   - Increment RetryCount
   - Platform adapter will create new execution
4. If retries exhausted:
   - Job remains in Failed state (terminal)

**Backoff calculation:**
<<<<<<< HEAD
=======
```go
backoff := baseDelay * math.Pow(multiplier, float64(attemptCount))
nextRetryTime := lastFailureTime + backoff
if now < nextRetryTime {
    return Result{Requeue: true, RequeueAfter: nextRetryTime - now}
}
```
>>>>>>> 70f0a8b (docs: write architecture/reconcilers.md - reconciliation pattern and control loops)

## Custom Reconcilers

You can write custom reconcilers for domain-specific logic:

<<<<<<< HEAD
=======
```go
// Example: MetricsReconciler that updates dashboards based on job results
type MetricsReconciler struct {
    metricsClient MetricsClient
    logger        Logger
}

func (r *MetricsReconciler) Reconcile(ctx context.Context, req Request) Result {
    job := &muto.AgentJob{}
    if err := r.client.Get(ctx, req.NamespacedName, job); err != nil {
        return Result{Requeue: true}
    }
    
    // Only process completed jobs
    if job.Status.State != muto.Completed {
        return Result{}
    }
    
    // Extract metrics from job output
    if job.Status.OutputMetrics != nil {
        if err := r.metricsClient.PublishMetrics(ctx, job); err != nil {
            r.logger.Error(err, "failed to publish metrics")
            return Result{Requeue: true, RequeueAfter: 30 * time.Second}
        }
    }
    
    return Result{}
}

// Register custom reconciler
func init() {
    reconcilers.Register("metrics", &MetricsReconciler{})
}
```
>>>>>>> 70f0a8b (docs: write architecture/reconcilers.md - reconciliation pattern and control loops)

To use custom reconcilers, configure them in Muto:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muto-config
data:
  reconcilers: |
    - name: tenant
      enabled: true
    - name: agentjob
      enabled: true
    - name: metrics
      enabled: true
      config:
        metricsEndpoint: http://prometheus:9090
```

## Reconciliation Queues

Muto uses work queues to manage reconciliation:

```
Event Stream
    │
    ├─ New AgentJob created
    ├─ AgentJob status updated
    ├─ Pod completed
    └─ Error occurred
         │
         ▼
┌──────────────────────┐
│ Work Queue           │
│ ├─ default/job-1    │
│ ├─ default/job-2    │
│ ├─ default/job-3    │
│ └─ ...               │
└──────────────────────┘
         │
    ┌────┴────┬────┐
    ▼         ▼    ▼
┌───────────┐
│ Worker 1  │  (Processes reconciliation)
└───────────┘
┌───────────┐
│ Worker 2  │
└───────────┘
┌───────────┐
│ Worker 3  │
└───────────┘
```

Workers are configurable:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muto-config
data:
  reconcilers: |
    - name: agentjob
      workerCount: 4  # 4 parallel workers for job reconciliation
      maxRetries: 5   # Retry up to 5 times
      backoffBase: 1s # Start with 1 second backoff
```

## Reconciliation Guarantees

Muto provides these guarantees:

### At-Least-Once Processing
Every resource change is processed at least once. If a reconciliation fails, it will be retried.

```
New AgentJob created
    │
    ├─ Reconciliation attempt 1: Network error
    │   -> Retry after 5s
    │
    ├─ Reconciliation attempt 2: API timeout
    │   -> Retry after 10s
    │
    └─ Reconciliation attempt 3: Success ✓
       Job scheduled
```

### No Infinite Loops
Reconciliation is idempotent, so repeated executions don't cause harm:

```
Resource X in state Pending
    │
    ├─ Reconciliation 1: Create platform resource -> Pending->Scheduled
    ├─ Reconciliation 2: Platform resource exists -> No duplicate created
    ├─ Reconciliation 3: No change detected -> No action
    └─ Continue monitoring until state changes
```

### Eventual Consistency
Even after transient failures, system converges to desired state:

```
Desired: Job should be running
Current: Failed due to temporary error

After reconciliation retries and automatic recovery:
    -> Job eventually transitions to Running
```

---

## Next Steps

- **[Agent Lifecycle](./agent-lifecycle.md)** — State transitions that reconcilers drive
- **[Platform Design](./platform-design.md)** — Adapters called by reconcilers
- **[Concepts (Control Loop)](../getting-started/concepts.md#control-loop)** — Return to concepts

**Last Updated:** 2026-09-03
