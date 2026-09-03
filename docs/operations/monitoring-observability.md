# Monitoring and Observability

Understand how to monitor Muto in production, including logs, metrics, tracing, dashboards, and alerting strategies.

## Overview

Muto is designed to be observable by default. The operator exports structured logs, Prometheus metrics, and OpenTelemetry traces that enable comprehensive monitoring of agent job execution and system health.

**Three pillars of observability:**
- **Logs**: Structured JSON events for debugging and audit trails
- **Metrics**: Prometheus metrics for monitoring job health, throughput, and latency
- **Traces**: Distributed tracing via OpenTelemetry for request flow visualization

## Structured Logging

### Log Configuration

Muto outputs JSON-structured logs by default, configurable via environment variables:

**MUTO_LOG_LEVEL** (`debug`, `info`, `warn`, `error`)
```bash
# Development (verbose)
export MUTO_LOG_LEVEL=debug

# Production (important events only)
export MUTO_LOG_LEVEL=info
```

**MUTO_LOG_FORMAT** (`json`, `text`)
```bash
# Structured JSON (production)
export MUTO_LOG_FORMAT=json

# Human-readable (development)
export MUTO_LOG_FORMAT=text
```

### Standard Log Events

Muto emits structured events for major lifecycle phases:

#### Operator Startup
```json
{
  "timestamp": "2026-09-03T10:30:45.123Z",
  "level": "info",
  "component": "operator",
  "event": "started",
  "version": "0.1.0",
  "platform": "kubernetes",
  "reconcilers": ["TenantReconciler", "AgentJobReconciler"]
}
```

#### Job Scheduling
```json
{
  "timestamp": "2026-09-03T10:35:20.456Z",
  "level": "info",
  "component": "scheduler",
  "event": "job_scheduled",
  "jobID": "job-abc123",
  "tenantID": "tenant-a",
  "phase": "Scheduled",
  "agents": ["agent-a", "agent-b"]
}
```

#### Job Completion
```json
{
  "timestamp": "2026-09-03T10:40:15.789Z",
  "level": "info",
  "component": "reconciler",
  "event": "job_completed",
  "jobID": "job-abc123",
  "tenantID": "tenant-a",
  "status": "Completed",
  "duration_seconds": 295,
  "agents_completed": 2
}
```

#### Reconciliation Errors
```json
{
  "timestamp": "2026-09-03T10:45:30.111Z",
  "level": "error",
  "component": "reconciler",
  "event": "reconciliation_failed",
  "jobID": "job-xyz789",
  "error": "failed to create pod: insufficient resources",
  "retry_attempt": 3,
  "next_retry_seconds": 32
}
```

### Viewing Logs

#### Kubernetes

```bash
# Operator logs
kubectl logs -n muto-system deployment/muto-operator

# Follow logs in real-time
kubectl logs -n muto-system deployment/muto-operator -f

# View logs from specific time
kubectl logs -n muto-system deployment/muto-operator --since=1h

# View logs for specific pod
kubectl logs -n muto-system pod/muto-operator-abc123

# Get logs from all replicas
kubectl logs -n muto-system deployment/muto-operator --all-containers
```

#### CloudFoundry

```bash
# View application logs
cf logs muto-operator

# Stream logs
cf logs muto-operator --follow

# View logs from specific instance
cf logs muto-operator --instance 0
```

### Log Aggregation

For production, aggregate logs to a centralized system:

**Elasticsearch + Logstash + Kibana (ELK):**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: logstash-config
  namespace: muto-system
data:
  logstash.conf: |
    input {
      kubernetes {
        namespace => "muto-system"
        pod_name_regexp => "^muto-operator"
      }
    }
    filter {
      json {
        source => "message"
      }
    }
    output {
      elasticsearch {
        hosts => ["elasticsearch:9200"]
        index => "muto-logs-%{+YYYY.MM.dd}"
      }
    }
```

**Loki + Promtail (Grafana):**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: promtail-config
  namespace: muto-system
data:
  config.yaml: |
    clients:
      - url: http://loki:3100/loki/api/v1/push
    scrape_configs:
      - job_name: kubernetes-pods
        kubernetes_sd_configs:
          - role: pod
        relabel_configs:
          - source_labels: [__meta_kubernetes_namespace]
            target_label: namespace
          - source_labels: [__meta_kubernetes_pod_name]
            target_label: pod
```

## Prometheus Metrics

### Metric Types

Muto exports the following metric families:

#### Counter Metrics

