# Scheduling Agent Jobs

Learn how to create and manage agent jobs in Muto, from basic definitions to advanced job lifecycle management.

## Overview

Agent Jobs are the primary way to execute agents in Muto. A job specifies what agents to run, how to configure them, and how to handle failures. This guide covers the complete job lifecycle: creation, execution, monitoring, and cleanup.

## Creating a Simple Agent Job

The simplest agent job runs a single agent with minimal configuration:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: my-first-job
  namespace: default
spec:
  tenant: default
  agents:
    - name: processor
      image: myorg/processor:latest
```

Apply this to your cluster:

```bash
kubectl apply -f job.yaml
kubectl get agentjobs
kubectl describe agentjob my-first-job
```

## Job Specification Reference

### Top-Level Fields

```yaml
apiVersion: muto.io/v1                    # Required: API version
kind: AgentJob                             # Required: Resource type
metadata:
  name: string                             # Required: Unique job name
  namespace: string                        # Required: K8s namespace
  labels:                                  # Optional: Labels for selection
    app: my-app
    version: v1
  annotations:                             # Optional: Metadata
    description: "Data processing job"
spec:
  tenant: string                           # Required: Tenant ID
  priority: integer                        # Optional: 0-100, higher = more urgent (default: 50)
  agents: []                               # Required: List of agents to execute
  timeout: duration                        # Optional: Job timeout (default: 1h)
  retryPolicy:                             # Optional: Failure retry behavior
    maxRetries: integer                    # Max retry attempts (default: 1)
    backoffSeconds: integer                # Initial backoff in seconds (default: 5)
    backoffMultiplier: float               # Backoff multiplier (default: 2.0)
    maxBackoffSeconds: integer             # Maximum backoff (default: 300)
  parallelism: integer                     # Optional: Parallel execution (default: 1)
  completionPolicy: string                 # Optional: serial|parallel|any (default: serial)
  activeDeadlineSeconds: integer           # Optional: Hard deadline in seconds
  ttlSecondsAfterFinished: integer         # Optional: Cleanup after completion
  nodeSelector: {}                         # Optional: K8s node selection
  affinity:                                # Optional: Pod affinity rules
    podAntiAffinity: ...
  tolerations: []                          # Optional: K8s taints tolerance
```

### Agent Definition

Each agent in the `agents` list defines:

```yaml
agents:
  - name: string                           # Required: Unique agent name in job
    image: string                          # Required: Docker image URI
    command: []                            # Optional: Override entrypoint
    args: []                               # Optional: Override arguments
    env:                                   # Optional: Environment variables
      - name: API_KEY
        value: "secret-value"
      - name: CONFIG_PATH
        valueFrom:
          configMapKeyRef:
            name: config
            key: path
    resources:                             # Optional: Compute resources
      requests:
        cpu: "500m"
        memory: "512Mi"
      limits:
        cpu: "2"
        memory: "2Gi"
    timeout: duration                      # Optional: Agent timeout (default: job timeout)
    retryPolicy:                           # Optional: Agent-level retries
      maxRetries: 2
      backoffSeconds: 10
    dependsOn: []                          # Optional: Dependency list
      - extractor
      - transformer
    volumeMounts:                          # Optional: Storage mounts
      - name: data
        mountPath: /data
    livenessProbe:                         # Optional: Liveness check
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 10
      periodSeconds: 30
    readinessProbe:                        # Optional: Readiness check
      httpGet:
        path: /ready
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 10
```

## Job Lifecycle and States

Agent jobs follow a state machine:

```
Pending
  ├─ (resources available) → Scheduled
  └─ (timeout) → Failed
       └─ (retry enabled) → Pending

Scheduled
  ├─ (platform accepted) → Running
  └─ (scheduling failed) → Failed
       └─ (retry enabled) → Pending

Running
  ├─ (completed successfully) → Completed
  ├─ (execution failed) → Failed
  │   └─ (retry enabled, retries < max) → Pending
  │   └─ (retries exhausted) → Failed (terminal)
  └─ (user cancellation) → Cancelled

Completed → Cleanup (auto-delete if TTL set)
Failed (terminal) → Cleanup (auto-delete if TTL set)
Cancelled → Cleanup
```

Check job status:

```bash
# Get job status summary
kubectl get agentjob my-job

