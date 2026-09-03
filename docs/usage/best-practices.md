# Best Practices for Agent Jobs

Guidelines for building reliable, efficient, and cost-effective Muto workflows in production.

## Job Sizing and Resource Allocation

### Understanding Resource Requests and Limits

Every agent should specify resource requests and limits:

```yaml
agents:
  - name: processor
    image: myorg/processor:v1
    resources:
      # Minimum resources guaranteed by scheduler
      requests:
        cpu: "500m"      # 0.5 CPU core
        memory: "512Mi"  # 512 MB
      
      # Maximum resources agent can consume
      limits:
        cpu: "2"         # 2 CPU cores
        memory: "2Gi"    # 2 GB
```

**Why both?**
- **Requests**: Scheduler reserves these resources. Too low → contention with other jobs; too high → wasted resources
- **Limits**: Kubernetes kills the agent if it exceeds. Too low → OOM kills; too high → runaway processes cost money

### Sizing Your Requests

#### CPU Sizing

1. **Profile your workload** on a test cluster
2. **Measure actual usage** during typical execution
3. **Add 20% buffer** for variance
4. **Set limit to 4x request** (allows burst, prevents runaway)

```bash
# Profile CPU usage
kubectl run test-processor --image=myorg/processor:v1 --requests=cpu=1 --limits=cpu=4
kubectl top pod test-processor --containers

# Example output:
# NAME               CPU     MEMORY
# test-processor     750m    512Mi

# Recommended requests: 750m + 20% = 900m
# Recommended limits: 900m * 4 = 3600m (3.6 CPUs)
```

#### Memory Sizing

