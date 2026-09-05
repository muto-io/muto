# Testing Strategy

Comprehensive guide to testing in Muto. We use multiple testing levels to ensure reliability and catch regressions early.

## Overview

Muto uses a three-level testing strategy:

| Level | Scope | Speed | Dependencies | Examples |
|-------|-------|-------|------------|----------|
| **Unit** | Single function/type | Fast (ms) | None | Job state validation, scheduler logic |
| **Integration** | Components + platform | Medium (s) | K8s/CF/Docker | Agent job creation, message bus flow |
| **E2E** | Full system | Slow (min) | K8s/CF/Docker | Multi-agent workflow, tenant isolation |

## Unit Tests

Fast, focused tests for individual functions and types.

### Structure

```go
package agent_test

import (
    "testing"
    "github.com/muto-io/muto/core/agent"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestAgent(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Agent Suite")
}

var _ = Describe("Job", func() {
    Describe("Validate", func() {
        It("rejects missing tenant", func() {
            job := &agent.AgentJob{
                Metadata: agent.Metadata{Name: "test"},
                // Tenant is missing
            }
            err := job.Validate()
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("tenant"))
        })

        It("accepts valid job", func() {
            job := &agent.AgentJob{
                Metadata: agent.Metadata{
                    Name: "test",
                    Tenant: "tenant-a",
                },
                Spec: agent.JobSpec{
                    Agents: []agent.AgentSpec{
                        {
                            Name:  "worker",
                            Image: "myimage:v1",
                        },
                    },
                },
            }
            err := job.Validate()
            Expect(err).NotTo(HaveOccurred())
        })
    })

    Describe("StateTransition", func() {
        It("transitions from Pending to Scheduled", func() {
            job := &agent.AgentJob{Status: agent.StatePending}
            err := job.Transition(agent.StateScheduled)
            Expect(err).NotTo(HaveOccurred())
            Expect(job.Status).To(Equal(agent.StateScheduled))
        })

        It("rejects invalid transitions", func() {
            job := &agent.AgentJob{Status: agent.StateCompleted}
            err := job.Transition(agent.StatePending)
            Expect(err).To(HaveOccurred())
        })
    })
})
```

### Running Unit Tests

**All unit tests:**
```bash
make test-unit
```

**Specific package:**
```bash
go test ./core/agent -v
```

**Single test:**
```bash
go test ./core/agent -run TestValidate -v
```

**With coverage:**
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Writing Unit Tests

**Best Practices:**

1. **Test one behavior per test**
   ```go
   It("rejects job with no agents", func() {
       // Single assertion
   })
   ```

2. **Use table-driven tests for variants**
   ```go
   DescribeTable("Validate",
       func(job *AgentJob, expectErr bool) {
           err := job.Validate()
           if expectErr {
               Expect(err).To(HaveOccurred())
           } else {
               Expect(err).NotTo(HaveOccurred())
           }
       },
       Entry("valid job", validJob, false),
       Entry("missing tenant", jobNoTenant, true),
       Entry("empty agents", jobNoAgents, true),
   )
   ```

3. **Mock external dependencies**
   ```go
   var _ = Describe("Scheduler", func() {
       var scheduler *core.DefaultScheduler
       var mockPlatform *MockPlatformAdapter

       BeforeEach(func() {
           mockPlatform = &MockPlatformAdapter{}
           scheduler = core.NewScheduler(mockPlatform)
       })

       It("allocates job to platform", func() {
           mockPlatform.On("AllocateResources").Return(nil)
           job := newTestJob()
           err := scheduler.Schedule(context.Background(), job)
           Expect(err).NotTo(HaveOccurred())
           mockPlatform.AssertCalled(t, "AllocateResources")
       })
   })
   ```

4. **Use meaningful assertion messages**
   ```go
   Expect(status).To(Equal("completed"), "job should complete after all agents finish")
   ```

## Integration Tests

Tests for components working together, using real platforms.

### Structure

```
test/
├── integration/
│   ├── helpers.go          # Shared test utilities
│   ├── k8s/
│   │   ├── suite_test.go   # Ginkgo test suite setup
│   │   ├── agent_job_test.go
│   │   └── messaging_test.go
│   └── cf/
│       ├── suite_test.go
│       └── agent_job_test.go
```

### Example Integration Test

