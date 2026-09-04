# Reconciler Configuration

Configure reconciler behavior to optimize performance and reliability.

## Overview

Reconcilers are the core control loops in Muto that ensure reality matches desired state. Proper configuration of reconciler behavior is critical for system performance, reliability, and resource usage.

## Reconciler Types

Muto provides built-in reconcilers, each with independent configuration:

| Reconciler | Purpose | Default Workers |
|------------|---------|-----------------|
| **TenantReconciler** | Creates and manages tenant namespaces/spaces | 2 |
| **AgentJobReconciler** | Schedules and monitors agent jobs | 5 |
| **AgentFleetReconciler** | Manages groups of related jobs | 2 |
| **EventWatcher** | Monitors platform events and triggers reconciliation | 3 |

## Global Reconciler Settings

### Worker Count

**Setting:** `MUTO_RECONCILER_WORKER_COUNT`

Number of concurrent reconciliation workers. Each worker processes one reconciliation request at a time.

```bash
# Development (low resource usage)
export MUTO_RECONCILER_WORKER_COUNT=2

# Standard (medium load)
export MUTO_RECONCILER_WORKER_COUNT=5

# Production (high throughput)
export MUTO_RECONCILER_WORKER_COUNT=20
```

**Tuning Guide:**
- Increase worker count if reconciliation queue is growing
- Decrease if CPU or memory usage is too high
- Each worker uses approximately 50MB of memory
- Monitor queue depth: `muto_reconciliation_queue_depth` metric

### Sync Period

**Setting:** `MUTO_RECONCILER_SYNC_PERIOD`

How often to perform a full resync (reconciliation of all resources).

```bash
# Fast convergence (higher overhead)
export MUTO_RECONCILER_SYNC_PERIOD=10s

# Balanced (recommended)
export MUTO_RECONCILER_SYNC_PERIOD=30s

# Low overhead (slower convergence)
export MUTO_RECONCILER_SYNC_PERIOD=5m
```

**Impact:**
- Shorter periods: Faster recovery from missed events, higher overhead
- Longer periods: Lower overhead, slower recovery
- Must be shorter than operator's watchdog timeout

### Exponential Backoff

When a reconciliation fails, subsequent attempts use exponential backoff to avoid hammering the system.

**Settings:**

```bash
# Base duration (initial backoff)
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_BASE=1s

# Maximum duration (cap on backoff)
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MAX=5m

# Multiplier for each retry
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MULTIPLIER=2.0
```

**Example Backoff Sequence:**
```
Attempt 1: Immediate
Attempt 2: 1s (base)
Attempt 3: 2s (1s * 2)
Attempt 4: 4s (2s * 2)
Attempt 5: 8s (4s * 2)
Attempt 6: 16s (8s * 2)
Attempt 7: 32s (16s * 2, capped at 5m max)
...
Attempt N: 5m (max)
```

**Tuning:**
```bash
# Fast recovery (aggressive retries)
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_BASE=500ms
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MAX=1m
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MULTIPLIER=1.5

# Conservative (gentle retries)
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_BASE=2s
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MAX=10m
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MULTIPLIER=3.0
```

### Max Retries

**Setting:** `MUTO_RECONCILER_MAX_RETRIES`

Maximum number of retries before giving up on a reconciliation.

```bash
# Strict (fewer retries)
export MUTO_RECONCILER_MAX_RETRIES=3

# Standard (balanced)
export MUTO_RECONCILER_MAX_RETRIES=5

# Lenient (many retries)
export MUTO_RECONCILER_MAX_RETRIES=15
```

---

## Tenant Reconciler Configuration

The TenantReconciler manages tenant namespaces and spaces.

### Configuration

```bash
# Number of workers for tenant operations
export MUTO_TENANT_RECONCILER_WORKERS=2

# Timeout for tenant creation
export MUTO_TENANT_RECONCILER_TIMEOUT=30s

# Retry limit for failing tenant operations
export MUTO_TENANT_RECONCILER_MAX_RETRIES=5
```

### Tuning

**When to increase worker count:**
- Slow tenant creation
- High number of tenants in system
- Frequent tenant additions

**Example for multi-tenant SaaS:**
```bash
export MUTO_TENANT_RECONCILER_WORKERS=10
export MUTO_RECONCILER_SYNC_PERIOD=15s
```

---

## Agent Job Reconciler Configuration

The AgentJobReconciler manages job scheduling, monitoring, and lifecycle.

### Configuration

```bash
# Number of workers for job reconciliation
export MUTO_AGENTJOB_RECONCILER_WORKERS=5

# Timeout for job operations
export MUTO_AGENTJOB_RECONCILER_TIMEOUT=60s

# Retry limit for failing job operations
export MUTO_AGENTJOB_RECONCILER_MAX_RETRIES=5
```

