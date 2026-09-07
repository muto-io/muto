# CloudFoundry Platform Architecture

Muto's CloudFoundry adapter enables agent job execution on CloudFoundry platforms. This section describes how Muto integrates with CloudFoundry.

## Deployment Model

Muto runs as a CloudFoundry application deployed to a CF space:

```
┌───────────────────────────────────────────────────┐
│     CloudFoundry Platform                         │
│                                                   │
│  ┌────────────────────────────────────────────┐  │
│  │  muto-controller (CF Application)           │  │
│  │                                             │  │
│  │  ├─ Scheduler                               │  │
│  │  ├─ Reconcilers                             │  │
│  │  ├─ Event Watcher                           │  │
│  │  └─ Platform Adapter (CloudFoundry)         │  │
│  └────────────────────────────────────────────┘  │
│         │                                         │
│         │ Manages Tasks and Apps                 │
│         │ via CF API                             │
│         ▼                                         │
│  ┌────────────────────────────────────────────┐  │
│  │  Tenant Spaces                              │  │
│  │  ├─ tenant-a-space                          │  │
│  │  │  ├─ Agent Tasks                          │  │
│  │  │  └─ Service Bindings                     │  │
│  │  │                                          │  │
│  │  ├─ tenant-b-space                          │  │
│  │  │  ├─ Agent Tasks                          │  │
│  │  │  └─ Service Bindings                     │  │
│  │  └─ ...                                      │  │
│  └────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────┘
```

## CF Concepts Mapping

Muto concepts map to CloudFoundry abstractions:

| Muto Concept | CF Mapping | Description |
|--------------|-----------|-------------|
| Tenant | Space | Logical isolation boundary |
| AgentJob | Task group | Unit of work (can have multiple tasks) |
| Agent | Task | Individual agent instance |
| Message Bus | Service instance | Shared backing service |
| Configuration | Environment variables | Passed via CF manifest |

## Job Execution on CloudFoundry

When an AgentJob is created, the controller:

1. **Selects or creates CF space** for the tenant
2. **Creates service bindings** (message bus, secrets store)
3. **For each agent:**
   - Creates a CF Task with specified image
   - Sets resource allocation (memory, CPU)
   - Configures environment variables
   - Binds services for configuration/secrets
4. **Monitors task state** via CF API
5. **On completion** logs results to logging service

## CF Task Model

A CF Task is a short-lived process that runs to completion:

```
┌──────────────────┐
│  AgentJob        │
│                  │
│  Agent 1 ──────► Task 1 (running on CF)
│  Agent 2 ──────► Task 2 (running on CF)
│  Agent 3 ──────► Task 3 (running on CF)
│                  │
└──────────────────┘
     │
     ▼
  Tasks complete -> Job status updated
```

**Task lifecycle:**
1. **PENDING** -> CF scheduling the task
2. **RUNNING** -> Task executing
3. **SUCCEEDED** -> Task exited with code 0
4. **FAILED** -> Task exited with non-zero code
5. **CANCELLED** -> Task was terminated

## Multi-Tenancy on CloudFoundry

Tenant isolation via CF spaces and RBAC:

### Space Isolation
- Each tenant gets a dedicated CF space
- Space developers can only access their own space
- Resource quotas limit space consumption

### CF RBAC
- **Space Developer** — Can create and manage tasks within space
- **Space Manager** — Can manage space (quotas, members)
- **Org Manager** — Controls organization-level settings

### Service Instances
Each tenant space has service bindings:
- **Message Bus** (NATS or Kafka service)
- **Secrets Store** (CredHub or Vault service)
- **Logging** (Splunk, ELK, etc.)

```yaml
# CF manifest for tenant-a
applications:
- name: muto-controller
  services:
  - tenant-a-nats      # Message bus for tenant-a
  - tenant-a-credhub   # Secrets for tenant-a
  - tenant-a-logging   # Logging for tenant-a
```

## Configuration Management

### Environment Variables
Agent configuration passed via environment:

```yaml
tasks:
  - name: extractor
    image: acme/extractor:v1
    env:
      MUTO_JOB_ID: "data-pipeline"
      MUTO_AGENT_ROLE: "extract"
      MUTO_TENANT_ID: "acme-corp"
      MUTO_MESSAGE_BUS_URL: "nats://nats:4222"
      PIPELINE_SOURCE: "s3://bucket/input"
```

### Service Bindings
Credentials injected via CF service binding:

```
VCAP_SERVICES environment variable contains:
{
  "nats": [{
    "credentials": {
      "url": "nats://user:pass@nats.example.com:4222"
    }
  }],
  "credhub": [{
    "credentials": {
      "url": "https://credhub:8844",
      "auth": {...}
    }
  }]
}
```

## Scaling Model

### Manual Scaling
Create multiple tasks for an agent:

```yaml
agents:
  - name: worker
    replicas: 5  # Creates 5 parallel tasks
```

### Resource Allocation
Each task gets:
```yaml
resources:
  memory: "512M"        # Task memory
  disk: "1G"            # Task disk quota
```

### Space Quotas
Organization enforces quotas:
```yaml
quota:
  total_memory: "100G"
  total_service_instances: 20
  memory_per_instance: "10G"
```

## Monitoring and Observability

### CF Logging
Task output captured in CF logging service:

```
[2026-09-05 10:30:00] Task 123 started
[2026-09-05 10:30:15] Processing input...
[2026-09-05 10:30:30] Output written to S3
[2026-09-05 10:30:35] Task completed successfully
```

### CF Events
Task lifecycle events available via CF API:

```
task.created   -> Task created
task.running   -> Task running
task.succeeded -> Task completed successfully
task.failed    -> Task failed
```

### Metrics
Exported from muto-controller:
- `muto_task_duration_seconds` — Task execution time
- `muto_task_failures_total` — Failed task count
- `muto_space_usage_percent` — Space quota utilization
- `muto_service_binding_errors` — Service binding failures

## Advantages of CloudFoundry Platform

✅ **Lightweight** — No need to manage infrastructure  
✅ **Multi-cloud** — Run on any CF-compatible platform  
✅ **Built-in security** — Isolation via spaces and RBAC  
✅ **Service brokers** — Access to managed services (databases, queues, etc.)  
✅ **Logging & metrics** — Built-in observability  
✅ **Cost efficient** — Pay-per-use model  
✅ **Compliance** — Auditing and compliance features  

## Limitations vs Kubernetes

- No native HPA (manual scaling via replicas)
- Limited networking control (no network policies)
- Less ecosystem diversity compared to K8s
- Task-based model (no persistent deployments)

## Event Integration

Muto watches CF events via CF API:

```
┌────────────────┐
│   CF Platform  │
│                │
│  Task created  ─┐
│  Task running  ─┼─► [Event Stream]
│  Task done     ─┤
│                 ▼
└────────────────┴─────────────────┐
                                   │
                      ┌────────────┴──────────┐
                      ▼                       ▼
              Muto EventWatcher       AgentJobReconciler
              (detects task state)    (updates job status)
```

## Related Documentation

- [:octicons-book-24: **Platform Design**](./platform-design.md) — Adapter abstraction
- [:octicons-book-24: **Architecture Overview**](./overview.md) — System architecture
- [:octicons-book-24: **Deployment**](../deployment/cloudfoundry/install.md) — CF installation guide
