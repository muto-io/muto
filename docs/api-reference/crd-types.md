# CRD Type Reference

Complete specification of Muto's Kubernetes Custom Resource Definitions (CRDs): AgentJob, Tenant, and ReconcilerConfig. Use this as a reference for creating and configuring agent workloads.

## Overview

Muto uses three primary CRDs to define infrastructure and workload behavior:

- **AgentJob**: Represents a request to execute one or more agents with specific parameters
- **Tenant**: Defines a logical boundary with isolated compute, messaging, and RBAC
- **ReconcilerConfig**: Configures reconciliation behavior (worker count, timeouts, polling intervals)

All CRDs are in API group `muto.io/v1alpha1`.

---

## AgentJob CRD

### Description

An AgentJob represents a declarative specification for executing one or more agents. The Muto operator reconciles the desired state by creating corresponding Kubernetes Pods and monitoring their execution.

### Metadata

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: <name>           # Required: Job identifier (unique in namespace)
  namespace: <namespace> # Required: Must match a Tenant's namespace
```

### Spec Fields

#### `spec.tenantRef` (string)
- **Required**: yes
- **Description**: Name of the Tenant that owns this AgentJob. The job must be created in the namespace specified by that Tenant.
- **Example**: `"tenant-a"`

#### `spec.trigger` (object)
- **Required**: yes
- **Description**: Specifies what initiates this job execution

##### Trigger Fields:

**`trigger.type`** (string)
- **Enum**: `event`, `cron`, `manual`
- **Required**: yes
- **Description**: How the job is triggered
  - `event`: Triggered by an incoming message on a message bus topic
  - `cron`: Triggered on a schedule (source must be a valid cron expression)
  - `manual`: Triggered explicitly via API or MCP tool
- **Example**: `"manual"`

**`trigger.source`** (string)
- **Required**: depends on type
- **Description**: Trigger source/origin
  - For `event`: Message bus topic name (e.g., `"workflow/data-ready"`)
  - For `cron`: Cron expression (e.g., `"0 2 * * *"` = 2 AM daily)
  - For `manual`: Optional, may be empty or contain context
- **Example**: `"workflow/data-ready"`

#### `spec.agents` (array of objects)
- **Required**: yes (at least one)
- **Description**: List of agent roles to spawn for this job

##### Agent Role Fields:

**`agents[].role`** (string)
- **Required**: yes
- **Description**: Functional name of the agent (e.g., `"extractor"`, `"worker"`, `"aggregator"`)
- **Used for**: Pod labeling, runner app naming on CloudFoundry, message routing
- **Example**: `"data-extractor"`

**`agents[].image`** (string)
- **Required**: yes (on Kubernetes)
- **Description**: Container image to run on Kubernetes (ignored on CloudFoundry)
- **Format**: Docker image reference (registry/name:tag)
- **Example**: `"gcr.io/myorg/data-processor:v1.2.3"`

**`agents[].command`** (string)
- **Required**: yes (on CloudFoundry)
- **Description**: Command to run on the CloudFoundry runner app (ignored on Kubernetes)
- **Format**: Shell command or script
- **Example**: `"./process.sh --input=/data/input.json"`

**`agents[].maxReplicas`** (integer)
- **Type**: int32
- **Default**: 1
- **Constraints**: minimum 1
- **Description**: Maximum number of concurrent instances for this agent role
- **Example**: `3` (allows up to 3 parallel instances)

#### `spec.messageBus` (object)
- **Required**: no
- **Description**: Configures message bus topic for agent-to-agent communication

##### Message Bus Fields:

**`messageBus.topic`** (string)
- **Required**: no (defaults to job ID)
- **Description**: Base topic name for agent communication. Automatically namespaced to `tenant.<tenantRef>.<topic>` at runtime
- **Example**: `"data-pipeline"`
- **Note**: Topic is prefixed with tenant ID to ensure isolation

#### `spec.ttlAfterCompletion` (integer)
- **Type**: int32
- **Default**: 0 (no automatic cleanup)
- **Constraints**: minimum 0
- **Unit**: seconds
- **Description**: Number of seconds to wait after the job reaches a terminal phase (Succeeded or Failed) before automatically deleting the job and all associated pods
- **Example**: `3600` (delete after 1 hour)

### Status Fields

#### `status.phase` (string)
- **Enum**: `Pending`, `Running`, `Succeeded`, `Failed`, `Terminating`
- **Description**: Current lifecycle phase of the AgentJob
  - `Pending`: Job created, agents not yet spawned
  - `Running`: One or more agents actively executing
  - `Succeeded`: All agents completed successfully
  - `Failed`: One or more agents failed
  - `Terminating`: Job and pods being cleaned up
- **Read-only**: Yes (set by operator)

#### `status.activeAgents` (integer)
- **Type**: int32
- **Default**: 0
- **Description**: Number of currently running agent instances across all roles
- **Read-only**: Yes (updated by operator)

#### `status.startedAt` (timestamp)
- **Format**: RFC3339 (e.g., `"2026-09-03T10:30:45Z"`)
- **Description**: Timestamp when the job transitioned to Running phase
- **Read-only**: Yes
- **Nullable**: Yes

#### `status.completedAt` (timestamp)
- **Format**: RFC3339
- **Description**: Timestamp when the job reached a terminal phase (Succeeded or Failed)
- **Read-only**: Yes
- **Nullable**: Yes

### Example: Complete AgentJob

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: data-pipeline-job
  namespace: tenant-a
spec:
  tenantRef: tenant-a
  trigger:
    type: event
    source: workflow/data-ready
  agents:
    - role: extractor
      image: gcr.io/myorg/extractor:v1.0.0
      maxReplicas: 1
    - role: processor
      image: gcr.io/myorg/processor:v1.0.0
      maxReplicas: 2
    - role: aggregator
      image: gcr.io/myorg/aggregator:v1.0.0
      maxReplicas: 1
  messageBus:
    topic: data-pipeline
  ttlAfterCompletion: 3600
```

