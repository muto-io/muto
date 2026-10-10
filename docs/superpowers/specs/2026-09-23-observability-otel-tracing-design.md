# Observability Phase 3: OpenTelemetry Tracing

**Status:** Approved, ready for implementation planning
**Issue:** [#79](https://github.com/muto-io/muto/issues/79) (Phase 3 — OpenTelemetry tracing)
**Scope:** Third of several independent slices of #79. Phase 1 ([#102](https://github.com/muto-io/muto/pull/102), bind addresses + structured logging) and Phase 2 ([#103](https://github.com/muto-io/muto/pull/103), Prometheus metrics) are separate, unmerged branches — this branch is cut from `main` independently of both and does not depend on either merging first. The remaining docs sweep (Phase 4) is a separate spec.

## Problem

The docs promise per-job trace trees and `MUTO_OTEL_*` configuration. None of it exists: no production code imports `go.opentelemetry.io/*`; the OTel API packages present in `go.mod` (`otel`, `otel/trace`, `otel/metric`, the `otelhttp` contrib package) are all `// indirect`, pulled in transitively (via `testcontainers-go`) with no SDK, no exporter, and nothing calling them. No `MUTO_OTEL_*` or `OTEL_*` env var is read anywhere.

## Scope decisions

1. **Standard `OTEL_*` env vars only — no `MUTO_OTEL_*` aliases.** The original issue flagged this as undecided. The OTel Go SDK's OTLP exporters and resource detection already read `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, etc. for free — building a `MUTO_OTEL_*` alias layer would be new code with no benefit over what any OTel-familiar operator already expects.
2. **No separate "enabled" flag.** Tracing turns on purely based on whether `OTEL_EXPORTER_OTLP_ENDPOINT` is set. If unset, `core/tracing.Init` is a no-op — the OTel API's own built-in default tracer is already a no-op until a real `TracerProvider` is installed, so "disabled" costs nothing extra to implement or document.
3. **Full scope now, not a smaller first slice.** Unlike Phase 2's Prometheus metrics (pull/scrape-based, blocked on `cmd/muto-mcp` having no HTTP endpoint), tracing is push-based (OTLP export) — both binaries can export traces independently with no scrape-endpoint dependency. There's no forced technical seam to split this phase on, so it covers everything the issue lists: SDK init in both binaries, reconciler/scheduler/platform-adapter/MCP-tool spans, and HTTP client propagation.
4. **OTLP over HTTP (`otlptracehttp`), not gRPC.** Matches the current OTel specification's default protocol (`http/protobuf`). Dual-protocol selection via `OTEL_EXPORTER_OTLP_PROTOCOL` is not implemented this phase — YAGNI until a concrete need for gRPC transport arises.
5. **Metrics-via-OTLP (the issue's "optional" bullet) is explicitly declined.** Phase 2 already covers metrics via Prometheus; exporting them a second way via OTLP has no identified consumer and is out of scope.

## Design

### 1. New dependency: the OTel SDK and OTLP exporter

`go.opentelemetry.io/otel/sdk` and `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` are genuinely new dependencies (unlike Phase 1's `zap` or Phase 2's `client_golang`, which only needed direct-ifying already-resolved transitive deps). `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/trace`, and `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` also move from indirect to direct.

### 2. `core/tracing` package — foundational primitives

Platform-agnostic, fits `core/`'s existing charter (no `controller-runtime` coupling, unlike Phase 2's `platform/k8s/metrics`):

```go
func Init(ctx context.Context, defaultServiceName string) (shutdown func(context.Context) error, err error)
func Wrap[T any](ctx context.Context, name string, fn func(context.Context) (T, error)) (T, error)
func WrapErr(ctx context.Context, name string, fn func(context.Context) error) error
```

`Init`: if `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, returns a no-op shutdown and does nothing else. If set, builds an `otlptracehttp` exporter, a `sdktrace.TracerProvider` with a `resource.Resource` combining `defaultServiceName` (used unless overridden) with `resource.WithFromEnv()` (so `OTEL_SERVICE_NAME`/`OTEL_RESOURCE_ATTRIBUTES` take precedence when set), and calls `otel.SetTracerProvider(...)`. The returned `shutdown` flushes buffered spans and must be called (e.g. via `defer`) before process exit.

`Wrap`/`WrapErr`: start a span named `name` under a package-level tracer, run `fn`, record any returned error on the span (`span.RecordError` + `span.SetStatus(codes.Error, ...)`), end the span, return `fn`'s result unchanged. Generic over the success type so it fits every call site's signature in this codebase — reconcilers (`(ctrl.Result, error)`), adapters (`(string, error)`, `(<-chan agent.Event, error)`), scheduler (`error`-only for `Schedule`/`Cancel`, `(*agent.Status, error)` for `Status`), and MCP tool handlers (`(*mcp.CallToolResult, error)`) — without `core/tracing` depending on any of those packages' concrete types.

### 3. Decorator-based instrumentation (not method-body edits)

Every surface the issue asks to instrument is called through a narrow interface, so each gets a decorator implementing that same interface, applied once at the construction/registration site:

| Surface | Decorator | Wired at |
|---|---|---|
| `TenantReconciler`, `AgentFleetReconciler`, `AgentJobReconciler` | `tracing.WrapReconciler(name string, r reconcile.Reconciler) reconcile.Reconciler` | Each reconciler's `SetupWithManager`, wrapping the argument to `.Complete(...)` |
| `K8sAdapter`, `CFAdapter` | `tracing.WrapPlatformAdapter(inner scheduler.PlatformAdapter) scheduler.PlatformAdapter` | `cmd/muto-operator/main.go` and `cmd/muto-mcp/main.go`, wrapping `platformAdapter` right after construction |
| `DefaultScheduler` | `tracing.WrapScheduler(inner scheduler.Scheduler) scheduler.Scheduler` | `cmd/muto-mcp/main.go`, wrapping `scheduler.NewDefaultScheduler(...)` |
| 5 MCP tool handlers | `tracing.WrapToolHandler(name string, h mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc` | `mcp/server/server.go`'s `registerTools()`, wrapping each `s.handleXxx` passed to `AddTool` |

This touches zero existing `Reconcile`/adapter/scheduler/handler method bodies — only their wiring points — and composes cleanly with Phase 2's `metrics.ObserveReconcile` (which wraps *inside* `Reconcile`'s body) with no conflict whenever both PRs eventually merge. Span names follow `"<Type>.<Method>"` (e.g. `"TenantReconciler.Reconcile"`, `"PlatformAdapter.SpawnAgent"`, `"Scheduler.Schedule"`, `"mcp.schedule_agent_job"`).

### 4. HTTP client trace propagation

- **A2A** (`core/a2a/client.go`): `httpClient: &http.Client{}` → `httpClient: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}`. `SendTask`/`GetTaskStatus` already build requests via `http.NewRequestWithContext(ctx, ...)`, so the active span in `ctx` propagates as a W3C `traceparent` header automatically.
- **CF** (`platform/cf/client.go`): `go-cfclient/v3`'s `config.New(apiURL, config.UserPassword(...), config.HttpClient(&http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}))` — the library's `config.HttpClient(*http.Client) Option` supports this with no vendoring.

### 5. Both binaries call `Init`/shutdown

`cmd/muto-operator/main.go` and `cmd/muto-mcp/main.go` each call `tracing.Init(ctx, "muto-operator")`/`tracing.Init(ctx, "muto-mcp")` early in `main()`, `defer shutdown(ctx)` (with a bounded timeout context for the deferred call, since shutdown flushes over the network), and fail fast (log + `os.Exit(1)`) if `Init` returns an error — matching this repo's established fail-fast convention for bad config (Phase 1's `buildLogger`).

### 6. Testing

- `core/tracing` unit tests: `Init` with the env var unset (no-op) and set (using `go.opentelemetry.io/otel/sdk/trace/tracetest`'s in-memory `SpanRecorder` to assert spans are actually created); `Wrap`/`WrapErr` behavior (span created, error recorded on failure, result passed through unchanged) against a `tracetest`-backed provider.
- Each decorator gets a focused unit test using the same `tracetest.SpanRecorder` pattern: wrap a stub/fake implementation of the relevant interface, call the decorated method, assert exactly one span was recorded with the right name and error status.
- No changes needed to existing reconciler/adapter/scheduler/MCP tests — decorators are applied at the wiring layer, not inside the tested types.

## Acceptance criteria

- [ ] With `OTEL_EXPORTER_OTLP_ENDPOINT` unset (the default), tracing is a true no-op: no exporter goroutine starts, no measurable overhead.
- [ ] With it set, one `AgentJob`'s lifecycle produces a single trace containing the reconcile spans (tenant, agentjob, agentfleet as applicable), platform adapter spans, and (when scheduled via MCP) scheduler and MCP tool spans, correctly nested.
- [ ] `OTEL_SERVICE_NAME` overrides each binary's default service name.
- [ ] The A2A and CF HTTP clients propagate W3C `traceparent` headers when tracing is enabled.
- [ ] Existing reconciler/adapter/scheduler/MCP tests are unmodified and still pass.

## Explicitly out of scope (tracked separately under #79)

- `MUTO_OTEL_*` env var aliases.
- OTLP metrics export (Phase 2's Prometheus metrics already cover this).
- `OTEL_EXPORTER_OTLP_PROTOCOL`-driven gRPC/HTTP selection (HTTP only, this phase).
- The remaining docs sweep (Phase 4).
