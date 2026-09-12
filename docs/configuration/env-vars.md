# Environment Variables Reference

Complete reference of all environment variables used by Muto.

## Overview

Muto is configured through environment variables that control:
- Platform selection (Kubernetes or CloudFoundry)
- Message bus connections (NATS or Kafka)
- Reconciliation behavior (worker counts, timeouts, retries)
- Resource management (quotas, limits)
- Observability (logging, metrics, tracing)
- Security (TLS, authentication, isolation)

## Core Settings

### MUTO_PLATFORM

**Type:** `string`  
**Default:** `k8s`  
**Valid Values:** `k8s`, `cf`

Selects the underlying platform for agent execution.

```bash
# Kubernetes
export MUTO_PLATFORM=k8s

# CloudFoundry
export MUTO_PLATFORM=cf
```

### MUTO_LOG_LEVEL

**Type:** `string`  
**Default:** `info`  
**Valid Values:** `debug`, `info`, `warn`, `error`

Controls verbosity of operator logs.

```bash
# Development (verbose)
export MUTO_LOG_LEVEL=debug

# Production (important events only)
export MUTO_LOG_LEVEL=info
```

### MUTO_LOG_FORMAT

**Type:** `string`  
**Default:** `json`  
**Valid Values:** `json`, `text`

Log output format. JSON is structured and better for automated parsing.

```bash
# Structured logging (recommended for production)
export MUTO_LOG_FORMAT=json

# Human-readable (recommended for development)
export MUTO_LOG_FORMAT=text
```

## Message Bus Configuration

### MUTO_MESSAGE_BUS_TYPE

**Type:** `string`  
**Default:** `nats`  
**Valid Values:** `nats`, `kafka`

Message bus implementation for inter-agent communication.

```bash
# NATS (low-latency, simple setup)
export MUTO_MESSAGE_BUS_TYPE=nats
export MUTO_NATS_URL=nats://nats-server:4222

# Kafka (high-throughput, complex setup)
export MUTO_MESSAGE_BUS_TYPE=kafka
export MUTO_KAFKA_BROKERS=kafka1:9092,kafka2:9092,kafka3:9092
```

### MUTO_NATS_URL

**Type:** `string`  
**Default:** `nats://localhost:4222`

Connection URL for NATS message bus.

```bash
export MUTO_NATS_URL=nats://user:password@nats-server:4222
```

### MUTO_KAFKA_BROKERS

**Type:** `string` (comma-separated)  
**Default:** `localhost:9092`

Kafka broker addresses for message bus.

```bash
export MUTO_KAFKA_BROKERS=kafka1:9092,kafka2:9092,kafka3:9092
```

## Reconciliation Settings

### MUTO_RECONCILER_WORKER_COUNT

**Type:** `integer`  
**Default:** `5`

Number of concurrent reconciliation workers.

```bash
# Development (low resource)
export MUTO_RECONCILER_WORKER_COUNT=2

# Production (high throughput)
export MUTO_RECONCILER_WORKER_COUNT=20
```

### MUTO_RECONCILER_SYNC_PERIOD

**Type:** `duration`  
**Default:** `5m`

How often to perform a full resync of all resources.

```bash
# Fast resync (more overhead)
export MUTO_RECONCILER_SYNC_PERIOD=1m

# Relaxed resync (lower overhead)
export MUTO_RECONCILER_SYNC_PERIOD=15m
```

## Tenant Configuration

### MUTO_MAX_TENANTS

**Type:** `integer`  
**Default:** `1000`

Maximum number of tenants allowed in the cluster.

```bash
export MUTO_MAX_TENANTS=500
```

### MUTO_MAX_JOBS_PER_TENANT

**Type:** `integer`  
**Default:** `1000`

Maximum concurrent jobs per tenant.

```bash
export MUTO_MAX_JOBS_PER_TENANT=500
```

## Resource Quotas

### MUTO_DEFAULT_CPU_REQUEST

**Type:** `string`  
**Default:** `500m`

Default CPU request for agents if not specified.

```bash
export MUTO_DEFAULT_CPU_REQUEST=1000m
```

### MUTO_DEFAULT_MEMORY_REQUEST

**Type:** `string`  
**Default:** `512Mi`

Default memory request for agents if not specified.

```bash
export MUTO_DEFAULT_MEMORY_REQUEST=1Gi
```

### MUTO_DEFAULT_CPU_LIMIT

**Type:** `string`  
**Default:** `2`

Default CPU limit for agents if not specified.

```bash
export MUTO_DEFAULT_CPU_LIMIT=4
```

### MUTO_DEFAULT_MEMORY_LIMIT

**Type:** `string`  
**Default:** `2Gi`

Default memory limit for agents if not specified.

```bash
export MUTO_DEFAULT_MEMORY_LIMIT=4Gi
```

## Kubernetes-Specific

### MUTO_KUBECONFIG

**Type:** `string`  
**Default:** `~/.kube/config`

Path to Kubernetes config file.

```bash
export MUTO_KUBECONFIG=/etc/kubernetes/admin.conf
```

### MUTO_KUBERNETES_NAMESPACE

