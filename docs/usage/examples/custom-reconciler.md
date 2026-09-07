# Example: Building Custom Reconcilers

Learn how to extend Muto with custom reconciliation logic for domain-specific orchestration.

## Overview

Reconcilers are the heart of Muto. They implement control loops that watch for state changes and take actions to reach desired state. This example shows how to build a custom reconciler for your specific needs.

## Understanding Reconcilers

A reconciler follows this pattern:

```
Watch Events → Detect Drift → Take Action → Verify → Repeat
```

Example: `AgentJobReconciler`:
- **Watch**: Monitor for new/changed AgentJob resources
- **Detect Drift**: Job spec says "Running", but pod doesn't exist
- **Take Action**: Create the pod
- **Verify**: Pod created successfully
- **Repeat**: Watch for next event

## Reconciler Interface

All reconcilers implement this interface:

```go
type Reconciler interface {
    // Reconcile processes one resource and returns whether to retry
    Reconcile(ctx context.Context, req Request) (Result, error)
}

// Request identifies the resource to reconcile
type Request struct {
    Namespace string // Kubernetes namespace
    Name      string // Resource name
}

// Result controls retry behavior
type Result struct {
    Requeue      bool          // True = retry after delay
    RequeueAfter time.Duration // How long to wait before retry
}
```

## Example: Job Status Tracker Reconciler

Let's build a reconciler that tracks job execution times and alerts if jobs exceed thresholds.

### Step 1: Define the CRD

First, define a custom resource to store tracking data:

```yaml
# monitoring-policy.yaml
apiVersion: muto.io/v1
kind: JobMonitoringPolicy
metadata:
  name: default-policy
spec:
  # Maximum allowed job duration
  maxDurationMinutes: 120
  
  # Alert if job duration exceeds threshold
  durationAlertThresholdPercent: 80
  
  # Track metrics
  trackMetrics: true
  
  # Notification webhook
  webhookUrl: "http://monitoring-service/alerts"
```

### Step 2: Implement the Reconciler (Go)

Create a custom reconciler:

```go
package reconcilers

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/go-logr/logr"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/predicate"

    mutov1 "github.com/muto-io/muto/api/v1"
)

// JobStatusTrackerReconciler tracks and alerts on job duration
type JobStatusTrackerReconciler struct {
    client.Client
    Log    logr.Logger
    Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=muto.io,resources=agentjobs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile implements the reconciliation logic
func (r *JobStatusTrackerReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    
    log := r.Log.WithValues("agentjob", req.NamespacedName)
    
    // Step 1: Fetch the job
    job := &mutov1.AgentJob{}
    if err := r.Get(ctx, req.NamespacedName, job); err != nil {
        // Job deleted, nothing to do
        if client.IgnoreNotFound(err) != nil {
            log.Error(err, "failed to fetch AgentJob")
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, nil
    }
    
    // Step 2: Only care about running or completed jobs
    if job.Status.Phase != "Running" && job.Status.Phase != "Completed" {
        return ctrl.Result{}, nil
    }
    
    // Step 3: Calculate current duration
    startTime := job.Status.StartTime
    if startTime == nil {
        return ctrl.Result{}, nil // Job hasn't started yet
    }
    
    var duration time.Duration
    if job.Status.Phase == "Running" {
        duration = time.Since(startTime.Time)
    } else if job.Status.CompletionTime != nil {
        duration = job.Status.CompletionTime.Time.Sub(startTime.Time)
    }
    
    log.Info("tracking job duration", "duration", duration.Minutes(), "phase", job.Status.Phase)
    
    // Step 4: Fetch monitoring policy
    policy := &mutov1.JobMonitoringPolicy{}
    policyName := types.NamespacedName{
        Namespace: job.Namespace,
        Name:      "default-policy",
    }
    
    if err := r.Get(ctx, policyName, policy); err != nil {
        log.Info("no monitoring policy found, skipping tracking")
        return ctrl.Result{}, nil
    }
    
    // Step 5: Check if job exceeds duration threshold
    maxDuration := time.Duration(policy.Spec.MaxDurationMinutes) * time.Minute
    
    if duration > maxDuration {
        log.Info("job exceeded maximum duration", 
            "maxDuration", maxDuration.Minutes(), 
            "actualDuration", duration.Minutes())
        
        // Send alert
        alert := map[string]interface{}{
            "jobName":       job.Name,
            "namespace":     job.Namespace,
            "maxDuration":   maxDuration.Minutes(),
            "actualDuration": duration.Minutes(),
            "percentage":    float64(duration) / float64(maxDuration) * 100,
            "timestamp":     time.Now().Unix(),
        }
        
        if err := r.sendAlert(ctx, policy.Spec.WebhookUrl, alert); err != nil {
            log.Error(err, "failed to send alert")
            // Don't fail reconciliation for webhook errors
        }
        
        // Record Kubernetes event
        r.recordEvent(job, "DurationExceeded", fmt.Sprintf(
            "Job exceeded max duration: %.1f minutes (threshold: %d minutes)",
            duration.Minutes(), policy.Spec.MaxDurationMinutes,
        ))
    }
    
    // Step 6: For running jobs, check again soon
    // For completed jobs, don't requeue
    if job.Status.Phase == "Running" {
        return ctrl.Result{
            Requeue:      true,
            RequeueAfter: 30 * time.Second,
        }, nil
    }
    
    return ctrl.Result{}, nil
}

// Helper: Send alert webhook
func (r *JobStatusTrackerReconciler) sendAlert(
    ctx context.Context,
    webhookUrl string,
    alert map[string]interface{},
) error {
    
    // Marshal alert to JSON
    payload, err := json.Marshal(alert)
    if err != nil {
        return fmt.Errorf("failed to marshal alert: %w", err)
    }
    
    // POST to webhook
    req, err := http.NewRequestWithContext(ctx, "POST", webhookUrl, 
        bytes.NewReader(payload))
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("failed to send webhook: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode >= 400 {
        body, _ := ioutil.ReadAll(resp.Body)
        return fmt.Errorf("webhook returned error: %d: %s", resp.StatusCode, body)
    }
    
    return nil
}

// Helper: Record Kubernetes event
func (r *JobStatusTrackerReconciler) recordEvent(
    job *mutov1.AgentJob,
    reason string,
    message string,
) {
    
    event := &corev1.Event{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s.%x", job.Name, time.Now().UnixNano()),
            Namespace: job.Namespace,
        },
        InvolvedObject: corev1.ObjectReference{
            APIVersion: mutov1.GroupVersion.String(),
            Kind:       "AgentJob",
            Name:       job.Name,
            Namespace:  job.Namespace,
            UID:        job.UID,
        },
        Reason:  reason,
        Message: message,
        Type:    "Warning",
        FirstTimestamp: metav1.NewTime(time.Now()),
        LastTimestamp:  metav1.NewTime(time.Now()),
        Count:  1,
    }
    
    if err := r.Create(context.Background(), event); err != nil {
        r.Log.Error(err, "failed to record event")
    }
}

// SetupWithManager registers this reconciler
func (r *JobStatusTrackerReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&mutov1.AgentJob{}).
        // Only reconcile running and completed jobs
        WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
            job, ok := obj.(*mutov1.AgentJob)
            if !ok {
                return false
            }
            return job.Status.Phase == "Running" || job.Status.Phase == "Completed"
        })).
        Complete(r)
}
```

