# Design: Platform-agnostic AgentJob reconciliation

- **Issue:** [muto-io/muto#113](https://github.com/muto-io/muto/issues/113) — "Use reconcilers from Platform Adapter"
- **Date:** 2026-10-12
- **Status:** Approved for planning

## Problem

`AgentJobReconciler` (in `platform/k8s/reconcilers/agentjob_reconciler.go`) builds and
manages `corev1.Pod` objects directly. It never touches `core/scheduler.PlatformAdapter`,
the abstraction that already exists specifically so platform differences (K8s vs
CloudFoundry) stay out of platform-agnostic code. Today, that abstraction is only used by
the MCP server's `DefaultScheduler` for direct, non-CRD job scheduling.

The practical effect: when the operator runs with `MUTO_PLATFORM=cf`, `AgentJob` custom
resources are still reconciled by code that unconditionally creates Kubernetes `Pod`
objects. CF has no `Pod` concept, so AgentJob CRs are non-functional on CF today, even
though a working `CFAdapter` already exists and is used by the MCP path.

## Goal

Route `AgentJobReconciler`'s agent-spawning, status-tracking, and termination through
`scheduler.PlatformAdapter`, so the same reconciler works correctly regardless of
`MUTO_PLATFORM`, matching the already-documented intent in
`docs/architecture/platform-design.md` ("the scheduler and reconcilers are completely
platform-agnostic").

## Non-goals

- Fixing `MaxReplicas` handling inside the adapters themselves beyond what's needed for
  unique naming (both adapters currently only look at `spec.Agents[0]`; the reconciler
  already handles the replica loop and will keep doing so by calling `SpawnAgent` once
  per replica).
- Per-tenant adapter selection (`Tenant.spec.platform`) described in
  `platform-design.md` — platform selection remains the existing single, global
  `MUTO_PLATFORM` env var. That doc already significantly overstates the current
  implementation (tracked separately under #84's doc-accuracy sweep) and is not expanded
  here.
- CredHub/secret resolution for CF, resource limits/requests, or any other
  `platform-design.md` aspiration not already implemented.

## Current architecture (relevant pieces)

- `core/scheduler.PlatformAdapter` interface: `SpawnAgent`, `TerminateAgent`,
  `WatchAgent` — implemented by `K8sAdapter` (`platform/k8s/adapter.go`) and `CFAdapter`
  (`platform/cf/adapter.go`).
- `core/agent.Spec` carries `TenantRef`, `Trigger`, `Agents []AgentRole`, `MessageBus
  MessageBusConfig{Topic}`, `TTLAfterCompletion`.
- `DefaultScheduler` (`core/scheduler/scheduler.go`) is the only current consumer of
  `PlatformAdapter`. It calls `SpawnAgent` per role, then fans in `WatchAgent` channels
  in a background goroutine (`watchJob`) to resolve the job's terminal phase. This
  in-memory model is acceptable there because `DefaultScheduler`'s job state is itself
  only ever in-memory (MCP-driven, not CRD-backed).
- `AgentJobReconciler` (`platform/k8s/reconcilers/agentjob_reconciler.go`) is CRD-backed
  (state lives in the `AgentJob` object in etcd) and must stay restart-safe: a crashed
  and restarted operator must be able to resume exactly where it left off by re-reading
  the CR, with no reliance on in-memory state. It currently does this by listing `Pod`s
  with a `muto.io/job` label selector and polling via `ctrl.Result{RequeueAfter: ...}`.

This restart-safety requirement is why the reconciler can't simply adopt
`DefaultScheduler`'s goroutine/channel-fan-in pattern wholesale — see "Status tracking"
below.

## Design

### 1. `core/agent.Spec` additions

```go
type Spec struct {
	TenantRef          string
	Trigger            Trigger
	Agents             []AgentRole
	MessageBus         MessageBusConfig
	TTLAfterCompletion int32
	Name               string            // unique instance hint, e.g. "<job>-<role>-<replica>"
	Namespace          string            // k8s target namespace hint; ignored by non-k8s adapters
	Labels             map[string]string // k8s pod labels hint; ignored by non-k8s adapters
}

type MessageBusConfig struct {
	Topic string
	Type  string // mirrors TenantBusSpec.Type, e.g. "a2a"; "" means no special handling
}
```

All new `Spec` fields and the new `MessageBus.Type` field are optional/additive.
Existing callers (`DefaultScheduler`) that don't set them get today's behavior
unchanged, because every adapter falls back to its current default when a field is
empty.

> **Correction found during implementation planning:** the existing integration test
> suite (`test/integration/k8s/agentjob_lifecycle_test.go`,
> `k8s_multiagent_coordination_test.go`, `k8s_advanced_failure_test.go`,
> `k8s_stress_comprehensive_test.go`, `failure_scenarios_test.go`,
> `helm_deployment_test.go`, `a2a_gateway_test.go`) extensively asserts on
> `muto.io/job`, `muto.io/tenant`, and `muto.io/role` pod labels (list-by-label,
> read-label-back). `K8sAdapter.SpawnAgent` only sets `muto.io/tenant` and
> `muto.io/role` today; the reconciler's own `buildPod` additionally sets `muto.io/job`.
> Moving pod creation into the adapter must preserve all three. Added:
> `Labels map[string]string` on `Spec` — when non-empty, `K8sAdapter.SpawnAgent` uses it
> verbatim as the pod's labels; when empty (the `DefaultScheduler`/MCP path, which never
> sets it), it falls back to today's `{"muto.io/tenant": ..., "muto.io/role": ...}`
> default, unchanged. The reconciler sets
> `{"muto.io/tenant": job.Spec.TenantRef, "muto.io/job": job.Name, "muto.io/role":
> roleSpec.Role}` per spawn call.

### 2. `core/scheduler.PlatformAdapter` additions

```go
type PlatformAdapter interface {
	SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error)
	TerminateAgent(ctx context.Context, agentID string) error
	WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error)
	AgentStatus(ctx context.Context, agentID string) (*agent.Event, error)
}
```

`AgentStatus` performs a single, synchronous check: `nil, nil` means the agent is still
running (no terminal event yet); a non-nil `*agent.Event` (`EventCompleted` or
`EventFailed`) means it has reached a terminal state. Both adapters already contain this
exact check inside their `WatchAgent` poll loop:

- `K8sAdapter.WatchAgent` loops on `client.Get` + a `switch pod.Status.Phase`.
- `CFAdapter.WatchAgent` loops on `client.GetTask` + a `switch task.State`.

Each adapter extracts that single check into a private helper (e.g.
`checkPodStatus`/`checkTaskStatus`) that both `WatchAgent` (looping) and the new
`AgentStatus` (single call) use. This is a refactor of existing logic, not new platform
logic.

Adding this method to the interface is a superset change — `DefaultScheduler` is
unaffected and continues to use `WatchAgent` exactly as today. It does, however, mean
every existing hand-written `PlatformAdapter` fake must grow a trivial `AgentStatus`
stub to keep compiling: `mockAdapter`, `completingAdapter`, `failingAdapter`, and
`blockingAdapter` in `core/scheduler/scheduler_test.go`, and `stubAdapter` in
`core/tracing/scheduler_test.go`. `core/tracing.tracedPlatformAdapter`
(`core/tracing/scheduler.go`) also needs a real (non-stub) `AgentStatus` method that
wraps `inner.AgentStatus` in a span, consistent with its other three methods.

### 3. `K8sAdapter` changes

- `SpawnAgent`: pod name becomes `spec.Name` when non-empty, else the current
  `muto-<tenant>-<role>` default (preserves exact behavior for `DefaultScheduler`
  callers, which never set `Name`). Pod namespace becomes `spec.Namespace` when
  non-empty, else `a.namespace` (the adapter's constructor default, used as-is by
  `DefaultScheduler`). Pod labels become `spec.Labels` when non-empty, else the
  current `{"muto.io/tenant": ..., "muto.io/role": ...}` default.
- **Opaque composite ID**: `SpawnAgent` returns `"<namespace>/<podName>"` instead of
  just `podName`. `TerminateAgent`, `WatchAgent`, and `AgentStatus` all parse this back
  into namespace + name via `strings.Cut(agentID, "/")`. This keeps the
  `PlatformAdapter` interface unchanged (still a flat opaque `agentID string` with no
  namespace parameter anywhere) while letting `K8sAdapter` place pods in
  per-tenant namespaces. `DefaultScheduler` never inspects the ID string's structure
  today (it only stores and later passes back whatever `SpawnAgent` returned), so this
  is safe for existing callers.
- **A2A injection**: when `spec.MessageBus.Type == a2a.BusTypeA2A`, `SpawnAgent` adds the
  `MUTO_A2A_GATEWAY` env var (`http://a2a-gateway.<namespace>.svc.cluster.local:8080`,
  using the resolved target namespace) and the `MUTO_A2A_TOKEN` `SecretKeyRef` pointing
  at the `muto-a2a-token` secret — this is exactly the logic currently inline in
  `AgentJobReconciler.buildPod`, relocated verbatim. The token value itself still never
  leaves the k8s `SecretKeyRef` indirection; nothing resolves it into a plain string at
  any point in this design.
- `TerminateAgent`: treat `apierrors.IsNotFound` as success (returns `nil`), matching
  `CFAdapter.TerminateAgent`'s existing swallow of already-terminal errors. Today it
  propagates the raw delete error.

### 4. `CFAdapter` changes

- Add `AgentStatus` (extracted from `WatchAgent`'s loop body, described above).
- No other changes: `spec.Name`/`spec.Namespace` are simply not read by `CFAdapter`
  (CF task GUIDs returned by `RunTask` are already globally unique; CF has no namespace
  concept).

### 5. `AgentJobStatus` (CRD) addition

```go
type AgentJobStatus struct {
	// ... existing fields unchanged ...
	AgentIDs []string `json:"agentIDs,omitempty"` // opaque adapter-returned IDs
}
```

Additive, backward-compatible field. Replaces the pod-label-selector (`muto.io/job`)
as the mechanism for the reconciler to know which agents belong to this job — necessary
because CF has nothing to list/select the way `client.List` with `MatchingLabels` does
for K8s.

### 6. `AgentJobReconciler` changes

Gains a field: `Adapter scheduler.PlatformAdapter`.

- **`reconcilePending`**: unchanged outer loop over `job.Spec.Agents` and replica count.
  For each replica `i`, builds a name matching today's exact `buildPod` scheme (plain
  `<job>-<role>` when there's only one replica, `<job>-<role>-<i>` when there are
  several) and:
  ```go
  spec := &agent.Spec{
  	TenantRef: job.Spec.TenantRef,
  	Agents:    []agent.AgentRole{roleSpec},
  	Name:      podName, // see naming scheme above
  	Namespace: job.Namespace,
  	Labels: map[string]string{
  		"muto.io/tenant": job.Spec.TenantRef,
  		"muto.io/job":    job.Name,
  		"muto.io/role":   roleSpec.Role,
  	},
  	MessageBus: agent.MessageBusConfig{
  		Topic: job.Spec.MessageBus.Topic,
  		Type:  tenant.Spec.MessageBus.Type,
  	},
  	TTLAfterCompletion: job.Spec.TTLAfterCompletion,
  }
  id, err := r.Adapter.SpawnAgent(ctx, spec)
  ```
  and appends `id` to a local slice that becomes `job.Status.AgentIDs` on the status
  update. `buildPod` is deleted — that logic now lives in `K8sAdapter.SpawnAgent`.
- **`reconcileRunning`**: replaces the `client.List` of Pods with a loop over
  `job.Status.AgentIDs` calling `r.Adapter.AgentStatus(ctx, id)`. Aggregation logic is
  unchanged in shape: `allDone`/`anyFailed` computed from each call's result (`nil`
  event ⇒ not done; `EventFailed` ⇒ failed; `EventCompleted` ⇒ done), same
  `RequeueAfter: 5s` when not all done. The old "no pods found yet, requeue 1s" branch
  is removed — agent IDs are already known and persisted by the time this phase runs,
  there is no equivalent "not yet visible" gap to wait out.
- **`reconcileTerminal`**: unchanged (TTL bookkeeping only, no pod access).
- **`reconcileTerminating`**: replaces the `client.List` + per-pod `client.Delete` with
  a loop over `job.Status.AgentIDs` calling `r.Adapter.TerminateAgent(ctx, id)`.
- **`SetupWithManager`**: drops `.Owns(&corev1.Pod{})`. The reconciler no longer creates
  or owns Pods directly, so there's nothing for this watch to observe; status transitions
  now happen purely through the `RequeueAfter` polling cadence already in place, the
  same model `CFAdapter`-backed jobs must use regardless.

### 7. `cmd/muto-operator/main.go`

```go
if err := (&reconcilers.AgentJobReconciler{
	Client:  mgr.GetClient(),
	Scheme:  mgr.GetScheme(),
	Adapter: platformAdapter,
}).SetupWithManager(mgr); err != nil {
```

The comment above the adapter-selection switch ("K8s reconcilers use mgr.GetClient()
directly and are platform-independent") is updated to describe the new, actually-true
state: `AgentJobReconciler` is platform-independent *because* it delegates to
`platformAdapter`, not in spite of bypassing it.

### 8. Trade-offs accepted (confirmed during design review)

- **No OwnerReference-based GC.** Today, Pods carry an `OwnerReference` to the
  `AgentJob`, so if the CR is deleted while the operator is down, Kubernetes garbage
  collects the Pods automatically on restart. Routing creation through `SpawnAgent`
  removes this: the adapter has no notion of the `AgentJob` object, and CF has no
  equivalent mechanism at all. Cleanup now relies entirely on
  `reconcileTerminating` running again and finding `job.Status.AgentIDs` (persisted in
  etcd) on restart. This matches CF's behavior today (which never had GC-on-delete) and
  unifies both platforms' cleanup semantics. The only new risk is a crash that loses the
  `AgentJob` object itself before `Terminating` completes — an existing risk class for
  CF already, now shared by K8s.
- **Status polling, not event-driven watch.** `AgentStatus` is a synchronous poll called
  from the existing `RequeueAfter` loop, not a `WatchAgent`-based background goroutine.
  This keeps the reconciler fully restart-safe (no in-memory state to lose) at the cost
  of up-to-5s-latency status updates — identical to today's K8s polling cadence, and no
  worse than CF's existing polling-only `WatchAgent` implementation.

## Testing plan

- `platform/k8s/adapter_test.go`: `SpawnAgent` honors `spec.Name`/`spec.Namespace` when
  set and falls back correctly when empty; composite ID round-trips through
  `TerminateAgent`/`WatchAgent`/`AgentStatus`; A2A env/secret injection when
  `MessageBus.Type == a2a.BusTypeA2A`; `TerminateAgent` returns `nil` on `NotFound`.
- `platform/cf/adapter_test.go`: new `AgentStatus` test mirroring the existing
  `WatchAgent` terminal-state test cases (succeeded/failed/still-running).
- `platform/k8s/reconcilers/agentjob_reconciler_test.go`: switches from asserting on
  real/fake-client Pods to injecting an in-memory fake `PlatformAdapter` (map of ID →
  state, with test hooks to flip an agent to completed/failed). Covers: multi-replica
  spawn populates `Status.AgentIDs` with one ID per replica; `reconcileRunning`
  transitions to `Succeeded`/`Failed` based on `AgentStatus` results and requeues while
  pending; `reconcileTerminating` calls `TerminateAgent` for every tracked ID.
- Existing `TestSpawnAgentCreatesPod`-style tests remain valid since
  `K8sAdapter.SpawnAgent`'s default (no `Name`/`Namespace` set) behavior is unchanged.

## Files touched

- `core/agent/types.go` (`Spec`, `MessageBusConfig`)
- `core/scheduler/scheduler.go` (interface), `core/scheduler/scheduler_test.go` (fake
  adapter stubs)
- `core/tracing/scheduler.go` (`tracedPlatformAdapter.AgentStatus`),
  `core/tracing/scheduler_test.go` (`stubAdapter` stub + wrap test)
- `platform/k8s/adapter.go`, `platform/k8s/adapter_test.go`
- `platform/cf/adapter.go`, `platform/cf/adapter_test.go`
- `platform/k8s/types/v1alpha1/agentjob_types.go` (+ regenerate
  `zz_generated.deepcopy.go` and `deploy/crds/muto.io_agentjobs.yaml` /
  `deploy/helm/muto/templates/crds/agentjobs.yaml` via `make generate`)
- `platform/k8s/reconcilers/agentjob_reconciler.go`,
  `platform/k8s/reconcilers/agentjob_reconciler_test.go`
- `cmd/muto-operator/main.go`

No changes needed in `test/integration/...` — those exercise either the MCP/
`DefaultScheduler` path (unaffected) or the `AgentJobReconciler` path at the black-box
CR/Pod level, which this design preserves (same `muto.io/job`/`muto.io/tenant`/
`muto.io/role` labels, same eventual `Phase`/`ActiveAgents` status fields). They require
a live k3s cluster and are not run as part of this plan's verification.
