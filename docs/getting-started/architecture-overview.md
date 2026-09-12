# How Muto Works: Architecture Overview

Understand the high-level design of Muto.

## System Components

Muto consists of three main layers:

### 1. User/API Layer
- **Claude/LLM**: Interacts with Muto via MCP protocol
- **kubectl/Helm**: Kubernetes tools for declarative management
- **CloudFoundry CLI**: CF tools for CloudFoundry deployments

### 2. Muto Orchestration Layer
- **Operator**: Kubernetes-native controller that manages state
- **Scheduler**: Decides where and when to run jobs
- **Reconcilers**: Control loops that ensure desired state
- **Message Bus**: NATS/Kafka for inter-agent communication

### 3. Execution Layer
- **Kubernetes Platform**: Pods, CRDs, namespaces, RBAC
- **CloudFoundry Platform**: Tasks, spaces, API
- **Container Runtimes**: Docker containers executing agents

## Data Flow

### Scenario 1: User Schedules an Agent Job (Kubernetes)

```
1. User/Claude submits AgentJob CRD
   kubectl apply -f job.yaml
   
2. Kubernetes API server persists it in etcd
   
3. Muto operator watches for new AgentJobs
   EventWatcher detects new job → triggers reconciliation
   
4. AgentJobReconciler processes the job:
   - Validates job spec (tenant, resources, etc.)
   - Updates job status: Pending → Scheduled
   - Creates corresponding K8s Pod/Job
   
5. Kubernetes scheduler assigns Pod to node
   kubelet pulls container image
   
6. Agent container starts executing
   EventWatcher monitors for completion
   
7. When done, reconciler updates AgentJob status:
   Running → Completed (or Failed)
   
8. User can retrieve results:
   kubectl logs agentjob/job-name
   kubectl get agentjob job-name -o json
```

### Scenario 2: Multi-Agent Workflow with Message Coordination

```
1. User defines AgentJob with multiple agents:
   - Agent A: Extract data
   - Agent B: Transform (depends on A)
   - Agent C: Aggregate (depends on B)
   
2. Scheduler creates execution plan:
   - Start Agent A
   
3. Agent A completes:
   - Publishes results to message bus topic "workflow/a-complete"
   
4. Agent B listens to "workflow/a-complete":
   - Receives notification, starts processing
   - Publishes to "workflow/b-complete"
   
5. Agent C listens to "workflow/b-complete":
   - Receives notification, aggregates results
   - Publishes final results
   
6. JobReconciler monitors for completion
   Job status updated to Completed
```

## Platform Abstraction

Muto abstracts away platform differences using the adapter pattern:

```
                     Muto Core
                  (Platform-agnostic)
                        │
            ┌───────────┼───────────┐
            │           │           │
            ▼           ▼           ▼
        Scheduler   Messaging   Reconcilers
            │           │           │
            └───────────┼───────────┘
                        │
            ┌───────────┴───────────┐
            │ PlatformAdapter       │
            │ Interface             │
            └───────────┬───────────┘
            ┌───────────┴───────────┐
            ▼                       ▼
       K8s Adapter            CF Adapter
       ├─ CreateJob           ├─ CreateTask
       ├─ GetStatus           ├─ GetStatus
       ├─ DeleteJob           ├─ DeleteTask
       └─ WatchEvents         └─ WatchEvents
            │                       │
            ▼                       ▼
       Kubernetes              CloudFoundry
       (Pods, CRDs)            (Tasks, Spaces)
```

## Reconciliation Loop (The Heart of Muto)

Reconciliation is a continuous control loop that ensures reality matches the desired state:

```
START
  │
  ▼
┌─────────────────────────────────────────────┐
│ WATCH: Monitor for events                   │
│ - New AgentJob created                      │
│ - Job status changed                        │
│ - Pod/Task completed                        │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────┐
│ DETECT DRIFT: Compare desired vs. actual    │
│ - Desired: AgentJob spec says "running"    │
│ - Actual: No Pod/Task exists                │
│ - Drift detected? → Action needed           │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────┐
│ ACT: Take corrective actions                │
│ - Create Pod if needed                      │
│ - Update status if changed                  │
│ - Retry if failed                           │
│ - Clean up if deleted                       │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────┐
│ VERIFY: Confirm actions succeeded           │
│ - Pod created? Status updated?              │
│ - If not, will retry in next loop           │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
            SLEEP (backoff)
                   │
                   └───────────────┐
                                   │
                                   ▼
                                START (loop)
```

**Key properties:**
- **Idempotent**: Running reconciliation twice is safe
- **Resilient**: Transient failures are retried
- **Observable**: Each loop iteration is logged
- **Autonomous**: No manual intervention needed

## Multi-Tenancy Model

Each tenant has complete isolation:

```
Tenant A              Tenant B              Tenant C
│                     │                     │
├─ Namespace: a       ├─ Namespace: b       ├─ Namespace: c
├─ RBAC: a-only       ├─ RBAC: b-only       ├─ RBAC: c-only
├─ Topics: a/*        ├─ Topics: b/*        ├─ Topics: c/*
└─ Jobs: isolated     └─ Jobs: isolated     └─ Jobs: isolated

Message Bus
├─ a/workflow/topic
├─ a/notifications/topic
├─ b/workflow/topic
├─ b/notifications/topic
└─ c/workflow/topic
   (Tenant C cannot see/access a/* or b/* topics)
```

## Monitoring & Observability

The operator exposes the observability that controller-runtime provides:

```
Muto Operator
        │
        ├─ Logs (plain-text key/value, stderr)
        │  └─ e.g. "adding tenant finalizer", reconcile errors
        │
        ├─ Prometheus Metrics (:8080/metrics)
        │  ├─ controller_runtime_reconcile_total (counter)
        │  ├─ controller_runtime_reconcile_time_seconds (histogram)
        │  └─ workqueue_depth (gauge)
        │
        └─ Health Probes (:8081/healthz, :8081/readyz)
```

Muto-specific job metrics and OpenTelemetry tracing are planned ([#79](https://github.com/muto-io/muto/issues/79)).

## Extensibility

Muto is designed to be extended:

### Custom Reconcilers
Write your own reconciler for domain-specific logic to handle custom workflows or integrate with external systems. Custom reconcilers implement the reconciliation interface and hook into the control loop.

### Custom Message Bus
Plug in your own message bus implementation to use a different transport (e.g., Redis, RabbitMQ) instead of the default NATS or Kafka. Custom implementations must support publish/subscribe semantics.

### Webhooks
Validate or mutate jobs before creation:
- **Validation webhooks:** Reject invalid jobs based on custom rules
- **Mutation webhooks:** Modify jobs before creation (e.g., inject sidecar containers)

---

## Next Steps

- **[Quick Start](./quick-start.md)** — See it in action
- **[Architecture Deep Dives](../architecture/)** — Detailed component docs
- **[Deployment](../deployment/)** — Run in production
