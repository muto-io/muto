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
**Default:** `kubernetes`  
**Valid Values:** `kubernetes`, `cloudfoundry`

Selects the underlying platform for agent execution.

```bash
# Kubernetes
export MUTO_PLATFORM=kubernetes

# CloudFoundry
export MUTO_PLATFORM=cloudfoundry
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

Output format for logs.

```bash
# Structured JSON (recommended for production)
export MUTO_LOG_FORMAT=json

# Human-readable text (development)
export MUTO_LOG_FORMAT=text
```

### MUTO_OPERATOR_BIND_ADDRESS

**Type:** `string`  
**Default:** `0.0.0.0:8080`

HTTP server bind address for health checks and metrics.

```bash
export MUTO_OPERATOR_BIND_ADDRESS=0.0.0.0:8080
```

### MUTO_WEBHOOK_PORT

**Type:** `integer`  
**Default:** `8443`

HTTPS port for admission webhooks (Kubernetes only).

```bash
export MUTO_WEBHOOK_PORT=8443
```

---

## Message Bus Configuration

### MUTO_MESSAGE_BUS_TYPE

**Type:** `string`  
**Default:** `nats`  
**Valid Values:** `nats`, `kafka`

Message bus implementation.

```bash
# NATS (simple, recommended for development)
export MUTO_MESSAGE_BUS_TYPE=nats

# Kafka (enterprise, recommended for production)
export MUTO_MESSAGE_BUS_TYPE=kafka
```

### MUTO_NATS_URL

**Type:** `string`  
**Default:** `nats://localhost:4222`

NATS server URL. Required when `MUTO_MESSAGE_BUS_TYPE=nats`.

```bash
# Single node
export MUTO_NATS_URL=nats://nats.muto-system:4222

# Cluster with multiple nodes
export MUTO_NATS_URL=nats://nats-1:4222,nats://nats-2:4222,nats://nats-3:4222

# With authentication
export MUTO_NATS_URL=nats://user:password@nats.muto-system:4222

# With TLS
export MUTO_NATS_URL=nats+tls://nats.muto-system:4222
```

### MUTO_NATS_CREDENTIALS_FILE

**Type:** `string`  
**Default:** (none)

Path to NATS credentials file for JWT authentication.

```bash
export MUTO_NATS_CREDENTIALS_FILE=/etc/muto/nats-credentials.txt
```

### MUTO_NATS_TLS_CERT

**Type:** `string`  
**Default:** (none)

Path to TLS certificate for NATS connection.

```bash
export MUTO_NATS_TLS_CERT=/etc/muto/nats-client.crt
```

### MUTO_NATS_TLS_KEY

**Type:** `string`  
**Default:** (none)

Path to TLS private key for NATS connection.

```bash
export MUTO_NATS_TLS_KEY=/etc/muto/nats-client.key
```

### MUTO_NATS_TLS_CA

**Type:** `string`  
**Default:** (none)

Path to TLS CA certificate for NATS connection verification.

```bash
export MUTO_NATS_TLS_CA=/etc/muto/nats-ca.crt
```

### MUTO_NATS_POOL_SIZE

**Type:** `integer`  
**Default:** `10`

Number of connections in NATS connection pool.

```bash
# Development (small pool)
export MUTO_NATS_POOL_SIZE=5

# Production (larger pool for higher throughput)
export MUTO_NATS_POOL_SIZE=50
```

### MUTO_NATS_MAX_RECONNECTS

**Type:** `integer`  
**Default:** `5`

Number of reconnection attempts before failure.

```bash
export MUTO_NATS_MAX_RECONNECTS=10
```

### MUTO_NATS_RECONNECT_WAIT

**Type:** `duration`  
**Default:** `2s`

Time to wait between reconnection attempts.

```bash
export MUTO_NATS_RECONNECT_WAIT=5s
```

### MUTO_KAFKA_BROKERS

**Type:** `string`  
**Default:** (none)

Comma-separated list of Kafka broker addresses. Required when `MUTO_MESSAGE_BUS_TYPE=kafka`.

```bash
# Single broker
export MUTO_KAFKA_BROKERS=kafka.muto-system:9092

# Multiple brokers
export MUTO_KAFKA_BROKERS=kafka-1:9092,kafka-2:9092,kafka-3:9092
```

