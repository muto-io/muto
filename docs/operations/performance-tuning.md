# Performance Tuning and Scaling

Optimize Muto for your workload: tuning reconcilers, message bus, resource allocation, and scaling strategies.

## Overview

Muto's performance depends on several factors:

- **Reconciler configuration** — How aggressively the system checks for drift
- **Message bus capacity** — Throughput and latency of inter-agent communication
- **Resource allocation** — CPU, memory, and disk available to operator and agents
- **Cluster capacity** — Number of nodes and their resources

This guide covers tuning each layer.

## Reconciler Tuning

Reconcilers continuously monitor and reconcile state. Tuning them affects latency and resource usage.

### Key Configuration Parameters

**MUTO_RECONCILER_WORKERS** (default: `4`)

Number of concurrent reconciliation workers.

```bash
# High-throughput: increase workers
export MUTO_RECONCILER_WORKERS=8

# Low-resource: reduce workers
export MUTO_RECONCILER_WORKERS=2
```

**Impact:**
- Higher workers = lower job latency, higher CPU usage
- Lower workers = higher latency, lower CPU usage
- Optimal: 2-4x your operator pod count

**MUTO_RECONCILER_POLL_INTERVAL_SECONDS** (default: `2`)

How often reconcilers check for drift (in seconds).

```bash
# Aggressive (lower latency)
export MUTO_RECONCILER_POLL_INTERVAL_SECONDS=1

# Conservative (lower CPU)
export MUTO_RECONCILER_POLL_INTERVAL_SECONDS=5
```

**Impact:**
- Shorter interval = faster job detection, higher CPU usage
- Longer interval = slower job detection, lower CPU usage
- Optimal: 1-5 seconds depending on latency requirements

**MUTO_MAX_RETRIES** (default: `3`)

Maximum retry attempts for failed reconciliations.

```bash
export MUTO_MAX_RETRIES=5
```

**MUTO_BACKOFF_EXPONENT** (default: `2.0`)

Exponential backoff multiplier for retries.

```bash
# Linear backoff (1 second, 1 second, 1 second)
export MUTO_BACKOFF_EXPONENT=1.0

# Exponential backoff (1 second, 2 seconds, 4 seconds, 8 seconds)
export MUTO_BACKOFF_EXPONENT=2.0
```

**MUTO_MAX_BACKOFF_SECONDS** (default: `60`)

Maximum wait time between retries.

```bash
export MUTO_MAX_BACKOFF_SECONDS=120
```

### Tuning for Different Workloads

#### Low-Latency Workloads

Minimize job scheduling delay.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muto-tuning
  namespace: muto-system
data:
  MUTO_RECONCILER_WORKERS: "8"
  MUTO_RECONCILER_POLL_INTERVAL_SECONDS: "1"
  MUTO_SCHEDULER_WORKERS: "8"
```

**Trade-off:** Higher CPU and memory usage.

#### High-Throughput Workloads

Process many jobs with moderate latency.

```yaml
data:
  MUTO_RECONCILER_WORKERS: "6"
  MUTO_RECONCILER_POLL_INTERVAL_SECONDS: "2"
  MUTO_SCHEDULER_WORKERS: "6"
  MUTO_JOB_BATCH_SIZE: "50"
```

**Trade-off:** Balanced resource usage and latency.

#### Low-Resource Workloads

Minimize CPU and memory for constrained environments.

```yaml
data:
  MUTO_RECONCILER_WORKERS: "2"
  MUTO_RECONCILER_POLL_INTERVAL_SECONDS: "5"
  MUTO_SCHEDULER_WORKERS: "1"
  MUTO_LOG_LEVEL: "warn"
```

**Trade-off:** Higher job scheduling latency.

## Message Bus Tuning

The message bus handles inter-agent communication.

### NATS Configuration

For development and small deployments:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nats-config
data:
  nats.conf: |
    # Server settings
    max_connections: 100000
    max_control_line: 4096
    max_payload: 1MB
    
    # Performance tuning
    lame_duck_duration: 30s
    ping_interval: 2m
    ping_max_outstanding: 2
    
    # Clustering (optional)
    cluster {
      listen: 0.0.0.0:6222
      authorization {
        user: internal
        password: internal
      }
    }
    
    # JetStream (persistent messaging)
    jetstream {
      store_dir: /data
      max_memory_store: 1GB
      max_file_store: 10GB
    }
```

### Kafka Configuration

For enterprise and high-throughput:

