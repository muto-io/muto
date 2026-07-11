# Muto — Agent Scheduler Design Spec

**Date:** 2026-07-11
**Status:** Approved
**Name origin:** MUTO (Massive Unidentified Terrestrial Organism) from the Godzilla universe — event-driven creatures that appear, accomplish a goal, and are destroyed. Maps to the controller's lifecycle model.

---

## Summary

Muto is a platform-agnostic agent scheduling controller. It dynamically schedules multi-agent workloads in response to events, manages agent-to-agent communication via a pluggable message bus, and cleans up all resources after completion. Tenants are isolated via K8s Namespaces (and CF Orgs/Spaces in a future adapter). An MCP server and Claude skill provide external access to the scheduler.

Primary implementation language: **Go**. Primary deployment target: **Kubernetes** (via CRD + controller-runtime operator). Cloud Foundry support is designed in as a stub adapter.

---

## Architecture Overview

The system is divided into four layers:

```
┌─────────────────────────────────────────────────┐
│                  cmd/                            │
│   muto-operator (binary)  muto-mcp (binary)     │
├─────────────────────────────────────────────────┤
│                platform/                         │
│   k8s/ (controller-runtime reconcilers)         │
│   cf/  (stub: CFAdapter implementing interface) │
├─────────────────────────────────────────────────┤
│                  core/                           │
│  scheduler/ · tenant/ · agent/ · messaging/     │
│  (zero K8s imports — pure Go interfaces)        │
├─────────────────────────────────────────────────┤
│                  mcp/                            │
│  server/ · tools/ (5 MCP tools over stdio/HTTP) │
└─────────────────────────────────────────────────┘
```

**Invariant:** `core/` never imports `sigs.k8s.io/...` or `k8s.io/...`. All K8s interactions happen in `platform/k8s/`.

---

## Repository Structure

```
muto/
├── cmd/
│   ├── muto-operator/        # K8s controller-manager entry point
│   └── muto-mcp/             # MCP server entry point
├── core/
│   ├── scheduler/            # AgentJob state machine, lifecycle
│   ├── tenant/               # Tenant model, isolation tiers
│   ├── messaging/            # MessageBus interface + registry
│   │   ├── nats/             # NATS JetStream implementation
│   │   └── kafka/            # Kafka (Sarama) implementation
│   └── agent/                # Agent spec, status, event types
├── platform/
│   ├── k8s/                  # controller-runtime reconcilers + CRD types
│   └── cf/                   # CF adapter stub
├── mcp/
│   ├── server/               # MCP stdio/HTTP server (mark3labs/mcp-go)
│   └── tools/                # Tool handlers: schedule, status, cancel, list, describe
├── skills/
│   └── muto/                 # Claude skill: SKILL.md
├── deploy/
│   ├── crds/                 # Generated CRD YAML (controller-gen)
│   ├── rbac/                 # ClusterRole, ServiceAccount, RoleBinding
│   └── kind/                 # kind cluster config for local dev
└── test/
    ├── unit/                 # Pure Go unit tests
    └── integration/          # kind-based integration tests (build tag: integration)
```

---

## CRDs

### Tenant

```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: acme
spec:
  namespace: acme-agents
  isolationTier: dedicated        # shared | dedicated
  messageBus:
    type: nats                    # nats | kafka
    dedicated: true
status:
  ready: true
```

**TenantReconciler** responsibilities:
- Ensure target Namespace exists with `muto.io/tenant: <name>` label
- If `isolationTier: dedicated`, provision a NATS or Kafka StatefulSet in the tenant namespace
- Create scoped RBAC (ServiceAccount, Role, RoleBinding) in the namespace
- Set `status.ready`

### AgentJob

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: analysis-job-42
  namespace: acme-agents
spec:
  tenantRef: acme
  trigger:
    type: event                   # event | cron | manual
    source: nats://tasks.inbound
  agents:
    - role: coordinator
      image: ghcr.io/acme/coordinator:latest
      maxReplicas: 1
    - role: worker
      image: ghcr.io/acme/worker:latest
      maxReplicas: 5
  messageBus:
    topic: acme.job.42
  ttlAfterCompletion: 300
status:
  phase: Running                  # Pending | Running | Succeeded | Failed | Terminating
  activeAgents: 3
  completedAt: null
```

**AgentJobReconciler** state machine:
```
Pending → Running      spawn coordinator + worker Pods per spec
Running → Succeeded    all agent Pods completed successfully
Running → Failed       any agent Pod failed beyond retry limit
Succeeded/Failed → Terminating   ttlAfterCompletion elapsed
Terminating → (deleted)          all child resources cleaned up, CR deleted
```

- Child Pods have `ownerReferences` pointing to the AgentJob (auto-GC on force-delete)
- Message bus connection injected as env vars into each agent Pod
- Topic names prefixed `tenant.<name>.` to enforce cross-tenant isolation

### AgentFleet

Optional grouping of related AgentJobs. Supports fleet-level cancel (sets all member jobs to Terminating) and aggregated status.

---

## Core Interfaces

```go
// core/scheduler
type Scheduler interface {
    Schedule(ctx context.Context, job *agent.Job) error
    Cancel(ctx context.Context, jobID string) error
    Status(ctx context.Context, jobID string) (*agent.Status, error)
    ListActive(ctx context.Context, tenantID string) ([]*agent.Job, error)
}

