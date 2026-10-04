# Reconciler Configuration

How Muto's Kubernetes reconcilers actually behave today, and what is and isn't tunable.

## Overview

Muto has three reconcilers, each a standard controller-runtime controller registered in `cmd/muto-operator/main.go`: `TenantReconciler`, `AgentJobReconciler`, and `AgentFleetReconciler`. None of them expose worker-count, sync-period, backoff, retry-limit, or job-timeout settings - the operator's manager (`newManager` in `cmd/muto-operator/main.go`) only configures the metrics and health-probe bind addresses; everything else is controller-runtime's own default behavior (1 concurrent reconcile per controller, controller-runtime's built-in rate-limiting requeue, no periodic full resync beyond the informer cache's default resync).

If you're trying to tune reconciliation performance today, there's nothing to set - the knobs described in older drafts of this page (`MUTO_RECONCILER_WORKER_COUNT`, `MUTO_RECONCILER_SYNC_PERIOD`, exponential backoff env vars, per-reconciler worker/timeout vars, a `ReconcilerConfig` CRD) don't exist in the code. This page now describes what each reconciler actually does instead.

## TenantReconciler

Watches `Tenant` resources (`ctrl.NewControllerManagedBy(mgr).For(&v1alpha1.Tenant{})`) and creates the tenant's namespace/A2A gateway deployment. No configurable timeout or retry limit; errors are returned to controller-runtime, which requeues with its default rate-limiting backoff.

## AgentJobReconciler

Watches `AgentJob` resources and the `Pod`s it owns (`.For(&v1alpha1.AgentJob{}).Owns(&corev1.Pod{})`). Its reconcile loop uses a few fixed, hardcoded requeue intervals - not configurable via any env var:

| Situation | Behavior |
|---|---|
| Job just transitioned to `Running`, no pods found yet | Requeue after `1s` |
| Pods exist but not all have reached a terminal phase | Requeue after `5s` |
| All pods finished (job reached `Succeeded`/`Failed`) | Requeue after `job.Spec.TTLAfterCompletion` seconds |
| Job is in a terminal phase and its TTL has elapsed | Transition to `Terminating` (no further requeue needed from this path) |

`AgentJob.spec.ttlAfterCompletion` (seconds, optional) is the only lifecycle timing field on the CRD today - it controls cleanup delay after completion, not a maximum job runtime. There is no `timeout` or `retryPolicy` field on `AgentJob`; a job that never reaches a terminal pod phase is requeued every 5s indefinitely.

## AgentFleetReconciler

Watches `AgentFleet` resources (`.For(&v1alpha1.AgentFleet{})`) and updates fleet status. No configurable timing; same controller-runtime default requeue/backoff as the others on error.

## EventWatcher

`platform/k8s/eventwatcher.go` defines an `EventWatcher` type that subscribes to a message-bus topic for a tenant and invokes a handler per message. It is **not currently instantiated anywhere in `cmd/muto-operator` or `cmd/muto-mcp`** - it exists in the codebase but isn't wired into either binary yet. It has no worker pool, queue, or Kubernetes-event-watching behavior (despite the name, it watches message-bus messages, not Pod/CRD/Namespace/ConfigMap changes - those are handled by the reconcilers' own controller-runtime watches via `For`/`Owns` above).

## Monitoring Reconciler Health

These are controller-runtime's own built-in Prometheus metrics (not Muto-specific), automatically exposed on the operator's `/metrics` endpoint - see [Environment Variables](./environment-variables.md) for `MUTO_METRICS_BIND_ADDRESS`.

```promql
# Reconciliation queue depth
workqueue_depth

# Reconciliation latency
controller_runtime_reconcile_time_seconds

# Failed reconciliations
controller_runtime_reconcile_errors_total

# Retry attempts
workqueue_retries_total
```

Example alerting rules:

```yaml
groups:
  - name: muto-reconcilers
    interval: 30s
    rules:
      - alert: ReconciliationQueueBacklog
        expr: sum by (controller) (workqueue_depth) > 100
        for: 5m
        annotations:
          summary: "Reconciliation queue is backing up"

      - alert: HighReconciliationErrors
        expr: sum by (controller) (rate(controller_runtime_reconcile_errors_total[5m])) > 0.1
        for: 5m
        annotations:
          summary: "High reconciliation error rate"

      - alert: SlowReconciliation
        expr: histogram_quantile(0.95, sum by (controller, le) (rate(controller_runtime_reconcile_time_seconds_bucket[5m]))) > 10
        for: 5m
        annotations:
          summary: "Reconciliation is slow (>10s p95)"
```

### Troubleshooting Checklist

```bash
# Check operator logs
kubectl logs -n muto-system deployment/muto-operator -f

# Check reconciler metrics (the image has no shell or curl, so port-forward)
kubectl port-forward -n muto-system deployment/muto-operator 8080:8080 &
curl -s localhost:8080/metrics | grep -E '^(controller_runtime_reconcile|workqueue_depth)'

# Describe a specific resource
kubectl describe agentjob <job-name> -n <namespace>

# Check reconciliation events
kubectl get events -n <namespace> --sort-by='.lastTimestamp'
```

## If You Need Reconciler Tuning

There's currently no supported way to change worker concurrency, requeue backoff, or add a job-level timeout/retry policy. If your workload needs this, it's a code change (e.g. `controller.Options{MaxConcurrentReconciles: ...}` passed to `ctrl.NewControllerManagedBy(mgr).WithOptions(...)`, or a new field on `AgentJob.Spec`), not a configuration one - open an issue rather than looking for an env var.

## See Also

- [Environment Variables](./environment-variables.md) - Core and platform configuration variables
- [Architecture: Reconcilers](../architecture/reconcilers.md) - How reconcilers work
- [Deployment: Production Checklist](../deployment/production-checklist.md) - Pre-launch verification
