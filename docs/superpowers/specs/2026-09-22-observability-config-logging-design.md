# Observability Phase 1: configurable bind addresses & structured logging

**Status:** Approved, ready for implementation planning
**Issue:** [#79](https://github.com/muto-io/muto/issues/79) (Phase 1 remainder — health probe registration was already fixed by #81)
**Scope:** First of several independent slices of #79. Prometheus metrics (Phase 2), OpenTelemetry tracing (Phase 3), and docs cleanup (Phase 4) are separate specs.

## Problem

`cmd/muto-operator/main.go` hardcodes the metrics and health-probe bind
addresses (`":8080"`, `":8081"`) even though the Helm chart's
`metrics.port` / `healthProbe.port` values change the container's exposed
`containerPort`. Setting a non-default port in `values.yaml` changes what
Kubernetes routes to, not what the binary listens on — probes and scrape
targets silently break. `metrics.enabled: false` only removes the
Prometheus scrape annotations; the metrics server still binds.

Both `cmd/muto-operator` and `cmd/muto-mcp` log with
`ctrl.SetLogger(stdr.New(log.Default()))`: plain text, fixed verbosity, no
`MUTO_LOG_LEVEL` / `MUTO_LOG_FORMAT` support despite both being documented
in `docs/operations/monitoring-observability.md` and
`docs/configuration/environment-variables.md`.

## Design

### 1. Bind addresses (`cmd/muto-operator` only)

New env vars, read in `main()` and passed into the existing `newManager`
helper (already parameterized on `metricsAddr, probeAddr` — see
`cmd/muto-operator/main_test.go`'s `TestManagerServesHealthProbes`):

| Env var | Default | Notes |
|---|---|---|
| `MUTO_METRICS_BIND_ADDRESS` | `:8080` | `"0"` disables the metrics server — controller-runtime's own convention, no separate "enabled" flag needed. |
| `MUTO_HEALTH_PROBE_BIND_ADDRESS` | `:8081` | |

`cmd/muto-mcp` has no HTTP metrics/health surface today; adding one is out
of scope for this phase (not called for by the issue's Phase 1 list).

### 2. Structured logging (both binaries)

Replace `stdr` with `sigs.k8s.io/controller-runtime/pkg/log/zap`. This is
already reachable with no new dependency: `go.uber.org/zap` is already in
`go.mod` (currently `// indirect`, via controller-runtime) and becomes
direct once imported.

A `buildLogger(format, level string) (logr.Logger, error)` helper,
colocated in each `main.go` next to `newManager`-style helpers:

| Env var | Values | Default |
|---|---|---|
| `MUTO_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `MUTO_LOG_FORMAT` | `json`, `console` | `json` |

The two are independent `zap.Opts` (`zap.Level(...)`, `zap.Encoder(...)`),
not `zap.UseDevMode(bool)` — dev-mode conflates encoder choice with level
defaults and stacktrace behavior, which would make the two env vars
interact in surprising ways. An unrecognized value for either returns an
error so `main()` fails fast (`log.Error` + `os.Exit(1)`) instead of
silently misconfiguring logging.

### 3. Helm chart (`deploy/helm/muto`)

- `values.yaml`: add `MUTO_LOG_LEVEL: info` and `MUTO_LOG_FORMAT: json` to
  the existing `env:` map. That map is already generically templated into
  the container via `range $key, $val := .Values.env` in
  `deployment-operator.yaml`, so no template change is needed for these two.
- `deployment-operator.yaml`: add two explicit env entries, derived from
  the *same* `.Values.metrics.port` / `.Values.healthProbe.port` /
  `.Values.metrics.enabled` that already drive `containerPort` and the
  scrape annotations — this is what closes the bug:
  ```yaml
  - name: MUTO_METRICS_BIND_ADDRESS
    {{- if .Values.metrics.enabled }}
    value: {{ printf ":%d" (.Values.metrics.port | int) | quote }}
    {{- else }}
    value: "0"
    {{- end }}
  - name: MUTO_HEALTH_PROBE_BIND_ADDRESS
    value: {{ printf ":%d" (.Values.healthProbe.port | int) | quote }}
  ```

**Out of scope:** no `muto-mcp` chart changes (no existing Deployment
template to wire into — it isn't chart-deployed today), no
`deploy/cf/manifest.yml` changes (CF's observability gaps are a separate,
larger item elsewhere in #79, not part of this phase).

### 4. Testing

Follows the existing pattern in `cmd/muto-operator/main_test.go`
(`package main`, real network listeners via a `freeAddr(t)` helper):

- Unit tests for `buildLogger` in both binaries: valid level/format
  combinations produce a working logger; invalid values return an error;
  `json` format actually emits parseable JSON lines (capture output via a
  buffered `zapcore.WriteSyncer` instead of stdout).
- Extend `TestManagerServesHealthProbes` (or add a sibling test) to cover
  `newManager` with `metricsAddr: "0"`, asserting the metrics port is not
  listening.
- A `helm template` smoke check confirming `MUTO_METRICS_BIND_ADDRESS`
  renders `"0"` when `metrics.enabled=false` and `":<port>"` otherwise.

## Acceptance criteria

- [ ] Setting `metrics.port`/`healthProbe.port` in `values.yaml` changes
      both the exposed `containerPort` and what the binary actually binds.
- [ ] `metrics.enabled: false` stops the metrics server from binding at
      all (not just removing scrape annotations).
- [ ] `MUTO_LOG_FORMAT=json` emits JSON lines; `MUTO_LOG_LEVEL=debug`
      increases verbosity. Both binaries.
- [ ] An invalid `MUTO_LOG_LEVEL`/`MUTO_LOG_FORMAT` value fails the
      process at startup with a clear error, rather than silently
      defaulting.
- [ ] Existing `TestManagerServesHealthProbes` and all other
      `cmd/muto-operator`/`cmd/muto-mcp` tests still pass.

## Explicitly out of scope (tracked separately under #79)

- Custom Prometheus `muto_*` metric families (Phase 2).
- OpenTelemetry tracing (Phase 3).
- Updating the 12+ docs pages that describe the not-yet-implemented pieces
  (Phase 4 — should follow once Phases 1-3 land, so docs describe
  reality once rather than in three separate partial passes).
- CloudFoundry observability (separate, larger item in #79).
