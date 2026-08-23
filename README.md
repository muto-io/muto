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
│    cf/                       Cloud Foundry PlatformAdapter       │
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

Data flow:
- **Kubernetes**: `Claude (via MCP) → mcp/tools → core/scheduler → platform/k8s → Kubernetes CRDs`
- **Cloud Foundry**: `Claude (via MCP) → mcp/tools → core/scheduler → platform/cf → CF Tasks`

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

## Cloud Foundry Deployment

Muto can also run as a long-lived Cloud Foundry app. See [Cloud Foundry Deployment Guide](deploy/cf/README.md) for step-by-step instructions.

### Key differences from Kubernetes

| Feature | Kubernetes | Cloud Foundry |
|---------|------------|---------------|
| State management | etcd (Kubernetes native) | Cloud Foundry app state |
| Buildpack | N/A | Binary buildpack |
| HTTP route | Service/Ingress | CF routes (disabled with `no-route: true`) |
| Health checks | Kubelet probes | CF process health checks |
| Scaling | Replicas via Kubernetes | Single instance on CF |

### Quick start on CF

```bash
# Build Linux binary
GOOS=linux GOARCH=amd64 make build

# Push to your CF space
cf push -f deploy/cf/manifest.yml -p bin/

# Set API credentials (never commit these)
cf set-env muto-operator CF_API_URL https://api.cf.example.com
cf set-env muto-operator CF_USERNAME admin
cf set-env muto-operator CF_PASSWORD your-password
cf restage muto-operator
```

See `deploy/cf/README.md` for full configuration reference.

---

## Testing

Muto includes a comprehensive e2e test suite with **39 tests** covering both Kubernetes and Cloud Foundry platforms:

- **11 Cloud Foundry adapter tests** — Mock CF API server with task lifecycle testing
- **28 Kubernetes tests** — Agent lifecycle, Helm deployment, failure scenarios, and stress testing

### Quick Start

```bash
# Run all tests (K8s + CF) — ~88 seconds total
make test-integration

# Run K8s tests only — K3s cluster spun up automatically
make test-integration-kind

# Run CF tests only — Mock server, no external dependencies
make test-integration-cf

# Run unit tests
make test-unit
```

### Local Development

```bash
# Run with verbose output
go test ./test/integration -tags integration -v

# Run specific test file
go test ./test/integration -tags integration -run TestAgentJob -v

# Increase timeout for slow machines
go test ./test/integration -tags integration -timeout 30m

# Verify tests compile without running
go build -tags integration ./test/integration/...
```

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
    topic: tenant.my-team.summarise-pr-42
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

Topic naming is enforced by `core/tenant`: every topic is automatically prefixed with `tenant.<tenantID>.` so jobs from different tenants never collide on the bus.

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

Apache License 2.0

Muto is licensed under the Apache License 2.0, which provides:
- Explicit patent grant and protection
- Clear rights for commercial and private use
- Compatibility with Kubernetes ecosystem standards
- Full protection for derivative works

See [LICENSE](LICENSE) for full license text and [NOTICE](NOTICE) for third-party component licenses.

For a detailed analysis of why Apache 2.0 was chosen for this project, see [LICENSE_ANALYSIS.md](docs/LICENSE_ANALYSIS.md).
