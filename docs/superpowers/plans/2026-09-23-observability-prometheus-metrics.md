# Observability Phase 2: Prometheus Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the six documented `muto_*` Prometheus metric families, instrument the three K8s reconcilers, and expose them via new Helm `Service`/`ServiceMonitor` resources.

**Architecture:** A new `platform/k8s/metrics` package holds package-level metric vars (registered on controller-runtime's own `metrics.Registry`, so they ride the existing `:8080/metrics` endpoint) plus a generic `ObserveReconcile` wrapper used by all three reconcilers, and two job-specific functions (`JobStarted`/`JobFinished`) called directly from `AgentJobReconciler` at its phase-transition points. The Helm chart gets a `Service` (always, when `metrics.enabled`) and an optional `ServiceMonitor` (opt-in, since not every cluster runs the Prometheus Operator).

**Tech Stack:** Go, `github.com/prometheus/client_golang` (`promauto`, already an indirect dependency via controller-runtime — becomes direct), `sigs.k8s.io/controller-runtime/pkg/metrics`, Helm.

**Spec:** `docs/superpowers/specs/2026-09-23-observability-prometheus-metrics-design.md`

## Global Constraints

- Package lives at `platform/k8s/metrics`, not `core/metrics` — `core/` has zero `controller-runtime`/`k8s.io` imports today and stays that way.
- Labels are bounded to exactly `tenant`, `reconciler`, `status`, `result` — never per-job or per-pod IDs.
- `DefaultScheduler` (`core/scheduler`) instrumentation is out of scope this phase (no metrics endpoint exists in `cmd/muto-mcp` to expose it on).
- Gauges (`muto_agents_running`, `muto_job_queue_depth`) are kept accurate by inline inc/dec at reconcile transition points, not a polling goroutine.
- `/metrics` stays plain HTTP (no `SecureServing`); this must be documented, not silently assumed.
- `muto_reconciliation_duration_seconds` buckets: `{.001,.005,.01,.05,.1,.5,1,5,10}`. `muto_job_duration_seconds` buckets: `{1,5,15,30,60,300,900,3600}`.
- `muto_jobs_total`/`muto_job_duration_seconds` status label values are lowercase (`succeeded`/`failed`), distinct from the CRD's own capitalized `Phase` field (`Succeeded`/`Failed`).

---

### Task 1: `platform/k8s/metrics` package

**Files:**
- Create: `platform/k8s/metrics/metrics.go`
- Test: `platform/k8s/metrics/metrics_test.go`

**Interfaces:**
- Produces:
  - `var ReconciliationsTotal *prometheus.CounterVec` (labels: `reconciler`, `result`)
  - `var ReconciliationDuration *prometheus.HistogramVec` (labels: `reconciler`)
  - `var JobsTotal *prometheus.CounterVec` (labels: `tenant`, `status`)
  - `var JobDuration *prometheus.HistogramVec` (labels: `tenant`, `status`)
  - `var AgentsRunning *prometheus.GaugeVec` (labels: `tenant`)
  - `var JobQueueDepth *prometheus.GaugeVec` (labels: `tenant`)
  - `func ObserveReconcile(reconciler string, fn func() (ctrl.Result, error)) (ctrl.Result, error)`
  - `func JobStarted(tenant string, agentCount int32)`
  - `func JobFinished(tenant, status string, startedAt time.Time, activeAgents int32)`

- [ ] **Step 1: Write the failing tests**

Create `platform/k8s/metrics/metrics_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package metrics_test

import (
	"errors"
	"testing"
	"time"

	"github.com/muto-io/muto/platform/k8s/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus/testutil"
	ctrl "sigs.k8s.io/controller-runtime"
)

// histogramSampleCount returns how many observations a HistogramVec's
// specific label combination has recorded. testutil.ToFloat64 only supports
// Gauge/Counter/Untyped (it panics on a Histogram), so this reads the
// observation count via the metric's protobuf Write method instead.
func histogramSampleCount(t *testing.T, o prometheus.Observer) uint64 {
	t.Helper()
	m, ok := o.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer %T does not implement prometheus.Metric", o)
	}
	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	return pb.GetHistogram().GetSampleCount()
}

func TestObserveReconcileSuccess(t *testing.T) {
	const reconciler = "test-observe-success"

	_, err := metrics.ObserveReconcile(reconciler, func() (ctrl.Result, error) {
		return ctrl.Result{}, nil
	})
	if err != nil {
		t.Fatalf("ObserveReconcile: %v", err)
	}

	if got := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues(reconciler, "success")); got != 1 {
		t.Errorf("ReconciliationsTotal{%s,success} = %v, want 1", reconciler, got)
	}
	if got := histogramSampleCount(t, metrics.ReconciliationDuration.WithLabelValues(reconciler)); got != 1 {
		t.Errorf("ReconciliationDuration{%s} sample count = %d, want 1", reconciler, got)
	}
}

func TestObserveReconcileError(t *testing.T) {
	const reconciler = "test-observe-error"
	wantErr := errors.New("boom")

	_, err := metrics.ObserveReconcile(reconciler, func() (ctrl.Result, error) {
		return ctrl.Result{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ObserveReconcile returned %v, want %v", err, wantErr)
	}

	if got := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues(reconciler, "error")); got != 1 {
		t.Errorf("ReconciliationsTotal{%s,error} = %v, want 1", reconciler, got)
	}
}

func TestJobStartedIncrementsGauges(t *testing.T) {
	const tenant = "test-job-started"

	metrics.JobStarted(tenant, 3)

	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues(tenant)); got != 3 {
		t.Errorf("AgentsRunning{%s} = %v, want 3", tenant, got)
	}
	if got := testutil.ToFloat64(metrics.JobQueueDepth.WithLabelValues(tenant)); got != 1 {
		t.Errorf("JobQueueDepth{%s} = %v, want 1", tenant, got)
	}
}

func TestJobFinishedRecordsOutcomeAndDrainsGauges(t *testing.T) {
	const tenant = "test-job-finished"

	metrics.JobStarted(tenant, 2)
	startedAt := time.Now().Add(-10 * time.Second)
	metrics.JobFinished(tenant, "succeeded", startedAt, 2)

	if got := testutil.ToFloat64(metrics.JobsTotal.WithLabelValues(tenant, "succeeded")); got != 1 {
		t.Errorf("JobsTotal{%s,succeeded} = %v, want 1", tenant, got)
	}
	if got := histogramSampleCount(t, metrics.JobDuration.WithLabelValues(tenant, "succeeded")); got != 1 {
		t.Errorf("JobDuration{%s,succeeded} sample count = %d, want 1", tenant, got)
	}
	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues(tenant)); got != 0 {
		t.Errorf("AgentsRunning{%s} = %v, want 0 (drained back to baseline)", tenant, got)
	}
	if got := testutil.ToFloat64(metrics.JobQueueDepth.WithLabelValues(tenant)); got != 0 {
		t.Errorf("JobQueueDepth{%s} = %v, want 0 (drained back to baseline)", tenant, got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails to compile**

Run: `go test ./platform/k8s/metrics/... -v`
Expected: FAIL — the `platform/k8s/metrics` package doesn't exist yet (`no such file or directory` / `cannot find package`).

- [ ] **Step 3: Implement the package**

Create `platform/k8s/metrics/metrics.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var factory = promauto.With(ctrlmetrics.Registry)

var (
	// ReconciliationsTotal counts every Reconcile call, by reconciler and
	// outcome.
	ReconciliationsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "muto_reconciliations_total",
		Help: "Total number of reconcile calls, by reconciler and result.",
	}, []string{"reconciler", "result"})

	// ReconciliationDuration observes how long each Reconcile call takes.
	ReconciliationDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muto_reconciliation_duration_seconds",
		Help:    "Duration of reconcile calls in seconds, by reconciler.",
		Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10},
	}, []string{"reconciler"})

	// JobsTotal counts AgentJobs that reached a terminal state.
	JobsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "muto_jobs_total",
		Help: "Total number of AgentJobs that reached a terminal state, by tenant and status.",
	}, []string{"tenant", "status"})

	// JobDuration observes AgentJob wall-clock duration from start to
	// completion.
	JobDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muto_job_duration_seconds",
		Help:    "Duration of AgentJobs from start to completion in seconds, by tenant and status.",
		Buckets: []float64{1, 5, 15, 30, 60, 300, 900, 3600},
	}, []string{"tenant", "status"})

	// AgentsRunning is the current count of agent pods running, by tenant.
	AgentsRunning = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "muto_agents_running",
		Help: "Number of agent pods currently running, by tenant.",
	}, []string{"tenant"})

	// JobQueueDepth is the current count of AgentJobs not yet in a
	// terminal state, by tenant.
	JobQueueDepth = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "muto_job_queue_depth",
		Help: "Number of AgentJobs not yet in a terminal state, by tenant.",
	}, []string{"tenant"})
)