---

## Tenant CRD

### Description

A Tenant represents a logical boundary for resource isolation in multi-tenant Muto deployments. Each tenant gets its own Kubernetes namespace, RBAC configuration, and (optionally) dedicated message bus instance.

### Metadata

```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: <name>  # Required: Tenant identifier (cluster-scoped)
```

### Spec Fields

#### `spec.namespace` (string)
- **Required**: yes
- **Description**: Kubernetes namespace where this tenant's agent pods and jobs run
- **Constraints**: Must be a valid Kubernetes namespace name (lowercase alphanumerics and hyphens)
- **Behavior**: TenantReconciler creates this namespace if it doesn't exist
- **Example**: `"tenant-a"`

#### `spec.isolationTier` (string)
- **Enum**: `shared`, `dedicated`
- **Required**: yes
- **Description**: Controls tenant isolation strictness
  - `shared`: Tenant shares the controller's message bus infrastructure (NATS/Kafka instance in muto-system namespace)
  - `dedicated`: Tenant gets its own message bus instance (StatefulSet in the tenant's namespace)
- **Default**: `shared`
- **Performance**: Shared is lower latency; dedicated is higher isolation
- **Example**: `"dedicated"`

#### `spec.messageBus` (object)
- **Required**: yes
- **Description**: Message bus configuration for this tenant

##### Message Bus Fields:

**`messageBus.type`** (string)
- **Enum**: `nats`, `kafka`, `a2a`
- **Required**: yes
- **Description**: Message bus implementation to use
  - `nats`: NATS JetStream (lightweight, low-latency, suitable for simple agent tasks)
  - `kafka`: Apache Kafka (high-throughput, durable, suitable for complex pipelines)
  - `a2a`: Agent-to-Agent (direct messaging, lowest latency, no persistence)
- **Example**: `"nats"`

**`messageBus.dedicated`** (boolean)
- **Default**: false
- **Required**: no
- **Description**: When true, provisions a per-tenant message bus instance in the tenant's namespace
- **Constraint**: Only applicable when parent Tenant has `isolationTier: dedicated`
- **Effect**: 
  - `true`: Tenant gets its own NATS StatefulSet or Kafka broker
  - `false`: Tenant uses shared cluster-level instance
- **Example**: `true`

### Status Fields

#### `status.ready` (boolean)
- **Default**: false
- **Description**: True once the tenant namespace, RBAC, and message bus (if dedicated) are fully provisioned and operational
- **Read-only**: Yes (set by TenantReconciler)

### Example: Shared Tenant (NATS)

```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: tenant-a
spec:
  namespace: tenant-a
  isolationTier: shared
  messageBus:
    type: nats
    dedicated: false
```

### Example: Dedicated Tenant (Kafka)

```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: tenant-b
spec:
  namespace: tenant-b
  isolationTier: dedicated
  messageBus:
    type: kafka
    dedicated: true
```

---

## ReconcilerConfig CRD

### Description

ReconcilerConfig controls the behavior of Muto's reconciliation loops. It allows fine-tuning of worker concurrency, retry timing, polling intervals, and timeouts across different reconciler types.

### Metadata

```yaml
apiVersion: muto.io/v1alpha1
kind: ReconcilerConfig
metadata:
  name: <name>
  namespace: muto-system  # Must be in muto-system namespace
```

### Spec Fields

#### `spec.reconcilerName` (string)
- **Required**: yes
- **Description**: Name of the reconciler to configure
- **Valid values**: `"agentjob"`, `"tenant"`, `"fleet"`, `"eventwatch"`
- **Example**: `"agentjob"`

#### `spec.workers` (integer)
- **Type**: int32
- **Default**: 5 (varies by reconciler)
- **Constraints**: minimum 1, maximum 256
- **Description**: Number of concurrent worker goroutines for this reconciler
- **Effect**: 
  - Higher = more parallelism, higher CPU/memory
  - Lower = less parallelism, slower reconciliation
- **Example**: `10`

#### `spec.pollIntervalSeconds` (integer)
- **Type**: int32
- **Default**: 30 (varies by reconciler)
- **Constraints**: minimum 1, maximum 3600
- **Unit**: seconds
- **Description**: How frequently the reconciler polls for changes when no events are available
- **Effect**:
  - Lower = faster drift detection, higher CPU
  - Higher = slower drift detection, lower CPU
- **Example**: `15`

#### `spec.maxRetries` (integer)
- **Type**: int32
- **Default**: 3
- **Constraints**: minimum 0
- **Description**: Maximum number of times a single reconciliation attempt can be retried
- **Example**: `5`

#### `spec.backoffExponent` (float32)
- **Default**: 2.0
- **Constraints**: 1.0 to 10.0
- **Description**: Exponential backoff multiplier for retry delays
- **Formula**: delay = baseDelay * (backoffExponent ^ retryCount)
- **Example**: `2.0` (doubling backoff: 1s, 2s, 4s, 8s, ...)

#### `spec.maxBackoffSeconds` (integer)
- **Type**: int32
- **Default**: 300
- **Constraints**: minimum 1
- **Unit**: seconds
- **Description**: Maximum delay between retries (caps exponential backoff)
- **Example**: `300` (5 minutes max)

#### `spec.operationTimeoutSeconds` (integer)
- **Type**: int32
- **Default**: 60
- **Constraints**: minimum 5
- **Unit**: seconds
- **Description**: Maximum time allowed for a single reconciliation operation to complete
- **Example**: `120`

#### `spec.watchTimeout` (integer)
- **Type**: int32
- **Default**: 600
- **Constraints**: minimum 10
- **Unit**: seconds
- **Description**: Timeout for watch connections to Kubernetes API server
- **Note**: Kubernetes recommends setting to (300, 600) range
- **Example**: `600`

### Status Fields

#### `status.applied` (boolean)
- **Default**: false
- **Description**: True if configuration has been successfully applied to the running reconciler
- **Read-only**: Yes

#### `status.lastUpdateTime` (timestamp)
- **Format**: RFC3339
- **Description**: When the configuration was last successfully applied
- **Read-only**: Yes
- **Nullable**: Yes

### Example: AgentJob Reconciler Config

```yaml
apiVersion: muto.io/v1alpha1
kind: ReconcilerConfig
metadata:
  name: agentjob-high-throughput
  namespace: muto-system
spec:
  reconcilerName: agentjob
  workers: 20
  pollIntervalSeconds: 10
  maxRetries: 5
  backoffExponent: 2.0
  maxBackoffSeconds: 300
  operationTimeoutSeconds: 120
  watchTimeout: 600
```

### Example: Tenant Reconciler Config (Conservative)

```yaml
apiVersion: muto.io/v1alpha1
kind: ReconcilerConfig
metadata:
  name: tenant-conservative
  namespace: muto-system
spec:
  reconcilerName: tenant
  workers: 2
  pollIntervalSeconds: 60
  maxRetries: 3
  backoffExponent: 1.5
  maxBackoffSeconds: 120
  operationTimeoutSeconds: 180
  watchTimeout: 300
```

---

## Type Constraints and Validation

### Field Validation Rules

#### AgentJob

| Field | Validation |
|-------|-----------|
| `metadata.name` | RFC 1123 (alphanumeric, hyphens, max 253 chars) |
| `metadata.namespace` | Must exist and have matching Tenant |
| `spec.tenantRef` | Must reference existing Tenant resource |
| `spec.trigger.type` | Must be one of: event, cron, manual |
| `spec.trigger.source` | If cron: must be valid cron expression |
| `spec.agents[]` | At least one agent required |
| `spec.agents[].role` | Non-empty string |
| `spec.agents[].image` | Valid Docker image reference (required on K8s) |
| `spec.agents[].maxReplicas` | >= 1 |
| `spec.ttlAfterCompletion` | >= 0 |

#### Tenant

| Field | Validation |
|-------|-----------|
| `metadata.name` | RFC 1123 (max 253 chars) |
| `spec.namespace` | Valid K8s namespace name, must match namespace rules |
| `spec.isolationTier` | One of: shared, dedicated |
| `spec.messageBus.type` | One of: nats, kafka, a2a |
| `spec.messageBus.dedicated` | Boolean |

#### ReconcilerConfig

| Field | Validation |
|-------|-----------|
| `metadata.namespace` | Must be muto-system |
| `spec.reconcilerName` | One of: agentjob, tenant, fleet, eventwatch |
| `spec.workers` | 1-256 |
| `spec.pollIntervalSeconds` | 1-3600 |
| `spec.maxRetries` | >= 0 |
| `spec.backoffExponent` | 1.0-10.0 |
| `spec.maxBackoffSeconds` | >= 1 |
| `spec.operationTimeoutSeconds` | >= 5 |
| `spec.watchTimeout` | >= 10 |

---

## Common Patterns

### Pattern 1: Simple Single-Agent Job

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: hello-world
  namespace: default
spec:
  tenantRef: default
  trigger:
    type: manual
  agents:
    - role: main
      image: alpine:latest
```

### Pattern 2: Multi-Stage Pipeline

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: etl-pipeline
  namespace: tenant-a
spec:
  tenantRef: tenant-a
  trigger:
    type: cron
    source: "0 2 * * *"  # Daily at 2 AM
  agents:
    - role: extract
      image: gcr.io/myorg/extract:v1
      maxReplicas: 1
    - role: transform
      image: gcr.io/myorg/transform:v1
      maxReplicas: 3
    - role: load
      image: gcr.io/myorg/load:v1
      maxReplicas: 1
  messageBus:
    topic: etl-pipeline
  ttlAfterCompletion: 86400  # Delete after 24 hours
```

### Pattern 3: High-Availability Tenant Setup

```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: production
spec:
  namespace: production
  isolationTier: dedicated
  messageBus:
    type: kafka
    dedicated: true
---
apiVersion: muto.io/v1alpha1
kind: ReconcilerConfig
metadata:
  name: prod-agentjob-config
  namespace: muto-system
spec:
  reconcilerName: agentjob
  workers: 50
  pollIntervalSeconds: 5
  maxRetries: 5
  backoffExponent: 2.0
  maxBackoffSeconds: 600
  operationTimeoutSeconds: 180
  watchTimeout: 600
```

---

## Subresources

### AgentJob Status Subresource

The AgentJob CRD has a status subresource, allowing separate permissions for updating status:

```bash
# Update spec
kubectl patch agentjob job-name --type merge -p '{"spec":{"ttlAfterCompletion":3600}}'

# Update status (typically done by operator)
kubectl patch agentjob job-name/status --type merge -p '{"status":{"phase":"Running"}}'
```

### Tenant Status Subresource

Similar to AgentJob, Tenant supports status updates:

```bash
kubectl patch tenant tenant-a/status --type merge -p '{"status":{"ready":true}}'
```

---

## API Versions and Deprecation

Current version: **v1alpha1**

- This is a pre-release API version
- Breaking changes may occur between v1alpha1 releases
- Migration path to v1 will be provided before v1 release

---

## Related Documentation

- **[Usage: Scheduling Agent Jobs](../usage/scheduling-agent-jobs.md)** — Practical examples of job creation
- **[Usage: Multi-Agent Patterns](../usage/multi-agent-patterns.md)** — Orchestration patterns using AgentJob
- **[Architecture: Agent Lifecycle](../architecture/agent-lifecycle.md)** — State machine details
- **[Configuration: Reconciler Setup](../configuration/reconciler-config.md)** — Reconciler tuning guide

---

**Last Updated:** 2026-09-03  
**API Version:** v1alpha1