# Get detailed status
kubectl describe agentjob my-job

# Get full YAML with status
kubectl get agentjob my-job -o yaml

# Watch status changes
kubectl get agentjobs --watch

# Get status of specific agent within job
kubectl get agentjob my-job -o jsonpath='{.status.agents[0]}'
```

## Timeout Configuration

Timeouts control maximum execution time. Muto supports multiple timeout levels:

### Job-Level Timeout

Specifies maximum time for entire job execution (all agents):

```yaml
spec:
  timeout: 30m                # Job must complete within 30 minutes
  agents:
    - name: slow-processor
      image: myorg/processor:latest
```

When timeout is reached, the job is cancelled and marked as failed.

### Agent-Level Timeout

Specifies timeout for individual agent execution:

```yaml
spec:
  timeout: 1h                 # Job timeout
  agents:
    - name: quick-task
      image: myorg/quick:latest
      timeout: 5m             # This agent has 5 minute limit
    - name: slow-task
      image: myorg/slow:latest
      timeout: 30m            # This agent has 30 minute limit
```

Individual agent timeouts cannot exceed job timeout.

### Hard Deadline

For critical jobs, use `activeDeadlineSeconds` for a hard cutoff:

```yaml
spec:
  timeout: 1h
  activeDeadlineSeconds: 3600  # Hard cutoff at 1 hour, no grace period
  agents:
    - name: critical-job
      image: myorg/critical:latest
```

### Timeout Best Practices

- **Set realistic timeouts**: Too short causes unnecessary failures; too long delays problem detection
- **Agent-specific timeouts**: Different agents have different performance characteristics
- **Add buffer time**: Account for scheduling, image pull, and startup delays
- **Monitor timeout errors**: Track which jobs frequently timeout to identify performance issues

```bash
# Find frequently-timing-out jobs
kubectl get agentjobs -o json | jq '.items[] | select(.status.reason == "DeadlineExceeded")'

# Set alert for timeout rate > 5%
# (See monitoring guide for Prometheus queries)
```

## Retry Configuration

Muto automatically retries failed jobs with exponential backoff.

### Basic Retry

Enable retries with a simple configuration:

```yaml
spec:
  agents:
    - name: processor
      image: myorg/processor:latest
  retryPolicy:
    maxRetries: 3              # Try up to 3 times after initial failure
    backoffSeconds: 5          # Start with 5 second backoff
```

Retry behavior:

1. First attempt fails at time 0s
2. Backoff 5s, retry at 5s
3. If fails, backoff 10s (5 * 2), retry at 15s
4. If fails, backoff 20s (10 * 2), retry at 35s
5. After 3 retries, mark as failed

### Custom Backoff Strategy

Control backoff multiplier and maximum:

```yaml
spec:
  retryPolicy:
    maxRetries: 5
    backoffSeconds: 2         # Start with 2 second backoff
    backoffMultiplier: 3      # Triple the backoff each time
    maxBackoffSeconds: 120    # Don't backoff more than 2 minutes
```

Timeline for this configuration:

- Attempt 1 (fails)
- Backoff 2s, retry at 2s
- Backoff 6s (2 * 3), retry at 8s
- Backoff 18s (6 * 3), retry at 26s
- Backoff 54s (18 * 3), retry at 80s
- Backoff 120s (capped), retry at 200s

### Disable Retries

Some jobs are not idempotent and should not be retried:

```yaml
spec:
  retryPolicy:
    maxRetries: 0             # No retries
  agents:
    - name: create-resource
      image: myorg/creator:latest
```

### Agent-Level Retries

Override job-level retry policy per agent:

```yaml
spec:
  retryPolicy:
    maxRetries: 1             # Job-level: 1 retry
  agents:
    - name: critical-job
      image: myorg/critical:latest
      retryPolicy:
        maxRetries: 5         # This agent: 5 retries
    - name: one-shot-job
      image: myorg/one-shot:latest
      retryPolicy:
        maxRetries: 0         # This agent: no retries