### Job Timeout Settings

**Default Timeout:**
```bash
export MUTO_JOB_TIMEOUT_DEFAULT=30m
```

**Maximum Timeout:**
```bash
# Maximum job runtime allowed
export MUTO_JOB_TIMEOUT_MAX=24h
```

Timeouts allow operators to set upper bounds on job execution:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: long-running-job
spec:
  agents:
    - name: processor
      image: myorg/processor:v1
  timeout: 2h  # This job can run up to 2 hours
  retryPolicy:
    maxRetries: 3
    backoffSeconds: 30
```

### Tuning

**For many concurrent jobs:**
```bash
export MUTO_AGENTJOB_RECONCILER_WORKERS=20
export MUTO_MAX_CONCURRENT_JOBS=500
export MUTO_RECONCILER_SYNC_PERIOD=15s
```

**For long-running jobs:**
```bash
export MUTO_JOB_TIMEOUT_MAX=48h
export MUTO_AGENTJOB_RECONCILER_TIMEOUT=120s
```

---

## Event Watcher Configuration

The EventWatcher monitors platform events and triggers reconciliation.

### Configuration

```bash
# Number of event processing workers
export MUTO_EVENT_WATCHER_WORKERS=3

# Buffer size for event queue
export MUTO_EVENT_WATCHER_QUEUE_SIZE=1000

# Timeout for event processing
export MUTO_EVENT_WATCHER_TIMEOUT=30s
```

### Event Types Watched

For Kubernetes:
- Pod creation/deletion/status change
- CRD resource changes (AgentJob, Tenant)
- Namespace creation/deletion
- ConfigMap/Secret changes

For CloudFoundry:
- Task start/stop/completion
- Space creation/deletion
- App changes
- Service updates

### Tuning

**For high-volume environments:**
```bash
export MUTO_EVENT_WATCHER_WORKERS=5
export MUTO_EVENT_WATCHER_QUEUE_SIZE=5000
```

---

## Performance Tuning Guide

### Scenario: Slow Job Scheduling

**Symptoms:**
- Jobs stuck in Pending state
- High reconciliation latency
- AgentJobReconciler queue growing

**Solution:**
```bash
# Increase job reconciler workers
export MUTO_AGENTJOB_RECONCILER_WORKERS=15

# Reduce backoff for faster retries
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_BASE=500ms
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MAX=2m

# More frequent full resync
export MUTO_RECONCILER_SYNC_PERIOD=15s
```

### Scenario: High CPU Usage

**Symptoms:**
- Operator CPU at 80%+
- High memory usage
- Excessive event processing

**Solution:**
```bash
# Reduce worker count
export MUTO_RECONCILER_WORKER_COUNT=5

# Increase sync period
export MUTO_RECONCILER_SYNC_PERIOD=60s

# Reduce event queue size
export MUTO_EVENT_WATCHER_QUEUE_SIZE=500

# Increase backoff to reduce retries
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MULTIPLIER=3.0
```

### Scenario: Many Failing Jobs

**Symptoms:**
- High retry rates
- Many job failures
- Operator logs show repeated reconciliation errors

**Solution:**
```bash
# Increase retry limit
export MUTO_RECONCILER_MAX_RETRIES=10

# More aggressive retry timing
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_BASE=500ms
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MULTIPLIER=1.5

# Investigate root cause in operator logs
kubectl logs -n muto-system deployment/muto-operator -f
```

### Scenario: Slow Tenant Creation

**Symptoms:**
- Tenants slow to become ready
- High latency in tenant operations
- TenantReconciler queue backing up

**Solution:**
```bash
# Increase tenant reconciler workers
export MUTO_TENANT_RECONCILER_WORKERS=8

# Reduce timeout for quick failure/retry
export MUTO_TENANT_RECONCILER_TIMEOUT=20s

# More frequent reconciliation
export MUTO_RECONCILER_SYNC_PERIOD=10s
```

---

## ReconcilerConfig CRD (Kubernetes)

On Kubernetes, you can create ReconcilerConfig resources to tune reconcilers per-namespace:

```yaml
apiVersion: muto.io/v1
kind: ReconcilerConfig
metadata:
  name: custom-reconciler
  namespace: muto-system
spec:
  # Apply to jobs in this namespace
  targetNamespace: production-tenant
  
  # Override global settings
  workerCount: 10
  syncPeriodSeconds: 20
  maxRetries: 8
  
  # Backoff configuration
  exponentialBackoff:
    baseSeconds: 1
    maxSeconds: 300
    multiplier: 2.0
  
  # Job-specific settings
  jobTimeout:
    default: "1h"
    maximum: "48h"
  
  # Resource limits
  resourceLimits:
    maxConcurrentJobs: 200
    maxQueueSize: 2000
