# Architecture Overview

Muto is a multi-platform agent orchestration system that manages the lifecycle of distributed agent jobs across Kubernetes and CloudFoundry. This section describes the core architectural components and how they interact.

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Muto Control Plane                       │
│  (Runs on Kubernetes or CloudFoundry cluster)               │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │  Scheduler   │  │ Reconcilers  │  │ Event Watch  │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│         │                 │                   │             │
│         └─────────────────┴───────────────────┘             │
│                           │                                 │
│         ┌─────────────────┴─────────────────┐               │
│         ▼                                   ▼               │
│  ┌──────────────────────┐       ┌──────────────────────┐   │
│  │  Platform Adapters   │       │   Message Bus API    │   │
│  │  (K8s, CloudFoundry) │       │ (NATS, Kafka, etc)   │   │
│  └──────────────────────┘       └──────────────────────┘   │
│         │                                   │               │
└─────────┼───────────────────────────────────┼───────────────┘
          │                                   │
    ┌─────┴──────┐                   ┌───────┴────────┐
    ▼            ▼                   ▼                ▼
┌────────┐ ┌──────────┐      ┌─────────┐    ┌──────────────┐
│  K8s   │ │CloudFound│      │  NATS   │    │  Kafka       │
│Cluster │ │ry        │      │         │    │              │
└────────┘ └──────────┘      └─────────┘    └──────────────┘
    │            │                │              │
    ▼            ▼                ▼              ▼
┌────────────────────────────────────────────────────────────┐
│                      Agent Jobs                            │
│  (Running distributed workloads coordinated via messages)  │
└────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Scheduler
Determines which platform (Kubernetes or CloudFoundry) should execute each agent job based on:
- Job requirements and constraints
- Platform resource availability
- Multi-tenancy policies
- Affinity and anti-affinity rules

### 2. Reconcilers
Control loops that drive cluster state toward the desired state:
- **TenantReconciler**: Creates/manages the per-tenant namespace and optional A2A gateway
- **AgentJobReconciler**: Handles job lifecycle (spawning agent pods/tasks, monitoring, requeueing)
- **AgentFleetReconciler**: Coordinates groups of jobs

There is no `EventWatcherReconciler`. `platform/k8s/eventwatcher.go` exists as a message-bus topic subscriber, but it's never instantiated by `muto-operator` or `muto-mcp` today.

See [:octicons-book-24: **Reconcilers**](./reconcilers.md) for detailed reconciliation patterns.

### 3. Platform Adapters
Abstract platform-specific details behind a unified interface:
- **Kubernetes Adapter**: Maps to Kubernetes Jobs, Deployments, and StatefulSets
- **CloudFoundry Adapter**: Maps to CF Tasks and Applications
- Each adapter implements the PlatformAdapter interface

### 4. Message Bus
Enables asynchronous inter-agent communication:
- Supports NATS, Kafka, or custom implementations
- Topic-based pub/sub with tenant isolation
- Ordered delivery and exactly-once semantics

See [:octicons-book-24: **Messaging Architecture**](./messaging.md) for protocol details.

## Key Architectural Principles

### Platform Agnosticism
The core logic (scheduler, reconcilers, job lifecycle) is completely independent of the underlying platform. Platform-specific concerns are isolated in adapter implementations.

### Multi-Tenancy
- Tenant isolation is enforced at multiple levels (namespaces, RBAC, message bus topics, resource quotas)
- Each tenant has dedicated resources and cannot access other tenants' data
- Billing and metering are tenant-aware

### Asynchronous Coordination
- Agents communicate via messages, not direct RPC calls
- Decouples job components and enables resilience
- Supports complex multi-stage workflows

### Declarative Management
- Users declare desired state via CRDs (for Kubernetes) or equivalent APIs
- System drives toward declared state through reconciliation loops
- Changes are incremental and safe (no state corruption)

## Tenant Isolation Model

```
┌─────────────────────────────────────┐
│         Muto Cluster                │
│                                     │
│  ┌──────────────────────────────┐  │
│  │  Tenant A (isolationTier: dedicated) │
│  │  ├─ Namespace: muto-tenant-a │  │
│  │  └─ Jobs: pipeline-a, ...    │  │
│  └──────────────────────────────┘  │
│                                     │
│  ┌──────────────────────────────┐  │
│  │  Tenant B (isolationTier: shared) │
│  │  ├─ Namespace: muto-tenant-b │  │
│  │  └─ Jobs: analytics-b, ...   │  │
│  └──────────────────────────────┘  │
│                                     │
└─────────────────────────────────────┘
```

What `isolationTier` actually does today: on CF, it drives real org/space isolation (dedicated = per-tenant org, shared = per-tenant space in one shared org - see [Multi-Tenant Setup](../configuration/multi-tenant-setup.md)). On K8s, the reconciler just creates the namespace from `spec.namespace` regardless of tier - there's no separate per-tenant message bus or RBAC provisioning on either platform yet.

## Data Flow: Job Execution

1. **User creates AgentJob** (via kubectl or API)
2. **AgentJobReconciler triggers platform adapter** to spawn agent pods/tasks
3. **Platform (K8s/CF) runs agents**
4. **AgentJobReconciler polls status** on a fixed requeue interval (not event-driven - see [Reconciler Configuration](../configuration/reconciler-config.md) for the real intervals)
5. **Job reaches a terminal phase**, status recorded

Whether and how agents themselves talk to each other over a message bus is up to the agent's own code - Muto injects `MUTO_TENANT`/`MUTO_ROLE`/`MUTO_JOB_ID`/`MUTO_MESSAGEBUS_TOPIC` into each container, but doesn't run or wire up NATS/Kafka itself today (see [Message Bus Setup](../configuration/message-bus-setup.md)).

## Security Model

What's real today:
- Kubernetes RBAC for the operator's own cluster access, via static Helm-templated manifests
- CF org/space isolation, or K8s namespace-per-tenant, depending on platform
- Credentials (CF, A2A tokens) are plain env vars/Secrets - no TLS, mTLS, or secret-manager integration is built in

See [:octicons-book-24: **Security Model**](./security-model.md) for the full picture, including what's deliberately not implemented yet.

## Related Documentation

- [:octicons-book-24: **Agent Lifecycle**](./agent-lifecycle.md) - Job state machine and transitions
- [:octicons-book-24: **Reconcilers**](./reconcilers.md) - Control loop architecture
- [:octicons-book-24: **Messaging**](./messaging.md) - Inter-agent communication
- [:octicons-book-24: **Platform Design**](./platform-design.md) - Adapter abstraction
- [:octicons-book-24: **Security Model**](./security-model.md) - Multi-tenancy and isolation