### Step 3: Register the Reconciler

Add to your main operator file (`cmd/muto-operator/main.go`):

```go
func main() {
    mgr, err := ctrl.NewManager(cfg, ctrl.Options{
        Scheme:             scheme,
        MetricsBindAddress: metricsAddr,
        LeaderElection:     enableLeaderElection,
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }
    
    // Register custom reconcilers
    if err = (&reconcilers.JobStatusTrackerReconciler{
        Client: mgr.GetClient(),
        Log:    ctrl.Log.WithName("reconcilers").WithName("JobStatusTracker"),
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "JobStatusTracker")
        os.Exit(1)
    }
    
    // ... start manager ...
}
```

### Step 4: Deploy and Test

Deploy your custom reconciler:

```bash
# Build operator with custom reconciler
make build

# Deploy
kubectl apply -f config/crd/  # Deploy JobMonitoringPolicy CRD
./bin/muto-operator

# Create monitoring policy
kubectl apply -f - << 'YAML'
apiVersion: muto.io/v1
kind: JobMonitoringPolicy
metadata:
  name: default-policy
  namespace: default
spec:
  maxDurationMinutes: 2      # Alert if jobs > 2 minutes
  webhookUrl: "http://webhook-service/alerts"
YAML

# Create a test job
kubectl apply -f - << 'YAML'
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: long-running-job
spec:
  tenant: default
  agents:
    - name: slow-agent
      image: alpine:latest
      command: ["sh", "-c"]
      args: ["sleep 180"]  # Sleep 3 minutes (exceeds threshold)
YAML

# Watch for alert
# Reconciler will detect duration > 2 minutes and send webhook alert
```

## Common Reconciler Patterns

### Pattern 1: Periodic Cleanup

Remove old jobs after specified time:

```go
func (r *JobCleanupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    job := &mutov1.AgentJob{}
    if err := r.Get(ctx, req.NamespacedName, job); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Only process completed/failed jobs older than TTL
    if job.Status.Phase != "Completed" && job.Status.Phase != "Failed" {
        return ctrl.Result{}, nil
    }
    
    ttl := job.Spec.TTLSecondsAfterFinished
    if ttl == nil {
        return ctrl.Result{}, nil
    }
    
    completionTime := job.Status.CompletionTime
    if completionTime == nil {
        return ctrl.Result{}, nil
    }
    
    deleteTime := completionTime.Add(time.Duration(*ttl) * time.Second)
    if time.Now().After(deleteTime) {
        // Delete job
        if err := r.Delete(ctx, job); err != nil {
            return ctrl.Result{}, err
        }
    }
    
    return ctrl.Result{}, nil
}
```