```

### Retry Best Practices

- **Check idempotency**: Retries only work safely for idempotent operations
- **Identify transient failures**: Network timeouts, temporary resource exhaustion
- **Avoid retry for permanent failures**: Invalid input, missing files, authorization errors
- **Set reasonable limits**: Excessive retries waste resources
- **Monitor retry rate**: High retry rates indicate upstream issues

```bash
# Monitor retry rate (requires Prometheus)
kubectl logs -l app=muto-operator | grep "retry attempt"
```

## Monitoring Job Progress

### Real-Time Status

Watch job execution in real time:

```bash
# Watch status changes
kubectl get agentjobs --watch

# Follow logs while job runs
kubectl logs --all-containers agentjob/my-job --follow

# Get current status
kubectl get agentjob my-job -o jsonpath='{.status.phase}'
# Output: Running, Completed, Failed, etc.
```

### Detailed Status Information

```bash
# Full status including agents
kubectl get agentjob my-job -o json | jq '.status'

# Check individual agent status
kubectl get agentjob my-job -o json | jq '.status.agents[]'

# Get job events (for debugging failures)
kubectl describe agentjob my-job | grep -A 20 "Events:"
```

### Status Fields

Every job has status fields:

```yaml
status:
  phase: Running              # Current phase (Pending, Scheduled, Running, Completed, Failed, Cancelled)
  reason: string              # Human-readable reason for current state
  message: string             # Detailed message
  startTime: timestamp        # When job started executing
  completionTime: timestamp   # When job finished
  duration: duration          # Total execution time
  
  agents:                     # Status of each agent
    - name: processor
      phase: Running
      containerID: "docker://abc123..."
      exitCode: null          # Exit code when completed
      
  conditions:                 # Job conditions
    - type: Ready
      status: "True"
    - type: Succeeded
      status: "False"
      reason: "Still running"
```

### Querying Jobs

Use kubectl queries to find jobs by status:

```bash
# Find all running jobs
kubectl get agentjobs --field-selector=status.phase=Running

# Find failed jobs
kubectl get agentjobs --field-selector=status.phase=Failed

# Find completed jobs
kubectl get agentjobs --field-selector=status.phase=Completed

# Find jobs by label
kubectl get agentjobs -l app=data-pipeline

# Find jobs created in last hour
kubectl get agentjobs --sort-by=metadata.creationTimestamp | tail -20
```

### Metrics and Observability

Muto exports Prometheus metrics for monitoring:

```
# Job counts
muto_jobs_total{status="completed"}
muto_jobs_total{status="failed"}
muto_jobs_total{status="running"}

# Job duration
muto_job_duration_seconds_bucket{job_name="my-job"}
muto_job_duration_seconds_sum{job_name="my-job"}

# Agent counts
muto_agents_running{job_name="my-job"}
muto_agent_duration_seconds{agent_name="processor"}

# Retry metrics
muto_job_retries_total{reason="timeout"}
muto_job_retries_total{reason="error"}
```

Scrape these metrics with Prometheus. See [Monitoring & Observability](../operations/monitoring-observability.md) (coming in Phase 8) for detailed setup.

## Cancellation

Cancel a running job at any time:

```bash
# Cancel a job
kubectl delete agentjob my-job

# Cancel multiple jobs
kubectl delete agentjobs -l app=data-pipeline

# Cancel with grace period (wait for cleanup)
kubectl delete agentjob my-job --grace-period=60

# Force immediate cancellation
kubectl delete agentjob my-job --grace-period=0 --force
```

When you delete a job:
1. Job is marked as Cancelled
2. Running agents receive SIGTERM (15 second grace period)
3. If agents don't stop, they receive SIGKILL
4. Job resources are cleaned up
5. Job is removed after TTL expires (if set)

### Job Cleanup

Control automatic cleanup with TTL:

```yaml
spec:
  ttlSecondsAfterFinished: 3600  # Auto-delete 1 hour after completion
  agents:
    - name: processor
      image: myorg/processor:latest
```

Without TTL, completed jobs remain indefinitely in the cluster. Set TTL to:
- `0` for immediate cleanup
- `3600` (1 hour) for short-lived tasks
- `86400` (1 day) for longer retention
- Omit for permanent retention (manual cleanup required)

```bash
# Manual cleanup of old jobs
kubectl delete agentjobs --field-selector=status.phase=Completed \
  --older-than=7d