### MUTO_KAFKA_SASL_ENABLED

**Type:** `boolean`  
**Default:** `false`

Enable SASL authentication for Kafka.

```bash
export MUTO_KAFKA_SASL_ENABLED=true
```

### MUTO_KAFKA_SASL_MECHANISM

**Type:** `string`  
**Default:** `PLAIN`  
**Valid Values:** `PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512`

SASL mechanism for Kafka authentication.

```bash
export MUTO_KAFKA_SASL_MECHANISM=SCRAM-SHA-256
```

### MUTO_KAFKA_SASL_USER

**Type:** `string`  
**Default:** (none)

Username for Kafka SASL authentication.

```bash
export MUTO_KAFKA_SASL_USER=muto
```

### MUTO_KAFKA_SASL_PASSWORD

**Type:** `string`  
**Default:** (none)

Password for Kafka SASL authentication.

```bash
export MUTO_KAFKA_SASL_PASSWORD=secret-password
```

### MUTO_KAFKA_TLS_ENABLED

**Type:** `boolean`  
**Default:** `false`

Enable TLS for Kafka connection.

```bash
export MUTO_KAFKA_TLS_ENABLED=true
```

### MUTO_KAFKA_TLS_CERT

**Type:** `string`  
**Default:** (none)

Path to TLS certificate for Kafka connection.

```bash
export MUTO_KAFKA_TLS_CERT=/etc/muto/kafka-client.crt
```

### MUTO_KAFKA_TLS_KEY

**Type:** `string`  
**Default:** (none)

Path to TLS private key for Kafka connection.

```bash
export MUTO_KAFKA_TLS_KEY=/etc/muto/kafka-client.key
```

### MUTO_KAFKA_TLS_CA

**Type:** `string`  
**Default:** (none)

Path to TLS CA certificate for Kafka connection verification.

```bash
export MUTO_KAFKA_TLS_CA=/etc/muto/kafka-ca.crt
```

### MUTO_KAFKA_CONSUMER_GROUP

**Type:** `string`  
**Default:** `muto-operator`

Consumer group ID for Kafka.

```bash
export MUTO_KAFKA_CONSUMER_GROUP=muto-operator
```

### MUTO_KAFKA_REPLICATION_FACTOR

**Type:** `integer`  
**Default:** `3`

Replication factor for Kafka topics created by Muto.

```bash
# Single replica (development)
export MUTO_KAFKA_REPLICATION_FACTOR=1

# Production (high availability)
export MUTO_KAFKA_REPLICATION_FACTOR=3
```

### MUTO_KAFKA_MIN_IN_SYNC_REPLICAS

**Type:** `integer`  
**Default:** `2`

Minimum number of in-sync replicas for Kafka topics.

```bash
export MUTO_KAFKA_MIN_IN_SYNC_REPLICAS=2
```

---

## Reconciler Configuration

### MUTO_RECONCILER_WORKER_COUNT

**Type:** `integer`  
**Default:** `5`

Number of concurrent reconciliation workers.

```bash
# Development
export MUTO_RECONCILER_WORKER_COUNT=2

# Production
export MUTO_RECONCILER_WORKER_COUNT=20
```

### MUTO_RECONCILER_SYNC_PERIOD

**Type:** `duration`  
**Default:** `30s`

Period for resync of all resources (full reconciliation loop).

```bash
# Frequent resyncs for quick convergence
export MUTO_RECONCILER_SYNC_PERIOD=10s

# Infrequent resyncs for low overhead
export MUTO_RECONCILER_SYNC_PERIOD=5m
```

### MUTO_RECONCILER_EXPONENTIAL_BACKOFF_BASE

**Type:** `duration`  
**Default:** `1s`

Base duration for exponential backoff on reconciliation failure.

```bash
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_BASE=2s
```

### MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MAX

**Type:** `duration`  
**Default:** `5m`

Maximum duration for exponential backoff.

```bash
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MAX=10m
```

### MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MULTIPLIER

**Type:** `float`  
**Default:** `2.0`

Multiplier for exponential backoff.

```bash
export MUTO_RECONCILER_EXPONENTIAL_BACKOFF_MULTIPLIER=1.5
```

