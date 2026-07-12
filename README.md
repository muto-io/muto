# Muto

**Muto** is a Kubernetes-native agent scheduler and orchestrator for multi-agent AI workloads.

> The name comes from the Godzilla universe: M.U.T.O. (Massive Unidentified Terrestrial Organism) — a creature that consumes energy and adapts. Fitting for a scheduler that consumes workloads and adapts to multi-tenant demand.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  cmd/                        Entry points                        │
│    muto-operator             Kubernetes controller manager       │
│    muto-mcp                  MCP server for Claude integration   │
├─────────────────────────────────────────────────────────────────┤
│  platform/                   Infrastructure adapters             │
│    k8s/                      Kubernetes PlatformAdapter +        │
│      reconcilers/            TenantReconciler, AgentJobReconciler│
│                              AgentFleetReconciler, EventWatcher  │
│    cf/                       Cloud Foundry stub (pluggable)      │
├─────────────────────────────────────────────────────────────────┤
│  core/                       Domain logic (platform-agnostic)    │
│    agent/                    Job and Spec types, state machine   │
│    scheduler/                DefaultScheduler, FSM transitions   │
│    tenant/                   Tenant validation, topic prefixing  │
│    messaging/                MessageBus interface + registry     │
│      nats/                   NATS implementation                 │
│      kafka/                  Kafka implementation                │
├─────────────────────────────────────────────────────────────────┤
│  mcp/                        Model Context Protocol layer        │
│    server/                   MCP server wiring (mcp-go)         │
│    tools/                    ScheduleAgentJob, GetJobStatus,     │
│                              ListJobs, CancelJob handlers        │
└─────────────────────────────────────────────────────────────────┘
```

Data flow: `Claude (via MCP) → mcp/tools → core/scheduler → platform/k8s → Kubernetes CRDs`

---

## Quick Start

### Prerequisites

- [Go 1.22+](https://golang.org/dl/)
- [kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Docker](https://www.docker.com/)

### 1. Spin up a local cluster

```bash
make kind-up
```

This creates a `muto-dev` kind cluster and installs the CRDs.

### 2. Run the operator

```bash
# Build binaries
make build

# Run the operator against your current kubeconfig context
./bin/muto-operator
```

### 3. Run the MCP server

```bash
./bin/muto-mcp
```

The MCP server exposes tools that Claude (or any MCP-compatible client) can call to schedule and manage agent jobs.

### 4. Use via Claude Desktop

Add the MCP server to your Claude Desktop config (`~/.claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "muto": {
      "command": "/path/to/bin/muto-mcp"
    }
  }
}
```

Claude can then call tools like `schedule_agent_job`, `get_job_status`, `list_jobs`, and `cancel_job` directly.

### Tear down

```bash
make kind-down
```

---

## Testing

### Unit tests

```bash
make test-unit
```

Runs all packages with `-short` flag. No external dependencies required.

### Integration tests

```bash
make test-integration
```

Integration tests spin up a real k3s cluster via [testcontainers-go](https://github.com/testcontainers/testcontainers-go) and exercise the full reconciler loop. Requires Docker.

To verify integration tests compile without running them:

```bash
go build -tags integration ./test/integration/...
```

---

## CRDs

All CRDs live in `deploy/crds/` and are in the `muto.io` API group (`v1alpha1`).

### Tenant

Cluster-scoped resource that represents an isolated workload tenant.

```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: my-team
spec:
  isolationTier: shared      # shared | dedicated
  namespace: my-team-ns
  messageBus:
    type: nats               # nats | kafka
    dedicated: false
```

- `shared` tier: tenants share a message bus topic pool, lower cost.
- `dedicated` tier: tenant gets its own bus instance, stronger isolation.

### AgentJob

Namespaced resource that describes a single agent workload run.

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: summarise-pr-42
  namespace: my-team-ns
spec:
  tenantRef: my-team
  agents:
    - role: worker
      image: ghcr.io/my-org/agent:latest
      maxReplicas: 3
  trigger:
    type: event
    source: github.pr.opened
  messageBus:
    topic: muto.my-team.summarise-pr-42
  ttlAfterCompletion: 300
```

Status phases: `Pending → Running → Succeeded | Failed`

### AgentFleet

Namespaced resource that groups multiple AgentJobs under a single tenant and tracks aggregate progress.

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentFleet
metadata:
  name: nightly-batch
  namespace: my-team-ns
spec:
  tenantRef: my-team
  jobRefs:
    - summarise-pr-42
    - review-pr-43
```

The fleet reconciler aggregates `runningJobs`, `completedJobs`, and `totalJobs` into its status.

---

## Message Bus

Muto ships two MessageBus implementations behind a pluggable interface (`core/messaging`).

| Backend | Use case | Default |
|---------|----------|---------|
| **NATS** | Simple tasks, low latency, single-region | Yes |
| **Kafka** | High-throughput, replay, cross-region fan-out | Optional |

The active bus is selected per-Tenant via `spec.messageBus.type`. A registry (`core/messaging/registry.go`) maps the string to the correct factory, making it straightforward to add new backends.

To add a custom backend implement the `MessageBus` interface and register it:

```go
messaging.Register("my-bus", func(cfg messaging.Config) (messaging.MessageBus, error) {
    return newMyBus(cfg)
})
```

---

## Multi-Tenancy

Muto enforces tenant isolation at two levels:

| Level | Shared tier | Dedicated tier |
|-------|-------------|----------------|
| Namespace | Shared cluster namespace | Tenant-owned namespace |
| Message bus | Shared NATS/Kafka, prefixed topics | Dedicated bus instance |
| Agent pods | Co-scheduled | Node-affinity / taint-based |

Topic naming is enforced by `core/tenant`: every topic is automatically prefixed with `muto.<tenantID>.` so jobs from different tenants never collide on the bus.

---

## API Reference

Full OpenAPI 3.0 schemas for all CRDs are in [`docs/api/openapi.yaml`](docs/api/openapi.yaml).

Every field also has inline documentation accessible via `kubectl explain`:

```bash
kubectl explain agentjob.spec
kubectl explain agentjob.spec.agents
kubectl explain tenant.spec.isolationTier
kubectl explain agentfleet.spec.jobRefs
```

---

## Project Layout

```
cmd/            Binary entry points
core/           Domain logic (no Kubernetes imports)
mcp/            MCP server and tool handlers
platform/       Infrastructure adapters (Kubernetes, CF)
deploy/         CRD manifests, kind config, RBAC
test/           Integration test suite
skills/         Claude skill definitions
```

---

## License

MIT
