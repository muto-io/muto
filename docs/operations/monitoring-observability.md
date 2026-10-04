# Monitoring and Observability

How to monitor the Muto operator as it works today: health probes, Prometheus metrics, and logs.

!!! note "Current state"
    Muto ships controller-runtime's built-in observability plus custom `muto_*` Prometheus metrics for reconciliations and AgentJobs, configurable JSON/console logging, and OpenTelemetry tracing (off by default — set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable). This page describes the operator's actual behavior.

## Overview

| Signal | Status | Where |
|---|---|---|
| Health probes | ✅ Available | `:8081/healthz`, `:8081/readyz` |
| Prometheus metrics | ⚠️ controller-runtime built-in metrics only | `:8080/metrics` |
| Logs | ⚠️ Plain-text key/value lines, fixed level and format | stderr (container logs) |
| Custom `muto_*` metrics | ✅ Available | `:8080/metrics` |
| Distributed tracing (OpenTelemetry) | ✅ Available (off by default) | OTLP/HTTP export; set `OTEL_EXPORTER_OTLP_ENDPOINT` to enable |

This applies to `muto-operator`. The MCP server (`muto-mcp`) communicates over stdio and has no health or metrics endpoints, but it does export OpenTelemetry traces when configured — tracing is push-based (OTLP export), so it needs no inbound endpoint.

