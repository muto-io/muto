# Observability Phase 3: OpenTelemetry Tracing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OpenTelemetry distributed tracing across both muto binaries — reconcile loops, the scheduler, both platform adapters, MCP tool invocations, and A2A/CF HTTP client propagation — off by default, on when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.

**Architecture:** A generic `core/tracing` package provides `Init` (SDK setup from standard `OTEL_*` env vars) and two generic wrapper functions, `Wrap[T]`/`WrapErr`, that start a span, run a function, record any error, and end the span. Every instrumented surface (reconcilers, platform adapters, scheduler, MCP tools) is called through a narrow Go interface, so each gets a small decorator implementing that interface and delegating to `Wrap`/`WrapErr` — applied once at the construction/registration site, never inside an existing method body. Decorators that would require a `controller-runtime` or `mcp-go` dependency live next to what they wrap (`platform/k8s/tracing`, `mcp/server`) rather than in `core/tracing`, keeping `core/` free of framework-specific imports (the same invariant Phase 2 established for `core/` vs `platform/k8s/`).

**Tech Stack:** Go, `go.opentelemetry.io/otel` + `otel/sdk` + `otel/exporters/otlp/otlptrace/otlptracehttp` (new dependencies — the SDK and exporter aren't in `go.mod` yet, only the API packages, pulled in transitively), `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` (already indirect, becomes direct), `go.opentelemetry.io/otel/sdk/trace/tracetest` (test-only).

**Spec:** `docs/superpowers/specs/2026-09-23-observability-otel-tracing-design.md`

## Global Constraints

- Standard `OTEL_*` env vars only — no `MUTO_OTEL_*` aliases.
- No separate "enabled" flag: tracing is on iff `OTEL_EXPORTER_OTLP_ENDPOINT` is set; otherwise `Init` is a no-op and the OTel API's built-in no-op tracer stays in effect (confirmed in practice: a span created via a package-level `otel.Tracer(...)` var *before* `otel.SetTracerProvider` is ever called is not recorded by anything — no explicit no-op provider construction needed).
- OTLP over HTTP (`otlptracehttp`) only, no gRPC/protocol selection.
- `core/` (and therefore `core/tracing`'s foundational pieces) must have zero `k8s.io`/`sigs.k8s.io/controller-runtime` imports. The `reconcile.Reconciler` decorator (`WrapReconciler`) is the one piece that needs controller-runtime, so it lives in `platform/k8s/tracing`, not `core/tracing` — a refinement of the spec's package-location description, made for the same reason Phase 2 placed its metrics package under `platform/k8s/`, not `core/`.
- `WrapPlatformAdapter`/`WrapScheduler` type against `core/scheduler`'s `PlatformAdapter`/`Scheduler` interfaces (both already platform-agnostic, defined in terms of `core/agent` types only) — these belong in `core/tracing` since depending on a sibling `core/` package doesn't violate the invariant above.
- Every decorator delegates to `core/tracing.Wrap`/`WrapErr` — never duplicates the span-start/record-error/end logic inline.
- Span names follow `"<Type>.<Method>"` (e.g. `"TenantReconciler.Reconcile"`, `"PlatformAdapter.SpawnAgent"`, `"Scheduler.Schedule"`) or `"mcp.<tool_name>"` for MCP tools.

---

### Task 1: `core/tracing` — `Init`, `Wrap`, `WrapErr`

**Files:**
- Create: `core/tracing/tracing.go`
- Test: `core/tracing/tracing_test.go`

**Interfaces:**
- Produces:
  - `func Init(ctx context.Context, defaultServiceName string) (func(context.Context) error, error)`
  - `func Wrap[T any](ctx context.Context, name string, fn func(context.Context) (T, error)) (T, error)`
  - `func WrapErr(ctx context.Context, name string, fn func(context.Context) error) error`

- [ ] **Step 1: Write the failing tests**

Create `core/tracing/tracing_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package tracing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/muto-io/muto/core/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// withRecorder installs a tracetest-backed TracerProvider as the global
// provider for the duration of the test, and restores the previous one
// afterward. core/tracing's package-level tracer var delegates to whatever
// provider is globally installed at each Start() call (not frozen at
// creation time), so this is sufficient to capture spans from Wrap/WrapErr
// without any test-only seam in the production code.
func withRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

func TestInitNoopWhenEndpointUnset(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := tracing.Init(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init returned a nil shutdown func")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown returned error: %v", err)
	}
}

func TestInitBuildsProviderWhenEndpointSet(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	shutdown, err := tracing.Init(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Shutting down immediately is safe even with nothing listening on the
	// endpoint - it just means the flush attempt fails silently (the SDK
	// logs it internally), not that Shutdown itself returns an error for
	// an unreachable collector on close.
	if err := shutdown(context.Background()); err != nil {
		t.Logf("shutdown against unreachable collector returned (expected, non-fatal): %v", err)
	}
}

func TestWrapRecordsSpanAndPassesResultThrough(t *testing.T) {
	sr := withRecorder(t)

	result, err := tracing.Wrap(context.Background(), "test.op", func(ctx context.Context) (string, error) {
		return "hello", nil
	})
	if err != nil || result != "hello" {
		t.Fatalf("Wrap: got (%q, %v), want (\"hello\", nil)", result, err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "test.op" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "test.op")
	}
	if spans[0].Status().Code != codes.Unset {
		t.Errorf("span status = %v, want Unset (no error)", spans[0].Status().Code)
	}
}

func TestWrapRecordsErrorOnSpan(t *testing.T) {
	sr := withRecorder(t)

	wantErr := errors.New("boom")
	_, err := tracing.Wrap(context.Background(), "test.fail", func(ctx context.Context) (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Wrap returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", spans[0].Status().Code)
	}
}

func TestWrapErrSuccessAndFailure(t *testing.T) {
	sr := withRecorder(t)

	if err := tracing.WrapErr(context.Background(), "test.errfn.ok", func(ctx context.Context) error {
		return nil
	}); err != nil {
		t.Fatalf("WrapErr: %v", err)
	}

	wantErr := errors.New("errfn boom")
	err := tracing.WrapErr(context.Background(), "test.errfn.fail", func(ctx context.Context) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WrapErr returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	if spans[0].Status().Code != codes.Unset {
		t.Errorf("spans[0] (%s) status = %v, want Unset", spans[0].Name(), spans[0].Status().Code)
	}
	if spans[1].Status().Code != codes.Error {
		t.Errorf("spans[1] (%s) status = %v, want Error", spans[1].Name(), spans[1].Status().Code)
	}
}
```

- [ ] **Step 2: Run it to verify it fails to compile**

Run: `go test ./core/tracing/... -v`
Expected: FAIL — the `core/tracing` package doesn't exist yet.

- [ ] **Step 3: Implement the package**

Create `core/tracing/tracing.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package tracing

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var tracer = otel.Tracer("github.com/muto-io/muto")

// Init sets up the global TracerProvider from standard OTEL_* environment
// variables (OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_EXPORTER_OTLP_HEADERS,
// OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES, ...), read automatically by
// the OTLP exporter and resource.WithFromEnv(). If
// OTEL_EXPORTER_OTLP_ENDPOINT is unset, this is a no-op: no exporter or
// provider is created, and the OTel API's built-in default (a no-op
// tracer) stays in effect for every caller, so tracing costs nothing when
// disabled. On success, returns a shutdown function the caller must invoke
// (e.g. via defer, with a bounded timeout context) to flush buffered spans
// before exit.
func Init(ctx context.Context, defaultServiceName string) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(defaultServiceName)),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Wrap starts a span named name, runs fn with the span-scoped context,
// records any error fn returns on the span (RecordError + an Error
// status), ends the span, and returns fn's result and error unchanged.
// Generic over the success type so it fits every instrumented call site in
// this codebase without core/tracing depending on any of their types.
func Wrap[T any](ctx context.Context, name string, fn func(context.Context) (T, error)) (T, error) {
	ctx, span := tracer.Start(ctx, name)
	defer span.End()
	result, err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

// WrapErr is Wrap for functions that return only an error.
func WrapErr(ctx context.Context, name string, fn func(context.Context) error) error {
	_, err := Wrap(ctx, name, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}
```

- [ ] **Step 4: Add the new dependencies to go.mod**

Run: `go mod tidy`
Expected: `go.opentelemetry.io/otel/sdk` and `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` are added as new direct dependencies (they weren't in `go.mod` at all before — unlike Phase 1/2, this isn't just flipping an `// indirect` marker). `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/trace`, `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` (used later in Tasks 7-8) move from indirect to direct or gain version-bump adjustments as needed. `go.sum` grows with the new transitive dependency tree (`google.golang.org/grpc`, protobuf codegen packages, etc. — the OTLP exporter's dependency tree, even for the HTTP-only exporter, brings in some of these transitively). Check `go build ./...` still succeeds afterward.

- [ ] **Step 5: Run the tests**

Run: `go test ./core/tracing/... -v`
Expected: PASS (all 6 tests)

- [ ] **Step 6: Commit**

```bash
git add core/tracing/ go.mod go.sum
git commit -m "feat: add core/tracing package with Init, Wrap, WrapErr

Init builds a real OTel SDK TracerProvider + OTLP/HTTP exporter from
standard OTEL_* env vars when OTEL_EXPORTER_OTLP_ENDPOINT is set;
otherwise it's a no-op and the API's built-in no-op tracer stays in
effect. Wrap/WrapErr are generic span-start/record-error/end helpers
every decorator in this plan builds on."
```

---

### Task 2: `core/tracing` — `WrapPlatformAdapter`, `WrapScheduler`

**Files:**
- Create: `core/tracing/scheduler.go`
- Test: `core/tracing/scheduler_test.go`

**Interfaces:**
- Consumes: `Wrap`, `WrapErr` from Task 1; `scheduler.PlatformAdapter`, `scheduler.Scheduler` from `core/scheduler` (unchanged, already-existing interfaces: `PlatformAdapter` has `SpawnAgent(ctx, *agent.Spec) (string, error)`, `TerminateAgent(ctx, string) error`, `WatchAgent(ctx, string) (<-chan agent.Event, error)`; `Scheduler` has `Schedule(ctx, *agent.Job) error`, `Cancel(ctx, string) error`, `Status(ctx, string) (*agent.Status, error)`, `ListActive(ctx, string) ([]*agent.Job, error)`).
- Produces:
  - `func WrapPlatformAdapter(inner scheduler.PlatformAdapter) scheduler.PlatformAdapter`
  - `func WrapScheduler(inner scheduler.Scheduler) scheduler.Scheduler`

- [ ] **Step 1: Write the failing tests**

Create `core/tracing/scheduler_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package tracing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/tracing"
)

type stubAdapter struct {
	spawnErr error
}

func (s *stubAdapter) SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error) {
	if s.spawnErr != nil {
		return "", s.spawnErr
	}
	return "agent-1", nil
}

func (s *stubAdapter) TerminateAgent(ctx context.Context, agentID string) error {
	return nil
}

func (s *stubAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event)
	close(ch)
	return ch, nil
}

func TestWrapPlatformAdapterSpawnAgent(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapPlatformAdapter(&stubAdapter{})
	id, err := wrapped.SpawnAgent(context.Background(), &agent.Spec{})
	if err != nil || id != "agent-1" {
		t.Fatalf("SpawnAgent: got (%q, %v), want (\"agent-1\", nil)", id, err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "PlatformAdapter.SpawnAgent" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "PlatformAdapter.SpawnAgent")
	}
}

func TestWrapPlatformAdapterSpawnAgentError(t *testing.T) {
	sr := withRecorder(t)

	wantErr := errors.New("spawn failed")
	wrapped := tracing.WrapPlatformAdapter(&stubAdapter{spawnErr: wantErr})
	_, err := wrapped.SpawnAgent(context.Background(), &agent.Spec{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SpawnAgent returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

func TestWrapPlatformAdapterTerminateAndWatch(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapPlatformAdapter(&stubAdapter{})
	if err := wrapped.TerminateAgent(context.Background(), "agent-1"); err != nil {
		t.Fatalf("TerminateAgent: %v", err)
	}
	if _, err := wrapped.WatchAgent(context.Background(), "agent-1"); err != nil {
		t.Fatalf("WatchAgent: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	names := map[string]bool{spans[0].Name(): true, spans[1].Name(): true}
	if !names["PlatformAdapter.TerminateAgent"] || !names["PlatformAdapter.WatchAgent"] {
		t.Errorf("unexpected span names: %v, %v", spans[0].Name(), spans[1].Name())
	}
}

type stubScheduler struct {
	scheduleErr error
}

func (s *stubScheduler) Schedule(ctx context.Context, job *agent.Job) error {
	return s.scheduleErr
}

func (s *stubScheduler) Cancel(ctx context.Context, jobID string) error {
	return nil
}

func (s *stubScheduler) Status(ctx context.Context, jobID string) (*agent.Status, error) {
	return &agent.Status{}, nil
}

func (s *stubScheduler) ListActive(ctx context.Context, tenantID string) ([]*agent.Job, error) {
	return nil, nil
}

func TestWrapSchedulerSchedule(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapScheduler(&stubScheduler{})
	if err := wrapped.Schedule(context.Background(), &agent.Job{}); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "Scheduler.Schedule" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "Scheduler.Schedule")
	}
}

func TestWrapSchedulerAllMethodsProduceSpans(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapScheduler(&stubScheduler{})
	_ = wrapped.Cancel(context.Background(), "job-1")
	_, _ = wrapped.Status(context.Background(), "job-1")
	_, _ = wrapped.ListActive(context.Background(), "tenant-1")

	spans := sr.Ended()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}
}
```

If `agent.Spec`, `agent.Event`, `agent.Job`, `agent.Status` field/type shapes above don't match `core/agent`'s actual definitions closely enough to compile as zero-value struct literals (`&agent.Spec{}`, `&agent.Job{}`), read `core/agent`'s type definitions first and adjust — these are used here only as opaque pass-through values (the decorator never inspects their fields), so any valid zero-value construction works.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./core/tracing/... -run TestWrapPlatformAdapter -v` and `go test ./core/tracing/... -run TestWrapScheduler -v`
Expected: FAIL — `WrapPlatformAdapter`/`WrapScheduler` are undefined.

- [ ] **Step 3: Implement the decorators**

Create `core/tracing/scheduler.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package tracing

import (
	"context"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
)

type tracedPlatformAdapter struct {
	inner scheduler.PlatformAdapter
}

// WrapPlatformAdapter returns a scheduler.PlatformAdapter that records a
// span around every call to inner.
func WrapPlatformAdapter(inner scheduler.PlatformAdapter) scheduler.PlatformAdapter {
	return &tracedPlatformAdapter{inner: inner}
}

func (t *tracedPlatformAdapter) SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error) {
	return Wrap(ctx, "PlatformAdapter.SpawnAgent", func(ctx context.Context) (string, error) {
		return t.inner.SpawnAgent(ctx, spec)
	})
}

func (t *tracedPlatformAdapter) TerminateAgent(ctx context.Context, agentID string) error {
	return WrapErr(ctx, "PlatformAdapter.TerminateAgent", func(ctx context.Context) error {
		return t.inner.TerminateAgent(ctx, agentID)
	})
}

func (t *tracedPlatformAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	return Wrap(ctx, "PlatformAdapter.WatchAgent", func(ctx context.Context) (<-chan agent.Event, error) {
		return t.inner.WatchAgent(ctx, agentID)
	})
}

type tracedScheduler struct {
	inner scheduler.Scheduler
}

// WrapScheduler returns a scheduler.Scheduler that records a span around
// every call to inner.
func WrapScheduler(inner scheduler.Scheduler) scheduler.Scheduler {
	return &tracedScheduler{inner: inner}
}

func (t *tracedScheduler) Schedule(ctx context.Context, job *agent.Job) error {
	return WrapErr(ctx, "Scheduler.Schedule", func(ctx context.Context) error {
		return t.inner.Schedule(ctx, job)
	})
}

func (t *tracedScheduler) Cancel(ctx context.Context, jobID string) error {
	return WrapErr(ctx, "Scheduler.Cancel", func(ctx context.Context) error {
		return t.inner.Cancel(ctx, jobID)
	})
}

func (t *tracedScheduler) Status(ctx context.Context, jobID string) (*agent.Status, error) {
	return Wrap(ctx, "Scheduler.Status", func(ctx context.Context) (*agent.Status, error) {
		return t.inner.Status(ctx, jobID)
	})
}

func (t *tracedScheduler) ListActive(ctx context.Context, tenantID string) ([]*agent.Job, error) {
	return Wrap(ctx, "Scheduler.ListActive", func(ctx context.Context) ([]*agent.Job, error) {
		return t.inner.ListActive(ctx, tenantID)
	})
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./core/tracing/... -v`
Expected: PASS (all tests from Task 1 and Task 2)

- [ ] **Step 5: Commit**

```bash
git add core/tracing/scheduler.go core/tracing/scheduler_test.go
git commit -m "feat: add WrapPlatformAdapter and WrapScheduler tracing decorators

Both scheduler.PlatformAdapter and scheduler.Scheduler are narrow
interfaces already platform-agnostic (core/agent types only), so a
generic decorator can wrap either implementation with a span per
method, applied once at construction time."
```

---

### Task 3: `platform/k8s/tracing` — `WrapReconciler`, wired into all 3 reconcilers

**Files:**
- Create: `platform/k8s/tracing/tracing.go`
- Test: `platform/k8s/tracing/tracing_test.go`
- Modify: `platform/k8s/reconcilers/tenant_reconciler.go`
- Modify: `platform/k8s/reconcilers/agentjob_reconciler.go`
- Modify: `platform/k8s/reconcilers/agentfleet_reconciler.go`

**Interfaces:**
- Consumes: `core/tracing.Wrap` from Task 1.
- Produces: `func WrapReconciler(name string, r reconcile.Reconciler) reconcile.Reconciler`

This decorator needs `sigs.k8s.io/controller-runtime/pkg/reconcile`, so it lives under `platform/k8s/`, not `core/tracing` — see Global Constraints.

- [ ] **Step 1: Write the failing test**

Create `platform/k8s/tracing/tracing_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package tracing_test

import (
	"context"
	"errors"
	"testing"

	k8stracing "github.com/muto-io/muto/platform/k8s/tracing"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func withRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

type stubReconciler struct {
	err error
}

func (s *stubReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, s.err
}

func TestWrapReconcilerRecordsSpan(t *testing.T) {
	sr := withRecorder(t)

	wrapped := k8stracing.WrapReconciler("tenant", &stubReconciler{})
	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "tenant" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "tenant")
	}
}

func TestWrapReconcilerRecordsError(t *testing.T) {
	sr := withRecorder(t)

	wantErr := errors.New("reconcile failed")
	wrapped := k8stracing.WrapReconciler("agentjob", &stubReconciler{err: wantErr})
	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reconcile returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

// WrapReconciler must satisfy reconcile.Reconciler so it can be passed
// straight to controller-runtime's Complete(...) - this is a compile-time
// check, not a runtime assertion.
var _ = func() {
	ctrl.NewControllerManagedBy(nil) // referenced only to keep the ctrl import used if needed
}
```

(The final `var _ = func() { ... }` block exists only to keep an unused-import compiler error from surfacing if you don't otherwise reference `ctrl` in this file — if your Go tooling already flags `ctrl` as unused without it, delete both the block and the `ctrl "sigs.k8s.io/controller-runtime"` import line; it's not load-bearing for the test.)

- [ ] **Step 2: Run it to verify it fails to compile**

Run: `go test ./platform/k8s/tracing/... -v`
Expected: FAIL — the `platform/k8s/tracing` package doesn't exist yet.

- [ ] **Step 3: Implement the decorator**

Create `platform/k8s/tracing/tracing.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package tracing

import (
	"context"

	"github.com/muto-io/muto/core/tracing"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type tracedReconciler struct {
	name  string
	inner reconcile.Reconciler
}

// WrapReconciler returns a reconcile.Reconciler that records a span named
// name around every call to inner.Reconcile. Apply it at the
// SetupWithManager call site (wrapping the argument to Complete(...)), not
// inside the reconciler's own Reconcile method.
func WrapReconciler(name string, inner reconcile.Reconciler) reconcile.Reconciler {
	return &tracedReconciler{name: name, inner: inner}
}

func (t *tracedReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return tracing.Wrap(ctx, t.name, func(ctx context.Context) (reconcile.Result, error) {
		return t.inner.Reconcile(ctx, req)
	})
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./platform/k8s/tracing/... -v`
Expected: PASS (both tests)

- [ ] **Step 5: Wire the decorator into all 3 reconcilers**

In `platform/k8s/reconcilers/tenant_reconciler.go`, add `"github.com/muto-io/muto/platform/k8s/tracing"` to the import block, then change:

```go
func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Tenant{}).
		Complete(r)
}
```

to:

```go
func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Tenant{}).
		Complete(tracing.WrapReconciler("TenantReconciler.Reconcile", r))
}
```

In `platform/k8s/reconcilers/agentjob_reconciler.go`, add the same import, then change:

```go
func (r *AgentJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentJob{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
```

to:

```go
func (r *AgentJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentJob{}).
		Owns(&corev1.Pod{}).
		Complete(tracing.WrapReconciler("AgentJobReconciler.Reconcile", r))
}
```

In `platform/k8s/reconcilers/agentfleet_reconciler.go`, add the same import, then change:

```go
func (r *AgentFleetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentFleet{}).
		Complete(r)
}
```

to:

```go
func (r *AgentFleetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentFleet{}).
		Complete(tracing.WrapReconciler("AgentFleetReconciler.Reconcile", r))
}
```

No other line in any of the three files changes — `Reconcile`'s own body, `reconcilePending`/`reconcileRunning`/etc., and every other method are untouched.

- [ ] **Step 6: Run the full reconciler and tracing test suites**

Run: `go test ./platform/k8s/... -v`
Expected: PASS — every existing reconciler test still passes (they construct `&TenantReconciler{...}` directly and call `.Reconcile(...)` on it, bypassing `SetupWithManager`/the decorator entirely, so they're unaffected by this change) plus the two new `platform/k8s/tracing` tests.

- [ ] **Step 7: Commit**

```bash
git add platform/k8s/tracing/ platform/k8s/reconcilers/tenant_reconciler.go platform/k8s/reconcilers/agentjob_reconciler.go platform/k8s/reconcilers/agentfleet_reconciler.go
git commit -m "feat: add WrapReconciler tracing decorator, wire into all 3 reconcilers

Wraps at the SetupWithManager/Complete(...) call site, not inside
Reconcile's body - controller-runtime's reconcile.Reconciler is a
one-method interface, so a decorator there needs no reconciler-specific
code and doesn't touch existing test coverage (those tests call
Reconcile directly on the unwrapped struct)."
```

---

### Task 4: `mcp/server` — `WrapToolHandler`, wired into `registerTools`

**Files:**
- Create: `mcp/server/tracing.go`
- Test: `mcp/server/tracing_test.go`
- Modify: `mcp/server/server.go`

**Interfaces:**
- Consumes: `core/tracing.Wrap` from Task 1; `mcpserver.ToolHandlerFunc` (`func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`, from `github.com/mark3labs/mcp-go/server`, already a direct dependency).
- Produces: `func WrapToolHandler(name string, h mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc`

This lives in `mcp/server` (not `core/tracing`) since it's the one caller of it, and depending on the MCP protocol library from a generic tracing package would be the wrong direction — `mcp/server` already depends on both `core/tracing` (transitively fine) and `mcp-go`.

- [ ] **Step 1: Write the failing test**

Create `mcp/server/tracing_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package server

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func withRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

func TestWrapToolHandlerRecordsSpan(t *testing.T) {
	sr := withRecorder(t)

	handler := WrapToolHandler("mcp.test_tool", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})

	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil || result == nil {
		t.Fatalf("handler: got (%v, %v), want a non-nil result and nil error", result, err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "mcp.test_tool" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "mcp.test_tool")
	}
}

func TestWrapToolHandlerRecordsError(t *testing.T) {
	sr := withRecorder(t)

	wantErr := errors.New("tool failed")
	handler := WrapToolHandler("mcp.failing_tool", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, wantErr
	})

	_, err := handler(context.Background(), mcp.CallToolRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("handler returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}
```

- [ ] **Step 2: Run it to verify it fails to compile**

Run: `go test ./mcp/server/... -v`
Expected: FAIL — `WrapToolHandler` is undefined.

- [ ] **Step 3: Implement the decorator**

Create `mcp/server/tracing.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/muto-io/muto/core/tracing"
)

// WrapToolHandler returns a ToolHandlerFunc that records a span named name
// around every call to h.
func WrapToolHandler(name string, h mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return tracing.Wrap(ctx, name, func(ctx context.Context) (*mcp.CallToolResult, error) {
			return h(ctx, req)
		})
	}
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./mcp/server/... -v`
Expected: PASS (both tests)

- [ ] **Step 5: Wire the decorator into `registerTools`**

In `mcp/server/server.go`, change:

```go
func (s *MutoMCPServer) registerTools() {
	s.srv.AddTool(scheduleAgentJobTool(), s.handleScheduleAgentJob)
	s.srv.AddTool(getJobStatusTool(), s.handleGetJobStatus)
	s.srv.AddTool(cancelJobTool(), s.handleCancelJob)
	s.srv.AddTool(listActiveAgentsTool(), s.handleListActiveAgents)
	s.srv.AddTool(describeTenantTool(), s.handleDescribeTenant)
}
```

to:

```go
func (s *MutoMCPServer) registerTools() {
	s.srv.AddTool(scheduleAgentJobTool(), WrapToolHandler("mcp.schedule_agent_job", s.handleScheduleAgentJob))
	s.srv.AddTool(getJobStatusTool(), WrapToolHandler("mcp.get_job_status", s.handleGetJobStatus))
	s.srv.AddTool(cancelJobTool(), WrapToolHandler("mcp.cancel_job", s.handleCancelJob))
	s.srv.AddTool(listActiveAgentsTool(), WrapToolHandler("mcp.list_active_agents", s.handleListActiveAgents))
	s.srv.AddTool(describeTenantTool(), WrapToolHandler("mcp.describe_tenant", s.handleDescribeTenant))
}
```

No handler method body changes.

- [ ] **Step 6: Run the full mcp test suite**

Run: `go test ./mcp/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add mcp/server/tracing.go mcp/server/tracing_test.go mcp/server/server.go
git commit -m "feat: add WrapToolHandler tracing decorator, wire into registerTools

Wraps at tool registration, not inside each handler body."
```

---

### Task 5: Wire tracing into `cmd/muto-operator`

**Files:**
- Modify: `cmd/muto-operator/main.go`

**Interfaces:**
- Consumes: `core/tracing.Init`, `core/tracing.WrapPlatformAdapter` from Tasks 1-2. `platform/k8s/tracing.WrapReconciler` is already applied inside each reconciler's `SetupWithManager` (Task 3) — nothing further needed here for reconcilers.

This task has no new tests of its own — `Init`/`WrapPlatformAdapter` are already unit-tested (Tasks 1-2); this task is wiring, verified by the existing `cmd/muto-operator` test suite continuing to pass and a manual build/vet check.

- [ ] **Step 1: Wire `Init`, shutdown, and `WrapPlatformAdapter` into `main()`**

In `cmd/muto-operator/main.go`, add `"context"`, `"time"`, and `"github.com/muto-io/muto/core/tracing"` to the import block. Change the start of `main()` from:

```go
func main() {
	ctrl.SetLogger(stdr.New(log.Default()))
	log := ctrl.Log.WithName("muto-operator")

	mgr, err := newManager(ctrl.GetConfigOrDie(), ":8080", ":8081")
	if err != nil {
		log.Error(err, "unable to start manager")
		os.Exit(1)
	}
```

to:

```go
func main() {
	ctrl.SetLogger(stdr.New(log.Default()))
	log := ctrl.Log.WithName("muto-operator")

	shutdownTracing, err := tracing.Init(context.Background(), "muto-operator")
	if err != nil {
		log.Error(err, "unable to initialize tracing")
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			log.Error(err, "tracing shutdown failed")
		}
	}()

	mgr, err := newManager(ctrl.GetConfigOrDie(), ":8080", ":8081")
	if err != nil {
		log.Error(err, "unable to start manager")
		os.Exit(1)
	}
```

(`err` is declared once via `:=` in the `tracing.Init` line; the later `mgr, err := newManager(...)` line still compiles, since `mgr` is a new variable in that `:=`.)

Then change:

```go
		platformAdapter = k8sadapter.NewK8sAdapter(c, namespace)
```

to:

```go
		platformAdapter = tracing.WrapPlatformAdapter(k8sadapter.NewK8sAdapter(c, namespace))
```

and change:

```go
		platformAdapter = cfplatform.NewCFAdapter(cfClient, cfplatform.CFAdapterConfig{
			IsolationTier: os.Getenv("CF_ISOLATION_TIER"),
			SharedOrgName: os.Getenv("CF_SHARED_ORG"),
		})
```

to:

```go
		platformAdapter = tracing.WrapPlatformAdapter(cfplatform.NewCFAdapter(cfClient, cfplatform.CFAdapterConfig{
			IsolationTier: os.Getenv("CF_ISOLATION_TIER"),
			SharedOrgName: os.Getenv("CF_SHARED_ORG"),
		}))
```

No other line in the file changes — reconciler `SetupWithManager` calls are untouched here (already wrapped in Task 3), and `mgr.Start(...)` at the end is unchanged.

- [ ] **Step 2: Build and run the existing test suite**

Run: `go build ./cmd/muto-operator/... && go vet ./cmd/muto-operator/... && go test ./cmd/muto-operator/... -v`
Expected: clean build, clean vet, all existing tests still PASS (`TestManagerServesHealthProbes` etc. — none of them touch `main()`, only `newManager`, which this task didn't change).

- [ ] **Step 3: Commit**

```bash
git add cmd/muto-operator/main.go
git commit -m "feat: wire OpenTelemetry tracing into muto-operator

Calls tracing.Init early in main(), defers a bounded-timeout shutdown,
and wraps the constructed platformAdapter (both the k8s and cf cases)
with tracing.WrapPlatformAdapter. Reconciler spans are already applied
inside each reconciler's SetupWithManager."
```

---

### Task 6: Wire tracing into `cmd/muto-mcp`

**Files:**
- Modify: `cmd/muto-mcp/main.go`

**Interfaces:**
- Consumes: `core/tracing.Init`, `core/tracing.WrapPlatformAdapter`, `core/tracing.WrapScheduler` from Tasks 1-2. `mcp/server.WrapToolHandler` is already applied inside `registerTools()` (Task 4) — nothing further needed here for MCP tools.

- [ ] **Step 1: Wire `Init`, shutdown, `WrapPlatformAdapter`, and `WrapScheduler` into `main()`**

In `cmd/muto-mcp/main.go`, add `"context"`, `"time"`, and `"github.com/muto-io/muto/core/tracing"` to the import block. Change:

```go
func main() {
	ctrl.SetLogger(stdr.New(log.Default()))
	log := ctrl.Log.WithName("muto-mcp")

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cfg := ctrl.GetConfigOrDie()
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "unable to create k8s client")
		os.Exit(1)
	}

	namespace := os.Getenv("MUTO_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	adapter := k8sadapter.NewK8sAdapter(c, namespace)
	sched := scheduler.NewDefaultScheduler(adapter)
	srv := server.New(sched)

	log.Info("starting muto-mcp server (stdio)")
	if err := srv.ServeStdio(); err != nil {
		log.Error(err, "mcp server exited")
		os.Exit(1)
	}
}
```

to:

```go
func main() {
	ctrl.SetLogger(stdr.New(log.Default()))
	log := ctrl.Log.WithName("muto-mcp")

	shutdownTracing, err := tracing.Init(context.Background(), "muto-mcp")
	if err != nil {
		log.Error(err, "unable to initialize tracing")
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			log.Error(err, "tracing shutdown failed")
		}
	}()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	cfg := ctrl.GetConfigOrDie()
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "unable to create k8s client")
		os.Exit(1)
	}

	namespace := os.Getenv("MUTO_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	adapter := tracing.WrapPlatformAdapter(k8sadapter.NewK8sAdapter(c, namespace))
	sched := tracing.WrapScheduler(scheduler.NewDefaultScheduler(adapter))
	srv := server.New(sched)

	log.Info("starting muto-mcp server (stdio)")
	if err := srv.ServeStdio(); err != nil {
		log.Error(err, "mcp server exited")
		os.Exit(1)
	}
}
```

(`err` from the `tracing.Init` call is reused by the later `c, err := client.New(...)` line via `:=`, valid since `c` is new.)

- [ ] **Step 2: Build and run**

Run: `go build ./cmd/muto-mcp/... && go vet ./cmd/muto-mcp/...`
Expected: clean (this package currently has no test files, matching its state going into this plan).

- [ ] **Step 3: Commit**

```bash
git add cmd/muto-mcp/main.go
git commit -m "feat: wire OpenTelemetry tracing into muto-mcp

Calls tracing.Init early in main(), defers a bounded-timeout shutdown,
and wraps both the platform adapter and the DefaultScheduler with
their tracing decorators. MCP tool spans are already applied inside
registerTools()."
```

---

### Task 7: `otelhttp` on the A2A HTTP client

**Files:**
- Modify: `core/a2a/client.go`
- Create: `core/a2a/client_internal_test.go` (the existing `client_test.go` is `package a2a_test`, an external test package with no access to the unexported `httpClient` field this test needs to inspect — a new internal test file, `package a2a`, sits alongside it; Go allows both in the same directory)

**Interfaces:**
- No new Go interfaces produced. Consumes `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` directly (already an indirect dependency, becomes direct).

- [ ] **Step 1: Write the failing test**

Create `core/a2a/client_internal_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package a2a

import (
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func TestNewClientUsesOtelhttpTransport(t *testing.T) {
	c, err := New(&Config{GatewayURL: "http://example.invalid"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := c.httpClient.Transport.(*otelhttp.Transport); !ok {
		t.Errorf("httpClient.Transport = %T, want *otelhttp.Transport", c.httpClient.Transport)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./core/a2a/... -run TestNewClientUsesOtelhttpTransport -v`
Expected: FAIL — `c.httpClient.Transport` is `nil` (the zero value of `http.Client{}`, which defaults to `http.DefaultTransport` only when actually used, not as an observable non-nil field), so the type assertion fails.

- [ ] **Step 3: Wrap the client's Transport**

In `core/a2a/client.go`, add `"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"` to the imports, then change:

```go
	return &A2AClient{
		gatewayURL: cfg.GatewayURL,
		authToken:  cfg.AuthToken,
		httpClient: &http.Client{},
	}, nil
```

to:

```go
	return &A2AClient{
		gatewayURL: cfg.GatewayURL,
		authToken:  cfg.AuthToken,
		httpClient: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}, nil
```

No other line in the file changes — `SendTask`/`GetTaskStatus`/`doRequest` already build requests via `http.NewRequestWithContext(ctx, ...)`, so the active span in `ctx` (if any) propagates as a W3C `traceparent` header automatically; when tracing is disabled (no-op tracer), `otelhttp.NewTransport` adds negligible overhead and no propagation header (a no-op span has an invalid `SpanContext`, which `otelhttp`'s propagator correctly skips).

- [ ] **Step 4: Run the tests**

Run: `go test ./core/a2a/... -v`
Expected: PASS (all existing tests plus the new one)

- [ ] **Step 5: Commit**

```bash
git add core/a2a/client.go core/a2a/client_internal_test.go
git commit -m "feat: propagate trace context on the A2A HTTP client

Wraps A2AClient's http.Client.Transport with otelhttp.NewTransport so
W3C traceparent headers propagate into agent gateways when tracing is
enabled; a no-op when it isn't."
```

---

### Task 8: `otelhttp` on the CF HTTP client

**Files:**
- Modify: `platform/cf/client.go`
- Create: `platform/cf/client_test.go` (doesn't exist yet — `platform/cf/adapter_test.go` tests `CFAdapter` against a mock `CFClient`, but nothing today tests `NewRealCFClient`/`client.go` directly)

**Interfaces:**
- No new Go interfaces produced. Consumes `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` (same as Task 7) and `go-cfclient/v3/config`'s existing `HttpClient(*http.Client) Option` and `Config.HTTPClient() *http.Client` getter (confirmed present via `go doc github.com/cloudfoundry/go-cfclient/v3/config.Config`).

- [ ] **Step 1: Write the failing test**

`NewRealCFClient` builds a `*config.Config` internally but doesn't expose it (it's consumed by `cfclient.New(cfg)` and discarded), so the strongest direct test targets the mechanism `NewRealCFClient` will rely on: `config.New` with the `config.HttpClient` option, verified via `Config.HTTPClient()`. Create `platform/cf/client_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0
package cf_test

import (
	"net/http"
	"testing"

	"github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/muto-io/muto/platform/cf"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func TestNewRealCFClientConstructsSuccessfully(t *testing.T) {
	// config.New validates apiURL eagerly but doesn't dial it, so a
	// syntactically valid, unreachable URL is enough to construct the client.
	_, err := cf.NewRealCFClient("https://example.invalid", "user", "pass")
	if err != nil {
		t.Fatalf("NewRealCFClient: %v", err)
	}
}

func TestConfigHttpClientOptionCarriesOtelhttpTransport(t *testing.T) {
	// NewRealCFClient passes an otelhttp-wrapped *http.Client into
	// config.New via this exact option; this test verifies the option
	// itself (and the library's HTTPClient() getter) behaves as
	// NewRealCFClient's implementation relies on.
	wrapped := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	cfg, err := config.New("https://example.invalid", config.UserPassword("user", "pass"),
		config.HttpClient(wrapped))
	if err != nil {
		t.Fatalf("config.New: %v", err)
	}
	if _, ok := cfg.HTTPClient().Transport.(*otelhttp.Transport); !ok {
		t.Errorf("cfg.HTTPClient().Transport = %T, want *otelhttp.Transport", cfg.HTTPClient().Transport)
	}
}
```

- [ ] **Step 2: Run both tests to confirm they already pass**

Run: `go test ./platform/cf/... -run 'TestNewRealCFClientConstructsSuccessfully|TestConfigHttpClientOptionCarriesOtelhttpTransport' -v`
Expected: PASS, both. Neither test depends on Step 3's change: `TestConfigHttpClientOptionCarriesOtelhttpTransport` exercises the `go-cfclient/v3` library's own option/getter mechanism directly (not `NewRealCFClient`), confirming the library behaves as `NewRealCFClient` is about to rely on; `TestNewRealCFClientConstructsSuccessfully` exercises existing, unchanged behavior. This is a regression-guard pair, not a red/green TDD cycle — there is nothing to make fail-then-pass here, since `NewRealCFClient` doesn't expose its internal config for direct inspection.

- [ ] **Step 3: Pass an otelhttp-wrapped client via `config.HttpClient`**

In `platform/cf/client.go`, add `"net/http"` and `"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"` to the imports, then change:

```go
	cfg, err := config.New(apiURL, config.UserPassword(username, password))
	if err != nil {
		return nil, fmt.Errorf("cf config: %w", err)
	}
```

to:

```go
	cfg, err := config.New(apiURL, config.UserPassword(username, password),
		config.HttpClient(&http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}))
	if err != nil {
		return nil, fmt.Errorf("cf config: %w", err)
	}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./platform/cf/... -v`
Expected: PASS (all existing tests plus the two new ones)

- [ ] **Step 5: Commit**

```bash
git add platform/cf/client.go platform/cf/client_test.go
git commit -m "feat: propagate trace context on the CF HTTP client

Passes an otelhttp-wrapped http.Client into go-cfclient/v3's
config.HttpClient option, so W3C traceparent headers propagate to the
CF API when tracing is enabled."
```

---

## Self-Review

**Spec coverage:**
- `core/tracing`'s `Init`/`Wrap`/`WrapErr` (spec §2) → Task 1, with the package-location refinement (§ Global Constraints) for `WrapReconciler` specifically.
- Decorator table (spec §3) → Tasks 2 (adapter/scheduler), 3 (reconciler), 4 (MCP tools).
- HTTP client propagation (spec §4) → Tasks 7-8.
- Both binaries call `Init`/shutdown (spec §5) → Tasks 5-6.
- Testing approach (spec §6) → covered inline in every task (`tracetest.SpanRecorder`-based unit tests for the package and every decorator; existing reconciler/adapter/scheduler/MCP tests confirmed unmodified and still passing since decorators apply at wiring points, not inside tested method bodies).
- All acceptance criteria are exercised: no-op-when-unset and provider-built-when-set are both directly tested in Task 1; per-surface span creation is tested in Tasks 1-4; `OTEL_SERVICE_NAME` override behavior is implemented via `resource.WithFromEnv()` layered after the default (verified against the real API before writing this plan, not assumed); HTTP propagation is wired in Tasks 7-8; unmodified existing tests are explicitly called out as a expected/checked outcome in Tasks 3-4's steps.

**Placeholder scan:** No TBD/TODO markers. Every code block is complete, and the trickier APIs (`Init`'s no-op/real-provider split, the global-tracer-provider delegation the tests rely on, `WrapReconciler`/`WrapToolHandler` against the real `reconcile.Reconciler`/`ToolHandlerFunc` interfaces, `tracetest.SpanRecorder`'s exact method names) were verified by compiling and running standalone scratch programs against the real dependency versions before being written into this plan, not assumed from documentation alone. Two steps (Task 7 Step 1, Task 8 Step 1) explicitly tell the implementer to read an existing test file first and adapt if the exact package name (`a2a` vs `a2a_test`) or library capability (whether `go-cfclient`'s `Client` exposes its transport) differs from what's assumed here — this is a deliberate, bounded escape hatch for the one or two details this plan couldn't verify without the actual file/library internals in hand, not a placeholder for missing design work.

**Type consistency:** `Wrap[T any](ctx, name, fn func(context.Context) (T, error)) (T, error)` and `WrapErr(ctx, name, fn func(context.Context) error) error` are defined once in Task 1 and used with matching signatures in every later task. `WrapPlatformAdapter`/`WrapScheduler` (Task 2), `WrapReconciler` (Task 3), and `WrapToolHandler` (Task 4) all delegate to exactly these two functions, never duplicating span-start/record-error/end logic. Span-name-string conventions (`"<Type>.<Method>"`, `"mcp.<tool>"`) are applied consistently across Tasks 3-4's wiring steps.
