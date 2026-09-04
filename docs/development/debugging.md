# Debugging Guide

Techniques for debugging, profiling, and troubleshooting Muto.

## Local Debugging

### Run with Verbose Logging

Enable debug-level logging:

```bash
# Run operator with debug logs
LOGGER_LEVEL=debug ./bin/muto-operator

# Or for MCP server
LOGGER_LEVEL=debug ./bin/muto-mcp
```

Check available log levels in your component's main.go.

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
            "program": "${workspaceFolder}/cmd/muto-operator",
            "env": {
                "LOGGER_LEVEL": "debug"
            }
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

Muto uses go-logr for structured logging. Log messages include:
- Timestamp
- Level (Info, Debug, Warn, Error)
- Message
- Key-value pairs

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

Logs are JSON-formatted for machine parsing:

```json
{
    "level": "info",
    "ts": "2026-09-03T10:30:45.123Z",
    "logger": "scheduler",
    "msg": "job scheduled",
    "jobID": "job-123",
    "tenant": "tenant-a",
    "agents": 2
}
```

**Filter logs:**
```bash
# Show only errors
kubectl logs deployment/muto-operator -n muto-system | jq 'select(.level == "error")'

# Show logs for specific job
kubectl logs deployment/muto-operator -n muto-system | jq 'select(.jobID == "job-123")'

# Show last 100 errors
kubectl logs deployment/muto-operator -n muto-system --tail=1000 | \
    jq 'select(.level == "error")' | tail -100
```

### Enable Verbose Logging in Kubernetes

Edit the operator deployment:

```bash
kubectl set env deployment/muto-operator \
    -n muto-system \
    LOGGER_LEVEL=debug
```

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

### Enable Distributed Tracing

Muto supports OpenTelemetry for tracing. Set environment variables:

```bash
# Enable OTLP exporter
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
OTEL_SERVICE_NAME=muto-operator

# Start operator
./bin/muto-operator
```

### View Traces

**With Jaeger UI:**
1. Open http://localhost:16686
2. Select service: `muto-operator`
3. Find traces by tag or operation name

**Example trace tags:**
```
jobID=job-123
tenant=tenant-a
operation=schedule
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
kubectl logs deployment/muto-operator -n muto-system | \
    jq 'select(.jobID == "job-123")'
```

**Check agent status:**
```bash
kubectl get agentjob job-123 -o yaml
kubectl describe agentjob job-123
```

**Check reconciler status:**
```bash
# Watch reconciliation attempts
kubectl logs deployment/muto-operator -n muto-system -f | \
    jq 'select(.msg | contains("reconcile"))'
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

**In operator logs, look for:**
```json
{
    "level": "error",
    "msg": "failed to connect to message bus",
    "error": "connection refused"
}
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

```bash
# Extract scheduling times from logs
kubectl logs deployment/muto-operator -n muto-system | \
    jq 'select(.msg == "job scheduled") | .duration_ms'

# Calculate statistics
kubectl logs deployment/muto-operator -n muto-system | \
    jq 'select(.msg == "job scheduled") | .duration_ms' | \
    awk '{sum+=$1; count++} END {print "Avg: " sum/count "ms"}'
```

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

**Last Updated:** 2026-09-03