**muto_jobs_total** — Total number of jobs processed
```
muto_jobs_total{tenant="tenant-a",status="completed"} 1024
muto_jobs_total{tenant="tenant-a",status="failed"} 12
```

**muto_reconciliations_total** — Total reconciliation attempts
```
muto_reconciliations_total{reconciler="AgentJobReconciler",result="success"} 5000
muto_reconciliations_total{reconciler="AgentJobReconciler",result="error"} 45
```

#### Gauge Metrics

**muto_agents_running** — Current number of running agents
```
muto_agents_running{tenant="tenant-a"} 42
muto_agents_running{tenant="tenant-b"} 28
```

**muto_job_queue_depth** — Jobs waiting to be scheduled
```
muto_job_queue_depth{tenant="tenant-a"} 12
```

#### Histogram Metrics

**muto_job_duration_seconds** — Job execution time distribution
```
muto_job_duration_seconds_bucket{le="1",tenant="tenant-a"} 100
muto_job_duration_seconds_bucket{le="10",tenant="tenant-a"} 450
muto_job_duration_seconds_bucket{le="60",tenant="tenant-a"} 980
muto_job_duration_seconds_sum{tenant="tenant-a"} 25000
muto_job_duration_seconds_count{tenant="tenant-a"} 1024
```

**muto_reconciliation_duration_seconds** — Reconciliation loop latency
```
muto_reconciliation_duration_seconds_bucket{le="0.1",reconciler="AgentJobReconciler"} 1000
muto_reconciliation_duration_seconds_bucket{le="1",reconciler="AgentJobReconciler"} 4950
```

### Scraping Metrics

#### Kubernetes (Prometheus Operator)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: muto-operator
  namespace: muto-system
spec:
  selector:
    matchLabels:
      app: muto-operator
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```

#### Standalone Prometheus

```yaml
scrape_configs:
  - job_name: 'muto-operator'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 30s
```

### PromQL Queries

**Job success rate (past 24h):**
```promql
sum(rate(muto_jobs_total{status="completed"}[24h])) 
/ 
sum(rate(muto_jobs_total[24h]))
```

**Average job duration:**
```promql
histogram_quantile(0.5, rate(muto_job_duration_seconds_bucket[5m]))
```

**P99 job duration:**
```promql
histogram_quantile(0.99, rate(muto_job_duration_seconds_bucket[5m]))
```

**Active agents per tenant:**
```promql
sum(muto_agents_running) by (tenant)
```

**Reconciliation error rate:**
```promql
sum(rate(muto_reconciliations_total{result="error"}[5m]))
/
sum(rate(muto_reconciliations_total[5m]))
```

## Distributed Tracing

### OpenTelemetry Setup

Muto supports OpenTelemetry for distributed tracing. Configure via environment variables:

**MUTO_OTEL_ENABLED** (`true`, `false`)
```bash
export MUTO_OTEL_ENABLED=true
```

**MUTO_OTEL_EXPORTER_OTLP_ENDPOINT**
```bash
export MUTO_OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
```

**MUTO_OTEL_SERVICE_NAME**
```bash
export MUTO_OTEL_SERVICE_NAME=muto-operator
```

### Trace Structure

Each job execution generates a trace spanning the entire lifecycle:

```
Trace: job-abc123
├─ Span: scheduler.schedule_job
│  ├─ Span: tenant.validate
│  ├─ Span: resource_allocation.compute
│  └─ Span: platform_adapter.create_job
├─ Span: reconciler.reconcile
│  ├─ Span: platform_adapter.get_status
│  ├─ Span: status_update.persist
│  └─ Span: event.watch
└─ Span: completion.record
```

### Jaeger Integration

Deploy Jaeger for trace visualization:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: jaeger
  namespace: muto-system
spec:
  ports:
    - name: otlp-grpc
      port: 4317
      targetPort: 4317
    - name: web
      port: 16686
      targetPort: 16686
  selector:
    app: jaeger
```

Access Jaeger UI: `http://localhost:16686`

## Dashboards

### Grafana Dashboard

**Job Overview Dashboard:**