// ObserveReconcile runs fn, recording its duration and result on
// ReconciliationDuration and ReconciliationsTotal under the given
// reconciler label. result is "success" if fn returns a nil error, "error"
// otherwise.
func ObserveReconcile(reconciler string, fn func() (ctrl.Result, error)) (ctrl.Result, error) {
	start := time.Now()
	result, err := fn()
	ReconciliationDuration.WithLabelValues(reconciler).Observe(time.Since(start).Seconds())
	status := "success"
	if err != nil {
		status = "error"
	}
	ReconciliationsTotal.WithLabelValues(reconciler, status).Inc()
	return result, err
}

// JobStarted records that agentCount agent pods were just created for an
// AgentJob belonging to tenant.
func JobStarted(tenant string, agentCount int32) {
	AgentsRunning.WithLabelValues(tenant).Add(float64(agentCount))
	JobQueueDepth.WithLabelValues(tenant).Inc()
}

// JobFinished records that an AgentJob belonging to tenant reached a
// terminal state. status must be "succeeded" or "failed" (lowercase —
// distinct from the AgentJob CRD's own capitalized Phase field). startedAt
// and activeAgents are the job's values from immediately before this
// transition.
func JobFinished(tenant, status string, startedAt time.Time, activeAgents int32) {
	JobsTotal.WithLabelValues(tenant, status).Inc()
	JobDuration.WithLabelValues(tenant, status).Observe(time.Since(startedAt).Seconds())
	AgentsRunning.WithLabelValues(tenant).Sub(float64(activeAgents))
	JobQueueDepth.WithLabelValues(tenant).Dec()
}
```

- [ ] **Step 4: Tidy modules**

Run: `go mod tidy`
Expected: `github.com/prometheus/client_model` loses its `// indirect` marker in `go.mod` (it's now imported directly by the test file); no version changes. Check with `git diff go.mod go.sum`.

