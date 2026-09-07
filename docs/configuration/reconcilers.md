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

**Type:** `duration`  
**Default:** `5m`

```bash
# Fast resync (detects drift faster, more overhead)
export MUTO_RECONCILER_SYNC_PERIOD=1m

# Standard (balance between detection and overhead)
export MUTO_RECONCILER_SYNC_PERIOD=5m

# Relaxed (minimal overhead, slower drift detection)
export MUTO_RECONCILER_SYNC_PERIOD=15m
```

**Tuning Guide:**
- Shorter period = catch drift faster (important for strict consistency)
- Longer period = lower overhead (for large clusters)
- Typical value: 5-15 minutes

### Reconciliation Timeout

**Setting:** `MUTO_RECONCILER_TIMEOUT`

Maximum time to wait for a reconciliation operation to complete.

**Type:** `duration`  
**Default:** `30s`

```bash
# Fast timeout (strict, may requeue)
export MUTO_RECONCILER_TIMEOUT=10s

# Standard timeout
export MUTO_RECONCILER_TIMEOUT=30s

# Relaxed timeout (for slow platforms)
export MUTO_RECONCILER_TIMEOUT=60s
```

## Per-Reconciler Configuration

### TenantReconciler

**Setting:** `MUTO_TENANT_RECONCILER_ENABLED`

Enable/disable tenant reconciliation.

```bash
export MUTO_TENANT_RECONCILER_ENABLED=true
```

**Setting:** `MUTO_TENANT_RECONCILER_WORKERS`

Worker count for tenant reconciliation (overrides global setting).

```bash
export MUTO_TENANT_RECONCILER_WORKERS=4
```

### AgentJobReconciler

**Setting:** `MUTO_AGENTJOB_RECONCILER_ENABLED`

Enable/disable agent job reconciliation.

```bash
export MUTO_AGENTJOB_RECONCILER_ENABLED=true
```

**Setting:** `MUTO_AGENTJOB_RECONCILER_WORKERS`

Worker count for job reconciliation (overrides global setting).

```bash
export MUTO_AGENTJOB_RECONCILER_WORKERS=10
```

**Setting:** `MUTO_JOB_DEFAULT_TIMEOUT`

Default timeout for jobs if not specified in AgentJob.

**Type:** `duration`  
**Default:** `1h`

```bash
export MUTO_JOB_DEFAULT_TIMEOUT=30m
```

### AgentFleetReconciler

**Setting:** `MUTO_AGENTFLEET_RECONCILER_ENABLED`

Enable/disable fleet reconciliation.

```bash
export MUTO_AGENTFLEET_RECONCILER_ENABLED=true
```

**Setting:** `MUTO_AGENTFLEET_RECONCILER_WORKERS`

Worker count for fleet reconciliation (overrides global setting).

```bash
export MUTO_AGENTFLEET_RECONCILER_WORKERS=3
```

### EventWatcher

**Setting:** `MUTO_EVENT_WATCHER_ENABLED`

Enable/disable platform event watching.

```bash
export MUTO_EVENT_WATCHER_ENABLED=true
```

**Setting:** `MUTO_EVENT_WATCHER_WORKERS`

Worker count for event processing (overrides global setting).

```bash
export MUTO_EVENT_WATCHER_WORKERS=5
```

**Setting:** `MUTO_EVENT_BUFFER_SIZE`

Buffer size for queued events.

**Type:** `integer`  
**Default:** `1000`

```bash
# Small buffer (low memory, risk of event drops)
export MUTO_EVENT_BUFFER_SIZE=100

# Large buffer (high memory, capture more events)
export MUTO_EVENT_BUFFER_SIZE=10000
```

## Retry Policy

### Reconciliation Retries

**Setting:** `MUTO_RECONCILER_MAX_RETRIES`

Maximum number of times to retry a failed reconciliation.

**Type:** `integer`  
**Default:** `3`

```bash
export MUTO_RECONCILER_MAX_RETRIES=5
```

### Backoff Strategy

**Setting:** `MUTO_RECONCILER_BACKOFF_INITIAL`

Initial backoff delay for failed reconciliations.

**Type:** `duration`  
**Default:** `100ms`

```bash
export MUTO_RECONCILER_BACKOFF_INITIAL=500ms
```

