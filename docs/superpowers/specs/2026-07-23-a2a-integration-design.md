# A2A Protocol Integration Design Spec

**Date:** 2026-07-23
**Status:** Approved
**Branch:** muto-implementation

---

## Summary

Integrate Google's Agent2Agent (A2A) protocol as a second communication channel alongside the existing NATS/Kafka message bus. Agents still use NATS/Kafka for event-driven pub/sub; A2A enables structured RPC-style task calls between agents via a per-tenant gateway. Configuration follows the exact same pattern as the existing Kafka config: `type: a2a` in `Tenant.spec.messageBus` with `dedicated: true` provisions an A2A gateway Deployment+Service in the tenant namespace.

---

## Architecture Overview

The integration adds two things: a **gateway provisioner** (reconciler logic) and an **A2A client layer** (core package). Everything else is unchanged.

```
┌─────────────────────────────────────────────────────────────────┐
│                          core/                                   │
│  messaging/   (unchanged — NATS, Kafka pub/sub)                 │
│  a2a/         (NEW — A2AConfig, A2AClient, BusTypeA2A const)    │
├─────────────────────────────────────────────────────────────────┤
│                        platform/k8s/                             │
│  reconcilers/tenant_reconciler.go  (extended — A2A gateway      │
│                                     Deployment+Service branch)  │
│  reconcilers/agentjob_reconciler.go (extended — inject          │
│                                      MUTO_A2A_GATEWAY env var)  │
├─────────────────────────────────────────────────────────────────┤
│                        deploy/                                   │
│  crds/muto.io_tenants.yaml  (regenerated — a2a BusType enum)   │
└─────────────────────────────────────────────────────────────────┘
```

**Invariant preserved:** `core/a2a/` imports no K8s packages. Gateway provisioning lives entirely in `platform/k8s/`.

**Unchanged:** `core/messaging/` interface and implementations, `AgentJob` CRD reconciler logic (only env var injection extended), MCP tools, scheduler state machine, CF adapter stub.

---

## CRD Changes

Only `Tenant` changes. `TenantBusSpec.Type` gains `a2a` as a valid enum value.

```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: acme
spec:
  namespace: acme-agents
  isolationTier: dedicated
  messageBus:
    type: a2a       # new value alongside nats | kafka
    dedicated: true # provisions gateway in tenant namespace
```

**`TenantBusSpec` kubebuilder marker update:**

```go
// +kubebuilder:validation:Enum=nats;kafka;a2a
Type string `json:"type"`
```

`AgentJob` CRD: no change. `TenantStatus`: no change — existing `ready: bool` covers gateway readiness identically to NATS/Kafka.

---

## `core/a2a/` Package

New package — no K8s imports.

```
core/a2a/
├── a2a.go      # A2AConfig, BusTypeA2A const
└── client.go   # A2AClient — thin HTTP wrapper for A2A task calls
```

### `a2a.go`

```go
package a2a

import "github.com/muto-io/muto/core/messaging"

const BusTypeA2A messaging.BusType = "a2a"

type Config struct {
    GatewayURL string
    AuthToken  string // Bearer token injected by reconciler via Secret
}
```

`BusTypeA2A` uses the `messaging.BusType` alias for consistency. A2A does **not** implement `MessageBus` — it is a parallel client layer, not a pub/sub bus.

### `client.go`

```go
type A2AClient struct {
    gatewayURL string
    httpClient *http.Client
    authToken  string
}

func New(cfg *Config) (*A2AClient, error) { ... } // errors if GatewayURL empty

// SendTask submits a task to a named agent via the A2A gateway.
func (c *A2AClient) SendTask(ctx context.Context, agentID string, payload []byte) (*TaskResult, error) { ... }

// GetTaskStatus polls task state from the gateway.
func (c *A2AClient) GetTaskStatus(ctx context.Context, taskID string) (*TaskResult, error) { ... }

type TaskResult struct {
    TaskID string
    State  string // submitted | working | completed | failed
    Output []byte
}
```

Agents construct `A2AClient` from `MUTO_A2A_GATEWAY` and `MUTO_A2A_TOKEN` env vars. `New()` returns an error immediately if `GatewayURL` is empty. Agents own retry logic — the client does not retry internally.