```json
{
  "title": "Muto Job Overview",
  "panels": [
    {
      "title": "Jobs Completed (24h)",
      "targets": [
        {
          "expr": "sum(rate(muto_jobs_total{status=\"completed\"}[24h]))"
        }
      ]
    },
    {
      "title": "Job Success Rate",
      "targets": [
        {
          "expr": "sum(rate(muto_jobs_total{status=\"completed\"}[24h])) / sum(rate(muto_jobs_total[24h]))"
        }
      ]
    },
    {
      "title": "P50 Job Duration",
      "targets": [
        {
          "expr": "histogram_quantile(0.5, rate(muto_job_duration_seconds_bucket[5m]))"
        }
      ]
    },
    {
      "title": "P99 Job Duration",
      "targets": [
        {
          "expr": "histogram_quantile(0.99, rate(muto_job_duration_seconds_bucket[5m]))"
        }
      ]
    },
    {
      "title": "Active Agents by Tenant",
      "targets": [
        {
          "expr": "sum(muto_agents_running) by (tenant)"
        }
      ]
    },
    {
      "title": "Reconciliation Error Rate",
      "targets": [
        {
          "expr": "sum(rate(muto_reconciliations_total{result=\"error\"}[5m])) / sum(rate(muto_reconciliations_total[5m]))"
        }
      ]
    }
  ]
}
```

### Kubernetes Dashboard

Kubernetes provides native dashboards. Access via:

```bash
kubectl top nodes
kubectl top pods -n muto-system
```

## Alerting

### Alert Rules (Prometheus)

**High job failure rate:**
```yaml
alert: MutoJobFailureRateHigh
expr: |
  sum(rate(muto_jobs_total{status="failed"}[5m]))
  /
  sum(rate(muto_jobs_total[5m]))
  > 0.05
for: 5m
annotations:
  summary: "Muto job failure rate > 5%"
  description: "{{ $value | humanizePercentage }} of jobs are failing"
```

**High reconciliation error rate:**
```yaml
alert: MutoReconciliationErrorRateHigh
expr: |
  sum(rate(muto_reconciliations_total{result="error"}[5m]))
  /
  sum(rate(muto_reconciliations_total[5m]))
  > 0.1
for: 2m
annotations:
  summary: "Muto reconciliation error rate > 10%"
```

**Job queue backlog:**
```yaml
alert: MutoJobQueueBacklog
expr: muto_job_queue_depth > 100
for: 10m
annotations:
  summary: "Muto job queue has {{ $value }} pending jobs"
```

**Operator down:**
```yaml
alert: MutoOperatorDown
expr: up{job="muto-operator"} == 0
for: 1m
annotations:
  summary: "Muto operator is down"
```

### Alertmanager Configuration

```yaml
global:
  resolve_timeout: 5m

route:
  receiver: 'default'
  group_by: ['alertname', 'cluster']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  routes:
    - match:
        alertname: MutoOperatorDown
      receiver: 'critical'
      group_wait: 1s
      repeat_interval: 1h

receivers:
  - name: 'default'
    slack_configs:
      - api_url: 'https://hooks.slack.com/...'
        channel: '#muto-alerts'
  - name: 'critical'
    slack_configs:
      - api_url: 'https://hooks.slack.com/...'
        channel: '#muto-critical'
    pagerduty_configs:
      - service_key: 'abc123...'
```

## Health Checks

### Kubernetes Liveness and Readiness Probes

Configure health checks in the operator deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muto-operator
  namespace: muto-system
spec:
  template:
    spec:
      containers:
      - name: operator
        image: muto-operator:latest
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
```

### Health Check Endpoints

Muto exposes health check endpoints:

**Liveness:** `/healthz` (is the operator running?)
```bash
curl http://localhost:8080/healthz
# Output: ok
```

**Readiness:** `/readyz` (is the operator ready to handle jobs?)
```bash
curl http://localhost:8080/readyz
# Output: ok
```

**Metrics:** `/metrics` (Prometheus metrics)
```bash
curl http://localhost:8080/metrics
# Output: Prometheus metrics in text format
```

---

## Best Practices

1. **Use structured JSON logs** — Query logs programmatically in aggregation systems
2. **Alert on job failure rates** — Not individual failures, but sustained error trends
3. **Set up traces early** — Distributed tracing is invaluable for debugging production issues
4. **Export metrics continuously** — Use Prometheus scraping, not polling from the operator
5. **Create per-tenant dashboards** — Multi-tenant systems need per-tenant visibility
6. **Monitor queue depth** — Watch for scheduler bottlenecks early
7. **Set realistic SLOs** — Define SLOs based on your actual requirements, not defaults
8. **Test alerting** — Verify alert routing and notification delivery before production

---

**See Also:**
- [Configuration: Environment Variables](../configuration/environment-variables.md)
- [Troubleshooting](./troubleshooting.md) — Common issues and diagnosis
- [Performance Tuning](./performance-tuning.md) — Optimizing Muto for your workload
