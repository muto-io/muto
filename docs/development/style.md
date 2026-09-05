# Code Style Guide

Code style guidelines for Muto. We follow Go conventions with additional team standards for consistency.

## Formatting

### Use goimports

Automatically format imports and code:

```bash
# Format single file
goimports -w filename.go

# Format entire directory
goimports -r ./...
```

Configure in your editor:

- **VS Code**: Set `"go.formatTool": "goimports"` in settings.json
- **GoLand**: Go -> Code Style -> Go -> set formatter to "goimports"

### Go fmt

All code must be formatted with `go fmt`:

```bash
# Format entire project
go fmt ./...
```

This is enforced in CI.

## Naming Conventions

### Package Names

**Lowercase, single word, descriptive:**

```go
package scheduler      // Good
package agent          // Good
package joblifecycle   // Good
package s              // Bad
package Job            // Bad
```

**Related packages** use lowercase prefix:

```go
platform/k8s/adapter.go
platform/k8s/reconcilers/job_reconciler.go

core/agent/types.go
core/agent/validation.go
```

### Types and Constants

**Exported types**: PascalCase

```go
// Good
type AgentJob struct {}
type Scheduler interface {}
const DefaultTimeout = 5 * time.Minute

// Bad
type agent_job struct {}
type scheduler interface {}
const default_timeout = 5 * time.Minute
```

**Receiver variables**: Short, clear names (1-2 chars)

```go
// Good
func (s *Scheduler) Schedule(ctx context.Context) error {
    s.mu.Lock()
}

func (j *AgentJob) Validate() error {
    // j is short for Job
}

// Bad
func (scheduler *Scheduler) Schedule(ctx context.Context) error {
    scheduler.mu.Lock()
}

func (agentJob *AgentJob) Validate() error {
    // Too verbose
}
```

### Functions and Methods

**Exported functions**: PascalCase, descriptive verbs

```go
// Good
func (s *Scheduler) Schedule(ctx context.Context, job *AgentJob) error
func (j *AgentJob) Validate() error
func (mb *NATS) Publish(ctx context.Context, topic string, msg []byte) error

// Bad
func (s *Scheduler) schedule(ctx context.Context, job *AgentJob) error  // lowercase
func (j *AgentJob) check() error  // vague verb
func (mb *NATS) pub(topic string, msg []byte) error  // abbreviation
```

**Unexported functions**: camelCase

```go
func (s *Scheduler) scheduleJob(ctx context.Context, job *AgentJob) error
func calculatePriority(job *AgentJob) int
```

### Variables

**Clear, context-specific names:**

```go
// Good
var timeout = 30 * time.Second
var maxRetries = 3
var isScheduled bool

// Bad
var t = 30 * time.Second  // unclear what 't' is
var mr = 3  // abbreviation
var scheduled bool  // use 'is' prefix for booleans
```

**Boolean variables** use 'is' prefix:

```go
// Good
var isRunning bool
var isCompleted bool
var hasRetried bool

// Bad
var running bool
var completed bool
var retried bool
```

### Error Variables

**Use standard error pattern:**

```go
// Define errors at package level
var (
    ErrJobNotFound = errors.New("agent job not found")
    ErrInvalidTenant = errors.New("tenant not found")
    ErrSchedulingFailed = errors.New("could not schedule job")
)

// Use them in functions
func (s *Scheduler) GetJob(id string) (*AgentJob, error) {
    if id == "" {
        return nil, ErrJobNotFound
    }
}
```

## Comments

### Function Comments

All exported functions must have comments:

```go
// Schedule assigns an agent job to a platform for execution.
// It validates tenant isolation and resource constraints before scheduling.
//
// Parameters:
//   - ctx: context for cancellation and deadlines
//   - job: the agent job to schedule (must be non-nil)
//
// Returns:
//   - string: unique job ID for tracking execution
//   - error: non-nil if validation or scheduling fails
func (s *Scheduler) Schedule(ctx context.Context, job *AgentJob) (string, error) {
```

