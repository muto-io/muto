# Core Concepts

Understand the fundamental concepts that make up Muto.

## Agent

An **Agent** is a containerized workload (application or microservice) that runs independently and can coordinate with other agents.

- **Stateless execution**: Each agent execution is independent
- **Containerized**: Runs as a Docker container on K8s or CF
- **Identity**: Each agent has a type/name for routing and coordination
- **Communication**: Agents communicate via message bus, not direct calls
- **Scalable**: Multiple instances can run in parallel

**Example:** A data processing agent that transforms input data and publishes results to a message bus.

## Agent Job

An **Agent Job** (or simply "Job") is a request to execute one or more agents with specific parameters.

- **Declarative**: Define what you want using a CRD/API, Muto handles how
- **Idempotent**: Running the same job twice produces the same result
- **Traceable**: Each job has a unique ID, version, and audit trail
- **Configurable**: Specify timeouts, retries, resource limits, environment variables
- **Monitorable**: Track status, logs, and metrics for each job

**Example:** "Run Agent A with input file X, Agent B processes its output, Agent C aggregates results."

## Job States

Every agent job follows a state machine:

```
Pending -> Scheduled -> Running -> Completed
  ↓                      ↓
Cancelled            Failed (↻ Retry)
```

- **Pending**: Job created, waiting for scheduler to process
- **Scheduled**: Scheduler accepted, resources allocated
- **Running**: Agent(s) actively executing
- **Completed**: Successfully finished
- **Failed**: Execution failed (may retry)
- **Cancelled**: User or system cancelled the job

## Tenant

A **Tenant** is a logical boundary for multi-tenant environments. Each tenant:

- **Isolated compute**: Runs in separate namespace (K8s) or space (CF)
- **Isolated messaging**: Message topics prefixed with tenant ID (e.g., `tenant-a/*`)
- **Isolated RBAC**: Only tenant's users can manage tenant's jobs
- **Isolated storage**: Separate etcd keys, separate message queue partitions
- **No cross-tenant visibility**: One tenant cannot see another's data or jobs

**Example:** In a SaaS platform, each customer is a tenant with guaranteed isolation.

## Platform

A **Platform** is the underlying execution environment.

- **Kubernetes**: Using K8s CRDs, namespaces, and control loops
- **CloudFoundry**: Using CF tasks, spaces, and API
- **Adapter pattern**: Muto core logic is platform-agnostic; adapters implement platform-specific details

Muto dynamically routes jobs to the appropriate platform adapter based on configuration.

## Reconciler

A **Reconciler** is a control loop that watches for desired state and makes reality match it.

- **Watch**: Monitor K8s/CF for events and resource state
- **Detect Drift**: Compare desired vs. actual state
- **Reconcile**: Take actions to reach desired state
- **Retry**: Handle transient failures with backoff

**Built-in Reconcilers:**
- **TenantReconciler**: Creates/manages tenant namespaces/spaces
- **AgentJobReconciler**: Creates/manages agent job executions
- **AgentFleetReconciler**: Manages groups of related jobs
- **EventWatcher**: Monitors K8s/CF events and triggers reconciliation

**Extensibility**: You can write custom reconcilers for domain-specific logic.

## Message Bus

A **Message Bus** enables asynchronous inter-agent communication.

- **Publish/Subscribe**: Agents publish messages to topics, others subscribe
- **Topic-based routing**: Messages routed by topic name (e.g., `tenant-a/data-pipeline/output`)
- **Persistent**: Messages retained for configurable period (configurable per implementation)
- **Tenant-scoped**: Topics are prefixed with tenant ID for isolation
- **Implementations**: NATS (simple), Kafka (enterprise), custom implementations

**Use Cases:**
- Agent A publishes results; Agent B subscribes and processes
- Multiple agents coordinate via event stream
- Job status notifications published to monitoring topics

**Example Message:**
```json
{
  "tenant": "tenant-a",
  "topic": "data-pipeline/transform-complete",
  "message": {
    "jobId": "job-123",
    "status": "completed",
    "outputPath": "s3://bucket/results"
  }
}
```

## Control Loop

Muto's core pattern: continuous reconciliation.

```
┌─────────────────────────────────────────────┐
│ Watch Events (K8s/CF resources change)      │
└──────────────────┬──────────────────────────┘
                   ▼
┌─────────────────────────────────────────────┐
│ Detect Drift (compare desired vs. actual)   │
└──────────────────┬──────────────────────────┘
                   ▼
┌─────────────────────────────────────────────┐
│ Reconcile (take corrective actions)         │
└──────────────────┬──────────────────────────┘
                   ▼
┌─────────────────────────────────────────────┐
│ Verify (confirm desired state reached)      │
└──────────────────┬──────────────────────────┘
                   ▼
            ┌──────────────┐
            │ Loop back    │
            │ to Watch     │
            └──────────────┘
```

This pattern ensures:
- **Resilience**: If a step fails, the next loop iteration will retry
- **Eventual consistency**: System converges to desired state even after failures
- **Observable**: Each loop iteration is logged and can be monitored

## Declarative vs. Imperative

Muto uses **declarative** configuration: you describe the desired state, and Muto ensures it's achieved.

**Declarative (Muto way):**
```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: data-pipeline
spec:
  agents:
    - name: extractor
      image: myorg/extractor:v1
      config:
        source: s3://bucket/data
    - name: processor
      image: myorg/processor:v1
      dependsOn: [extractor]
```

You declare "I want Agent extractor and processor running," Muto handles scheduling, dependencies, retries.

**Imperative (traditional way):**
```bash
# Run extractor, wait for it
kubectl run extractor --image=myorg/extractor:v1
# Monitor logs manually
kubectl logs extractor
# Run processor after extractor
kubectl run processor --image=myorg/processor:v1 --requires extractor
```

You manually orchestrate each step. Muto automates this.

## Summary Diagram

```
User/LLM
   │
   ├─ Define AgentJob (declarative)
   │
   ▼
Muto Scheduler
   │
   ├─ Select Tenant
   ├─ Select Platform (K8s or CF)
   ├─ Allocate Resources
   │
   ▼
Reconcilers (control loops)
   │
   ├─ Watch Platform Events
   ├─ Detect Drift
   ├─ Apply Corrections
   │
   ▼
Agents Execute
   │
   └─ Publish Results to Message Bus
        │
        └─ Other Agents Subscribe
```

---

## Next Steps

- **[Quick Start](../getting-started/quick-start.md)** — See concepts in action
- **Architecture Overview** — Deeper technical details (coming in Phase 2)