### Pattern 2: Automatic Scaling

Scale agents based on queue depth:

```go
func (r *JobScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    job := &mutov1.AgentJob{}
    if err := r.Get(ctx, req.NamespacedName, job); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    if job.Status.Phase != "Running" {
        return ctrl.Result{}, nil
    }
    
    // Check queue depth from message bus
    queueDepth, err := r.messagebus.GetQueueDepth(job.Spec.Tenant)
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // Scale parallelism based on queue
    desiredParallelism := calculateParallelism(queueDepth)
    if desiredParallelism != job.Spec.Parallelism {
        job.Spec.Parallelism = desiredParallelism
        if err := r.Update(ctx, job); err != nil {
            return ctrl.Result{}, err
        }
    }
    
    return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}
```

### Pattern 3: Cost Optimization

Pause jobs during high-cost periods:

```go
func (r *JobCostOptimizerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    job := &mutov1.AgentJob{}
    if err := r.Get(ctx, req.NamespacedName, job); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    if job.Status.Phase != "Scheduled" {
        return ctrl.Result{}, nil
    }
    
    // Check if job has low priority and we're in peak pricing hours
    if job.Spec.Priority < 50 && r.isPeakPricingHours() {
        // Delay job to off-peak hours
        delay := r.timeUntilOffPeakHours()
        return ctrl.Result{RequeueAfter: delay}, nil
    }
    
    // Job can start
    job.Status.Phase = "Running"
    return ctrl.Result{}, r.Status().Update(ctx, job)
}
```

## Testing Custom Reconcilers

```go
// test/custom_reconciler_test.go
package test

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    ctrl "sigs.k8s.io/controller-runtime"
    
    "github.com/muto-io/muto/reconcilers"
)

func TestJobStatusTrackerReconciler(t *testing.T) {
    // Create mock client
    client := fake.NewClientBuilder().
        WithObjects(
            &mutov1.AgentJob{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-job",
                    Namespace: "default",
                },
                Status: mutov1.AgentJobStatus{
                    Phase:     "Running",
                    StartTime: &metav1.Time{Time: time.Now().Add(-3 * time.Minute)},
                },
            },
        ).
        Build()
    
    // Create reconciler
    reconciler := &reconcilers.JobStatusTrackerReconciler{
        Client: client,
        Log:    logr.Discard(),
    }
    
    // Reconcile
    req := ctrl.Request{
        NamespacedName: types.NamespacedName{
            Name:      "test-job",
            Namespace: "default",
        },
    }
    
    result, err := reconciler.Reconcile(context.Background(), req)
    assert.NoError(t, err)
    assert.True(t, result.Requeue)
}
```

## Debugging Custom Reconcilers

### Enable Debug Logging

```bash
# Run with debug logging
MUTO_LOG_LEVEL=debug ./bin/muto-operator

# Watch reconciliation logs
kubectl logs -f deployment/muto-operator -n muto-system | grep "JobStatusTracker"
```

### Inspect Reconciler State

```bash
# List custom resources
kubectl get jobmonitoringpolicies

# Check reconciliation events
kubectl describe agentjob test-job

# View reconciler metrics
kubectl port-forward -n muto-system svc/muto-operator 8080:8080
curl http://localhost:8080/metrics | grep custom_reconciler
```

### Common Issues

**Issue**: Reconciler not running
```bash
# Check if registered in manager
kubectl logs deployment/muto-operator | grep "SetupWithManager"

# Verify CRD is installed
kubectl get crd jobmonitoringpolicies.muto.io
```

**Issue**: Reconciler runs but doesn't take action
```bash
# Check RBAC permissions
kubectl describe clusterrole muto-operator

# Add permissions if needed
kubectl edit clusterrole muto-operator
# Add rules for custom resources
```

**Issue**: High CPU usage from reconciler
```bash
# Increase requeue interval
result.RequeueAfter = 5 * time.Minute  # Was 30s

# Add predicate filters to reduce reconciliation triggers
WithEventFilter(predicate....)
```

## Next Steps

- **[Scheduling Agent Jobs](../scheduling-agent-jobs.md)** — Job configuration
- **[Architecture: Reconcilers](../../architecture/reconcilers.md)** — How reconcilers work
- **[Development Setup](../../development/setup.md)** — Set up dev environment
- **[Monitoring](../../operations/monitoring-observability.md)** (coming in Phase 8) — Monitor reconcilers

---

**Last Updated:** 2026-09-03