### MUTO_RECONCILER_MAX_RETRIES

**Type:** `integer`  
**Default:** `5`

Maximum number of retries for failed reconciliation.

```bash
export MUTO_RECONCILER_MAX_RETRIES=10
```

### MUTO_JOB_TIMEOUT_DEFAULT

**Type:** `duration`  
**Default:** `30m`

Default timeout for agent jobs.

```bash
export MUTO_JOB_TIMEOUT_DEFAULT=1h
```

### MUTO_JOB_TIMEOUT_MAX

**Type:** `duration`  
**Default:** `24h`

Maximum allowed timeout for agent jobs.

```bash
export MUTO_JOB_TIMEOUT_MAX=48h
```

---

## Kubernetes Platform Settings

### MUTO_KUBECONFIG

**Type:** `string`  
**Default:** `$HOME/.kube/config`

Path to kubeconfig file.

```bash
export MUTO_KUBECONFIG=/etc/muto/kubeconfig.yaml
```

### MUTO_K8S_NAMESPACE

**Type:** `string`  
**Default:** `muto-system`

Kubernetes namespace for Muto operator.

```bash
export MUTO_K8S_NAMESPACE=muto-system
```

### MUTO_K8S_LEADER_ELECTION_ENABLED

**Type:** `boolean`  
**Default:** `true`

Enable leader election for high availability.

```bash
export MUTO_K8S_LEADER_ELECTION_ENABLED=true
```

### MUTO_K8S_LEADER_ELECTION_NAMESPACE

**Type:** `string`  
**Default:** `muto-system`

Namespace for leader election lease.

```bash
export MUTO_K8S_LEADER_ELECTION_NAMESPACE=muto-system
```

### MUTO_K8S_LEADER_ELECTION_LEASE_DURATION

**Type:** `duration`  
**Default:** `15s`

Duration that the leader holds the lease before giving it up.

```bash
export MUTO_K8S_LEADER_ELECTION_LEASE_DURATION=20s
```

### MUTO_K8S_LEADER_ELECTION_RENEW_DEADLINE

**Type:** `duration`  
**Default:** `10s`

Time within which the leader must renew the lease.

```bash
export MUTO_K8S_LEADER_ELECTION_RENEW_DEADLINE=15s
```

### MUTO_K8S_LEADER_ELECTION_RETRY_PERIOD

**Type:** `duration`  
**Default:** `2s`

Time between retries of attempting to acquire the lease.

```bash
export MUTO_K8S_LEADER_ELECTION_RETRY_PERIOD=5s
```

---

## CloudFoundry Platform Settings

### MUTO_CF_API_URL

**Type:** `string`  
**Default:** (none)

CloudFoundry API URL.

```bash
export MUTO_CF_API_URL=https://api.cloudfoundry.example.com
```

### MUTO_CF_USERNAME

**Type:** `string`  
**Default:** (none)

CloudFoundry username for authentication.

```bash
export MUTO_CF_USERNAME=muto-user
```

### MUTO_CF_PASSWORD

**Type:** `string`  
**Default:** (none)

CloudFoundry password for authentication.

```bash
export MUTO_CF_PASSWORD=secret-password
```

### MUTO_CF_ORG

**Type:** `string`  
**Default:** (none)

Default CloudFoundry organization.

```bash
export MUTO_CF_ORG=muto-org
```

### MUTO_CF_SPACE

**Type:** `string`  
**Default:** (none)

Default CloudFoundry space.

```bash
export MUTO_CF_SPACE=muto-space
```

### MUTO_CF_SKIP_SSL_VALIDATION

**Type:** `boolean`  
**Default:** `false`

Skip SSL certificate validation for CloudFoundry API (NOT recommended for production).

```bash
export MUTO_CF_SKIP_SSL_VALIDATION=false
```

---

## TLS and Security

### MUTO_TLS_ENABLED

**Type:** `boolean`  
**Default:** `true`

Enable TLS for operator webhooks and metrics server.

```bash
export MUTO_TLS_ENABLED=true
```

### MUTO_TLS_CERT_FILE

**Type:** `string`  
**Default:** `/etc/muto/certs/tls.crt`