// core/messaging
type MessageBus interface {
    Publish(ctx context.Context, topic string, msg []byte) error
    Subscribe(ctx context.Context, topic string, handler MsgHandler) error
    Close() error
}

// platform adapter (implemented by platform/k8s and platform/cf)
type PlatformAdapter interface {
    SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error)
    TerminateAgent(ctx context.Context, agentID string) error
    WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error)
}
```

---

## Event Trigger Flow

```
External event
  → MessageBus topic (NATS/Kafka)
  → EventWatcher goroutine (long-running, started by controller-manager)
  → creates AgentJob CR in tenant namespace
  → AgentJobReconciler reconciles
  → spawns agent Pods via PlatformAdapter
  → agents communicate over tenant-scoped MessageBus topics
  → all Pods complete
  → ttlAfterCompletion elapses
  → cleanup: Pods deleted, AgentJob CR deleted
```

---

## Multi-Tenancy

| Concern | Mechanism |
|---|---|
| K8s isolation | Separate Namespace per tenant, RBAC scoped to namespace |
| CF isolation | Org (hard) or Space (soft) per tenant — future adapter |
| Message bus (shared tier) | Topic prefix `tenant.<name>.` enforced by core/scheduler |
| Message bus (dedicated tier) | Per-tenant NATS or Kafka StatefulSet in tenant namespace |
| Cross-tenant blocking | AgentJobReconciler validates namespace matches Tenant.spec.namespace |

---

## MCP Server

Binary: `cmd/muto-mcp`. Uses `github.com/mark3labs/mcp-go`. Supports stdio (local/Claude Desktop) and HTTP+SSE (remote/production).

### Tools

| Tool | Action |
|---|---|
| `schedule_agent_job` | Creates an AgentJob CR. Returns job ID. |
| `get_job_status` | Returns phase, active agents, timestamps. |
| `cancel_job` | Sets job to Terminating, triggers cleanup. |
| `list_active_agents` | Lists all running AgentJobs for a tenant. |
| `describe_tenant` | Returns isolation tier, bus type, namespace, readiness. |

The MCP layer is thin — tool handlers validate input, call `core.Scheduler`, return formatted response. No business logic in `mcp/`.

---

## Claude Skill

Location: `skills/muto/SKILL.md`

Trigger: `/muto` or when user asks to schedule agents, check job status, or manage Muto workloads.

Documented workflow:
1. `describe_tenant` — verify tenant is ready and note isolation tier
2. `schedule_agent_job` — construct and submit job spec
3. Poll `get_job_status` until terminal phase (Succeeded/Failed)
4. `cancel_job` if user requests early termination
5. `list_active_agents` for fleet observability

---

## Testing

### Unit Tests
- Tag: none (run with `go test ./...`)
- No K8s cluster required
- Cover: scheduler state machine, tenant validation, topic prefix enforcement, MessageBus mock implementations, reconciler logic via controller-runtime fake client

### Integration Tests
- Tag: `//go:build integration`
- Require: Docker + kind
- `TestMain`: spin up kind cluster → apply CRDs → start controller-manager in-process → run tests → tear down

**Scenarios:**
1. Full lifecycle: Tenant → AgentJob → agents complete → cleanup verified
2. TTL cleanup: child Pods deleted after `ttlAfterCompletion`
3. Dedicated isolation: `dedicated` tenant gets own NATS StatefulSet
4. Multi-agent messaging: coordinator + 2 workers exchange messages → Succeeded
5. Cross-tenant isolation: agent in tenant A cannot publish to tenant B topic prefix
6. MCP round-trip: `schedule_agent_job` → `get_job_status` → `cancel_job`

### kind Config
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
  - role: worker
```

---

## Key Dependencies

| Package | Purpose |
|---|---|
| `sigs.k8s.io/controller-runtime` | Operator framework, reconcilers |
| `sigs.k8s.io/controller-tools` | CRD manifest generation (controller-gen) |
| `github.com/mark3labs/mcp-go` | MCP server SDK |
| `github.com/nats-io/nats.go` | NATS JetStream client |
| `github.com/IBM/sarama` | Kafka client |
| `k8s.io/client-go` | K8s API client |
| `github.com/onsi/ginkgo/v2` | BDD test framework |
| `github.com/onsi/gomega` | Test matchers |

---

## Makefile Targets

```
make generate          # controller-gen: CRD manifests + deepcopy funcs
make build             # build muto-operator + muto-mcp binaries
make test-unit         # go test ./... -short
make test-integration  # go test ./test/integration/... -tags integration
make kind-up           # create kind cluster + install CRDs
make kind-down         # destroy kind cluster
make docker-build      # build operator + mcp server container images
```

---

## Observability

- Prometheus metrics via controller-runtime metrics server (port 8080)
- Custom metrics: `muto_agentjob_total{phase,tenant}`, `muto_agentjob_duration_seconds{tenant}`
- Structured logging via `logr` (controller-runtime standard)

---

## CF Adapter (Stub)

`platform/cf/adapter.go` contains `CFAdapter` implementing `PlatformAdapter`:
- `SpawnAgent` → CF Push app to tenant org/space (not implemented)
- `TerminateAgent` → CF stop + delete app (not implemented)
- Tenant isolation: org = hard isolation, space = soft isolation

No CF code ships in v1 beyond the stub and doc comments. Extension point is fully defined.
