# Observability Phase 2: Prometheus metrics

**Status:** Approved, ready for implementation planning
**Issue:** [#79](https://github.com/muto-io/muto/issues/79) (Phase 2 — Prometheus metrics)
**Scope:** Second of several independent slices of #79. Phase 1 (bind addresses + structured logging, [#102](https://github.com/muto-io/muto/pull/102)) is separate and does not need to be merged first. OpenTelemetry tracing (Phase 3) and the remaining docs sweep (Phase 4) are separate specs.

## Problem

The docs promise `muto_jobs_total`, `muto_reconciliations_total`, `muto_agents_running`, `muto_job_queue_depth`, `muto_job_duration_seconds`, and `muto_reconciliation_duration_seconds`. None exist: no code in the repo calls `promauto`, `prometheus.New*`, or `MustRegister`. `:8080/metrics` (now configurable per Phase 1) only exposes controller-runtime's own built-in metrics.

## Scope decisions

Two scoping questions were resolved before design:

1. **`DefaultScheduler` (`core/scheduler`) is out of scope for this phase.** It is only ever instantiated inside `cmd/muto-mcp`, a stdio server with no metrics HTTP endpoint (Phase 1 did not add one). Instrumenting it now would produce metrics nothing can scrape. `cmd/muto-operator`'s K8s path manages `AgentJob`s directly through the reconciler and never touches `DefaultScheduler`. Follow-up once/if an MCP metrics endpoint exists.
2. **Gauges are kept accurate by inline inc/dec at reconcile transition points**, not a periodic `List()`-based recomputation goroutine. This can drift after an operator restart, but K8s controllers do a full relist on startup, which self-heals the drift within one resync cycle — not worth the extra goroutine/polling-interval complexity this phase.
3. **`/metrics` stays plain HTTP, documented as such.** Matches controller-runtime's own default and most in-cluster Prometheus setups, which restrict scraping at the network layer (NetworkPolicy, ServiceMonitor namespace scoping) rather than app-level auth. Adding `SecureServing` + `filters.WithAuthenticationAndAuthorization` is more setup (RBAC for TokenReview/SubjectAccessReview) for a security posture most clusters don't need; documented explicitly so operators who do need it know to restrict at the network layer.

## Design

### 1. Package location: `platform/k8s/metrics`, not `core/metrics`

`core/` (`agent`, `a2a`, `messaging`, `scheduler`, `tenant`) has zero `controller-runtime`/`k8s.io` imports today — it's deliberately platform-agnostic. A metrics package built around `ctrl.Result` and controller-runtime's `metrics.Registry` is K8s-specific and belongs under `platform/k8s/`, alongside the reconcilers it instruments. (The issue's own suggestion of "`core/metrics`" was hedged with "e.g." — this is a considered deviation, not an oversight.)

### 2. Wrapping pattern

A generic wrapper handles the two reconciliation-level metrics identically for all three reconcilers:

```go
func ObserveReconcile(reconciler string, fn func() (ctrl.Result, error)) (ctrl.Result, error)
```

Each `Reconcile()` becomes `return metrics.ObserveReconcile("tenant", func() (ctrl.Result, error) { ...existing body... })`. Job-specific metrics get small dedicated functions called directly from `AgentJobReconciler` at its exact phase-transition points, since only that reconciler has the domain context (tenant, phase, agent count) they need:

```go
func JobStarted(tenant string, agentCount int32)
func JobFinished(tenant, status string, startedAt time.Time, activeAgents int32)
```

Two alternatives were considered and rejected: a decorator wrapping the whole `ctrl.Manager` registration can't see job-specific fields (only the returned `ctrl.Result`/error), so it wouldn't reduce instrumentation work, just add an abstraction layer; and duplicating the timer/counter boilerplate inline in all three `Reconcile()` methods with no shared helper is exactly the "verbatim duplication of a logic block" class of defect flagged in Phase 1's reviews.

### 3. Metric definitions

All six documented families. Labels are bounded to `tenant`, `reconciler`, `status`, `result` — never per-job or per-pod IDs.

| Metric | Type | Labels | Buckets / notes |
|---|---|---|---|
| `muto_reconciliations_total` | Counter | `reconciler` (`tenant`/`agentjob`/`agentfleet`), `result` (`success`/`error`) | |
| `muto_reconciliation_duration_seconds` | Histogram | `reconciler` | `{.001,.005,.01,.05,.1,.5,1,5,10}` — reconciles are sub-second K8s API calls |
| `muto_jobs_total` | Counter | `tenant`, `status` (`succeeded`/`failed`) | Incremented exactly once, when `AgentJobReconciler` flips phase to a terminal state |
| `muto_job_duration_seconds` | Histogram | `tenant`, `status` | `CompletedAt - StartedAt`; buckets `{1,5,15,30,60,300,900,3600}` — agent jobs run minutes-to-hours, not milliseconds |
| `muto_agents_running` | Gauge | `tenant` | `+totalAgents` on pod creation, `-activeAgents` on reaching terminal |
| `muto_job_queue_depth` | Gauge | `tenant` | Non-terminal (`Pending`+`Running`) `AgentJob` count per tenant; same inc/dec points as `agents_running` |

`AgentFleetReconciler` only gets the two reconciliation-level metrics — it aggregates existing `AgentJob` status and doesn't create or complete jobs itself.

### 4. Instrumentation points

- `TenantReconciler.Reconcile` (`tenant_reconciler.go:39`) and `AgentFleetReconciler.Reconcile` (`agentfleet_reconciler.go:17`): wrap the whole body in `metrics.ObserveReconcile("tenant"/"agentfleet", func() (ctrl.Result, error) { ... })`.
- `AgentJobReconciler.Reconcile` (`agentjob_reconciler.go:25`): same wrap with `"agentjob"`, plus:
  - `reconcilePending` (line 44, right after the pod-creation loop succeeds): `metrics.JobStarted(tenant.Name, totalAgents)`.
  - `reconcileRunning` (line 98, exactly where `job.Status.Phase` flips to `"Succeeded"`/`"Failed"` — a single, one-time transition per job guarded by the existing `allDone` check): `metrics.JobFinished(job.Spec.TenantRef, status, job.Status.StartedAt.Time, job.Status.ActiveAgents)`, using the string already available in `job.Spec.TenantRef` — no new lookup needed.
- `reconcileTerminal`/`reconcileTerminating` need no new calls — the job already left the running gauges in `reconcileRunning`, and their reconciliation-level metrics come free from the outer wrap.

### 5. Helm resources (`deploy/helm/muto`)

- New `templates/service.yaml`: a `Service` exposing the metrics port, selecting on `{{ include "muto.selectorLabels" . }}` (the same labels the Deployment's pods carry), gated by the existing `.Values.metrics.enabled`, port from `.Values.metrics.port`.
- New `templates/servicemonitor.yaml`: a `monitoring.coreos.com/v1` `ServiceMonitor`, gated by a new `.Values.metrics.serviceMonitor.enabled` (default `false` — most clusters don't have prometheus-operator installed). Enabling it without the CRD present fails `helm install` with Kubernetes' own clear error — the standard pattern other charts use; no capability-detection logic needed.
- `values.yaml`: add `metrics.serviceMonitor.enabled: false` and `metrics.serviceMonitor.interval: 30s`.
- `docs/operations/monitoring-observability.md`: note that `/metrics` is unauthenticated by design, with a pointer to NetworkPolicy/ServiceMonitor namespace scoping for clusters that need to restrict it.

### 6. Testing

The three existing reconciler tests use a fake client (`sigs.k8s.io/controller-runtime/pkg/client/fake`), not envtest — fast, no cluster needed, cheap to extend.

- New `platform/k8s/metrics/metrics_test.go`: unit tests for `ObserveReconcile` (success/error paths increment the right counter+histogram label) and `JobStarted`/`JobFinished` (gauges move by the right delta), using `github.com/prometheus/client_golang/prometheus/testutil` (`testutil.ToFloat64`), already available transitively. Metric vars are package-level (required for `promauto.With(ctrlmetrics.Registry)` registration), so each test case uses a unique label value (e.g. `"test-tenant-<n>"`) to avoid cross-test interference on the shared registry.
- Extend the three existing reconciler test files with one assertion each: after calling `Reconcile()`, the relevant `testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues(...))` increased — confirms the wrapper is actually wired into `Reconcile()`, not just correct in isolation.
- `helm lint` + `helm template` checks confirming the `Service` renders when `metrics.enabled` and the `ServiceMonitor` renders only when `metrics.serviceMonitor.enabled=true`.

## Acceptance criteria

- [ ] `GET :8080/metrics` exposes all six `muto_*` families with the label sets above.
- [ ] `muto_jobs_total`/`muto_job_duration_seconds` increment exactly once per `AgentJob`, at the terminal-phase transition.
- [ ] `muto_agents_running`/`muto_job_queue_depth` return to their pre-job values once a job completes (no permanent drift within a single test run).
- [ ] Existing reconciler tests (fake-client and envtest) still pass.
- [ ] `helm lint` clean; `Service` renders iff `metrics.enabled`; `ServiceMonitor` renders iff `metrics.serviceMonitor.enabled`.
- [ ] `docs/operations/monitoring-observability.md` states the metrics endpoint is unauthenticated by design.

## Explicitly out of scope (tracked separately under #79)

- `DefaultScheduler`/`cmd/muto-mcp` instrumentation (needs an MCP metrics endpoint first).
- Periodic/`List()`-based gauge recomputation (inline inc/dec is sufficient for this phase).
- Metrics endpoint authentication (`SecureServing`).
- OpenTelemetry tracing (Phase 3) and the remaining docs sweep (Phase 4).