**Setting:** `MUTO_RECONCILER_BACKOFF_MAX`

Maximum backoff delay.

**Type:** `duration`  
**Default:** `5m`

```bash
export MUTO_RECONCILER_BACKOFF_MAX=10m
```

## Performance Tuning

### Optimization for Small Clusters

```bash
# Use fewer workers, longer sync period
export MUTO_RECONCILER_WORKER_COUNT=2
export MUTO_RECONCILER_SYNC_PERIOD=10m
export MUTO_RECONCILER_TIMEOUT=60s
export MUTO_EVENT_BUFFER_SIZE=100
```

**Use case:** Development, small test clusters

### Optimization for Medium Clusters

```bash
# Balanced configuration
export MUTO_RECONCILER_WORKER_COUNT=5
export MUTO_RECONCILER_SYNC_PERIOD=5m
export MUTO_RECONCILER_TIMEOUT=30s
export MUTO_EVENT_BUFFER_SIZE=1000
```

**Use case:** Staging, small production deployments

### Optimization for Large Clusters

```bash
# More workers, faster detection
export MUTO_RECONCILER_WORKER_COUNT=20
export MUTO_RECONCILER_SYNC_PERIOD=2m
export MUTO_RECONCILER_TIMEOUT=10s
export MUTO_EVENT_BUFFER_SIZE=5000
```

**Use case:** Large production deployments with many tenants/jobs

### Optimization for High-Throughput Workloads

```bash
# Aggressive settings for rapid job scheduling
export MUTO_RECONCILER_WORKER_COUNT=50
export MUTO_AGENTJOB_RECONCILER_WORKERS=30
export MUTO_RECONCILER_SYNC_PERIOD=1m
export MUTO_EVENT_BUFFER_SIZE=10000
export MUTO_RECONCILER_MAX_RETRIES=5
```

**Use case:** Burst job scheduling, rapid scaling

## Monitoring Reconciler Health

### Key Metrics

Monitor these Prometheus metrics:

- `muto_reconciliation_queue_depth` — Number of pending reconciliations
- `muto_reconciliation_duration_seconds` — Time to complete reconciliations
- `muto_reconciliation_errors_total` — Failed reconciliation count
- `muto_reconciler_workers_busy` — Number of active workers
- `muto_event_buffer_size` — Number of buffered events

### Alerts to Configure

**Alert: High Reconciliation Queue Depth**

```
muto_reconciliation_queue_depth > 100
```

**Alert: Reconciliation Taking Too Long**

```
moto_reconciliation_duration_seconds > 30s
```

**Alert: Event Buffer Approaching Capacity**

```
muto_event_buffer_size > 9000
```

## Troubleshooting

### Problem: Reconciliation Queue Growing

**Symptoms:**
- `muto_reconciliation_queue_depth` steadily increasing
- Jobs slow to start or become stuck in Pending

**Solutions:**
1. Increase worker count: `MUTO_RECONCILER_WORKER_COUNT`
2. Increase sync period: `MUTO_RECONCILER_SYNC_PERIOD`
3. Check platform API responsiveness
4. Review logs for specific reconciliation errors

### Problem: High Memory Usage

**Symptoms:**
- Pod memory growing over time
- OOMKilled after running for hours

**Solutions:**
1. Reduce worker count: `MUTO_RECONCILER_WORKER_COUNT`
2. Reduce event buffer: `MUTO_EVENT_BUFFER_SIZE`
3. Increase sync period to reduce frequency
4. Monitor for resource leaks in logs

### Problem: Events Being Dropped

**Symptoms:**
- Job status updates delayed or missing
- Jobs stuck in Running state after completion

**Solutions:**
1. Increase event buffer: `MUTO_EVENT_BUFFER_SIZE`
2. Increase event watcher workers: `MUTO_EVENT_WATCHER_WORKERS`
3. Check platform event API is responding

## Related Documentation

- [:octicons-book-24: **Architecture: Reconcilers**](../architecture/reconcilers.md) — Reconciliation design patterns
- [:octicons-book-24: **Environment Variables**](./env-vars.md) — All configuration options
- [:octicons-book-24: **Best Practices**](../usage/best-practices.md) — Production configuration guide