### Inline Comments

Comment complex logic, not obvious code:

```go
// Good: explains why
// Use exponential backoff to avoid thundering herd
backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second

// Bad: states what code obviously does
// Increment attempt
attempt++

// Calculate result
result := a + b
```

### TODO Comments

Avoid TODOs in merged code. Use GitHub issues instead:

```go
// Don't do this in merged code:
// TODO: optimize this later

// Instead, create an issue and comment with the issue number:
// FIXME: #123 - this logic is slow and needs optimization
```

## Error Handling

### Check Errors Immediately

```go
// Good
err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}

// Bad
var err error
err = doSomething()
// ... lots of other code ...
if err != nil {
    // Lost context
}
```

### Wrap Errors with Context

Use `fmt.Errorf` with `%w` for error wrapping:

```go
// Good: preserves error chain
if err != nil {
    return fmt.Errorf("failed to schedule job %s: %w", job.Name, err)
}

// Bad: loses error chain
if err != nil {
    return errors.New("failed to schedule")
}

// Bad: concatenation (use fmt.Errorf)
if err != nil {
    return errors.New("failed to schedule: " + err.Error())
}
```

### Custom Error Types

For complex errors, define custom types:

```go
// Define
type SchedulingError struct {
    JobID  string
    Reason string
    Cause  error
}

func (e *SchedulingError) Error() string {
    return fmt.Sprintf("scheduling error for job %s: %s (%v)", e.JobID, e.Reason, e.Cause)
}

func (e *SchedulingError) Unwrap() error {
    return e.Cause
}

// Use
if err != nil {
    return &SchedulingError{
        JobID:  job.ID,
        Reason: "insufficient resources",
        Cause:  err,
    }
}
```

## Interfaces

### Design for Abstraction

Define interfaces for platform differences:

```go
// Good: platform-agnostic
type PlatformAdapter interface {
    CreateJob(ctx context.Context, job *AgentJob) (string, error)
    DeleteJob(ctx context.Context, jobID string) error
    GetJobStatus(ctx context.Context, jobID string) (*JobStatus, error)
}

// Good: implementations
type KubernetesAdapter struct { ... }
func (ka *KubernetesAdapter) CreateJob(ctx context.Context, job *AgentJob) (string, error) { ... }

type CloudFoundryAdapter struct { ... }
func (ca *CloudFoundryAdapter) CreateJob(ctx context.Context, job *AgentJob) (string, error) { ... }
```

### Small, Focused Interfaces

```go
// Good: specific responsibility
type MessageBus interface {
    Publish(ctx context.Context, topic string, message []byte) error
    Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
}

// Bad: too many responsibilities
type MessageBusAndStorage interface {
    Publish(ctx context.Context, topic string, message []byte) error
    Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
    Store(key string, value interface{}) error
    Retrieve(key string) (interface{}, error)
}
```

## Concurrency

### Use Contexts

Always pass context for cancellation and timeouts:

```go
// Good
func (s *Scheduler) Schedule(ctx context.Context, job *AgentJob) (string, error) {
    select {
    case <-ctx.Done():
        return "", ctx.Err()
    case result := <-ch:
        return result, nil
    }
}

// Bad: no timeout or cancellation support
func (s *Scheduler) Schedule(job *AgentJob) (string, error) {
    // what if this hangs forever?
}
```

### Protect Shared State

Use mutexes for shared state:

```go
type Scheduler struct {
    mu    sync.RWMutex  // Protect jobs map
    jobs  map[string]*AgentJob
}

func (s *Scheduler) GetJob(id string) *AgentJob {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.jobs[id]
}

func (s *Scheduler) AddJob(job *AgentJob) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.jobs[job.ID] = job
}
```

### Goroutine Management

```go
// Good: track and cancel goroutines
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go func() {
    for {
        select {
        case <-ctx.Done():
            return  // Exit cleanly on cancellation
        case event := <-events:
            handleEvent(event)
        }
    }
}()

// Bad: goroutine leaks
go func() {
    for {
        event := <-events  // Never exits if context cancelled
        handleEvent(event)
    }
}()
```