Path to TLS certificate file.

```bash
export MUTO_TLS_CERT_FILE=/etc/muto/certs/tls.crt
```

### MUTO_TLS_KEY_FILE

**Type:** `string`  
**Default:** `/etc/muto/certs/tls.key`

Path to TLS private key file.

```bash
export MUTO_TLS_KEY_FILE=/etc/muto/certs/tls.key
```

### MUTO_MTLS_ENABLED

**Type:** `boolean`  
**Default:** `false`

Enable mutual TLS (mTLS) for agent-to-operator communication.

```bash
export MUTO_MTLS_ENABLED=true
```

### MUTO_MTLS_CA_FILE

**Type:** `string`  
**Default:** (none)

Path to CA certificate file for mTLS.

```bash
export MUTO_MTLS_CA_FILE=/etc/muto/certs/ca.crt
```

### MUTO_MTLS_CLIENT_CERT_FILE

**Type:** `string`  
**Default:** (none)

Path to client certificate file for mTLS.

```bash
export MUTO_MTLS_CLIENT_CERT_FILE=/etc/muto/certs/client.crt
```

### MUTO_MTLS_CLIENT_KEY_FILE

**Type:** `string`  
**Default:** (none)

Path to client private key file for mTLS.

```bash
export MUTO_MTLS_CLIENT_KEY_FILE=/etc/muto/certs/client.key
```

---

## Observability

### MUTO_METRICS_ENABLED

**Type:** `boolean`  
**Default:** `true`

Enable Prometheus metrics export.

```bash
export MUTO_METRICS_ENABLED=true
```

### MUTO_METRICS_PORT

**Type:** `integer`  
**Default:** `8081`

Port for Prometheus metrics server.

```bash
export MUTO_METRICS_PORT=8081
```

### MUTO_TRACING_ENABLED

**Type:** `boolean`  
**Default:** `false`

Enable OpenTelemetry distributed tracing.

```bash
export MUTO_TRACING_ENABLED=true
```

### MUTO_TRACING_JAEGER_ENDPOINT

**Type:** `string`  
**Default:** (none)

Jaeger collector endpoint for tracing.

```bash
export MUTO_TRACING_JAEGER_ENDPOINT=http://jaeger-collector.monitoring:14268/api/traces
```

### MUTO_TRACING_SAMPLE_RATE

**Type:** `float`  
**Default:** `0.1`

Sampling rate for traces (0.0 = none, 1.0 = all).

```bash
# Sample 10% of traces
export MUTO_TRACING_SAMPLE_RATE=0.1

# Sample all traces
export MUTO_TRACING_SAMPLE_RATE=1.0
```

---

## Resource Management

### MUTO_RESOURCE_CPU_REQUEST

**Type:** `string`  
**Default:** `100m`

Default CPU request for agent jobs.

```bash
export MUTO_RESOURCE_CPU_REQUEST=500m
```

### MUTO_RESOURCE_CPU_LIMIT

**Type:** `string`  
**Default:** `1000m`

Default CPU limit for agent jobs.

```bash
export MUTO_RESOURCE_CPU_LIMIT=2000m
```

### MUTO_RESOURCE_MEMORY_REQUEST

**Type:** `string`  
**Default:** `256Mi`

Default memory request for agent jobs.

```bash
export MUTO_RESOURCE_MEMORY_REQUEST=512Mi
```

### MUTO_RESOURCE_MEMORY_LIMIT

**Type:** `string`  
**Default:** `512Mi`

Default memory limit for agent jobs.

```bash
export MUTO_RESOURCE_MEMORY_LIMIT=1Gi
```

### MUTO_MAX_CONCURRENT_JOBS

**Type:** `integer`  
**Default:** `100`

Maximum number of concurrent agent jobs.

```bash
export MUTO_MAX_CONCURRENT_JOBS=500
```

---

## Multi-Tenancy

### MUTO_ISOLATION_LEVEL

**Type:** `string`  
**Default:** `strong`  
**Valid Values:** `strong`, `moderate`

Tenant isolation level.

```bash
# Strong isolation (compute, network, storage, messaging)
export MUTO_ISOLATION_LEVEL=strong

# Moderate isolation (compute and messaging only)
export MUTO_ISOLATION_LEVEL=moderate
```

