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
- **TenantReconciler**: Manages tenant isolation and resources
- **AgentJobReconciler**: Handles job lifecycle (scheduling, monitoring, retry)
- **AgentFleetReconciler**: Coordinates groups of jobs
- **EventWatcherReconciler**: Responds to platform events

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
│  │  Tenant A (isolation: dedicated)  │
│  │  ├─ Namespace: muto-tenant-a │  │
│  │  ├─ Message Bus: nats-a      │  │
│  │  ├─ RBAC: sa-tenant-a        │  │
│  │  └─ Jobs: pipeline-a, ...    │  │
│  └──────────────────────────────┘  │
│                                     │
│  ┌──────────────────────────────┐  │
│  │  Tenant B (isolation: shared) │  │
│  │  ├─ Namespace: muto-tenant-b │  │
│  │  ├─ Message Bus: shared nats │  │
│  │  ├─ RBAC: sa-tenant-b        │  │
│  │  └─ Jobs: analytics-b, ...   │  │
│  └──────────────────────────────┘  │
│                                     │
└─────────────────────────────────────┘
```

Each tenant can choose between:
- **Dedicated**: Isolated message bus and resources
- **Shared**: Uses shared message bus with topic-based isolation

## Data Flow: Job Execution

1. **User creates AgentJob** (via kubectl or API)
2. **TenantReconciler validates** the job against tenant resources
3. **Scheduler selects platform** based on job requirements
4. **AgentJobReconciler triggers platform adapter** to create execution resources
5. **Platform (K8s/CF) runs agents** and reports status
6. **EventWatcher observes** platform events
7. **AgentJobReconciler updates** job status
8. **Agents communicate** via message bus (tenant-scoped topics)
9. **Job completes**, terminal state recorded, resources cleaned up

## Security Model

Muto implements multi-layered security:
- RBAC for Kubernetes access control
- Network policies for platform isolation
- TLS for all inter-component communication
- Secure credential management via Kubernetes Secrets or CredHub

See [:octicons-book-24: **Security Model**](./security-model.md) for comprehensive security details.

## Related Documentation

- [:octicons-book-24: **Agent Lifecycle**](./agent-lifecycle.md) — Job state machine and transitions
- [:octicons-book-24: **Reconcilers**](./reconcilers.md) — Control loop architecture
- [:octicons-book-24: **Messaging**](./messaging.md) — Inter-agent communication
- [:octicons-book-24: **Platform Design**](./platform-design.md) — Adapter abstraction
- [:octicons-book-24: **Security Model**](./security-model.md) — Multi-tenancy and isolation
