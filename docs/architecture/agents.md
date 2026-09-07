# Agent Model

An agent is a containerized workload that executes within a Muto job. This section describes how agents are defined, executed, and coordinated.

## What is an Agent?

An agent is:
- A **containerized application** (Docker image, OCI container)
- **Stateless** — No persistent local state between executions
- **Coordinated** — Communicates with other agents via message bus
- **Observable** — Provides logs and metrics for monitoring
- **Resource-bounded** — Runs with defined CPU/memory limits

An agent executes a specific task:
- Extract data from a source
- Transform data in a pipeline
- Load results to a destination
- Coordinate with other agents

## Agent Definition

Agents are declared in AgentJob specifications:

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: data-pipeline
spec:
  tenantRef: acme-corp
  agents:
    - name: extractor        # Agent name (unique within job)
      role: extract          # Agent role (used in topic naming)
      image: acme/extract:v1 # Container image
      replicas: 1            # Number of instances
      resources:
        requests:
          cpu: "500m"
          memory: "512Mi"
        limits:
          cpu: "2"
          memory: "2Gi"
      env:
        - name: SOURCE_URL
          value: "s3://bucket/input"
        - name: MESSAGE_BUS_URL
          valueFrom:
            secretKeyRef:
              name: message-bus
              key: url
    
    - name: transformer      # Second agent
      role: transform
      image: acme/transform:v1
      replicas: 3            # Run 3 parallel instances
      resources:
        requests:
          cpu: "1"
          memory: "1Gi"
```

## Agent Lifecycle

### States

An agent progresses through lifecycle states:

```
┌─────────┐
│ Created │ (Agent object created)
└────┬────┘
     │
     ▼
┌─────────┐
│ Pending │ (Waiting for platform resources)
└────┬────┘
     │
     ▼
┌─────────┐
│ Running │ (Executing on platform)
└────┬────┘
     │
     ├──────────────────┐
     │                  │
     ▼                  ▼
┌──────────┐      ┌────────┐
│ Succeeded│      │ Failed │ (Exit code non-zero)
└──────────┘      └────┬───┘
                       │
                   (May retry)
```

### State Transitions

**Created -> Pending**
- Platform allocated resources
- Container image being pulled

**Pending -> Running**
- Container started
- Process running

**Running -> Succeeded**
- Process exited with code 0
- Agent completed task successfully

**Running -> Failed**
- Process exited with non-zero code
- Agent encountered error (may retry per policy)

**Failed -> Pending** (if retry allowed)
- Retry policy permits another attempt
- RetryCount incremented
- Agent re-scheduled

## Agent Communication

Agents communicate via the message bus using topic-based pub/sub:

```
┌────────────────────────────────────────────────┐
│  Agent Job: data-pipeline                      │
│                                                │
│  Extractor (running)                           │
│  │                                             │
│  └─► Topic: tenant-a/data-pipeline/extract/complete
│       {"status": "done", "output": "s3://..."}
│                │                               │
│                ├─────────────────┐             │
│                │                 │             │
│                ▼                 ▼             │
│         Transformer (subscribed)  Loader      │
│         │ (starts processing)    (waiting)    │
│         │                                     │
│         └─► Topic: tenant-a/data-pipeline/transform/complete
│              {"status": "done", "rows": 1500}
│                     │                         │
│                     ▼                         │
│              Loader (starts)                  │
│              │                                │
│              └─► Topic: tenant-a/data-pipeline/load/complete
│                   {"status": "done"}
│                          │                    │
│                          ▼                    │
│                   Job completed              │
└────────────────────────────────────────────────┘
```

### Topic Naming

Agents publish to topics following this pattern:

```
tenant.<tenantID>/<workflow>/<agent-role>/<event>
```

**Example:**
- `tenant.acme-corp/data-pipeline/extract/complete`
- `tenant.acme-corp/data-pipeline/transform/complete`
- `tenant.acme-corp/data-pipeline/load/complete`

### Message Format

Agents publish/receive standardized messages:

```json
{
  "id": "msg-unique-123",
  "timestamp": "2026-09-05T10:30:45.123Z",
  "tenantID": "acme-corp",
  "jobID": "data-pipeline",
  "sourceAgent": "extractor-pod-1",
  "sourceRole": "extract",
  "type": "JobComplete",
  "version": "1.0",
  "correlationID": "corr-abc123",
  "headers": {
    "priority": "high"
  },
  "payload": {
    "status": "succeeded",
    "itemsProcessed": 1500,
    "outputPath": "s3://bucket/extract-output.json"
  }
}
```

## Resource Management

### Request vs Limit

Each agent specifies resource constraints:

- **Requests** — Resources guaranteed by scheduler
  - Platform reserves these resources
  - Agent scheduled only if resources available
  - Use for capacity planning

- **Limits** — Maximum resources agent can consume
  - Platform kills/throttles if exceeded
  - Prevents runaway processes
  - On Kubernetes: OOMKilled if exceeds memory
  - On CF: Task killed if exceeds memory

**Example:**
```yaml
resources:
  requests:
    cpu: "500m"        # 0.5 CPU guaranteed
    memory: "512Mi"    # 512MB guaranteed
  limits:
    cpu: "2"           # Max 2 CPUs
    memory: "2Gi"      # Max 2GB
