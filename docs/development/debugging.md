# Debugging Guide

Techniques for debugging, profiling, and troubleshooting Muto.

## Local Debugging

### Run Locally

Run the binaries against the cluster of your current kubeconfig context:

```bash
# Run operator
./bin/muto-operator

# Or the MCP server
./bin/muto-mcp
```

Log verbosity is fixed at `0`, so there is no debug level to enable yet; see [Verbose Logging](#verbose-logging).

### Using Delve Debugger

**Install Delve:**
```bash
go install github.com/go-delve/delve/cmd/dlv@latest
```

**Debug a binary:**
```bash
# Build with debug symbols (default)
make build

# Run under debugger
dlv exec ./bin/muto-operator
```

**In debugger:**
```
(dlv) break main.main           # Set breakpoint
(dlv) continue                  # Run to breakpoint
(dlv) next                      # Step to next line
(dlv) step                      # Step into function
(dlv) print variable            # Inspect variable
(dlv) locals                    # Show local variables
(dlv) quit                      # Exit
```

**Debug with VS Code:**

Create `.vscode/launch.json`:
```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Muto Operator",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/muto-operator"
        }
    ]
}
```

### Debug Tests

**Add breakpoint in test:**
```bash
# Run test under debugger
dlv test ./core/scheduler -- -test.run TestSchedule
```

**Debug with VS Code:**
```json
{
    "name": "Test",
    "type": "go",
    "request": "launch",
    "mode": "test",
    "program": "${fileDirname}",
    "args": ["-test.run", "TestSchedule"]
}
```

## Logging

### Structured Logging with go-logr

Muto uses go-logr for structured logging with the [`stdr`](https://github.com/go-logr/stdr) backend, which writes to stderr. Log lines include:
- Timestamp (second precision)
- Logger name
- Verbosity (`"level"=0`; messages logged with `V(1)` or higher are currently discarded)
- Message
- Key-value pairs (errors are logged with an `"error"` key)

**Examples:**

```go
import "github.com/go-logr/logr"

// Get logger from context
log := logr.FromContext(ctx)

// Log with key-value pairs
log.Info("job scheduled",
    "jobID", job.ID,
    "tenant", job.Tenant,
    "agents", len(job.Spec.Agents),
)

// Log errors
log.Error(err, "failed to schedule job",
    "jobID", job.ID,
    "attempt", attempt,
)
```

### Log Parsing

Logs are plain-text key/value lines, not JSON:

```
2026/09/12 09:46:39 "level"=0 "msg"="adding tenant finalizer" "controller"="tenant" "controllerGroup"="muto.io" "controllerKind"="Tenant" "Tenant"={"name"="demo-tenant"} "namespace"="" "name"="demo-tenant" "reconcileID"="c0c3b48e-78e4-42f5-86ae-334acbd8aa8f" "tenant"="demo-tenant" "finalizer"="muto.io/tenant-cleanup"
```

**Filter logs:**
```bash
# Show only errors
kubectl logs deployment/muto-operator -n muto-system | grep '"error"='

# Show logs for a specific object
kubectl logs deployment/muto-operator -n muto-system | grep '"name"="my-job"'

# Show logs of one reconciler
kubectl logs deployment/muto-operator -n muto-system | grep '"controller"="agentjob"'
```

### Verbose Logging

Log verbosity is fixed at `0`: `log.V(1).Info(...)` and higher are discarded, and no environment variable raises the level. While debugging, log at `V(0)` (see below) or use a debugger. Configurable log levels are planned in [#79](https://github.com/muto-io/muto/issues/79).

View logs:
```bash
kubectl logs deployment/muto-operator -n muto-system -f
```

### Temporary Debug Statements

For temporary debugging (never commit these):

```go
// Use logr for temporary debug output
log := logr.FromContext(ctx)
log.Info("DEBUG: job state before transition",
    "jobID", job.ID,
    "currentState", job.Status,
    "targetState", targetState,
)

// Inspect variable in test
t.Logf("DEBUG: job = %+v", job)

// Print to stderr for quick debugging (prefer logging)
fmt.Fprintf(os.Stderr, "DEBUG: value = %v\n", value)
```

## Profiling

### CPU Profiling

Identify performance bottlenecks:

```bash
# Run operator with CPU profiling
go run ./cmd/muto-operator -cpuprofile=cpu.prof

# Run tests with profiling
go test -cpuprofile=cpu.prof ./core/scheduler

# Analyze profile
go tool pprof cpu.prof
```

In the pprof interactive shell:

```
(pprof) top           # Show top functions by CPU time
(pprof) list Schedule # Show source code with CPU usage
(pprof) pdf           # Generate PDF graph
(pprof) quit
```

### Memory Profiling

Find memory leaks:

```bash
# Run with memory profiling
go run ./cmd/muto-operator -memprofile=mem.prof

# Analyze
go tool pprof mem.prof
(pprof) top          # Show top memory users
(pprof) alloc_space  # Show all allocations
```

### Continuous Profiling in Kubernetes

Run pprof server in operator:

```bash
# Enable pprof endpoint (add to operator startup)
import _ "net/http/pprof"

# In main.go, start HTTP server
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Access profiles:

```bash
# Port-forward to operator pod
kubectl port-forward deployment/muto-operator 6060:6060 -n muto-system

# In another terminal
go tool pprof http://localhost:6060/debug/pprof/profile
```

## Tracing

Distributed tracing isn't implemented yet: the operator doesn't initialize an OpenTelemetry SDK, and `OTEL_*` environment variables have no effect. OpenTelemetry tracing is planned in [#79](https://github.com/muto-io/muto/issues/79).

To follow a single reconciliation, filter the logs by its `reconcileID`:

```bash
kubectl logs deployment/muto-operator -n muto-system | grep '"reconcileID"="<id>"'
```

## Common Issues and Solutions

### Operator Pod Stuck in CrashLoopBackOff

**Check logs:**
```bash
kubectl logs -p deployment/muto-operator -n muto-system  # Previous logs
kubectl describe pod -n muto-system -l app=muto-operator  # Events
```

**Common causes:**
- Invalid configuration (missing env vars)
- Failed to connect to Kubernetes API
- Failed to connect to message bus

**Fix:**
```bash
# Check operator config
kubectl get configmap muto-config -n muto-system -o yaml

# Check events
kubectl get events -n muto-system --sort-by='.lastTimestamp'
```

### Job Stuck in Pending State

**Check operator logs:**
```bash
kubectl logs deployment/muto-operator -n muto-system | grep '"name"="job-123"'
```

**Check agent status:**
```bash
kubectl get agentjob job-123 -o yaml
kubectl describe agentjob job-123
```

**Check reconciler status:**
```bash
# Watch AgentJob reconciliations
kubectl logs deployment/muto-operator -n muto-system -f | grep '"controller"="agentjob"'
```

### Message Bus Connection Failures

**Check message bus connectivity:**

For NATS:
```bash
# Test NATS connection
nc -zv nats-server 4222

# Check NATS logs
kubectl logs deployment/nats -n nats-io
```

For Kafka:
```bash
# Test Kafka broker
nc -zv kafka-broker 9092

# Check Kafka logs
kubectl logs statefulset/kafka -n kafka
```

**In operator logs, look for errors:**
```bash
kubectl logs deployment/muto-operator -n muto-system | grep '"error"='
```

### Tests Failing with Timeout

**Increase timeout:**
```bash
go test ./test/integration/... -timeout 40m -v
```

**Check resource availability:**
```bash
# Ensure Docker has enough memory
docker stats

# Ensure cluster has resources
kubectl describe nodes
kubectl top nodes
```

**Run test with more logging:**
```bash
go test ./test/integration/k8s/... -run TestAgentJobLifecycle -v -timeout 30m 2>&1 | tee test.log
```

### Memory Leaks in Operator

**Detect with profiling:**
```bash
# Take baseline
go tool pprof http://localhost:6060/debug/pprof/heap > heap1.txt

# Wait 5 minutes
sleep 300

# Take another sample
go tool pprof http://localhost:6060/debug/pprof/heap > heap2.txt

# Compare
go tool pprof -base heap1.txt heap2.txt
```

**Check for common leaks:**
1. Goroutines not exiting (check context handling)
2. Channels not being closed
3. Maps growing unbounded (add cleanup logic)
4. Resource handles not released

## Advanced Debugging

### Trace Syscalls (Linux)

```bash
# Trace operator syscalls
strace -e trace=network,file ./bin/muto-operator 2>&1 | head -50

# Follow specific patterns
strace -e trace=connect,write ./bin/muto-operator 2>&1 | grep -i error
```

### Race Detection

Find race conditions:

```bash
# Run tests with race detector
go test -race ./...

# Run specific test with race detector
go test -race ./core/scheduler -run TestSchedule
```

**Output shows:**
```
==================
WARNING: DATA RACE
Write at 0x00c000234000 by goroutine 34:
    github.com/muto-io/muto/core/scheduler.(*Scheduler).updateJob()
        scheduler.go:156 +0x44

Previous read at 0x00c000234000 by goroutine 33:
    github.com/muto-io/muto/core/scheduler.(*Scheduler).getJob()
        scheduler.go:98 +0x40
==================
```

**Fix by protecting shared state with locks:**
```go
type Scheduler struct {
    mu   sync.Mutex
    jobs map[string]*AgentJob
}
```

### Goroutine Leaks

```bash
# Check goroutine count
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# Save before/after
curl ... > goroutines1.txt
sleep 60
curl ... > goroutines2.txt

# Compare
diff goroutines1.txt goroutines2.txt
```

## Debugging Kubernetes Integration

### Verify CRDs are installed

```bash
kubectl get crds | grep muto
kubectl describe crd agentjobs.muto.io
```

### Check RBAC permissions

```bash
# Verify service account permissions
kubectl auth can-i list agentjobs --as=system:serviceaccount:muto-system:muto-operator

# Check role bindings
kubectl get rolebindings -n muto-system
kubectl get clusterrolebindings | grep muto
```

### Watch API events

```bash
# Watch AgentJob creation events
kubectl get events -n muto-system -w

# Describe job to see status conditions
kubectl describe agentjob job-123

# Get raw job definition
kubectl get agentjob job-123 -o yaml
```

## Debugging CloudFoundry Integration

### Check CF API connectivity

```bash
# Test CF API endpoint
curl -v https://api.cloudfoundry.example.com/v3/info

# Check operator CF credentials
kubectl get secret -n muto-system cf-credentials -o yaml
```

### Monitor CF tasks

```bash
# List CF tasks for tenant
cf target -o tenant-a
cf tasks

# Get task logs
cf logs task-id

# Get task status
cf task task-id
```

## Performance Analysis

### Measure scheduler latency

The operator doesn't log per-job scheduling durations. Use the reconcile latency metrics instead:

```bash
kubectl port-forward -n muto-system deployment/muto-operator 8080:8080 &
curl -s localhost:8080/metrics | grep -E '^controller_runtime_reconcile_time_seconds_(sum|count)\{controller="agentjob"\}'
```

Divide `_sum` by `_count` for the average AgentJob reconcile duration, or use the PromQL queries in [Monitoring and Observability](../operations/monitoring-observability.md#promql-queries).

### Check resource usage

```bash
# Operator resource usage
kubectl top pod -n muto-system -l app=muto-operator

# Message bus usage
kubectl top pod -n nats-io  # or -n kafka
```

---

## Next Steps

- [Setup Guide](./setup.md) — Development environment setup
- [Testing Strategy](./testing-strategy.md) — Comprehensive testing
- [Code Style](./style.md) — Coding standards

---