- [ ] **Step 5: Run the tests**

Run: `go test ./platform/k8s/metrics/... -v`
Expected: PASS (all 4 tests)

- [ ] **Step 6: Commit**

```bash
git add platform/k8s/metrics/ go.mod go.sum
git commit -m "feat: add platform/k8s/metrics package

Registers the 6 documented muto_* metric families on controller-runtime's
metrics.Registry (rides the existing :8080/metrics endpoint) and provides
ObserveReconcile (generic reconcile-duration/result wrapper) and
JobStarted/JobFinished (AgentJob-specific gauge/counter updates)."
```

---

### Task 2: Instrument `TenantReconciler`

**Files:**
- Modify: `platform/k8s/reconcilers/tenant_reconciler.go`
- Modify: `platform/k8s/reconcilers/tenant_reconciler_test.go`

**Interfaces:**
- Consumes: `metrics.ObserveReconcile(reconciler string, fn func() (ctrl.Result, error)) (ctrl.Result, error)` from Task 1.

- [ ] **Step 1: Write the failing test**

Add this test to `platform/k8s/reconcilers/tenant_reconciler_test.go` (alongside the existing tests — keep the existing `import` block, add `"github.com/muto-io/muto/platform/k8s/metrics"` and `"github.com/prometheus/client_golang/prometheus/testutil"` to it):

```go
func TestTenantReconcilerRecordsMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:     "metrics-tenant-agents",
			IsolationTier: "shared",
			MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant).
		WithStatusSubresource(&v1alpha1.Tenant{}).Build()
	r := &reconcilers.TenantReconciler{Client: fakeClient, Scheme: scheme}

	before := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues("tenant", "success"))

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "metrics-tenant"},
	})
	if err != nil {
		t.Fatal(err)
	}

	after := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues("tenant", "success"))
	if after != before+1 {
		t.Errorf("ReconciliationsTotal{tenant,success} = %v, want %v", after, before+1)
	}
}
```