```

### Storage

Agents should be **stateless** — store output externally:

```yaml
agents:
  - name: processor
    image: acme/processor:v1
    env:
      - name: OUTPUT_BUCKET
        value: "s3://acme-corp/results"
      - name: OUTPUT_PATH
        value: "/tmp/output"  # Local ephemeral storage
```

Muto supports:
- **Object storage** (S3, GCS) — Preferred for sharing between agents
- **Databases** — For structured results
- **Message bus** — For inter-agent coordination
- **Ephemeral local storage** — For temporary scratch space

## Reliability Patterns

### Idempotency

Agents should be **idempotent** — safe to run multiple times:

```yaml
# ✅ Good: Idempotent
- Agent checks if output already exists
- If exists, returns existing output (doesn't reprocess)
- Safe to retry without duplication

# ❌ Bad: Non-idempotent
- Agent always processes input, increments counter
- Multiple runs cause incorrect results
- Retry causes duplicate processing
```

### Retry Policy

Jobs can define retry behavior:

```yaml
spec:
  agents:
    - name: api-caller
      retryPolicy:
        maxRetries: 3          # Max retry attempts
        backoffSeconds: 5      # Initial backoff
        backoffMultiplier: 2   # Exponential: 5s, 10s, 20s
        retryableExitCodes:    # Which codes to retry
          - 1                  # Transient error
          - 255                # Timeout
```

### Timeout

Agents have maximum execution time:

```yaml
spec:
  timeout: 30m        # Job timeout (all agents must complete)
  agents:
    - name: processor
      timeout: 20m    # Agent timeout (individual)
```

On timeout:
- Agent process killed
- Job transitions to Failed
- May retry if policy allows

## Agent Coordination Patterns

### Sequential Execution

```yaml
Job: extract → transform → load
     │           │           │
     ▼           ▼           ▼
  Agent A    Agent B     Agent C
  
  Messages flow sequentially
  A publishes → B receives & starts → B publishes → C receives & starts
```

### Parallel Execution

```yaml
Job: process (3 replicas in parallel)
     │
     ├──► Agent 1 ─┐
     ├──► Agent 2  ├──► Aggregator
     └──► Agent 3 ─┘
  
  All process in parallel, then aggregate results
```

### Fan-out/Fan-in

```yaml
Coordinator publishes work items
        │
        ├──► Worker 1 ─┐
        ├──► Worker 2  ├──► Aggregator
        └──► Worker 3 ─┘
  
  Coordinator sends tasks to multiple workers
  Workers process in parallel
  Aggregator collects results
```

## Best Practices

### 1. Keep Agents Focused
- One responsibility per agent
- Avoid monolithic agents that do everything
- Compose jobs from simple agents

### 2. Design for Failure
- Expect network failures
- Implement retry logic
- Make operations idempotent
- Use timeouts to detect hangs

### 3. Log Appropriately
- Write to stdout/stderr (platform captures)
- Structure logs for parsing
- Include correlation IDs for tracing
- Log at appropriate levels (INFO, WARN, ERROR)

### 4. Resource Right-Sizing
- Profile workload to determine needs
- Set requests to typical usage
- Set limits to 4x requests (allow bursting)
- Leave headroom for variance

### 5. Handle Signals
- Listen for SIGTERM (graceful shutdown)
- Flush pending work before exit
- Clean up resources (close files, DB connections)
- Exit with code 0 for success

### 6. Message Design
- Keep messages small (< 1MB)
- Use references (S3 paths) instead of embedding data
- Include correlation IDs for tracing
- Version message formats

## Running Local Agents

For development/testing:

```bash
# Run agent locally with environment config
export MUTO_JOB_ID=test-job
export MUTO_AGENT_ROLE=processor
export MUTO_MESSAGE_BUS_URL=nats://localhost:4222
docker run -e MUTO_JOB_ID -e MUTO_AGENT_ROLE -e MUTO_MESSAGE_BUS_URL \
  acme/processor:v1
```

## Related Documentation

- [:octicons-book-24: **Agent Lifecycle**](./agent-lifecycle.md) — Job state machine
- [:octicons-book-24: **Messaging Architecture**](./messaging.md) — Inter-agent communication
- [:octicons-book-24: **Best Practices**](../usage/best-practices.md) — Production guidance