---

## Gateway Provisioning

### TenantReconciler

New branch parallel to existing NATS/Kafka logic:

```go
switch tenant.Spec.MessageBus.Type {
case messaging.BusTypeNATS:
    // existing
case messaging.BusTypeKafka:
    // existing
case a2a.BusTypeA2A:
    if tenant.Spec.MessageBus.Dedicated {
        return r.reconcileA2AGateway(ctx, tenant)
    }
}
```

`reconcileA2AGateway` provisions three resources in the tenant namespace:

| Resource | Details |
|---|---|
| `Deployment` | A2A gateway image — configurable via operator env var `MUTO_A2A_GATEWAY_IMAGE` (default image TBD: use a confirmed A2A-compatible gateway image before shipping) |
| `Service` | ClusterIP, port 8080, predictable DNS `a2a-gateway.<namespace>.svc.cluster.local` |
| `Secret` | Bearer token `muto-a2a-token` — generated once, stable across reconcile loops |

All three carry `ownerReferences` pointing to the Tenant CR — auto-GC on tenant deletion.

`TenantStatus.Ready` is set `true` only once the gateway Deployment reports `availableReplicas >= 1`.

### AgentJobReconciler

`buildPod()` is extended to inject gateway coordinates when the tenant uses A2A:

```go
// injected when tenant.Spec.MessageBus.Type == a2a.BusTypeA2A
{Name: "MUTO_A2A_GATEWAY", Value: "http://a2a-gateway." + job.Namespace + ".svc.cluster.local:8080"},
{Name: "MUTO_A2A_TOKEN",   Value: token}, // read from muto-a2a-token Secret
```

Gateway URL is derived deterministically from the Service name — no lookup required. Token is read from the `muto-a2a-token` Secret in the tenant namespace.

The `AgentJobReconciler` must fetch the parent `Tenant` object (via `job.Spec.TenantRef`) during `buildPod()` to determine `messageBus.type` and decide whether to inject A2A env vars. This is a new Get call in the reconciler — the Tenant is cluster-scoped so no namespace qualifier is needed.

---

## Error Handling

- `reconcileA2AGateway`: Deployment, Service, or Secret creation failure returns the error and lets controller-runtime requeue. No partial-state silent failures.
- `A2AClient.New`: returns error immediately if `GatewayURL` is empty, rather than failing on first call.
- `SendTask` / `GetTaskStatus`: return typed errors wrapping HTTP status codes. Callers own retry logic.

---

## Testing

| Layer | Coverage |
|---|---|
| Unit — `core/a2a/` | `A2AClient` against `httptest.Server` stubs; `New()` error on empty URL |
| Unit — `tenant_reconciler` | Fake client: Deployment + Service + Secret created for `type: a2a, dedicated: true`; none created for `dedicated: false` |
| Unit — `agentjob_reconciler` | Pod env vars: `MUTO_A2A_GATEWAY` present when tenant type is `a2a`, absent otherwise |
| Integration | `type: a2a` Tenant → gateway Deployment+Service provisioned → AgentJob pods receive correct env vars → `TenantStatus.Ready = true` |

No new integration test infrastructure needed — existing kind cluster and in-process controller-manager setup covers all scenarios.

---

## Files Changed

| File | Change |
|---|---|
| `core/a2a/a2a.go` | New |
| `core/a2a/client.go` | New |
| `core/a2a/client_test.go` | New |
| `platform/k8s/types/v1alpha1/tenant_types.go` | Add `a2a` to enum marker |
| `platform/k8s/reconcilers/tenant_reconciler.go` | Add `reconcileA2AGateway` branch |
| `platform/k8s/reconcilers/tenant_reconciler_test.go` | New A2A test cases |
| `platform/k8s/reconcilers/agentjob_reconciler.go` | Extend `buildPod()` env var injection |
| `platform/k8s/reconcilers/agentjob_reconciler_test.go` | New A2A env var assertions |
| `deploy/crds/muto.io_tenants.yaml` | Regenerated via `make generate` |
| `test/integration/` | New A2A gateway lifecycle scenario |

---

## Key Dependencies

No new Go dependencies required. The A2A gateway runs as an external image. `net/http` (stdlib) is sufficient for the `A2AClient`.