```

Apply with:
```bash
kubectl apply -f reconciler-config.yaml
```

---

## Monitoring Reconciler Health

### Key Metrics

Monitor these Prometheus metrics to understand reconciler health:

```promql
# Reconciliation queue depth
muto_reconciliation_queue_depth

# Reconciliation latency
muto_reconciliation_duration_seconds

# Failed reconciliations
muto_reconciliation_errors_total

# Retry attempts
muto_reconciliation_retries_total

# Worker utilization
muto_reconciler_workers_active
```

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: muto-reconcilers
    interval: 30s
    rules:
      - alert: ReconciliationQueueBacklog
        expr: muto_reconciliation_queue_depth > 100
        for: 5m
        annotations:
          summary: "Reconciliation queue is backing up"
          
      - alert: HighReconciliationErrors
        expr: rate(muto_reconciliation_errors_total[5m]) > 0.1
        for: 5m
        annotations:
          summary: "High reconciliation error rate"
          
      - alert: SlowReconciliation
        expr: histogram_quantile(0.95, muto_reconciliation_duration_seconds) > 10
        for: 5m
        annotations:
          summary: "Reconciliation is slow (>10s p95)"
```

### Troubleshooting Checklist

```bash
# Check operator logs
kubectl logs -n muto-system deployment/muto-operator -f

# Check reconciler metrics
kubectl exec -n muto-system deployment/muto-operator -- curl localhost:8081/metrics | grep reconcil

# Check event watcher queue
kubectl exec -n muto-system deployment/muto-operator -- curl localhost:8081/metrics | grep event_watcher

# Get reconciler configuration
kubectl get reconcilerconfigs -A

# Describe a specific resource
kubectl describe agentjob <job-name> -n <namespace>

# Check reconciliation events
kubectl get events -n <namespace> --sort-by='.lastTimestamp'
```

---

## Best Practices

1. **Start conservative.** Start with default settings and tune based on observed behavior.

2. **Monitor before tuning.** Collect metrics for at least one hour before making changes.

3. **Change one setting at a time.** This helps isolate the effect of each change.

4. **Test in staging.** Always test reconciler configuration changes in a staging environment first.

5. **Document your tuning.** Record why you changed each setting for future reference.

6. **Regular reviews.** Review reconciler configuration quarterly or when workload changes significantly.

7. **Balance trade-offs.** Higher worker count = faster reconciliation but higher resource usage.

8. **Watch queue depth.** If queue depth is consistently > 50, increase worker count.

---

## Common Configurations

### Development/Testing

```bash
export MUTO_RECONCILER_WORKER_COUNT=2
export MUTO_RECONCILER_SYNC_PERIOD=30s
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_BASE=1s
export MUTO_RECONCILER_MAX_RETRIES=3
```

### Small Production Cluster (< 50 jobs)

```bash
export MUTO_RECONCILER_WORKER_COUNT=5
export MUTO_RECONCILER_SYNC_PERIOD=30s
export MUTO_AGENTJOB_RECONCILER_WORKERS=5
export MUTO_TENANT_RECONCILER_WORKERS=2
export MUTO_MAX_CONCURRENT_JOBS=50
```

### Medium Production Cluster (50-500 jobs)

```bash
export MUTO_RECONCILER_WORKER_COUNT=10
export MUTO_RECONCILER_SYNC_PERIOD=20s
export MUTO_AGENTJOB_RECONCILER_WORKERS=12
export MUTO_TENANT_RECONCILER_WORKERS=4
export MUTO_MAX_CONCURRENT_JOBS=200
```

### Large Production Cluster (> 500 jobs)

```bash
export MUTO_RECONCILER_WORKER_COUNT=20
export MUTO_RECONCILER_SYNC_PERIOD=15s
export MUTO_AGENTJOB_RECONCILER_WORKERS=25
export MUTO_TENANT_RECONCILER_WORKERS=8
export MUTO_EVENT_WATCHER_WORKERS=5
export MUTO_MAX_CONCURRENT_JOBS=500
```

---

## See Also

- [Environment Variables](./environment-variables.md) — All configuration options
- [Message Bus Setup](./message-bus-setup.md) — Message bus tuning
- [Architecture: Reconcilers](../architecture/reconcilers.md) — How reconcilers work
- [Deployment: Production Checklist](../deployment/production-checklist.md) — Pre-launch verification

---

**Last Updated:** 2026-09-03