(This uses `before`/`after` deltas rather than an exact absolute value because the `"tenant"` reconciler label is shared with every other test in this file that calls `TenantReconciler.Reconcile` — a delta check stays correct regardless of test execution order.)

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./platform/k8s/reconcilers/... -run TestTenantReconcilerRecordsMetrics -v`
Expected: FAIL — `after != before+1` (the counter never moves, since `Reconcile` doesn't call `ObserveReconcile` yet).

- [ ] **Step 3: Wrap `Reconcile` in `ObserveReconcile`**

In `platform/k8s/reconcilers/tenant_reconciler.go`, add `"github.com/muto-io/muto/platform/k8s/metrics"` to the import block, then change:

```go
func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("tenant", req.Name)

	tenant := &v1alpha1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !tenant.DeletionTimestamp.IsZero() {
		return r.finalizeTenant(ctx, tenant, logger)
	}

	if !controllerutil.ContainsFinalizer(tenant, tenantFinalizer) {
		logger.Info("adding tenant finalizer", "finalizer", tenantFinalizer)
		controllerutil.AddFinalizer(tenant, tenantFinalizer)
		if err := r.Update(ctx, tenant); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	if err := r.ensureNamespace(ctx, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure namespace: %w", err)
	}

	switch tenant.Spec.MessageBus.Type {
	case a2a.BusTypeA2A:
		if tenant.Spec.MessageBus.Dedicated {
			if err := r.reconcileA2AGateway(ctx, tenant); err != nil {
				return ctrl.Result{}, fmt.Errorf("reconcile a2a gateway: %w", err)
			}
		}
	}

	tenant.Status.Ready = true
	if err := r.Status().Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
```

to:

```go
func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return metrics.ObserveReconcile("tenant", func() (ctrl.Result, error) {
		logger := log.FromContext(ctx).WithValues("tenant", req.Name)

		tenant := &v1alpha1.Tenant{}
		if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		if !tenant.DeletionTimestamp.IsZero() {
			return r.finalizeTenant(ctx, tenant, logger)
		}

		if !controllerutil.ContainsFinalizer(tenant, tenantFinalizer) {
			logger.Info("adding tenant finalizer", "finalizer", tenantFinalizer)
			controllerutil.AddFinalizer(tenant, tenantFinalizer)
			if err := r.Update(ctx, tenant); err != nil {
				return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
			}
		}

		if err := r.ensureNamespace(ctx, tenant); err != nil {
			return ctrl.Result{}, fmt.Errorf("ensure namespace: %w", err)
		}

		switch tenant.Spec.MessageBus.Type {
		case a2a.BusTypeA2A:
			if tenant.Spec.MessageBus.Dedicated {
				if err := r.reconcileA2AGateway(ctx, tenant); err != nil {
					return ctrl.Result{}, fmt.Errorf("reconcile a2a gateway: %w", err)
				}
			}
		}

		tenant.Status.Ready = true
		if err := r.Status().Update(ctx, tenant); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	})
}
```

Every other method in the file (`finalizeTenant`, `ensureNamespace`, etc.) is unchanged.

- [ ] **Step 4: Run the tests**

Run: `go test ./platform/k8s/reconcilers/... -v`
Expected: PASS (all existing `TenantReconciler` tests plus the new `TestTenantReconcilerRecordsMetrics`)

- [ ] **Step 5: Commit**

```bash
git add platform/k8s/reconcilers/tenant_reconciler.go platform/k8s/reconcilers/tenant_reconciler_test.go
git commit -m "feat: instrument TenantReconciler with muto_reconciliations_total/duration"
```

---

### Task 3: Instrument `AgentFleetReconciler`

**Files:**
- Modify: `platform/k8s/reconcilers/agentfleet_reconciler.go`
- Create: `platform/k8s/reconcilers/agentfleet_reconciler_test.go` (no test file exists for this reconciler yet)

**Interfaces:**
- Consumes: `metrics.ObserveReconcile` from Task 1.

- [ ] **Step 1: Write the failing test**

Create `platform/k8s/reconcilers/agentfleet_reconciler_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package reconcilers_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/platform/k8s/metrics"
	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAgentFleetReconcilerAggregatesJobCounts(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-1", Namespace: "ns"},
		Status:     v1alpha1.AgentJobStatus{Phase: "Running"},
	}
	fleet := &v1alpha1.AgentFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-1", Namespace: "ns"},
		Spec:       v1alpha1.AgentFleetSpec{JobRefs: []string{"job-1"}},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, fleet).
		WithStatusSubresource(&v1alpha1.AgentFleet{}).Build()

	r := &reconcilers.AgentFleetReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "fleet-1", Namespace: "ns"},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated := &v1alpha1.AgentFleet{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "fleet-1", Namespace: "ns"}, updated)
	if updated.Status.RunningJobs != 1 {
		t.Errorf("RunningJobs = %d, want 1", updated.Status.RunningJobs)
	}
}

func TestAgentFleetReconcilerRecordsMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	fleet := &v1alpha1.AgentFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-2", Namespace: "ns"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet).
		WithStatusSubresource(&v1alpha1.AgentFleet{}).Build()
	r := &reconcilers.AgentFleetReconciler{Client: fakeClient, Scheme: scheme}

	before := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues("agentfleet", "success"))

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "fleet-2", Namespace: "ns"},
	})
	if err != nil {
		t.Fatal(err)
	}

	after := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues("agentfleet", "success"))
	if after != before+1 {
		t.Errorf("ReconciliationsTotal{agentfleet,success} = %v, want %v", after, before+1)
	}
}
```

If `v1alpha1.AgentFleetStatus`, `v1alpha1.AgentFleetSpec`, or `v1alpha1.AgentJobStatus` field names above don't match `platform/k8s/types/v1alpha1`'s actual definitions, read that package first and adjust the field names in this test to match — the reconciler code you're instrumenting (`agentfleet_reconciler.go`, already in the repo) uses `fleet.Spec.JobRefs`, `fleet.Status.TotalJobs`, `fleet.Status.RunningJobs`, `fleet.Status.CompletedJobs`, and `job.Status.Phase`, which is the authoritative source for these names.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./platform/k8s/reconcilers/... -run TestAgentFleetReconciler -v`
Expected: `TestAgentFleetReconcilerAggregatesJobCounts` PASSes already (it exercises existing, correct behavior — this is a new regression-guard test for a reconciler that had none). `TestAgentFleetReconcilerRecordsMetrics` FAILs (`after != before+1`, since `Reconcile` doesn't call `ObserveReconcile` yet).

- [ ] **Step 3: Wrap `Reconcile` in `ObserveReconcile`**

Replace the full contents of `platform/k8s/reconcilers/agentfleet_reconciler.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package reconcilers

import (
	"context"

	"github.com/muto-io/muto/platform/k8s/metrics"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentFleetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *AgentFleetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return metrics.ObserveReconcile("agentfleet", func() (ctrl.Result, error) {
		fleet := &v1alpha1.AgentFleet{}
		if err := r.Get(ctx, req.NamespacedName, fleet); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		var total, running, completed int32
		for _, jobRef := range fleet.Spec.JobRefs {
			job := &v1alpha1.AgentJob{}
			if err := r.Get(ctx, client.ObjectKey{Name: jobRef, Namespace: req.Namespace}, job); err != nil {
				continue
			}
			total++
			switch job.Status.Phase {
			case "Running":
				running++
			case "Succeeded", "Failed":
				completed++
			}
		}

		fleet.Status.TotalJobs = total
		fleet.Status.RunningJobs = running
		fleet.Status.CompletedJobs = completed
		return ctrl.Result{}, r.Status().Update(ctx, fleet)
	})
}

func (r *AgentFleetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentFleet{}).
		Complete(r)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./platform/k8s/reconcilers/... -v`
Expected: PASS (both new tests, plus every existing reconciler test)

- [ ] **Step 5: Commit**

```bash
git add platform/k8s/reconcilers/agentfleet_reconciler.go platform/k8s/reconcilers/agentfleet_reconciler_test.go
git commit -m "feat: instrument AgentFleetReconciler with muto_reconciliations_total/duration

Also adds this reconciler's first test file (it had none) covering both
its existing job-count aggregation behavior and the new metrics wrap."
```

---

### Task 4: Instrument `AgentJobReconciler`

**Files:**
- Modify: `platform/k8s/reconcilers/agentjob_reconciler.go`
- Modify: `platform/k8s/reconcilers/agentjob_reconciler_test.go`

**Interfaces:**
- Consumes: `metrics.ObserveReconcile`, `metrics.JobStarted(tenant string, agentCount int32)`, `metrics.JobFinished(tenant, status string, startedAt time.Time, activeAgents int32)` from Task 1.

- [ ] **Step 1: Write the failing test**

Add this test to `platform/k8s/reconcilers/agentjob_reconciler_test.go` (add `"github.com/muto-io/muto/platform/k8s/metrics"` and `"github.com/prometheus/client_golang/prometheus/testutil"` to its existing import block):

```go
func TestAgentJobReconcilerRecordsMetricsOnCompletion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-job-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:  "metrics-job-agents",
			MessageBus: v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-job", Namespace: "metrics-job-agents"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "metrics-job-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "metrics-job", Namespace: "metrics-job-agents"}}

	// Pending -> Running: creates the pod and should record JobStarted.
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues("metrics-job-tenant")); got != 1 {
		t.Errorf("AgentsRunning{metrics-job-tenant} = %v, want 1 after pod creation", got)
	}

	// Mark the pod Succeeded so the next reconcile sees the job as done.
	podList := &corev1.PodList{}
	if err := fakeClient.List(ctx, podList, client.InNamespace("metrics-job-agents")); err != nil {
		t.Fatal(err)
	}
	if len(podList.Items) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(podList.Items))
	}
	pod := podList.Items[0]
	pod.Status.Phase = corev1.PodSucceeded
	if err := fakeClient.Status().Update(ctx, &pod); err != nil {
		t.Fatal(err)
	}

	jobsTotalBefore := testutil.ToFloat64(metrics.JobsTotal.WithLabelValues("metrics-job-tenant", "succeeded"))

	// Running -> Succeeded: should record JobFinished.
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	if got := testutil.ToFloat64(metrics.JobsTotal.WithLabelValues("metrics-job-tenant", "succeeded")); got != jobsTotalBefore+1 {
		t.Errorf("JobsTotal{metrics-job-tenant,succeeded} = %v, want %v", got, jobsTotalBefore+1)
	}
	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues("metrics-job-tenant")); got != 0 {
		t.Errorf("AgentsRunning{metrics-job-tenant} = %v, want 0 after completion", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./platform/k8s/reconcilers/... -run TestAgentJobReconcilerRecordsMetricsOnCompletion -v`
Expected: FAIL — `AgentsRunning{metrics-job-tenant} = 0, want 1 after pod creation` (neither `JobStarted` nor `JobFinished` is called yet).

- [ ] **Step 3: Wrap `Reconcile` and add the job-transition calls**

In `platform/k8s/reconcilers/agentjob_reconciler.go`, add `"strings"` and `"github.com/muto-io/muto/platform/k8s/metrics"` to the import block. Change:

```go
func (r *AgentJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	job := &v1alpha1.AgentJob{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	switch job.Status.Phase {
	case "", "Pending":
		return r.reconcilePending(ctx, job)
	case "Running":
		return r.reconcileRunning(ctx, job)
	case "Succeeded", "Failed":
		return r.reconcileTerminal(ctx, job)
	case "Terminating":
		return r.reconcileTerminating(ctx, job)
	}
	return ctrl.Result{}, nil
}
```

to:

```go
func (r *AgentJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return metrics.ObserveReconcile("agentjob", func() (ctrl.Result, error) {
		job := &v1alpha1.AgentJob{}
		if err := r.Get(ctx, req.NamespacedName, job); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		switch job.Status.Phase {
		case "", "Pending":
			return r.reconcilePending(ctx, job)
		case "Running":
			return r.reconcileRunning(ctx, job)
		case "Succeeded", "Failed":
			return r.reconcileTerminal(ctx, job)
		case "Terminating":
			return r.reconcileTerminating(ctx, job)
		}
		return ctrl.Result{}, nil
	})
}
```

Then in `reconcilePending`, change:

```go
	now := metav1.Now()
	job.Status.Phase = "Running"
	job.Status.ActiveAgents = int32(totalAgents)
	job.Status.StartedAt = &now
	return ctrl.Result{}, r.Status().Update(ctx, job)
}
```

to:

```go
	metrics.JobStarted(tenant.Name, int32(totalAgents))

	now := metav1.Now()
	job.Status.Phase = "Running"
	job.Status.ActiveAgents = int32(totalAgents)
	job.Status.StartedAt = &now
	return ctrl.Result{}, r.Status().Update(ctx, job)
}
```

(`tenant` is already in scope — it's the `*v1alpha1.Tenant` fetched at the top of `reconcilePending`.)

Then in `reconcileRunning`, change:

```go
	if !allDone {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	now := metav1.Now()
	job.Status.CompletedAt = &now
	job.Status.ActiveAgents = 0
	if anyFailed {
		job.Status.Phase = "Failed"
	} else {
		job.Status.Phase = "Succeeded"
	}
	return ctrl.Result{RequeueAfter: time.Duration(job.Spec.TTLAfterCompletion) * time.Second},
		r.Status().Update(ctx, job)
}
```

to:

```go
	if !allDone {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	phase := "Succeeded"
	if anyFailed {
		phase = "Failed"
	}

	var startedAt time.Time
	if job.Status.StartedAt != nil {
		startedAt = job.Status.StartedAt.Time
	}
	metrics.JobFinished(job.Spec.TenantRef, strings.ToLower(phase), startedAt, job.Status.ActiveAgents)

	now := metav1.Now()
	job.Status.CompletedAt = &now
	job.Status.ActiveAgents = 0
	job.Status.Phase = phase
	return ctrl.Result{RequeueAfter: time.Duration(job.Spec.TTLAfterCompletion) * time.Second},
		r.Status().Update(ctx, job)
}
```

This computes `phase` once and derives the lowercase metric `status` from it (`strings.ToLower`), instead of duplicating the `if anyFailed` branch a second time.

No changes are needed in `reconcileTerminal`, `reconcileTerminating`, or `buildPod` — the job already left the running gauges in `reconcileRunning`, and their own reconciliation-level metrics come from the outer `ObserveReconcile` wrap.

- [ ] **Step 4: Run the tests**

Run: `go test ./platform/k8s/reconcilers/... -v`
Expected: PASS (all existing `AgentJobReconciler` tests plus the new `TestAgentJobReconcilerRecordsMetricsOnCompletion`)

- [ ] **Step 5: Commit**

```bash
git add platform/k8s/reconcilers/agentjob_reconciler.go platform/k8s/reconcilers/agentjob_reconciler_test.go
git commit -m "feat: instrument AgentJobReconciler with job and reconciliation metrics

Wraps Reconcile in metrics.ObserveReconcile, and calls JobStarted/
JobFinished at the exact Pending->Running and Running->terminal phase
transitions."
```

---

### Task 5: Helm `Service`/`ServiceMonitor` and docs

**Files:**
- Create: `deploy/helm/muto/templates/service.yaml`
- Create: `deploy/helm/muto/templates/servicemonitor.yaml`
- Modify: `deploy/helm/muto/values.yaml`
- Modify: `docs/operations/monitoring-observability.md`

**Interfaces:**
- Consumes: existing `.Values.metrics.enabled`, `.Values.metrics.port` (from Phase 1), and the `muto.labels`/`muto.selectorLabels`/`muto.fullname` template helpers (`_helpers.tpl`). No Go interfaces — this task is Helm/docs only.

There's no Helm unit-test framework in this repo — verification is direct `helm lint`/`helm template` inspection, as in Phase 1.

- [ ] **Step 1: Add the `Service` template**

Create `deploy/helm/muto/templates/service.yaml`:

```yaml
{{- if .Values.metrics.enabled -}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "muto.fullname" . }}-metrics
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "muto.labels" . | nindent 4 }}
spec:
  selector:
    {{- include "muto.selectorLabels" . | nindent 4 }}
  ports:
    - name: metrics
      port: {{ .Values.metrics.port }}
      targetPort: metrics
      protocol: TCP
{{- end }}
```

`targetPort: metrics` refers to the pod's named `metrics` container port, already defined in `deployment-operator.yaml` (`- name: metrics` / `containerPort: {{ .Values.metrics.port }}`) — so this Service always points at the right port even if `metrics.port` is overridden.

- [ ] **Step 2: Add the `ServiceMonitor` template and its values**

Create `deploy/helm/muto/templates/servicemonitor.yaml`:

```yaml
{{- if and .Values.metrics.enabled .Values.metrics.serviceMonitor.enabled -}}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "muto.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "muto.labels" . | nindent 4 }}
    {{- with .Values.metrics.serviceMonitor.additionalLabels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  selector:
    matchLabels:
      {{- include "muto.selectorLabels" . | nindent 6 }}
  endpoints:
    - port: metrics
      interval: {{ .Values.metrics.serviceMonitor.interval }}
      path: /metrics
{{- end }}
```

`endpoints[].port: metrics` refers to the `Service`'s port *name* (`service.yaml`'s `- name: metrics`), not the pod's — that's how `ServiceMonitor` discovery works.

In `deploy/helm/muto/values.yaml`, change:

```yaml
metrics:
  enabled: true
  port: 8080
```

to:

```yaml
metrics:
  enabled: true
  port: 8080
  serviceMonitor:
    enabled: false
    interval: 30s
    additionalLabels: {}
```

- [ ] **Step 3: Lint and render the chart**

Run: `helm lint deploy/helm/muto`
Expected: `0 chart(s) failed`

Run: `helm template deploy/helm/muto | grep -A6 '^kind: Service$'`
Expected: a `Service` named `<release>-muto-metrics` (or similar, per `muto.fullname`) with `port: 8080` and `targetPort: metrics`.

Run: `helm template deploy/helm/muto | grep -c '^kind: ServiceMonitor$'`
Expected: `0` (disabled by default)

Run: `helm template deploy/helm/muto --set metrics.serviceMonitor.enabled=true | grep -A8 '^kind: ServiceMonitor$'`
Expected: a `ServiceMonitor` with `interval: 30s` and `path: /metrics`.

Run: `helm template deploy/helm/muto --set metrics.enabled=false | grep -c '^kind: Service$'`
Expected: `0` (the metrics `Service` doesn't render when metrics are disabled — even though the chart also has a pre-existing unrelated resource kind check here, `grep -c '^kind: Service$'` only matches this new template since no other template in the chart is a bare `Service`)

- [ ] **Step 4: Update the docs to match — the existing `PodMonitor` workaround is now obsolete**

`docs/operations/monitoring-observability.md`'s `### Scraping` subsection currently says "The chart doesn't create a Service, so a `ServiceMonitor` has nothing to select" and walks through a manual `PodMonitor` workaround — both false as of this task, and directly contradicted by the `Service`/`ServiceMonitor` templates just added. It also still says "`metrics.enabled: false` only removes the annotations; the operator still serves `:8080/metrics`" — already false since Phase 1 (`MUTO_METRICS_BIND_ADDRESS=0` actually disables the server). Read the file's current `### Scraping` subsection (`docs/operations/monitoring-observability.md`, search for `### Scraping`) to find its exact current boundaries, then replace it — from the `### Scraping` heading up to (but not including) the next `### PromQL Queries` heading — with:

```markdown
### Scraping

When `metrics.enabled` is `true` (the default), the Helm chart adds these annotations to the operator pod:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "8080"
prometheus.io/path: "/metrics"
```

Prometheus setups that honor these annotations pick up the operator automatically. A typical example is `kubernetes_sd_configs` with `role: pod` plus annotation relabeling. Setting `metrics.enabled: false` stops the operator from serving `:8080/metrics` at all (`MUTO_METRICS_BIND_ADDRESS=0`), not just removing the annotations.

The chart also creates a `Service` exposing the metrics port whenever `metrics.enabled` is `true`, and an optional `ServiceMonitor` (`monitoring.coreos.com/v1`, requires the Prometheus Operator's CRDs) when `metrics.serviceMonitor.enabled` is also set — off by default, since not every cluster runs the Prometheus Operator:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    interval: 30s
    # Some Prometheus Operator setups only watch ServiceMonitors carrying a
    # specific label, e.g. release: kube-prometheus-stack:
    additionalLabels: {}
```

**The metrics endpoint is unauthenticated by design** (plain HTTP, no `SecureServing`), matching controller-runtime's own default. If your cluster's security posture requires restricting who can scrape it, do so at the network layer — a `NetworkPolicy` scoping access to your Prometheus namespace — rather than assuming the endpoint itself checks credentials.

Quick check:

```bash
kubectl port-forward -n muto-system deployment/muto-operator 8080:8080 &
curl -s http://localhost:8080/metrics | grep '^controller_runtime_reconcile_total'
```

Excerpt after creating a Tenant and an AgentJob:

```
controller_runtime_reconcile_total{controller="agentjob",result="requeue_after"} 6
controller_runtime_reconcile_total{controller="agentjob",result="success"} 1
controller_runtime_reconcile_total{controller="tenant",result="success"} 2
```

```

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/muto/templates/service.yaml deploy/helm/muto/templates/servicemonitor.yaml deploy/helm/muto/values.yaml docs/operations/monitoring-observability.md
git commit -m "feat: add Helm Service/ServiceMonitor for metrics, update scraping docs

Service always renders when metrics.enabled (default true); ServiceMonitor
is opt-in via metrics.serviceMonitor.enabled (default false), since not
every cluster runs the Prometheus Operator. Replaces the docs' now-obsolete
manual PodMonitor workaround and corrects the metrics.enabled=false claim
that Phase 1 (bind addresses) already made stale."
```

---

## Self-Review

**Spec coverage:**
- Package location + wrapping pattern (spec §1-2) → Task 1.
- All 6 metric definitions with correct labels/buckets (spec §3) → Task 1.
- Instrumentation points in all 3 reconcilers (spec §4) → Tasks 2, 3, 4.
- Helm `Service`/`ServiceMonitor` + docs (spec §5) → Task 5.
- Testing approach (spec §6) → covered inline in every task (unit tests in Task 1, extended reconciler tests in Tasks 2-4, `helm template` checks in Task 5).
- All acceptance criteria are exercised: metric families exist and are labeled correctly (Task 1 tests), jobs_total/duration fire exactly once at the terminal transition (Task 4's test asserts before/after deltas across two `Reconcile` calls), gauges return to baseline after a job completes (Task 1's `TestJobFinishedRecordsOutcomeAndDrainsGauges` and Task 4's completion test both check this), existing tests still pass (every task re-runs the full reconciler/metrics suite), `helm lint`/`template` checks (Task 5), docs note the endpoint is unauthenticated (Task 5).
- The AgentFleetReconciler test file didn't exist before this plan — Task 3 creates it, noted explicitly as a deviation from the spec's literal "extend the three existing reconciler test files" (only two existed).
- The docs update grew slightly beyond the spec's "add a short note" — Task 5 also replaces the now-obsolete `PodMonitor` workaround paragraph, since leaving it in place next to the new `Service`/`ServiceMonitor` resources would leave the docs internally contradictory. Explained inline in Task 5.

**Placeholder scan:** No TBD/TODO markers; every step has literal code or an exact shell command with expected output. `histogramSampleCount`, `promauto.With(ctrlmetrics.Registry)`, and the full `ObserveReconcile` pattern were verified to compile and behave as described (run standalone before writing this plan) rather than assumed from API docs alone.

**Type consistency:** `ObserveReconcile(reconciler string, fn func() (ctrl.Result, error)) (ctrl.Result, error)`, `JobStarted(tenant string, agentCount int32)`, and `JobFinished(tenant, status string, startedAt time.Time, activeAgents int32)` are defined once in Task 1 and used with matching signatures in Tasks 2-4. Metric var names (`ReconciliationsTotal`, `ReconciliationDuration`, `JobsTotal`, `JobDuration`, `AgentsRunning`, `JobQueueDepth`) are identical between Task 1's definitions and every later task's test assertions.