```yaml
apiVersion: kafka.strimzi.io/v1beta2
kind: Kafka
metadata:
  name: muto-kafka
spec:
  kafka:
    version: 3.4.0
    replicas: 3
    resources:
      requests:
        memory: "1Gi"
        cpu: "500m"
      limits:
        memory: "2Gi"
        cpu: "1000m"
    config:
      # Performance tuning
      num.network.threads: 4
      num.io.threads: 4
      socket.send.buffer.bytes: 102400
      socket.receive.buffer.bytes: 102400
      socket.request.max.bytes: 104857600
      
      # Replication
      default.replication.factor: 3
      min.insync.replicas: 2
      
      # Retention
      log.retention.hours: 168
      log.segment.bytes: 1073741824
```

### Topic Configuration

Configure topics for optimal throughput:

```bash
# High-throughput topic (many partitions)
kafka-topics.sh --create \
  --topic muto-workflow \
  --partitions 10 \
  --replication-factor 3 \
  --config min.insync.replicas=2 \
  --config compression.type=snappy

# Low-latency topic (few partitions, low retention)
kafka-topics.sh --create \
  --topic muto-events \
  --partitions 3 \
  --replication-factor 3 \
  --config retention.ms=3600000
```

### Connection Pooling

Configure message bus client for optimal connections:

```bash
export MUTO_MESSAGE_BUS_CONNECTION_POOL_SIZE=32
export MUTO_MESSAGE_BUS_PUBLISH_TIMEOUT_MS=5000
export MUTO_MESSAGE_BUS_SUBSCRIBE_BUFFER_SIZE=1000
```

## Resource Allocation

### Operator Pod Resources

Set CPU and memory requests/limits appropriately:

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
        resources:
          requests:
            # These values are guaranteed
            cpu: 500m
            memory: 512Mi
          limits:
            # Max the pod can use
            cpu: 2000m
            memory: 2Gi
```

**Sizing guidelines:**

| Workload | CPU Request | Memory Request | CPU Limit | Memory Limit |
|----------|-------------|-----------------|-----------|--------------|
| Small (< 100 jobs/min) | 200m | 256Mi | 1000m | 1Gi |
| Medium (100-500 jobs/min) | 500m | 512Mi | 2000m | 2Gi |
| Large (500-2000 jobs/min) | 1000m | 1Gi | 4000m | 4Gi |
| XL (> 2000 jobs/min) | 2000m | 2Gi | 8000m | 8Gi |

### Agent Pod Resources

Control resources available to agent containers:

```bash
# Environment variable to set default agent limits
export MUTO_AGENT_RESOURCE_LIMIT_CPU=4000m
export MUTO_AGENT_RESOURCE_LIMIT_MEMORY=4Gi
export MUTO_AGENT_RESOURCE_REQUEST_CPU=1000m
export MUTO_AGENT_RESOURCE_REQUEST_MEMORY=1Gi
```

Or specify per-job:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: resource-intensive-job
spec:
  agents:
  - name: processor
    image: myorg/processor:v1
    resources:
      requests:
        cpu: 2000m
        memory: 4Gi
      limits:
        cpu: 4000m
        memory: 8Gi
```

## Horizontal Scaling

### Multiple Operator Replicas

Scale operator for high availability and throughput:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muto-operator
  namespace: muto-system
spec:
  replicas: 3  # Scale to 3 replicas
  template:
    spec:
      affinity:
        # Spread replicas across nodes
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - muto-operator
              topologyKey: kubernetes.io/hostname
```

**Impact of scaling:**
- 1 replica: Single point of failure, but minimal resource usage
- 3 replicas: High availability, distributed work, moderate resource usage
- N replicas: Linear throughput increase (up to message bus limit)

### Node Scaling

Add more nodes when operator CPU is > 80%:

```bash
# Check current usage
kubectl top nodes
kubectl top pods -n muto-system

# Scale cluster (e.g., GKE)
gcloud container clusters resize muto-cluster --num-nodes 10

# Or use cluster autoscaling
kubectl edit deployment muto-autoscaler
```

## Benchmarking and Performance Testing

### Load Testing

Generate synthetic job load to test capacity:

```bash
#!/bin/bash
# load-test.sh - Simple load test

NUM_JOBS=1000
START_TIME=$(date +%s)

for i in $(seq 1 $NUM_JOBS); do
  kubectl apply -f - <<EOF
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: load-test-$i
  namespace: default
spec:
  tenant: default
  agents:
  - name: test-agent
    image: alpine:latest
    command: ["sh", "-c"]
    args: ["echo 'Job $i'; sleep 5"]
  timeout: 60s
EOF
done

# Wait for all jobs to complete
sleep 30
while [ $(kubectl get agentjob -o json | jq '.items | length') -gt 0 ]; do
  echo "Waiting for jobs to complete..."
  sleep 10