```go
// test/integration/k8s/agent_job_test.go
package k8s_test

import (
    "context"
    "time"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/muto-io/muto/test/integration"
)

var _ = Describe("AgentJob Execution", Label("integration", "k8s"), func() {
    var ctx context.Context
    var testEnv *integration.TestEnvironment

    BeforeEach(func() {
        ctx = context.Background()
        // Setup: Create test cluster, apply CRDs, start operator
        testEnv = integration.NewTestEnvironment("muto-test")
        err := testEnv.Setup(ctx)
        Expect(err).NotTo(HaveOccurred())
    })

    AfterEach(func() {
        // Cleanup: Delete test cluster
        testEnv.Teardown(ctx)
    })

    It("creates and monitors a simple agent job", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
        defer cancel()

        // Create test job
        job := &agent.AgentJob{
            Metadata: agent.Metadata{
                Name:   "test-job",
                Tenant: "default",
            },
            Spec: agent.JobSpec{
                Agents: []agent.AgentSpec{
                    {
                        Name:    "worker",
                        Image:   "alpine:latest",
                        Command: []string{"echo"},
                        Args:    []string{"hello world"},
                    },
                },
            },
        }

        // Submit to cluster
        err := testEnv.CreateAgentJob(ctx, job)
        Expect(err).NotTo(HaveOccurred())

        // Wait for completion
        status, err := testEnv.WaitForJobCompletion(ctx, job.Metadata.Name)
        Expect(err).NotTo(HaveOccurred())
        Expect(status).To(Equal(agent.StateCompleted))

        // Verify logs
        logs, err := testEnv.GetJobLogs(ctx, job.Metadata.Name)
        Expect(err).NotTo(HaveOccurred())
        Expect(logs).To(ContainSubstring("hello world"))
    })

    It("isolates jobs by tenant", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
        defer cancel()

        // Create two jobs in different tenants
        jobA := testJob("job-a", "tenant-a")
        jobB := testJob("job-b", "tenant-b")

        err := testEnv.CreateAgentJob(ctx, jobA)
        Expect(err).NotTo(HaveOccurred())
        err = testEnv.CreateAgentJob(ctx, jobB)
        Expect(err).NotTo(HaveOccurred())

        // Verify isolation: tenant-a cannot see tenant-b's job
        jobs, err := testEnv.ListJobsForTenant(ctx, "tenant-a")
        Expect(err).NotTo(HaveOccurred())
        Expect(jobs).To(HaveLen(1))
        Expect(jobs[0].Metadata.Name).To(Equal("job-a"))
    })
})

// Helper to create test job
func testJob(name, tenant string) *agent.AgentJob {
    return &agent.AgentJob{
        Metadata: agent.Metadata{Name: name, Tenant: tenant},
        Spec: agent.JobSpec{
            Agents: []agent.AgentSpec{
                {Name: "worker", Image: "alpine:latest", Command: []string{"true"}},
            },
        },
    }
}
```

### Running Integration Tests

**All integration tests:**
```bash
make test-integration
```

**Kubernetes only:**
```bash
make test-integration-k8s
```

**CloudFoundry only:**
```bash
make test-integration-cf
```

**Specific test with timeout:**
```bash
go test ./test/integration/k8s/... -run TestAgentJobLifecycle -v -timeout 30m
```

### Integration Test Requirements

1. **Setup/Teardown**: Each test must clean up its resources
2. **Timeouts**: Always set context timeouts for operations
3. **Platform-specific**: K8s tests in `test/integration/k8s/`, CF in `test/integration/cf/`
4. **Labels**: Mark with `Label("integration")` for filtering

## End-to-End Tests

Full system tests running both Kubernetes and CloudFoundry scenarios.

### Scenarios

E2E tests verify:

1. **Agent Job Lifecycle**: Create -> Schedule -> Run -> Complete
2. **Multi-Agent Workflows**: Sequential and parallel execution
3. **Message Coordination**: Agents communicate via message bus
4. **Tenant Isolation**: Complete isolation between tenants
5. **Failure Handling**: Retries and error recovery
6. **Scaling**: Multiple concurrent jobs

### Running E2E Tests

```bash
# Full E2E suite (K8s + CF)
make test-e2e

# Just K8s E2E
make test-integration-k8s

# Just CF E2E
make test-integration-cf
```

Expected duration: 10-20 minutes

## CI/CD Integration

### GitHub Actions

Tests run automatically on:
- Every push to main
- Every pull request
- Nightly scheduled runs

**Pipeline stages:**
```
1. Unit Tests (5 min)
2. Lint (2 min)
3. Build (3 min)
4. Integration Tests (15 min)
5. E2E Tests (20 min)
```

Failing tests block PR merge.

### Running Locally Before Push

```bash
# Quick checks (5 min)
make test-unit
go fmt ./...
golangci-lint run ./...

# Full test suite (30 min)
make test-unit
make test-integration
```

## Test Data and Fixtures

### Test Helpers

Common test utilities in `test/integration/helpers.go`:

```go
// Create test environment
testEnv := integration.NewTestEnvironment("test-name")
testEnv.Setup(ctx)
defer testEnv.Teardown(ctx)

// Create test cluster
cluster := testEnv.Cluster()

// Create test job
job := integration.NewTestAgentJob("name", "tenant", "image")
```

### Mock Objects

For unit testing without external dependencies:

```go
type MockMessageBus struct {
    PublishCalls []PublishCall
}

func (m *MockMessageBus) Publish(ctx context.Context, topic string, msg []byte) error {
    m.PublishCalls = append(m.PublishCalls, PublishCall{Topic: topic, Msg: msg})
    return nil
}
```

## Troubleshooting Tests

### Integration tests timeout

Increase timeout:
```bash
go test ./test/integration/... -timeout 40m -v
```

### Tests pass locally but fail in CI

1. Check Go version: `go version` (must be 1.26+)
2. Check Docker: `docker ps` (must be running)
3. Check available resources: `docker stats`
4. Clear cache: `go clean -modcache && go mod tidy`

### Flaky tests

Flaky tests (passing sometimes, failing sometimes):
1. Add more explicit waits and retries
2. Increase timeout allowance
3. Check for race conditions: `go test -race ./...`

Run with race detector enabled:
```bash
make test-unit
GOFLAGS="-race" go test ./...
```

### Docker image pull failures

```bash
# Verify Docker can pull images
docker pull alpine:latest

# Clear Docker cache if needed
docker system prune -a
```

## Coverage Goals

Target coverage by component:

| Component | Target | Current |
|-----------|--------|---------|
| core/scheduler | 85% | 84% |
| core/agent | 90% | 88% |
| platform/k8s | 75% | 72% |
| platform/cf | 75% | 70% |
| mcp/tools | 80% | 78% |

View coverage:
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

---

## Next Steps

- [Setup Guide](./setup.md) — Configure development environment
- [Code Style](./style.md) — Coding standards
- [Debugging Guide](./debugging.md) — Troubleshooting techniques

---

**Last Updated:** 2026-09-03
