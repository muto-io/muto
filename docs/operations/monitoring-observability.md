# Monitoring and Observability

How to monitor the Muto operator as it works today: health probes, Prometheus metrics, and logs.

!!! note "Current state"
    Muto currently ships only the observability that controller-runtime provides out of the box. Custom `muto_*` metrics, JSON logs with configurable levels, and OpenTelemetry tracing are **not implemented yet**; they are tracked in [#79]. This page describes the operator's actual behavior.

## Overview

| Signal | Status | Where |
|---|---|---|
| Health probes | ✅ Available | `:8081/healthz`, `:8081/readyz` |
| Prometheus metrics | ⚠️ controller-runtime built-in metrics only | `:8080/metrics` |
| Logs | ⚠️ Plain-text key/value lines, fixed level and format | stderr (container logs) |
| Custom `muto_*` metrics | ❌ Planned ([#79]) | — |
| Distributed tracing (OpenTelemetry) | ❌ Planned ([#79]) | — |

This applies to `muto-operator`. The MCP server (`muto-mcp`) communicates over stdio and has no health, metrics or tracing endpoints.

Both ports are hard-coded in `cmd/muto-operator/main.go`. No flag or environment variable changes them.

The commands on this page assume the chart was installed with `helm install muto ...`, which creates the Deployment `muto` in `muto-system`. Adjust the name if you used a different release name.

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
kubectl port-forward -n muto-system deployment/muto 8081:8081 &
curl http://localhost:8081/healthz   # ok
curl http://localhost:8081/readyz    # ok
```

!!! warning "Operator builds before the fix for #80"
    Earlier builds didn't register any health checks, so `/healthz` and `/readyz` returned `404`. The operator pod never became Ready, and the liveness probe restarted it about every minute. See [#80](https://github.com/muto-io/muto/issues/80).

### CloudFoundry

`deploy/cf/manifest.yml` runs the operator with `no-route: true` and `health-check-type: process`, so CF only checks that the process is alive. Don't switch to an `http` health check. CF sends HTTP health checks to the app port (`8080` by default), which is the operator's metrics server, and `/healthz` returns `404` there.

## Prometheus Metrics

The operator serves Prometheus metrics at `:8080/metrics` over plain HTTP without authentication. These are the metrics that controller-runtime and client-go register by default. Muto doesn't register any metrics of its own yet.

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

Labeled histograms and gauges appear only after their first observation. For example, `controller_runtime_reconcile_time_seconds` and `workqueue_depth` show up once the operator has reconciled an object.

Job-level metrics aren't available yet. These include job counts by result, job duration, running agents, per-tenant labels, and message bus metrics; they're planned in [#79]. Until then, the `agentjob` reconcile metrics are the closest signal.

### Scraping

When `metrics.enabled` is `true` (the default), the Helm chart adds these annotations to the operator pod:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "8080"
prometheus.io/path: "/metrics"
```

Prometheus setups that honor these annotations pick up the operator automatically. A typical example is `kubernetes_sd_configs` with `role: pod` plus annotation relabeling. Setting `metrics.enabled: false` only removes the annotations; the operator still serves `:8080/metrics`.

The chart doesn't create a Service, so a `ServiceMonitor` has nothing to select. With the Prometheus Operator, use a `PodMonitor` on the operator pod's `metrics` port:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: muto-operator
  namespace: muto-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: muto
  podMetricsEndpoints:
    - port: metrics
      path: /metrics
      interval: 30s
```

Depending on your Prometheus Operator configuration, the `PodMonitor` may need a label that your `Prometheus` resource selects, for example `release: kube-prometheus-stack`.

Quick check:

```bash
kubectl port-forward -n muto-system deployment/muto 8080:8080 &
curl -s http://localhost:8080/metrics | grep '^controller_runtime_reconcile_total'
```

Excerpt after creating a Tenant and an AgentJob:

```
controller_runtime_reconcile_total{controller="agentjob",result="requeue_after"} 6
controller_runtime_reconcile_total{controller="agentjob",result="success"} 1
controller_runtime_reconcile_total{controller="tenant",result="success"} 2
```

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

Both binaries log through go-logr with the [`stdr`](https://github.com/go-logr/stdr) backend, which writes to **stderr** using Go's standard `log` package. Each line has a timestamp, the logger name, and key/value pairs:

```
2026/09/12 09:43:29 muto-operator: "level"=0 "msg"="starting muto-operator" "platform"="k8s"
2026/09/12 09:46:39 "level"=0 "msg"="adding tenant finalizer" "controller"="tenant" "controllerGroup"="muto.io" "controllerKind"="Tenant" "Tenant"={"name"="demo-tenant"} "namespace"="" "name"="demo-tenant" "reconcileID"="c0c3b48e-78e4-42f5-86ae-334acbd8aa8f" "tenant"="demo-tenant" "finalizer"="muto.io/tenant-cleanup"
```

- The format is fixed. Logs are **not JSON**, and there are no `MUTO_LOG_LEVEL` or `MUTO_LOG_FORMAT` settings.
- Verbosity is fixed at `0`. Messages logged with `V(1)` or higher are discarded.
- Errors are logged with an `"error"` key.
- Timestamps have second precision and no time zone. The distroless image has no time zone data, so they are UTC.

### Viewing Logs

#### Kubernetes

```bash
# Operator logs
kubectl logs -n muto-system deployment/muto

# Follow logs in real time
kubectl logs -n muto-system deployment/muto -f

# Logs from the last hour
kubectl logs -n muto-system deployment/muto --since=1h

# Logs from before the last container restart
kubectl logs -n muto-system deployment/muto --previous
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
kubectl logs -n muto-system deployment/muto | grep '"error"='

# One reconciler
kubectl logs -n muto-system deployment/muto | grep '"controller"="agentjob"'

# One object
kubectl logs -n muto-system deployment/muto | grep '"name"="my-job"'

# One reconciliation
kubectl logs -n muto-system deployment/muto | grep '"reconcileID"="<id>"'
```

### Log Aggregation

Any collector that ships container logs works, such as Fluent Bit, Grafana Alloy/Promtail, Vector, or Filebeat. The lines aren't JSON, so parse them with a regex or logfmt-style parser rather than a JSON parser.

## Distributed Tracing

Tracing isn't implemented. The operator doesn't initialize an OpenTelemetry SDK, and `OTEL_*` or `MUTO_OTEL_*` environment variables have no effect. The `go.opentelemetry.io` modules in `go.mod` are indirect dependencies of the test tooling. OpenTelemetry tracing with OTLP export is planned in [#79].

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
- `metrics.enabled: false` doesn't turn off the metrics server.
- The metrics endpoint is plain HTTP without authentication. Restrict access with a NetworkPolicy.
- `/readyz` doesn't reflect API server connectivity or cache sync.
- There are no Muto-specific metrics, no JSON logs or log levels, and no tracing yet ([#79]).
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
- [Troubleshooting](./troubleshooting.md) — Common issues and diagnosis
- [Performance Tuning](./performance-tuning.md) — Optimizing Muto for your workload

[#79]: https://github.com/muto-io/muto/issues/79