Both ports default to the values shown above, but the bind addresses are configurable via the `MUTO_METRICS_BIND_ADDRESS` and `MUTO_HEALTH_PROBE_BIND_ADDRESS` environment variables (see [Environment Variables](../configuration/environment-variables.md)); setting `MUTO_METRICS_BIND_ADDRESS=0` disables the metrics server entirely. The Helm chart only exposes the port *number* via `metrics.port`/`healthProbe.port` - see [Scraping](#scraping) for how `metrics.enabled` drives the bind address.

The commands on this page assume the chart was installed with `helm install muto-operator ...`, which creates the Deployment `muto-operator` in `muto-system`. Adjust the name if you used a different release name.

## Health Checks

The operator serves its probe endpoints on port **8081**:

| Endpoint | Purpose | Healthy response |
|---|---|---|
| `/healthz` | Liveness | `200 ok` |
| `/readyz` | Readiness | `200 ok` |

Both endpoints use controller-runtime's `healthz.Ping` check. They confirm that the operator process is running and serving HTTP. They do **not** check API server connectivity or informer cache sync.

The Helm chart configures the probes on the named container port `health` (`healthProbe.port`, default `8081`):

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: health
  initialDelaySeconds: 15
  periodSeconds: 20
readinessProbe:
  httpGet:
    path: /readyz
    port: health
  initialDelaySeconds: 5
  periodSeconds: 10
```

The operator image is distroless (no shell, no `curl`), so check the endpoints with a port-forward:

```bash
kubectl port-forward -n muto-system deployment/muto-operator 8081:8081 &
curl http://localhost:8081/healthz   # ok
curl http://localhost:8081/readyz    # ok
```

### CloudFoundry

`deploy/cf/manifest.yml` runs the operator with `no-route: true` and `health-check-type: process`, so CF only checks that the process is alive. Don't switch to an `http` health check. CF sends HTTP health checks to the app port (`8080` by default), which is the operator's metrics server, and `/healthz` returns `404` there.

## Prometheus Metrics

The operator serves Prometheus metrics at `:8080/metrics` over plain HTTP without authentication. This includes both the metrics that controller-runtime and client-go register by default, and Muto's own `muto_*` families.

### Available Metrics

**Reconciliation.** The `controller` label is `tenant`, `agentjob` or `agentfleet`.

| Metric | Type | Description |
|---|---|---|
| `controller_runtime_reconcile_total` | counter | Reconciliations by `controller` and `result` (`success`, `error`, `requeue`, `requeue_after`) |
| `controller_runtime_reconcile_errors_total` | counter | Reconciliations that returned an error |
| `controller_runtime_terminal_reconcile_errors_total` | counter | Reconciliations that returned a terminal (not retried) error |
| `controller_runtime_reconcile_panics_total` | counter | Panics recovered in a reconciler |
| `controller_runtime_reconcile_timeouts_total` | counter | Reconciliations that hit the reconcile timeout |
| `controller_runtime_reconcile_time_seconds` | histogram | Reconcile duration |
| `controller_runtime_active_workers` | gauge | Workers currently reconciling |
| `controller_runtime_max_concurrent_reconciles` | gauge | Maximum number of concurrent reconciles per controller |

**Work queues.** Labels are `controller` and `name`; `workqueue_depth` also has `priority`.

| Metric | Type | Description |
|---|---|---|
| `workqueue_depth` | gauge | Objects waiting to be reconciled |
| `workqueue_adds_total` | counter | Objects added to the queue |
| `workqueue_retries_total` | counter | Objects re-queued after an error or requeue |
| `workqueue_queue_duration_seconds` | histogram | Time an object waits in the queue before it is reconciled |
| `workqueue_work_duration_seconds` | histogram | Time spent reconciling an object |
| `workqueue_unfinished_work_seconds` | gauge | Seconds of reconcile work in progress |
| `workqueue_longest_running_processor_seconds` | gauge | Duration of the longest-running reconcile |

**Kubernetes API client**

| Metric | Type | Description |
|---|---|---|
| `rest_client_requests_total` | counter | Requests to the API server by `code`, `method` and `host` |

The endpoint also exposes Go runtime (`go_*`) and process (`process_*`) metrics. The `certwatcher_*` and `controller_runtime_*webhook_panics_total` counters stay at `0` because the operator serves no webhooks.

**Job & reconciliation metrics (`muto_*`).** Registered on the same registry as the metrics above, so they appear on the same `:8080/metrics` endpoint with no extra setup.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `muto_reconciliations_total` | counter | `reconciler` (`tenant`, `agentjob`, `agentfleet`), `result` (`success`, `error`) | Total reconcile calls |
| `muto_reconciliation_duration_seconds` | histogram | `reconciler` | Reconcile call duration |
| `muto_jobs_total` | counter | `tenant`, `status` (`succeeded`, `failed`) | AgentJobs that reached a terminal state |
| `muto_job_duration_seconds` | histogram | `tenant`, `status` | AgentJob duration from start to completion |
| `muto_agents_running` | gauge | `tenant` | Agent pods currently running |
| `muto_job_queue_depth` | gauge | `tenant` | AgentJobs currently in the `Running` phase (despite the name, this does not include `Pending` jobs - see Known Limitations) |

`muto_reconciliations_total{reconciler="agentjob",result="success"}` is dominated by `AgentJobReconciler`'s 5-second polling requeue while a job runs (unlike `controller_runtime_reconcile_total`, which separates `requeue_after` from `success`) - expect a high baseline rate, not an anomaly.

Labeled histograms and gauges appear only after their first observation. For example, `controller_runtime_reconcile_time_seconds` and `workqueue_depth` show up once the operator has reconciled an object.

Job-level `muto_*` metrics are documented above. Message bus metrics are not implemented yet; they're planned in [#79].

### Scraping

When `metrics.enabled` is `true` (the default), the Helm chart adds these annotations to the operator pod:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "8080"
prometheus.io/path: "/metrics"
```

Prometheus setups that honor these annotations pick up the operator automatically. A typical example is `kubernetes_sd_configs` with `role: pod` plus annotation relabeling. Setting `metrics.enabled: false` disables the metrics server itself, not just its discovery. The Helm chart sets `MUTO_METRICS_BIND_ADDRESS=0` on the container, which controller-runtime treats as server disabled, and it also skips the annotations and the metrics `Service`/`ServiceMonitor`.

The chart also creates a `Service` exposing the metrics port whenever `metrics.enabled` is `true`, and an optional `ServiceMonitor` (`monitoring.coreos.com/v1`, requires the Prometheus Operator's CRDs) when `metrics.serviceMonitor.enabled` is also set - off by default, since not every cluster runs the Prometheus Operator:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 30s
    # Some Prometheus Operator setups only watch ServiceMonitors carrying a
    # specific label, e.g. release: kube-prometheus-stack:
    additionalLabels: {}
```

**The metrics endpoint is unauthenticated by design** (plain HTTP, no `SecureServing`), matching controller-runtime's own default. If your cluster's security posture requires restricting who can scrape it, do so at the network layer - a `NetworkPolicy` scoping access to your Prometheus namespace - rather than assuming the endpoint itself checks credentials.

Quick check:

```bash
kubectl port-forward -n muto-system deployment/muto-operator 8080:8080 &
curl -s http://localhost:8080/metrics | grep '^controller_runtime_reconcile_total'
```

Excerpt after creating a Tenant and an AgentJob:

```
controller_runtime_reconcile_total{controller="agentjob",result="requeue_after"} 6
controller_runtime_reconcile_total{controller="agentjob",result="success"} 1
controller_runtime_reconcile_total{controller="tenant",result="success"} 2
```

### OTLP Export

Prometheus scraping is pull-based and requires something in-cluster to reach the operator's `/metrics` port - not an option on every platform (e.g. CF, where there's no `ServiceMonitor` equivalent). As an alternative, the operator can push the same `muto_reconciliations_total`, `muto_reconciliation_duration_seconds`, `muto_jobs_total`, `muto_job_duration_seconds`, `muto_agents_running`, and `muto_job_queue_depth` observations over OTLP, under OTel's dotted naming convention (`muto.jobs.total`, etc.). It's off by default and independent of Prometheus scraping - enabling one doesn't disable the other.

Set `OTEL_EXPORTER_OTLP_ENDPOINT` (shared with [tracing](#distributed-tracing)) or `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` to enable it; the same `OTEL_SERVICE_NAME`/`OTEL_RESOURCE_ATTRIBUTES` resource attributes apply. See `core/metrics.Init` in `cmd/muto-operator/main.go`.

### PromQL Queries

**Reconcile rate per controller:**
```promql
sum by (controller) (rate(controller_runtime_reconcile_total[5m]))
```

**Reconcile error ratio:**
```promql
sum by (controller) (rate(controller_runtime_reconcile_errors_total[5m]))
/
sum by (controller) (rate(controller_runtime_reconcile_total[5m]))
```

**P99 reconcile duration:**
```promql
histogram_quantile(0.99, sum by (controller, le) (rate(controller_runtime_reconcile_time_seconds_bucket[5m])))
```

**Reconcile backlog:**
```promql
sum by (controller) (workqueue_depth)
```

**P95 time objects wait in the queue:**
```promql
histogram_quantile(0.95, sum by (controller, le) (rate(workqueue_queue_duration_seconds_bucket[5m])))
```

**Failed API server requests by status code:**
```promql
sum by (code) (rate(rest_client_requests_total{code!~"2.."}[5m]))
```

## Logging

The `muto-operator` writes to **stdout** and `muto-mcp` writes to **stderr**. Each line has a timestamp, the logger name, and key/value pairs:

JSON (default):
```json
{"level":"info","ts":"2026-09-12T09:43:29.123Z","logger":"muto-operator","msg":"starting muto-operator","platform":"k8s"}
{"level":"info","ts":"2026-09-12T09:46:39.456Z","logger":"muto-operator.tenant","msg":"adding tenant finalizer","controllerGroup":"muto.io","controllerKind":"Tenant","Tenant":{"name":"demo-tenant"},"namespace":"","name":"demo-tenant","reconcileID":"c0c3b48e-78e4-42f5-86ae-334acbd8aa8f","tenant":"demo-tenant","finalizer":"muto.io/tenant-cleanup"}
```

Console (`MUTO_LOG_FORMAT=console`):
```
2026-09-12T09:43:29.123Z	INFO	muto-operator	starting muto-operator	{"platform": "k8s"}
```

- Format and level are configurable via `MUTO_LOG_FORMAT` (`json`, default, or `console`) and `MUTO_LOG_LEVEL` (`debug`/`info`/`warn`/`error`, default `info`); both are matched case-insensitively. See [Environment Variables](../configuration/environment-variables.md).
- Setting `MUTO_LOG_LEVEL=debug` raises verbosity so `V(1)`-and-above messages are emitted too; `warn` or `error` narrows output instead.
- Errors are logged with an `"error"` key.
- Timestamps are ISO 8601 with millisecond precision and in UTC.

### Viewing Logs

#### Kubernetes

```bash
# Operator logs
kubectl logs -n muto-system deployment/muto-operator

# Follow logs in real time
kubectl logs -n muto-system deployment/muto-operator -f

# Logs from the last hour
kubectl logs -n muto-system deployment/muto-operator --since=1h

# Logs from before the last container restart
kubectl logs -n muto-system deployment/muto-operator --previous
```

#### CloudFoundry

The operator writes to stderr, so its lines appear as `ERR` in `cf logs`.

```bash
# Recent logs
cf logs muto-operator --recent

# Stream logs
cf logs muto-operator
```

### Filtering Logs

```bash
# Errors only
kubectl logs -n muto-system deployment/muto-operator | grep '"error"='

# One reconciler
kubectl logs -n muto-system deployment/muto-operator | grep '"controller"="agentjob"'

# One object
kubectl logs -n muto-system deployment/muto-operator | grep '"name"="my-job"'

# One reconciliation
kubectl logs -n muto-system deployment/muto-operator | grep '"reconcileID"="<id>"'
```

### Log Aggregation

Any collector that ships container logs works, such as Fluent Bit, Grafana Alloy/Promtail, Vector, or Filebeat. The lines aren't JSON, so parse them with a regex or logfmt-style parser rather than a JSON parser.

## Distributed Tracing

OpenTelemetry tracing is available, off by default. Set `OTEL_EXPORTER_OTLP_ENDPOINT` (or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`) to enable it — traces export over OTLP/HTTP. Standard `OTEL_*` variables are honored (`OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_TRACES_SAMPLER`, ...); there are no `MUTO_OTEL_*` aliases.

Spans cover reconcile loops (`TenantReconciler`, `AgentJobReconciler`, `AgentFleetReconciler`), the scheduler, both platform adapters (K8s and CF), and MCP tool invocations, and the A2A/CF HTTP clients propagate W3C `traceparent` headers.

An MCP-scheduled job produces a single trace end to end: the tool call, `Scheduler.Schedule`, and `PlatformAdapter.SpawnAgent` all share one trace ID, verified in `test/integration/k8s/tracing_test.go`. The one span that's deliberately excluded is `PlatformAdapter.WatchAgent` - its goroutine is started against a detached `context.Background()` (see the comment on `DefaultScheduler.Schedule`) so it outlives the request that spawned it, and therefore starts its own trace rather than joining the scheduling one.

A K8s `AgentJob` CR created directly (not via the MCP scheduler - see [Known Limitations](#known-limitations)) is a separate story: the reconcile loop is triggered by a watch event with no causal link to whatever created or updated the resource, so `AgentJobReconciler`'s span tree is never part of the same trace as that caller. This is standard for watch-driven controllers, not a gap specific to Muto.

## Dashboards

Muto doesn't ship dashboards. The metrics above are standard controller-runtime metrics, so generic controller-runtime dashboards work, such as those generated by Kubebuilder's Grafana plugin. You can also build panels from the [PromQL queries](#promql-queries) above.

For resource usage, use the container metrics of the operator pod (`container_cpu_usage_seconds_total`, `container_memory_working_set_bytes`) and `kubectl top pods -n muto-system`.

## Alerting

Example Prometheus rules using the available metrics. Adjust thresholds, and the `job` label in `MutoOperatorDown`, to your setup.

```yaml
groups:
  - name: muto-operator
    rules:
      - alert: MutoOperatorDown
        expr: absent(up{job="muto-operator"} == 1)
        for: 5m
        annotations:
          summary: "Muto operator metrics endpoint is not being scraped"

      - alert: MutoReconcileErrorRateHigh
        expr: |
          sum by (controller) (rate(controller_runtime_reconcile_errors_total[5m]))
          /
          sum by (controller) (rate(controller_runtime_reconcile_total[5m]))
          > 0.1
        for: 10m
        annotations:
          summary: "More than 10% of {{ $labels.controller }} reconciliations fail"

      - alert: MutoWorkqueueBacklog
        expr: sum by (controller) (workqueue_depth) > 100
        for: 10m
        annotations:
          summary: "{{ $labels.controller }} work queue has {{ $value }} pending objects"

      - alert: MutoReconcilePanics
        expr: increase(controller_runtime_reconcile_panics_total[15m]) > 0
        annotations:
          summary: "The {{ $labels.controller }} reconciler panicked"

      # Requires kube-state-metrics
      - alert: MutoOperatorRestarting
        expr: increase(kube_pod_container_status_restarts_total{namespace="muto-system", container="muto-operator"}[30m]) > 2
        annotations:
          summary: "Muto operator restarted more than twice in 30 minutes"
```

## Known Limitations

- The metrics (`:8080`) and health probe (`:8081`) addresses are fixed. The Helm values `metrics.port` and `healthProbe.port` only change the container port declarations, so changing them breaks scraping or the probes.
- `metrics.enabled: false` turns off the metrics server entirely (via `MUTO_METRICS_BIND_ADDRESS=0`), not just its Service/ServiceMonitor/annotations.
- The metrics endpoint is plain HTTP without authentication. Restrict access with a NetworkPolicy.
- `/readyz` doesn't reflect API server connectivity or cache sync.
- A K8s `AgentJob` CR created directly, outside the MCP scheduler, gets its own disconnected span tree from `AgentJobReconciler` - reconcile is watch-triggered, with no causal link back to whatever created the resource. The MCP-scheduled path (tool call → scheduler → platform adapter) *is* a single trace; see [Distributed Tracing](#distributed-tracing).
- `muto_agents_running` and `muto_job_queue_depth` can read stale or negative values across an operator restart or if a running AgentJob is deleted directly.
- `muto-mcp` has no observability endpoints.

---

## Best Practices

1. **Alert on sustained error ratios.** Don't page on individual reconcile errors.
2. **Watch queue depth and queue latency.** A growing `workqueue_depth` is the earliest sign of a backlog.
3. **Keep the chart's port defaults.** The binary doesn't read `metrics.port` or `healthProbe.port`.
4. **Parse logs as key/value text.** A JSON parser drops or mangles the operator's lines.
5. **Restrict access to `:8080`.** The metrics endpoint is unauthenticated.

---

**See Also:**
- [Configuration: Environment Variables](../configuration/environment-variables.md)
- [Troubleshooting](./troubleshooting.md) - Common issues and diagnosis
- [Performance Tuning](./performance-tuning.md) - Optimizing Muto for your workload