1. **Measure peak memory** during execution
2. **Add 30% buffer** for variance
3. **Set limit = request** (memory doesn't burst like CPU)

```bash
# Monitor memory usage
while true; do
  echo "$(date): $(docker stats --no-stream | grep myorg/processor)"
  sleep 5
done

# Set requests to peak + 30%
```

### Resource Limits by Workload Type

| Workload | CPU Request | CPU Limit | Memory Request | Memory Limit |
|----------|-------------|-----------|----------------|--------------|
| I/O-bound (API calls, network) | 100-250m | 500m-1 | 64-256Mi | Same as request |
| CPU-bound (compute) | 500m-2 | 4-8 | 256Mi-1Gi | Same as request |
| Memory-bound (processing large files) | 250m-1 | 2-4 | 1Gi-4Gi | Same as request |
| Data-intensive ETL | 1-2 | 4-8 | 2Gi-8Gi | Same as request |

### Cost Optimization Through Right-Sizing

```yaml
# ❌ BAD: Over-provisioning (wasted money)
resources:
  requests:
    cpu: "4"
    memory: "8Gi"
  limits:
    cpu: "8"
    memory: "8Gi"
# Actual usage: 500m CPU, 256Mi memory
# Wasted: 3.5 CPUs, ~7.75Gi memory

# ✅ GOOD: Right-sized (efficient)
resources:
  requests:
    cpu: "600m"      # 500m actual + 20% buffer
    memory: "330Mi"  # 256Mi actual + 30% buffer
  limits:
    cpu: "2400m"     # 600m * 4 for burst
    memory: "330Mi"  # Memory doesn't burst
# Cost: ~20% of over-provisioned example
```

## Timeout Configuration

### Understanding Timeout Levels

Muto supports three timeout levels:

```yaml
spec:
  timeout: 1h                    # Job timeout: total time for all agents
  activeDeadlineSeconds: 3600    # Hard deadline: no grace period
  agents:
    - name: processor
      timeout: 30m               # Agent timeout: individual agent limit
```

### Timeout Strategy

1. **Set job timeout** based on expected total duration
2. **Set agent timeout** based on individual stage performance
3. **Add buffer** for scheduling, image pulls, startup
4. **Monitor for timeout patterns** and adjust

### Calculating Timeouts

```yaml
# Example: Data pipeline with 3 stages

# Stage 1: Extract data (typically 10-15 minutes)
- name: extractor
  timeout: 20m          # 15m typical + 5m buffer

# Stage 2: Process (typically 30-40 minutes)
- name: processor
  timeout: 50m          # 40m typical + 10m buffer

# Stage 3: Aggregate (typically 5-10 minutes)
- name: aggregator
  timeout: 15m          # 10m typical + 5m buffer

# Job timeout: 20m + 50m + 15m = 85m, round up
spec:
  timeout: 90m          # Add another buffer for scheduling

# Hard deadline: same as job timeout
spec:
  activeDeadlineSeconds: 5400  # 90 minutes
```

### Handling Timeout Failures

When jobs timeout, they fail. Plan for recovery:

```yaml
# Strategy 1: Retry with longer timeout
spec:
  timeout: 1h
  retryPolicy:
    maxRetries: 2
  agents:
    - name: processor
      image: myorg/processor:v1
      # Second retry can have longer timeout
      # (implement in your agent's initialization)

# Strategy 2: Increase parallelism to process faster
spec:
  timeout: 30m
  parallelism: 4       # Process 4 chunks in parallel instead of 1
  agents:
    - name: processor
      image: myorg/processor:v1

# Strategy 3: Sample input data for testing
spec:
  timeout: 10m
  agents:
    - name: processor
      image: myorg/processor:v1
      env:
        - name: SAMPLE_PERCENT
          value: "10"  # Test on 10% of data first
```

## Message Bus Optimization

### Message Retention Strategy

Configure message retention based on your workflow:

```yaml
# NATS: Configure retention when publishing
# Keep messages for 1 day (86400 seconds)
nats pub acme-corp/workflow/complete \
  '{"status":"ok"}' \
  --max-msgs 10000 \
  --max-bytes 1073741824 \
  --max-age 86400s

# Kafka: Configure per topic
kafka-configs.sh --bootstrap-server localhost:9092 \
  --entity-type topics \
  --entity-name acme-corp-workflow \
  --alter \
  --add-config retention.ms=86400000
```

### Message Size Guidelines

Keep messages small for efficiency:

```yaml
# ❌ BAD: Large message (entire dataset)
message:
  data: "<entire CSV file contents>"  # Could be 100MB+

# ✅ GOOD: Small message (reference to data)
message:
  status: "complete"
  dataPath: "s3://bucket/output.csv"
  checksum: "abc123def456"
  sizeBytes: 1048576
```

### Message Ordering and Idempotency

For reliable coordination:

1. **Design idempotent message handlers** (safe to process twice)
2. **Use message IDs** to track what's been processed
3. **Store processed message IDs** (in database or local storage)


## Monitoring and Observability

### Key Metrics to Track

Monitor these metrics for production health:

```
# Job Execution
muto_jobs_total{status="completed"}      # Successful jobs
muto_jobs_total{status="failed"}         # Failed jobs
muto_job_duration_seconds               # Job execution time
muto_job_retries_total                  # Retry rate

# Agent Performance
muto_agents_running                     # Agents actively running
muto_agent_duration_seconds             # Agent execution time
muto_agent_errors_total                 # Agent error count

# Resource Usage
muto_job_cpu_usage_cores                # CPU consumption
muto_job_memory_usage_bytes             # Memory consumption
muto_job_storage_usage_bytes            # Storage usage

# Message Bus
muto_message_bus_publish_latency_ms     # Message publish latency
muto_message_bus_subscribe_latency_ms   # Message subscribe latency
muto_message_bus_queue_depth            # Pending messages
```

### Alert Rules

Set up Prometheus alerts for critical issues:

```yaml
# prometheus-rules.yaml
groups:
  - name: muto-alerts
    interval: 30s
    rules:
      # Alert: High job failure rate
      - alert: HighJobFailureRate
        expr: |
          rate(muto_jobs_total{status="failed"}[5m]) /
          rate(muto_jobs_total[5m]) > 0.05
        for: 5m
        annotations:
          summary: "More than 5% of jobs are failing"
      
      # Alert: Job timeout rate
      - alert: HighTimeoutRate
        expr: |
          rate(muto_job_retries_total{reason="timeout"}[5m]) > 0.1
        for: 5m
        annotations:
          summary: "More than 10% retry rate due to timeouts"
      
      # Alert: Message bus latency
      - alert: HighMessageLatency
        expr: |
          histogram_quantile(0.95, muto_message_bus_publish_latency_ms) > 1000
        for: 5m
        annotations:
          summary: "Message publish latency p95 > 1 second"
```

### Logging Strategy

Structure logs for easy debugging:

```bash
# Good: Structured JSON logs
{"timestamp":"2026-09-03T10:30:45Z","level":"info","job":"data-pipeline","phase":"running","message":"Agent started","agent":"processor","duration":45}

# Extract logs by job:
kubectl logs agentjob/data-pipeline -c processor | jq 'select(.agent=="processor")'

# Filter by error
kubectl logs agentjob/data-pipeline | jq 'select(.level=="error")'
```

## Cost Optimization

### 1. Right-Size Resources

See Resource Sizing section above. This is the biggest cost driver.

### 2. Schedule Jobs During Off-Peak Hours

```yaml
# Use priority to defer low-priority jobs
spec:
  priority: 1              # Low priority
  agents:
    - name: reporting
      image: myorg/reporting:v1
  
  # In your reconciler:
  # - Check if off-peak hours
  # - If yes, start job
  # - If no, requeue until off-peak
```

### 3. Use Spot Instances for Non-Critical Jobs

```yaml
# Kubernetes: Use spot instance nodeSelector
spec:
  nodeSelector:
    karpenter.sh/capacity-type: spot   # Cheap but can be interrupted
  affinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchLabels:
            cloud.google.com/gke-preemptible: "true"
  tolerations:
    - key: cloud.google.com/gke-preemptible
      operator: Equal
      value: "true"
      effect: NoSchedule
```

### 4. Consolidate Jobs

```yaml
# ❌ Bad: 10 separate jobs (10x overhead)
for i in {0..9}; do
  kubectl apply -f job-$i.yaml
done

# ✅ Good: One job with 10 agents (less overhead)
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: bulk-processing
spec:
  agents:
    - name: processor-0
      image: myorg/processor:v1
    - name: processor-1
      image: myorg/processor:v1
    # ... processor-2 through processor-9 ...
```

### 5. Cache Expensive Computations

```yaml
# Store results in shared cache
- name: compute-expensive
  image: myorg/compute:v1
  env:
    - name: CACHE_DIR
      value: /shared/cache
  volumeMounts:
    - name: cache
      mountPath: /shared/cache

# Share cache across jobs
volumes:
  - name: cache
    persistentVolumeClaim:
      claimName: compute-cache
```

### 6. Delete Completed Jobs Promptly

```yaml
# Auto-cleanup after completion
spec:
  ttlSecondsAfterFinished: 3600  # Delete 1 hour after completion
  # Don't set to 0 (immediate) - you won't see logs
  # Set to reasonable value (1-7 days) based on debugging needs
```

## Scaling Strategies

### Horizontal Scaling (More Agents)

For embarrassingly parallel work:

```yaml
# Before: 1 agent, 1 hour
spec:
  agents:
    - name: processor
      image: myorg/processor:v1

# After: 10 agents in parallel, ~6 minutes
spec:
  agents:
    - name: processor-0 through processor-9
      image: myorg/processor:v1
      env:
        - name: WORKER_ID
          value: "0" through "9"
        - name: TOTAL_WORKERS
          value: "10"
```

### Vertical Scaling (Bigger Agents)

For single-threaded work that needs more power:

```yaml
# Before: 1 CPU, 1GB memory, 1 hour
resources:
  requests:
    cpu: "1"
    memory: "1Gi"

# After: 4 CPUs, 4GB memory, 15 minutes (4x faster)
resources:
  requests:
    cpu: "4"
    memory: "4Gi"
```

### Adaptive Scaling

Scale based on queue depth:


### Cluster Capacity Planning

Monitor cluster utilization:

```bash
# Current usage
kubectl describe nodes | grep -A 5 "Allocated resources"

# Project capacity needs
# If average utilization > 70%, plan to scale cluster
# If peak utilization > 85%, you need more headroom

# Example: Monitor over time
for i in {1..30}; do
  date
  kubectl describe nodes | grep -A 2 "Allocated resources" | grep "requests"
  sleep 60
done
```

## Error Handling and Recovery

### Transient vs. Permanent Errors

Design retries appropriately:

```yaml
# ✅ Good: Retry on transient errors
retryPolicy:
  maxRetries: 3
  backoffSeconds: 5

agents:
  - name: api-call
    image: myorg/api-client:v1
    # Transient: network timeout, service unavailable
    # Will succeed on retry after service recovers

# ❌ Bad: Don't retry permanent errors
# Invalid input, missing files, auth errors won't be fixed by retrying
```

### Graceful Degradation

Handle missing dependencies:


### Idempotent Operations

Make jobs safe to retry:

```yaml
# Good: Idempotent (safe to run multiple times)
- name: update-database
  image: myorg/db-updater:v1
  command: ["sh", "-c"]
  args:
    - |
      # Upsert is idempotent
      sqlite3 db.sqlite3 "INSERT OR REPLACE INTO results VALUES(...)"

# Bad: Non-idempotent (unsafe to retry)
- name: send-notification
  image: myorg/mailer:v1
  command: ["sh", "-c"]
  args:
    - |
      # Sends email every time; retries send duplicate emails
      sendmail user@example.com "Job completed"
```

## Testing and Validation

### Test Timeout Behavior

```bash
# Create a job that will timeout
kubectl apply -f - << 'YAML'
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: timeout-test
spec:
  timeout: 10s
  agents:
    - name: sleeper
      image: alpine:latest
      command: ["sleep", "30"]
YAML

# Watch it timeout
kubectl get agentjob timeout-test --watch

# Should reach "Failed" with "Timeout" reason
kubectl get agentjob timeout-test -o json | jq '.status | {phase, reason}'
```

### Test Retry Behavior

```bash
# Create a job that will fail and retry
kubectl apply -f - << 'YAML'
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: retry-test
spec:
  retryPolicy:
    maxRetries: 3
  agents:
    - name: flaky
      image: alpine:latest
      command: ["sh", "-c"]
      args: ["[ $RANDOM -gt 16384 ] || exit 1"]  # 50% failure rate
YAML

# Should eventually succeed with retries
kubectl get agentjob retry-test --watch
```

### Performance Testing

```bash
# Create jobs to test scaling
for i in {0..9}; do
  kubectl apply -f job-template.yaml --env JOB_ID=$i
done

# Monitor resource usage
kubectl top pods -l app=processor --container=processor --sort-by=memory
```

## Documentation and Runbooks

### Job Definition Template

Create a template for your organization:

```yaml
# Template: standard-job.yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: CHANGE_ME
  namespace: CHANGE_ME
  labels:
    app: CHANGE_ME
    owner: CHANGE_ME
spec:
  tenant: CHANGE_ME
  priority: 50
  timeout: 1h              # Adjust based on workload
  activeDeadlineSeconds: 3600
  ttlSecondsAfterFinished: 86400
  
  retryPolicy:
    maxRetries: 2
    backoffSeconds: 30
    maxBackoffSeconds: 300
  
  agents:
    - name: processor
      image: myorg/processor:CHANGE_ME
      resources:
        requests:
          cpu: "500m"      # Profile and adjust
          memory: "512Mi"
        limits:
          cpu: "2"
          memory: "2Gi"
      timeout: 55m         # Slightly less than job timeout
```

### Runbook: Debugging Failed Job

```markdown
## Problem: Job Failed in Production

### Step 1: Get Job Status
```bash
kubectl describe agentjob <job-name>
kubectl get agentjob <job-name> -o json | jq '.status'
```

### Step 2: Check Logs
```bash
kubectl logs agentjob/<job-name> --all-containers=true --tail=500
```

### Step 3: Common Issues
- **Timeout**: Increase timeout or optimize performance
- **OOM Kill**: Increase memory requests/limits
- **Network Error**: Check connectivity and message bus
- **Invalid Input**: Validate input data format

### Step 4: Recovery
- Fix underlying issue
- Delete old job: `kubectl delete agentjob <job-name>`
- Resubmit: `kubectl apply -f job.yaml`
```

## Next Steps

- **[Scheduling Agent Jobs](./scheduling-agent-jobs.md)** — Job specification reference
- **[Multi-Agent Patterns](./multi-agent-patterns.md)** — Orchestration patterns
- **[Monitoring & Observability](../operations/monitoring-observability.md)** (coming in Phase 8) — Setup observability
- **[Configuration](../configuration/)** — Fine-tune settings

---

**Last Updated:** 2026-09-03
