# Observability Phase 1: Config & Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the operator's metrics/health bind addresses configurable end-to-end (Helm values → binary), and replace both binaries' plain-text `stdr` logging with structured, level/format-configurable zap logging.

**Architecture:** Two small, independently testable config seams — `newManager(cfg, metricsAddr, probeAddr)` (already exists and tested) gets its hardcoded addresses replaced with env reads; a new `buildLogger(format, level string, out io.Writer) (logr.Logger, error)` function (duplicated per-binary, since `cmd/muto-operator` and `cmd/muto-mcp` are separate `package main`s that can't share code without a new shared package, which is out of scope here) validates and builds a zap-backed `logr.Logger`. The Helm chart's existing generic `env:` map and its `metrics`/`healthProbe` values get wired to the same env vars.

**Tech Stack:** Go, controller-runtime v0.25.1, `sigs.k8s.io/controller-runtime/pkg/log/zap` (wraps `go.uber.org/zap`, already an indirect dependency), Helm.

**Spec:** `docs/superpowers/specs/2026-09-22-observability-config-logging-design.md`

## Global Constraints

- Configuration is env-var only (`MUTO_*` prefix), matching the existing codebase convention — no `flag.*` package usage anywhere in `cmd/`.
- `go.uber.org/zap` is already in `go.mod` as `// indirect` (pulled in transitively by controller-runtime); importing it directly requires `go mod tidy` to flip that marker, but adds no new module.
- JSON is the default log format (`MUTO_LOG_FORMAT=json`); `console` is available for local/dev use. The two knobs (`MUTO_LOG_LEVEL`, `MUTO_LOG_FORMAT`) must be independent — do not use `zap.UseDevMode`, which conflates format with level/stacktrace defaults.
- An invalid `MUTO_LOG_LEVEL` or `MUTO_LOG_FORMAT` value must fail the process at startup with a clear error, not silently default.
- No Helm chart changes for `muto-mcp` (it has no existing Deployment template — not chart-deployed today) or `deploy/cf/manifest.yml` (CF observability is a separate, larger item elsewhere in #79).
- `cmd/muto-operator/main.go`'s existing `newManager(cfg *rest.Config, metricsAddr, probeAddr string) (ctrl.Manager, error)` is the tested seam for bind addresses — thread values through it, don't reimplement its logic.

---

### Task 1: `cmd/muto-operator` — configurable metrics/health bind addresses

**Files:**
- Modify: `cmd/muto-operator/main.go:59` (the `newManager(ctrl.GetConfigOrDie(), ":8080", ":8081")` call and the lines around it)
- Test: `cmd/muto-operator/main_test.go`

**Interfaces:**
- Consumes: existing `newManager(cfg *rest.Config, metricsAddr, probeAddr string) (ctrl.Manager, error)` — unchanged signature.
- Produces: nothing new for other tasks (this task only changes `main()`'s call site).

- [ ] **Step 1: Write a test proving `newManager` honors an arbitrary metrics address**

  `newManager` already accepts `metricsAddr` as a parameter and passes it straight to `metricsserver.Options.BindAddress`, but the only existing test (`TestManagerServesHealthProbes`) always passes `"0"` (disabled) for it, so nothing today proves a real address is actually honored. Add this test to `cmd/muto-operator/main_test.go`, right after `TestManagerServesHealthProbes`:

  ```go
  // TestManagerServesMetrics guards the metrics bind address that
  // MUTO_METRICS_BIND_ADDRESS (and, through it, the Helm chart's
  // values.metrics.port) is supposed to control: newManager must bind the
  // metrics server on the address it's given, not a hardcoded one.
  func TestManagerServesMetrics(t *testing.T) {
  	metricsAddr := freeAddr(t)

  	// No controllers are registered, so the manager never contacts this API server.
  	mgr, err := newManager(&rest.Config{Host: "http://127.0.0.1:1"}, metricsAddr, "0")
  	if err != nil {
  		t.Fatalf("newManager: %v", err)
  	}

  	ctx, cancel := context.WithCancel(context.Background())
  	done := make(chan error, 1)
  	go func() { done <- mgr.Start(ctx) }()
  	t.Cleanup(func() {
  		cancel()
  		if err := <-done; err != nil {
  			t.Errorf("manager exited with error: %v", err)
  		}
  	})

  	if got := getStatus(t, "http://"+metricsAddr+"/metrics"); got != http.StatusOK {
  		t.Errorf("GET /metrics: status %d, want %d", got, http.StatusOK)
  	}
  }
  ```

  This test exercises `newManager`'s existing, already-correct plumbing — it should pass immediately, before any other change in this task. It's a regression guard for Step 2, not a red/green TDD cycle, because the bug being fixed is in `main()`, not in `newManager`.

- [ ] **Step 2: Run it to confirm it already passes**

  Run: `go test ./cmd/muto-operator/... -run TestManagerServesMetrics -v`
  Expected: PASS

- [ ] **Step 3: Replace the hardcoded addresses in `main()` with env reads**

  In `cmd/muto-operator/main.go`, change:

  ```go
  func main() {
  	ctrl.SetLogger(stdr.New(log.Default()))
  	log := ctrl.Log.WithName("muto-operator")

  	mgr, err := newManager(ctrl.GetConfigOrDie(), ":8080", ":8081")
  ```

  to:

  ```go
  func main() {
  	ctrl.SetLogger(stdr.New(log.Default()))
  	log := ctrl.Log.WithName("muto-operator")

  	metricsAddr := os.Getenv("MUTO_METRICS_BIND_ADDRESS")
  	if metricsAddr == "" {
  		metricsAddr = ":8080"
  	}
  	probeAddr := os.Getenv("MUTO_HEALTH_PROBE_BIND_ADDRESS")
  	if probeAddr == "" {
  		probeAddr = ":8081"
  	}

  	mgr, err := newManager(ctrl.GetConfigOrDie(), metricsAddr, probeAddr)
  ```

  (`os` is already imported in this file. Logging is untouched here — Task 2 replaces the `stdr` line.)

- [ ] **Step 4: Run the full package test suite**

  Run: `go test ./cmd/muto-operator/... -v`
  Expected: PASS (`TestManagerServesHealthProbes` and the new `TestManagerServesMetrics` both green)

- [ ] **Step 5: Commit**

  ```bash
  git add cmd/muto-operator/main.go cmd/muto-operator/main_test.go
  git commit -m "feat: make operator metrics/health bind addresses configurable

Reads MUTO_METRICS_BIND_ADDRESS and MUTO_HEALTH_PROBE_BIND_ADDRESS
(defaulting to :8080/:8081), instead of hardcoding them, so the Helm
chart's metrics.port/healthProbe.port values can actually reach the
binary."
  ```

---

### Task 2: `cmd/muto-operator` — structured JSON logging

**Files:**
- Modify: `cmd/muto-operator/main.go` (imports, new `buildLogger` function, `main()`'s logger setup)
- Test: `cmd/muto-operator/main_test.go`

**Interfaces:**
- Produces: `func buildLogger(format, level string, out io.Writer) (logr.Logger, error)` in `package main` (`cmd/muto-operator`). Package-local — `cmd/muto-mcp` gets its own copy in Task 3, not a shared package (see Global Constraints).

- [ ] **Step 1: Write the failing test**

  Add to `cmd/muto-operator/main_test.go`:

  ```go
  func TestBuildLogger(t *testing.T) {
  	t.Run("valid combinations", func(t *testing.T) {
  		for _, level := range []string{"debug", "info", "warn", "error"} {
  			for _, format := range []string{"json", "console"} {
  				t.Run(level+"/"+format, func(t *testing.T) {
  					var buf bytes.Buffer
  					if _, err := buildLogger(format, level, &buf); err != nil {
  						t.Fatalf("buildLogger(%q, %q): %v", format, level, err)
  					}
  				})
  			}
  		}
  	})

  	t.Run("invalid level", func(t *testing.T) {
  		var buf bytes.Buffer
  		if _, err := buildLogger("json", "trace", &buf); err == nil {
  			t.Fatal("expected error for invalid MUTO_LOG_LEVEL, got nil")
  		}
  	})

  	t.Run("invalid format", func(t *testing.T) {
  		var buf bytes.Buffer
  		if _, err := buildLogger("yaml", "info", &buf); err == nil {
  			t.Fatal("expected error for invalid MUTO_LOG_FORMAT, got nil")
  		}
  	})

  	t.Run("json format emits parseable JSON", func(t *testing.T) {
  		var buf bytes.Buffer
  		logger, err := buildLogger("json", "info", &buf)
  		if err != nil {
  			t.Fatalf("buildLogger: %v", err)
  		}
  		logger.Info("test message", "key", "value")

  		line := buf.Bytes()
  		if len(line) == 0 {
  			t.Fatal("expected log output, got none")
  		}
  		var decoded map[string]interface{}
  		if err := json.Unmarshal(line, &decoded); err != nil {
  			t.Fatalf("log line is not valid JSON: %v\nline: %s", err, line)
  		}
  		if decoded["msg"] != "test message" {
  			t.Errorf("msg = %v, want %q", decoded["msg"], "test message")
  		}
  		if decoded["key"] != "value" {
  			t.Errorf("key = %v, want %q", decoded["key"], "value")
  		}
  	})

  	t.Run("error level filters info messages", func(t *testing.T) {
  		var buf bytes.Buffer
  		logger, err := buildLogger("json", "error", &buf)
  		if err != nil {
  			t.Fatalf("buildLogger: %v", err)
  		}
  		logger.Info("should be filtered")
  		if buf.Len() != 0 {
  			t.Errorf("expected no output at error level for an Info call, got: %s", buf.Bytes())
  		}
  	})
  }
  ```

  Add `"bytes"` and `"encoding/json"` to the test file's import block (currently `context`, `net`, `net/http`, `testing`, `time`, `k8s.io/client-go/rest`).

- [ ] **Step 2: Run it to verify it fails to compile**

  Run: `go test ./cmd/muto-operator/... -run TestBuildLogger -v`
  Expected: FAIL with `undefined: buildLogger`

- [ ] **Step 3: Implement `buildLogger` and wire it into `main()`**

  Replace the import block in `cmd/muto-operator/main.go`:

  ```go
  import (
  	"fmt"
  	"log"
  	"os"

  	"github.com/go-logr/stdr"
  	cfplatform "github.com/muto-io/muto/platform/cf"
  	k8sadapter "github.com/muto-io/muto/platform/k8s"
  	"github.com/muto-io/muto/platform/k8s/reconcilers"
  	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
  	"github.com/muto-io/muto/core/scheduler"
  	corev1 "k8s.io/api/core/v1"
  	"k8s.io/apimachinery/pkg/runtime"
  	"k8s.io/client-go/rest"
  	ctrl "sigs.k8s.io/controller-runtime"
  	"sigs.k8s.io/controller-runtime/pkg/client"
  	"sigs.k8s.io/controller-runtime/pkg/healthz"
  	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
  )
  ```

  with:

  ```go
  import (
  	"fmt"
  	"io"
  	"os"

  	"github.com/go-logr/logr"
  	cfplatform "github.com/muto-io/muto/platform/cf"
  	k8sadapter "github.com/muto-io/muto/platform/k8s"
  	"github.com/muto-io/muto/platform/k8s/reconcilers"
  	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
  	"github.com/muto-io/muto/core/scheduler"
  	"go.uber.org/zap/zapcore"
  	corev1 "k8s.io/api/core/v1"
  	"k8s.io/apimachinery/pkg/runtime"
  	"k8s.io/client-go/rest"
  	ctrl "sigs.k8s.io/controller-runtime"
  	"sigs.k8s.io/controller-runtime/pkg/client"
  	"sigs.k8s.io/controller-runtime/pkg/healthz"
  	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
  	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
  )
  ```

  Add `buildLogger` right after the `init()` block (before `newManager`):

  ```go
  // buildLogger builds a zap-backed logr.Logger at the given level, writing
  // out in the given format. format must be "json" or "console"; level must
  // be one of "debug", "info", "warn", "error". The two are independent
  // knobs (not zap.UseDevMode, which conflates encoder choice with level and
  // stacktrace defaults).
  func buildLogger(format, level string, out io.Writer) (logr.Logger, error) {
  	var levelOpt zapcore.Level
  	switch level {
  	case "debug":
  		levelOpt = zapcore.DebugLevel
  	case "info":
  		levelOpt = zapcore.InfoLevel
  	case "warn":
  		levelOpt = zapcore.WarnLevel
  	case "error":
  		levelOpt = zapcore.ErrorLevel
  	default:
  		return logr.Logger{}, fmt.Errorf("invalid MUTO_LOG_LEVEL %q: must be one of debug, info, warn, error", level)
  	}

  	var encoderOpt ctrlzap.Opts
  	switch format {
  	case "json":
  		encoderOpt = ctrlzap.JSONEncoder()
  	case "console":
  		encoderOpt = ctrlzap.ConsoleEncoder()
  	default:
  		return logr.Logger{}, fmt.Errorf("invalid MUTO_LOG_FORMAT %q: must be one of json, console", format)
  	}

  	return ctrlzap.New(ctrlzap.Level(levelOpt), encoderOpt, ctrlzap.WriteTo(out)), nil
  }
  ```

  Change the start of `main()` (building on Task 1's version) from:

  ```go
  func main() {
  	ctrl.SetLogger(stdr.New(log.Default()))
  	log := ctrl.Log.WithName("muto-operator")

  	metricsAddr := os.Getenv("MUTO_METRICS_BIND_ADDRESS")
  ```

  to:

  ```go
  func main() {
  	logFormat := os.Getenv("MUTO_LOG_FORMAT")
  	if logFormat == "" {
  		logFormat = "json"
  	}
  	logLevel := os.Getenv("MUTO_LOG_LEVEL")
  	if logLevel == "" {
  		logLevel = "info"
  	}
  	logger, err := buildLogger(logFormat, logLevel, os.Stdout)
  	if err != nil {
  		fmt.Fprintf(os.Stderr, "invalid logging configuration: %v\n", err)
  		os.Exit(1)
  	}
  	ctrl.SetLogger(logger)
  	log := ctrl.Log.WithName("muto-operator")

  	metricsAddr := os.Getenv("MUTO_METRICS_BIND_ADDRESS")
  ```

  (The later `mgr, err := newManager(...)` line still works: `err` was already declared above by `logger, err := buildLogger(...)`, and `:=` is valid there because `mgr` is a new variable.)

- [ ] **Step 4: Tidy modules**

  Run: `go mod tidy`
  Expected: `go.mod`'s `go.uber.org/zap` line loses its `// indirect` marker; no version changes. Check with `git diff go.mod go.sum`.

- [ ] **Step 5: Run the full package test suite**

  Run: `go test ./cmd/muto-operator/... -v`
  Expected: PASS (all of `TestManagerServesHealthProbes`, `TestManagerServesMetrics`, `TestBuildLogger` and its subtests)

- [ ] **Step 6: Commit**

  ```bash
  git add cmd/muto-operator/main.go cmd/muto-operator/main_test.go go.mod go.sum
  git commit -m "feat: structured JSON logging for muto-operator

Replaces stdr with sigs.k8s.io/controller-runtime/pkg/log/zap.
MUTO_LOG_LEVEL (debug/info/warn/error, default info) and
MUTO_LOG_FORMAT (json/console, default json) are independent knobs;
an invalid value fails startup with a clear error instead of
silently defaulting."
  ```

---

### Task 3: `cmd/muto-mcp` — structured JSON logging

**Files:**
- Modify: `cmd/muto-mcp/main.go`
- Create: `cmd/muto-mcp/main_test.go`

**Interfaces:**
- Produces: `func buildLogger(format, level string, out io.Writer) (logr.Logger, error)` in `package main` (`cmd/muto-mcp`) — same signature as Task 2's, independent implementation (separate binary, no shared package; see Global Constraints).

- [ ] **Step 1: Write the failing test**

  Create `cmd/muto-mcp/main_test.go` (identical test logic to Task 2's `TestBuildLogger`, since it's testing an identically-behaved local function):

  ```go
  // SPDX-License-Identifier: Apache-2.0
  package main

  import (
  	"bytes"
  	"encoding/json"
  	"testing"
  )

  func TestBuildLogger(t *testing.T) {
  	t.Run("valid combinations", func(t *testing.T) {
  		for _, level := range []string{"debug", "info", "warn", "error"} {
  			for _, format := range []string{"json", "console"} {
  				t.Run(level+"/"+format, func(t *testing.T) {
  					var buf bytes.Buffer
  					if _, err := buildLogger(format, level, &buf); err != nil {
  						t.Fatalf("buildLogger(%q, %q): %v", format, level, err)
  					}
  				})
  			}
  		}
  	})

  	t.Run("invalid level", func(t *testing.T) {
  		var buf bytes.Buffer
  		if _, err := buildLogger("json", "trace", &buf); err == nil {
  			t.Fatal("expected error for invalid MUTO_LOG_LEVEL, got nil")
  		}
  	})

  	t.Run("invalid format", func(t *testing.T) {
  		var buf bytes.Buffer
  		if _, err := buildLogger("yaml", "info", &buf); err == nil {
  			t.Fatal("expected error for invalid MUTO_LOG_FORMAT, got nil")
  		}
  	})

  	t.Run("json format emits parseable JSON", func(t *testing.T) {
  		var buf bytes.Buffer
  		logger, err := buildLogger("json", "info", &buf)
  		if err != nil {
  			t.Fatalf("buildLogger: %v", err)
  		}
  		logger.Info("test message", "key", "value")

  		line := buf.Bytes()
  		if len(line) == 0 {
  			t.Fatal("expected log output, got none")
  		}
  		var decoded map[string]interface{}
  		if err := json.Unmarshal(line, &decoded); err != nil {
  			t.Fatalf("log line is not valid JSON: %v\nline: %s", err, line)
  		}
  		if decoded["msg"] != "test message" {
  			t.Errorf("msg = %v, want %q", decoded["msg"], "test message")
  		}
  		if decoded["key"] != "value" {
  			t.Errorf("key = %v, want %q", decoded["key"], "value")
  		}
  	})

  	t.Run("error level filters info messages", func(t *testing.T) {
  		var buf bytes.Buffer
  		logger, err := buildLogger("json", "error", &buf)
  		if err != nil {
  			t.Fatalf("buildLogger: %v", err)
  		}
  		logger.Info("should be filtered")
  		if buf.Len() != 0 {
  			t.Errorf("expected no output at error level for an Info call, got: %s", buf.Bytes())
  		}
  	})
  }
  ```

- [ ] **Step 2: Run it to verify it fails to compile**

  Run: `go test ./cmd/muto-mcp/... -run TestBuildLogger -v`
  Expected: FAIL with `undefined: buildLogger`

- [ ] **Step 3: Implement `buildLogger` and wire it into `main()`**

  Replace the full contents of `cmd/muto-mcp/main.go`:

  ```go
  // SPDX-License-Identifier: Apache-2.0
  package main

  import (
  	"fmt"
  	"io"
  	"os"

  	"github.com/go-logr/logr"
  	k8sadapter "github.com/muto-io/muto/platform/k8s"
  	"github.com/muto-io/muto/core/scheduler"
  	"github.com/muto-io/muto/mcp/server"
  	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
  	"go.uber.org/zap/zapcore"
  	corev1 "k8s.io/api/core/v1"
  	"k8s.io/apimachinery/pkg/runtime"
  	ctrl "sigs.k8s.io/controller-runtime"
  	"sigs.k8s.io/controller-runtime/pkg/client"
  	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
  )

  // buildLogger builds a zap-backed logr.Logger at the given level, writing
  // out in the given format. format must be "json" or "console"; level must
  // be one of "debug", "info", "warn", "error". The two are independent
  // knobs (not zap.UseDevMode, which conflates encoder choice with level and
  // stacktrace defaults).
  func buildLogger(format, level string, out io.Writer) (logr.Logger, error) {
  	var levelOpt zapcore.Level
  	switch level {
  	case "debug":
  		levelOpt = zapcore.DebugLevel
  	case "info":
  		levelOpt = zapcore.InfoLevel
  	case "warn":
  		levelOpt = zapcore.WarnLevel
  	case "error":
  		levelOpt = zapcore.ErrorLevel
  	default:
  		return logr.Logger{}, fmt.Errorf("invalid MUTO_LOG_LEVEL %q: must be one of debug, info, warn, error", level)
  	}

  	var encoderOpt ctrlzap.Opts
  	switch format {
  	case "json":
  		encoderOpt = ctrlzap.JSONEncoder()
  	case "console":
  		encoderOpt = ctrlzap.ConsoleEncoder()
  	default:
  		return logr.Logger{}, fmt.Errorf("invalid MUTO_LOG_FORMAT %q: must be one of json, console", format)
  	}

  	return ctrlzap.New(ctrlzap.Level(levelOpt), encoderOpt, ctrlzap.WriteTo(out)), nil
  }

  func main() {
  	logFormat := os.Getenv("MUTO_LOG_FORMAT")
  	if logFormat == "" {
  		logFormat = "json"
  	}
  	logLevel := os.Getenv("MUTO_LOG_LEVEL")
  	if logLevel == "" {
  		logLevel = "info"
  	}
  	logger, err := buildLogger(logFormat, logLevel, os.Stdout)
  	if err != nil {
  		fmt.Fprintf(os.Stderr, "invalid logging configuration: %v\n", err)
  		os.Exit(1)
  	}
  	ctrl.SetLogger(logger)
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

- [ ] **Step 4: Run the full package test suite**

  Run: `go test ./cmd/muto-mcp/... -v`
  Expected: PASS (`TestBuildLogger` and its subtests)

- [ ] **Step 5: Build both binaries to catch any cross-package drift**

  Run: `go build ./...`
  Expected: no errors

- [ ] **Step 6: Commit**

  ```bash
  git add cmd/muto-mcp/main.go cmd/muto-mcp/main_test.go
  git commit -m "feat: structured JSON logging for muto-mcp

Mirrors cmd/muto-operator's buildLogger: MUTO_LOG_LEVEL and
MUTO_LOG_FORMAT, independent knobs, fail-fast on an invalid value."
  ```

---

### Task 4: Helm chart wiring

**Files:**
- Modify: `deploy/helm/muto/values.yaml`
- Modify: `deploy/helm/muto/templates/deployment-operator.yaml`

**Interfaces:**
- Consumes: `MUTO_METRICS_BIND_ADDRESS`, `MUTO_HEALTH_PROBE_BIND_ADDRESS` (Task 1), `MUTO_LOG_LEVEL`, `MUTO_LOG_FORMAT` (Task 2) — the exact env var names the operator binary now reads.
- Produces: nothing consumed by later tasks (this is the last task in the plan).

There's no Helm unit-test framework in this repo (only `helm lint` in CI) — verification here is direct `helm template` inspection instead of a red/green test cycle.

- [ ] **Step 1: Add the two new logging keys to `values.yaml`'s existing `env` map**

  In `deploy/helm/muto/values.yaml`, change:

  ```yaml
  env:
    MUTO_PLATFORM: k8s
    MUTO_NAMESPACE: default
    # CF platform vars — only used when MUTO_PLATFORM=cf
  ```

  to:

  ```yaml
  env:
    MUTO_PLATFORM: k8s
    MUTO_NAMESPACE: default
    MUTO_LOG_LEVEL: info
    MUTO_LOG_FORMAT: json
    # CF platform vars — only used when MUTO_PLATFORM=cf
  ```

  This map is already generically templated into the container (`range $key, $val := .Values.env` in `deployment-operator.yaml`), so no template change is needed for these two.

- [ ] **Step 2: Derive the bind-address env vars from the existing `metrics`/`healthProbe` values**

  In `deploy/helm/muto/templates/deployment-operator.yaml`, change:

  ```yaml
          env:
            {{- range $key, $val := .Values.env }}
            {{- if $val }}
            - name: {{ $key }}
              value: {{ $val | quote }}
            {{- end }}
            {{- end }}
          ports:
  ```

  to:

  ```yaml
          env:
            {{- range $key, $val := .Values.env }}
            {{- if $val }}
            - name: {{ $key }}
              value: {{ $val | quote }}
            {{- end }}
            {{- end }}
            - name: MUTO_METRICS_BIND_ADDRESS
              {{- if .Values.metrics.enabled }}
              value: {{ printf ":%d" (.Values.metrics.port | int) | quote }}
              {{- else }}
              value: "0"
              {{- end }}
            - name: MUTO_HEALTH_PROBE_BIND_ADDRESS
              value: {{ printf ":%d" (.Values.healthProbe.port | int) | quote }}
          ports:
  ```

  This reuses the exact same `.Values.metrics.port` / `.Values.healthProbe.port` / `.Values.metrics.enabled` that already drive `containerPort` and the scrape annotations a few lines above — closing the mismatch the issue reported, by construction (both now read from one source of truth).

- [ ] **Step 3: Lint the chart**

  Run: `helm lint deploy/helm/muto`
  Expected: `0 chart(s) failed`

- [ ] **Step 4: Verify default rendering**

  Run: `helm template deploy/helm/muto | grep -A2 'MUTO_METRICS_BIND_ADDRESS\|MUTO_HEALTH_PROBE_BIND_ADDRESS\|MUTO_LOG_LEVEL\|MUTO_LOG_FORMAT'`
  Expected output includes:
  ```
  - name: MUTO_METRICS_BIND_ADDRESS
    value: ":8080"
  - name: MUTO_HEALTH_PROBE_BIND_ADDRESS
    value: ":8081"
  - name: MUTO_LOG_LEVEL
    value: "info"
  - name: MUTO_LOG_FORMAT
    value: "json"
  ```

- [ ] **Step 5: Verify `metrics.enabled=false` disables the metrics bind address**

  Run: `helm template deploy/helm/muto --set metrics.enabled=false | grep -A1 'MUTO_METRICS_BIND_ADDRESS'`
  Expected:
  ```
  - name: MUTO_METRICS_BIND_ADDRESS
    value: "0"
  ```

- [ ] **Step 6: Verify a custom port propagates**

  Run: `helm template deploy/helm/muto --set healthProbe.port=9091 | grep -A1 'MUTO_HEALTH_PROBE_BIND_ADDRESS'`
  Expected:
  ```
  - name: MUTO_HEALTH_PROBE_BIND_ADDRESS
    value: ":9091"
  ```

- [ ] **Step 7: Commit**

  ```bash
  git add deploy/helm/muto/values.yaml deploy/helm/muto/templates/deployment-operator.yaml
  git commit -m "feat: wire metrics/health/logging config through the Helm chart

values.metrics.port, values.healthProbe.port, and values.metrics.enabled
now reach the operator binary via MUTO_METRICS_BIND_ADDRESS and
MUTO_HEALTH_PROBE_BIND_ADDRESS, closing the mismatch where only the
exposed containerPort changed. MUTO_LOG_LEVEL/MUTO_LOG_FORMAT are
now chart-configurable too."
  ```

---

## Self-Review

**Spec coverage:**
- Bind addresses (spec §1) → Task 1.
- Structured logging, both binaries (spec §2) → Tasks 2 and 3.
- Helm chart wiring (spec §3) → Task 4.
- Testing (spec §4) → covered inline in each task (unit tests in Tasks 1-3, `helm template` checks in Task 4).
- All five acceptance criteria in the spec are exercised: bind-address passthrough (Task 1 + 4), `metrics.enabled=false` disabling (Task 4 Step 5), JSON/level env vars (Task 2/3 tests + Task 4), fail-fast on invalid values (Task 2/3 `TestBuildLogger` invalid-level/format subtests), existing tests still passing (every task re-runs the full package suite).

**Placeholder scan:** No TBD/TODO markers; every step has literal code or an exact shell command with expected output.

**Type consistency:** `buildLogger(format, level string, out io.Writer) (logr.Logger, error)` is identical across Tasks 2 and 3 (independently implemented, as called out in each task's Interfaces block). `newManager(cfg *rest.Config, metricsAddr, probeAddr string) (ctrl.Manager, error)` is unchanged from its current signature in Task 1.