## Logging

### Use Structured Logging

Use `go-logr` for structured logs:

```go
import "github.com/go-logr/logr"

// Good
log := logr.FromContext(ctx)
log.Info("job scheduled",
    "jobID", job.ID,
    "tenant", job.Tenant,
    "platform", "kubernetes")

// Bad
log.Println("Job scheduled: " + job.ID)  // No structure, hard to parse
log.Printf("job=%s tenant=%s", job.ID, job.Tenant)  // Manual formatting
```

### Log Levels

- **Info**: Important state changes (job created, scheduled, completed)
- **Debug**: Detailed flow (reconciliation loops, validation steps)
- **Warning**: Recoverable errors (retry attempt, transient failure)
- **Error**: Significant failures (scheduling failed, platform unavailable)

```go
log.Info("job submitted", "jobID", jobID)
log.Debug("checking platform availability", "platform", platform)
log.WithValues("attempt", attempt).Info("retrying job")
log.Error(err, "failed to create platform job", "jobID", jobID)
```

## Linting

### Enable All Linters

Run golangci-lint with all linters enabled:

```bash
golangci-lint run ./...
```

### Common Linting Issues

**Unused variables:**
```go
// Bad: linter complains
var unused string

// Good: remove it
// or use _ if truly unneeded
_ = unused
```

**Long lines:**
```bash
# Run with line length check
golangci-lint run ./... --deadline=5m
```

**Cyclomatic complexity:**
Keep functions simple, under 10 lines of logic when possible.

## Dependency Management

### Use go mod

All dependencies managed in `go.mod`:

```bash
# Add dependency
go get github.com/user/package@latest

# Remove unused
go mod tidy

# Verify integrity
go mod verify
```

### Avoid Circular Dependencies

Structure packages to avoid circular imports:

```
// Good: clean dependency flow
agent.go          (no imports from scheduler)
    ↓
scheduler.go      (imports agent)
    ↓
reconcilers.go    (imports scheduler)

// Bad: circular
agent ↔ scheduler
```

## Testing Naming

### Test Function Names

```go
// Good: clearly describes what is tested
func TestSchedulerSchedulesJobSuccessfully(t *testing.T) {}
func TestJobValidationRejectsEmptyAgents(t *testing.T) {}

// Bad: vague
func TestScheduler(t *testing.T) {}
func TestJob(t *testing.T) {}
```

### Ginkgo Test Descriptions

```go
Describe("Scheduler", func() {
    Describe("Schedule", func() {
        It("assigns job to available platform", func() { ... })
        It("validates tenant isolation", func() { ... })
        It("rejects invalid jobs", func() { ... })
    })
})
```

## Project Layout

```
muto/
├── cmd/               # Entry points (main.go)
│   ├── muto-operator/
│   └── muto-mcp/
├── core/              # Domain logic (platform-agnostic)
│   ├── agent/         # Agent job types
│   ├── scheduler/     # Scheduling logic
│   ├── tenant/        # Tenant management
│   └── messaging/     # Message bus interface
├── platform/          # Platform adapters
│   ├── k8s/           # Kubernetes adapter
│   │   └── reconcilers/
│   └── cf/            # CloudFoundry adapter
├── mcp/               # MCP integration
│   ├── server/
│   └── tools/
├── deploy/            # Deployment configs
├── test/              # Integration/E2E tests
└── docs/              # Documentation
```

Keep related code close together. If a file exceeds 500 lines, split it.

---

## Linting Checklist

Before committing:

```bash
# Format
go fmt ./...

# Imports
goimports -r ./...

# Vet
go vet ./...

# Lint
golangci-lint run ./...

# Tests
go test ./...
```

---

## References

- [Effective Go](https://golang.org/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)

---

**Last Updated:** 2026-09-03