**Type:** `string`  
**Default:** `muto-system`

Namespace for Muto system components.

```bash
export MUTO_KUBERNETES_NAMESPACE=muto
```

## CloudFoundry-Specific

### MUTO_CF_API_URL

**Type:** `string`  
**Default:** (required)

CloudFoundry API endpoint.

```bash
export MUTO_CF_API_URL=https://api.cf.example.com
```

### MUTO_CF_USERNAME

**Type:** `string`  
**Default:** (required)

CloudFoundry username for authentication.

```bash
export MUTO_CF_USERNAME=muto-service
```

### MUTO_CF_PASSWORD

**Type:** `string`  
**Default:** (required)

CloudFoundry password for authentication.

```bash
export MUTO_CF_PASSWORD=secure-password
```

### MUTO_CF_ORG

**Type:** `string`  
**Default:** (required)

CloudFoundry organization name.

```bash
export MUTO_CF_ORG=my-org
```

## Observability

### MUTO_METRICS_ENABLED

**Type:** `boolean`  
**Default:** `true`

Enable Prometheus metrics collection.

```bash
export MUTO_METRICS_ENABLED=true
```

### MUTO_METRICS_PORT

**Type:** `integer`  
**Default:** `8080`

Port for Prometheus metrics endpoint.

```bash
export MUTO_METRICS_PORT=9090
```

### MUTO_TRACING_ENABLED

**Type:** `boolean`  
**Default:** `false`

Enable distributed tracing (OpenTelemetry).

```bash
export MUTO_TRACING_ENABLED=true
```

### MUTO_JAEGER_ENDPOINT

**Type:** `string`  
**Default:** `http://localhost:6831`

Jaeger collector endpoint for tracing.

```bash
export MUTO_JAEGER_ENDPOINT=http://jaeger:6831
```

## Security

### MUTO_TLS_ENABLED

**Type:** `boolean`  
**Default:** `true`

Enable TLS for inter-component communication.

```bash
export MUTO_TLS_ENABLED=true
```

### MUTO_TLS_CERT_PATH

**Type:** `string`  
**Default:** `/etc/muto/tls/tls.crt`

Path to TLS certificate file.

```bash
export MUTO_TLS_CERT_PATH=/certs/server.crt
```

### MUTO_TLS_KEY_PATH

**Type:** `string`  
**Default:** `/etc/muto/tls/tls.key`

Path to TLS private key file.

```bash
export MUTO_TLS_KEY_PATH=/certs/server.key
```

### MUTO_RBAC_ENABLED

**Type:** `boolean`  
**Default:** `true`

Enable RBAC enforcement (Kubernetes only).

```bash
export MUTO_RBAC_ENABLED=true
```

## Webhooks

### MUTO_WEBHOOKS_ENABLED

**Type:** `boolean`  
**Default:** `false`

Enable job lifecycle webhooks.

```bash
export MUTO_WEBHOOKS_ENABLED=true
```

### MUTO_WEBHOOK_URLS

**Type:** `string` (comma-separated)  
**Default:** (empty)

Webhook endpoints for job events.

```bash
export MUTO_WEBHOOK_URLS=https://monitor:8080/webhooks,https://alerts:8080/webhooks
```

### MUTO_WEBHOOK_EVENTS

**Type:** `string` (comma-separated)  
**Default:** `job.created,job.completed,job.failed`

Which events to send to webhooks.

```bash
export MUTO_WEBHOOK_EVENTS=job.created,job.started,job.completed,job.failed
```

## Configuration Examples

### Development Setup

```bash
export MUTO_PLATFORM=k8s
export MUTO_LOG_LEVEL=debug
export MUTO_LOG_FORMAT=text
export MUTO_MESSAGE_BUS_TYPE=nats
export MUTO_NATS_URL=nats://localhost:4222
export MUTO_RECONCILER_WORKER_COUNT=2
export MUTO_METRICS_ENABLED=true
export MUTO_TLS_ENABLED=false
```

### Production Setup

```bash
export MUTO_PLATFORM=k8s
export MUTO_LOG_LEVEL=info
export MUTO_LOG_FORMAT=json
export MUTO_MESSAGE_BUS_TYPE=kafka
export MUTO_KAFKA_BROKERS=kafka1:9092,kafka2:9092,kafka3:9092
export MUTO_RECONCILER_WORKER_COUNT=20
export MUTO_MAX_JOBS_PER_TENANT=1000
export MUTO_METRICS_ENABLED=true
export MUTO_TRACING_ENABLED=true
export MUTO_JAEGER_ENDPOINT=http://jaeger:6831
export MUTO_TLS_ENABLED=true
export MUTO_WEBHOOKS_ENABLED=true
export MUTO_WEBHOOK_URLS=https://monitor:443/webhooks
```

## Related Documentation

- [:octicons-book-24: **Message Bus Setup**](./message-bus-setup.md) — Detailed message bus configuration
- [:octicons-book-24: **Reconciler Configuration**](./reconciler-config.md) — Tuning reconciler behavior
- [:octicons-book-24: **Multi-Tenant Setup**](./multi-tenant-setup.md) — Multi-tenancy configuration