### MUTO_TENANT_QUOTA_ENABLED

**Type:** `boolean`  
**Default:** `true`

Enable tenant resource quotas.

```bash
export MUTO_TENANT_QUOTA_ENABLED=true
```

### MUTO_TENANT_QUOTA_DEFAULT_CPU

**Type:** `string`  
**Default:** `10`

Default CPU quota (in cores) per tenant.

```bash
export MUTO_TENANT_QUOTA_DEFAULT_CPU=50
```

### MUTO_TENANT_QUOTA_DEFAULT_MEMORY

**Type:** `string`  
**Default:** `20Gi`

Default memory quota per tenant.

```bash
export MUTO_TENANT_QUOTA_DEFAULT_MEMORY=100Gi
```

### MUTO_TENANT_NAMESPACE_PREFIX

**Type:** `string`  
**Default:** `tenant-`

Prefix for tenant namespaces (Kubernetes only).

```bash
export MUTO_TENANT_NAMESPACE_PREFIX=tenant-
```

---

## Configuration Examples

### Development Environment

```bash
# Local development with NATS
export MUTO_PLATFORM=kubernetes
export MUTO_LOG_LEVEL=debug
export MUTO_MESSAGE_BUS_TYPE=nats
export MUTO_NATS_URL=nats://localhost:4222
export MUTO_RECONCILER_WORKER_COUNT=2
```

### Production Environment (Kubernetes)

```bash
# Production K8s with Kafka and high availability
export MUTO_PLATFORM=kubernetes
export MUTO_LOG_LEVEL=info
export MUTO_MESSAGE_BUS_TYPE=kafka
export MUTO_KAFKA_BROKERS=kafka-1:9092,kafka-2:9092,kafka-3:9092
export MUTO_KAFKA_SASL_ENABLED=true
export MUTO_KAFKA_SASL_MECHANISM=SCRAM-SHA-256
export MUTO_KAFKA_TLS_ENABLED=true
export MUTO_RECONCILER_WORKER_COUNT=20
export MUTO_K8S_LEADER_ELECTION_ENABLED=true
export MUTO_TLS_ENABLED=true
export MUTO_METRICS_ENABLED=true
export MUTO_TRACING_ENABLED=true
export MUTO_MAX_CONCURRENT_JOBS=500
```

### Production Environment (CloudFoundry)

```bash
# Production CloudFoundry with secure configuration
export MUTO_PLATFORM=cloudfoundry
export MUTO_CF_API_URL=https://api.cf.production.com
export MUTO_CF_USERNAME=muto-service-account
export MUTO_LOG_LEVEL=info
export MUTO_MESSAGE_BUS_TYPE=kafka
export MUTO_KAFKA_BROKERS=kafka-1:9092,kafka-2:9092
export MUTO_KAFKA_SASL_ENABLED=true
export MUTO_KAFKA_TLS_ENABLED=true
export MUTO_RECONCILER_WORKER_COUNT=15
export MUTO_TLS_ENABLED=true
export MUTO_TRACING_ENABLED=true
```

---

## Best Practices

1. **Never commit secrets** to version control. Use Kubernetes Secrets or CloudFoundry user-provided services.

2. **Use environment files** in production:
   ```bash
   source /etc/muto/muto.env
   ```

3. **Validate configuration** at startup:
   ```bash
   ./bin/muto-operator --validate-config
   ```

4. **Monitor configuration changes** with event logging enabled.

5. **Test configuration changes** in a staging environment first.

6. **Use TLS in production** — never disable `MUTO_TLS_ENABLED` in production.

7. **Enable tracing** for troubleshooting, but adjust sampling rate for performance.

---

## See Also

- [Reconciler Configuration](./reconciler-config.md) — Detailed reconciler settings
- [Message Bus Setup](./message-bus-setup.md) — Message bus tuning
- [Multi-Tenant Setup](./multi-tenant-setup.md) — Tenant configuration
- [TLS and Security](./tls-security.md) — Security configuration
- [Deployment Guide](../deployment/kubernetes/install.md) — Installation steps
- [Architecture Overview](../architecture/overview.md) — How configuration is used

---

**Last Updated:** 2026-09-03