done

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))
THROUGHPUT=$((NUM_JOBS * 60 / DURATION))

echo "Completed $NUM_JOBS jobs in $DURATION seconds"
echo "Throughput: $THROUGHPUT jobs/minute"
```

### Metrics to Monitor

During load testing, monitor these metrics:

```promql
# Jobs per second
rate(muto_jobs_total[1m])

# P95 job latency
histogram_quantile(0.95, rate(muto_job_duration_seconds_bucket[1m]))

# Operator CPU usage
container_cpu_usage_seconds_total

# Operator memory usage
container_memory_usage_bytes

# Reconciliation error rate
rate(muto_reconciliations_total{result="error"}[1m])

# Message bus latency
rate(muto_message_bus_latency_seconds[1m])
```

### Performance Profiling

Enable profiling to identify bottlenecks:

```bash
# Enable CPU profiling
export MUTO_PROFILE_CPU=true
export MUTO_PROFILE_OUTPUT_DIR=/tmp/profiles

# Run workload
# Then analyze
go tool pprof /tmp/profiles/cpu.prof

# Common commands in pprof:
# top       - Show top functions by CPU
# list      - Show source code
# web       - Generate graph (requires graphviz)
```

## Optimization Checklist

Use this checklist to optimize your Muto deployment:

- [ ] **Operator Resources**: Match resource requests to workload size (see sizing table)
- [ ] **Reconciler Workers**: Set to 2-4x operator replicas
- [ ] **Poll Interval**: Balance latency vs. CPU (1-5 seconds)
- [ ] **Message Bus**: Use Kafka for high-throughput, NATS for simple
- [ ] **Connection Pooling**: Enable for message bus clients
- [ ] **Multiple Replicas**: Use 3+ operator replicas for HA
- [ ] **Node Affinity**: Spread operator replicas across nodes
- [ ] **Monitoring**: Enable metrics and set up dashboards
- [ ] **Alerting**: Set alerts for error rates and latency
- [ ] **Load Testing**: Validate capacity before production
- [ ] **Profiling**: Identify bottlenecks with CPU/memory profiling

## Common Optimization Scenarios

### Scenario 1: High Job Scheduling Latency

**Problem:** Jobs take > 10 seconds from creation to execution

**Solution:**
```bash
# Increase reconciler workers
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_RECONCILER_WORKERS=8

# Decrease poll interval
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_RECONCILER_POLL_INTERVAL_SECONDS=1

# Scale operator to 3 replicas
kubectl scale deployment muto-operator -n muto-system --replicas=3
```

### Scenario 2: High CPU Usage

**Problem:** Operator CPU consistently > 80%

**Solution:**
```bash
# Increase poll interval (slower detection but lower CPU)
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_RECONCILER_POLL_INTERVAL_SECONDS=5

# Reduce reconciler workers
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_RECONCILER_WORKERS=2

# Or scale to more replicas (distribute load)
kubectl scale deployment muto-operator -n muto-system --replicas=3
```

### Scenario 3: Job Execution Bottleneck

**Problem:** Jobs complete but agents are waiting for message bus

**Solution:**
```bash
# Increase message bus connection pool
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_MESSAGE_BUS_CONNECTION_POOL_SIZE=64

# For Kafka, increase broker throughput
kubectl edit statefulset kafka -n kafka
# Increase: num.network.threads, num.io.threads

# Scale message bus (add replicas)
kubectl scale statefulset nats -n message-bus --replicas=3
```

### Scenario 4: Backlog of Pending Jobs

**Problem:** Jobs are created but not scheduled

**Solution:**
```bash
# Increase scheduler workers
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_SCHEDULER_WORKERS=8

# Check if jobs are valid
kubectl get agentjobs -o wide | grep Pending

# Increase operator CPU limits
kubectl edit deployment/muto-operator -n muto-system
# Increase spec.template.spec.containers[0].resources.limits.cpu
```

---

## Best Practices

1. **Measure before optimizing** — Use metrics to identify actual bottlenecks
2. **Change one variable at a time** — Makes it easier to see the impact
3. **Load test new configurations** — Verify changes improve performance
4. **Monitor continuously** — Set up dashboards and alerts
5. **Plan for growth** — Capacity plan for 2-3x your current load
6. **Document your tuning** — Keep notes on what works for your workload

---

**See Also:**
- [Monitoring and Observability](./monitoring-observability.md) — Collecting performance data
- [Configuration Reference](../configuration/environment-variables.md) — All tunable parameters
- [Architecture Overview](../architecture/overview.md) — Understanding the system design