# Cleanup and archive logs
kubectl logs agentjob/old-job > archive/old-job.log
kubectl delete agentjob old-job
```

## Advanced Job Configuration

### Job Parallelism

Run multiple agent instances in parallel:

```yaml
spec:
  parallelism: 3              # Run up to 3 agents simultaneously
  agents:
    - name: worker
      image: myorg/worker:latest
```

Each parallel instance gets a unique ordinal (0, 1, 2) available as `JOB_INDEX` environment variable.

### Completion Policy

Control how job completes:

```yaml
spec:
  completionPolicy: any       # Job completes when any agent succeeds
  # Options: serial (all succeed), parallel (all complete), any (first succeeds)
  agents:
    - name: fast-processor
      image: myorg/fast:latest
    - name: slow-processor
      image: myorg/slow:latest
```

### Priority

Jobs with higher priority are scheduled first:

```yaml
spec:
  priority: 100               # High priority (0-100 scale)
  agents:
    - name: critical-job
      image: myorg/critical:latest
```

### Pod Affinity

Control which nodes jobs run on:

```yaml
spec:
  affinity:
    podAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
              - key: app
                operator: In
                values:
                  - database
          topologyKey: kubernetes.io/hostname
  agents:
    - name: processor
      image: myorg/processor:latest
```

## Complete Job Example

A production-ready job with all key features:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: daily-data-pipeline
  namespace: production
  labels:
    app: data-pipeline
    schedule: daily
  annotations:
    description: "Daily data processing and aggregation"
spec:
  tenant: acme-corp
  priority: 75
  timeout: 2h
  activeDeadlineSeconds: 7200
  parallelism: 1
  completionPolicy: serial
  ttlSecondsAfterFinished: 86400          # Keep for 1 day
  
  nodeSelector:
    workload-type: batch
  
  retryPolicy:
    maxRetries: 2
    backoffSeconds: 30
    backoffMultiplier: 2
    maxBackoffSeconds: 300
  
  agents:
    # Extract data from sources
    - name: extractor
      image: myorg/extractor:v2.1.0
      timeout: 30m
      resources:
        requests:
          cpu: "500m"
          memory: "512Mi"
        limits:
          cpu: "2"
          memory: "2Gi"
      env:
        - name: DATA_SOURCES
          value: "s3://bucket/data,db://postgres"
        - name: OUTPUT_PATH
          value: "/tmp/extracted"
      retryPolicy:
        maxRetries: 3
      livenessProbe:
        httpGet:
          path: /health
          port: 8080
        initialDelaySeconds: 10
        periodSeconds: 30
    
    # Transform extracted data
    - name: transformer
      image: myorg/transformer:v1.5.0
      timeout: 45m
      dependsOn:
        - extractor
      resources:
        requests:
          cpu: "1"
          memory: "1Gi"
        limits:
          cpu: "4"
          memory: "4Gi"
      env:
        - name: INPUT_PATH
          value: "/tmp/extracted"
        - name: OUTPUT_PATH
          value: "/tmp/transformed"
        - name: RULES_CONFIG
          valueFrom:
            configMapKeyRef:
              name: transform-rules
              key: rules.json
      retryPolicy:
        maxRetries: 2
    
    # Aggregate results
    - name: aggregator
      image: myorg/aggregator:v1.2.0
      timeout: 20m
      dependsOn:
        - transformer
      resources:
        requests:
          cpu: "250m"
          memory: "256Mi"
        limits:
          cpu: "1"
          memory: "1Gi"
      env:
        - name: INPUT_PATH
          value: "/tmp/transformed"
        - name: OUTPUT_PATH
          value: "s3://bucket/aggregated"
      retryPolicy:
        maxRetries: 1
```

## Next Steps

- **[Multi-Agent Patterns](./multi-agent-patterns.md)** — Build complex workflows
- **[Best Practices](./best-practices.md)** — Optimize job performance
- **[Examples](./examples/)** — See real-world usage patterns
- **[Monitoring](../operations/monitoring-observability.md)** (coming in Phase 8) — Track and debug jobs

---

**Last Updated:** 2026-09-03
