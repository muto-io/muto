# Unit Tests

Comprehensive guide to writing unit tests in Muto.

## Overview

Unit tests verify individual functions and types in isolation. They are:

- **Fast** — Run in milliseconds
- **Isolated** — No external dependencies
- **Deterministic** — Same input always gives same output
- **Focused** — Test one behavior per test

## Test Structure

All unit tests use Ginkgo for consistent structure:

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

var _ = Describe("AgentJob", func() {
    Describe("Validate", func() {
        It("rejects jobs with missing tenant", func() {
            job := &agent.AgentJob{
                Metadata: agent.Metadata{Name: "test"},
            }
            err := job.Validate()
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("tenant"))
        })

        It("accepts valid jobs", func() {
            job := &agent.AgentJob{
                Metadata: agent.Metadata{
                    Name:   "test",
                    Tenant: "tenant-a",
                },
                Spec: agent.JobSpec{
                    Agents: []agent.AgentSpec{
                        {Name: "worker", Image: "image:v1"},
                    },
                },
            }
            err := job.Validate()
            Expect(err).NotTo(HaveOccurred())
        })
    })
})
```

## Test File Organization

Unit tests live next to source code:

```
cmd/
├── operator.go
└── operator_test.go       # Tests for operator.go

core/
├── scheduler.go
├── scheduler_test.go      # Tests for scheduler.go
└── agent/
    ├── job.go
    ├── job_test.go        # Tests for job.go
    └── state_machine.go
        state_machine_test.go
```

**Naming convention:** `_test.go` suffix

## Writing Effective Unit Tests

### 1. Test One Behavior Per Test

Each test should verify exactly one behavior:

```go
// ✅ Good: Single responsibility
It("rejects job with no agents", func() {
    job := &agent.AgentJob{Spec: agent.JobSpec{}}
    err := job.Validate()
    Expect(err).To(HaveOccurred())
})

// ❌ Bad: Multiple behaviors
It("validates job", func() {
    job1 := &agent.AgentJob{Spec: agent.JobSpec{}}
    err := job1.Validate()
    Expect(err).To(HaveOccurred())
    
    job2 := &agent.AgentJob{...valid...}
    err = job2.Validate()
    Expect(err).NotTo(HaveOccurred())
})
```

### 2. Use Descriptive Test Names

Test names should describe what behavior is being tested:

```go
// ✅ Good: Describes behavior
It("transitions from Pending to Scheduled", func() { ... })
It("rejects invalid state transitions", func() { ... })
It("increments retry count on failure", func() { ... })

// ❌ Bad: Vague
It("test transition", func() { ... })
It("state machine works", func() { ... })
It("handles retries", func() { ... })
```

### 3. Table-Driven Tests

Use table tests for multiple related test cases:

```go
// ✅ Good: DRY, comprehensive
DescribeTable("Validate",
    func(job *agent.AgentJob, expectErr bool) {
        err := job.Validate()
        if expectErr {
            Expect(err).To(HaveOccurred())
        } else {
            Expect(err).NotTo(HaveOccurred())
        }
    },
    Entry("valid job", validJob, false),
    Entry("missing tenant", jobNoTenant, true),
    Entry("empty agents list", jobNoAgents, true),
    Entry("invalid image", jobBadImage, true),
)

// ❌ Bad: Repetitive
It("validates valid job", func() { ... })
It("rejects job missing tenant", func() { ... })
It("rejects job with no agents", func() { ... })
It("rejects job with bad image", func() { ... })
```

### 4. Arrange-Act-Assert Pattern

Structure tests clearly:

```go
It("schedules job on available node", func() {
    // Arrange: set up test data
    job := &agent.AgentJob{...}
    mockScheduler := NewMockScheduler()
    mockScheduler.AddNode(&Node{Name: "node-1", Free: true})
    
    // Act: execute the behavior
    err := mockScheduler.Schedule(job)
    
    // Assert: verify results
    Expect(err).NotTo(HaveOccurred())
    Expect(mockScheduler.JobOnNode(job, "node-1")).To(BeTrue())
})
```

### 5. Test Error Cases

Always test both success and failure paths:

```go
Describe("CreateJob", func() {
    It("creates job when input is valid", func() {
        job, err := scheduler.CreateJob(validSpec)
        Expect(err).NotTo(HaveOccurred())
        Expect(job.ID).NotTo(BeEmpty())
    })

    It("rejects job when tenant is invalid", func() {
        spec := validSpec
        spec.Tenant = "invalid@#$"
        _, err := scheduler.CreateJob(spec)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("invalid tenant"))
    })

    It("rejects job when resources are too large", func() {
        spec := validSpec
        spec.Resources.Memory = "1000Gi" // Unreasonable
        _, err := scheduler.CreateJob(spec)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("resource"))
    })
})
```

## Mocking Dependencies

### Mock Objects

Mock external dependencies to keep tests isolated:

```go
// Define mock
type MockMessageBus struct {
    PublishCalls []PublishCall
    PublishErr   error
}

func (m *MockMessageBus) Publish(ctx context.Context, topic string, msg []byte) error {
    m.PublishCalls = append(m.PublishCalls, PublishCall{
        Topic: topic,
        Msg:   msg,
    })
    return m.PublishErr
}

// Use in test
It("publishes job status change", func() {
    mockBus := &MockMessageBus{}
    scheduler := NewScheduler(mockBus)
    
    job := &agent.AgentJob{...}
    err := scheduler.Schedule(job)
    
    Expect(err).NotTo(HaveOccurred())
    Expect(len(mockBus.PublishCalls)).To(Equal(1))
    Expect(mockBus.PublishCalls[0].Topic).To(Equal("job.scheduled"))
})
```

### Testing with Mocks

```go
Describe("Scheduler", func() {
    var scheduler *Scheduler
    var mockPlatform *MockPlatformAdapter

    BeforeEach(func() {
        mockPlatform = &MockPlatformAdapter{}
        scheduler = NewScheduler(mockPlatform)
    })

    It("allocates resources via platform", func() {
        mockPlatform.On("AllocateResources").Return(nil)
        job := newTestJob()
        
        err := scheduler.Schedule(context.Background(), job)
        
        Expect(err).NotTo(HaveOccurred())
        mockPlatform.AssertCalled(t, "AllocateResources")
    })

    It("retries allocation on transient failure", func() {
        callCount := 0
        mockPlatform.On("AllocateResources").Run(func(args mock.Arguments) {
            callCount++
        }).Return(errors.New("transient")).Once()
        mockPlatform.On("AllocateResources").Return(nil).Once()
        
        err := scheduler.Schedule(context.Background(), job)
        
        Expect(err).NotTo(HaveOccurred())
        Expect(callCount).To(Equal(2)) // Retried
    })
})
```

## Test Helpers

### Helper Functions

Create reusable test helpers:

```go
// test/common/helpers.go
package test

import "github.com/muto-io/muto/core/agent"

func NewTestAgentJob(name, tenant string) *agent.AgentJob {
    return &agent.AgentJob{
        Metadata: agent.Metadata{
            Name:   name,
            Tenant: tenant,
        },
        Spec: agent.JobSpec{
            Agents: []agent.AgentSpec{
                {
                    Name:    "worker",
                    Image:   "alpine:latest",
                    Command: []string{"true"},
                },
            },
        },
    }
}

func NewTestJobWithSpec(name string, spec agent.JobSpec) *agent.AgentJob {
    job := NewTestAgentJob(name, "default")
    job.Spec = spec
    return job
}

// Use in tests
It("processes job", func() {
    job := test.NewTestAgentJob("test", "tenant-a")
    // ...
})
```

### Test Fixtures

Share test data across multiple tests:

```go
// test/fixtures/jobs.go
package fixtures

var (
    ValidJobSpec = JobSpec{
        Agents: []AgentSpec{
            {Name: "worker", Image: "myimage:v1"},
        },
    }

    MultiAgentSpec = JobSpec{
        Agents: []AgentSpec{
            {Name: "producer", Image: "prod:v1"},
            {Name: "consumer", Image: "cons:v1"},
        },
    }

    LongRunningSpec = JobSpec{
        Timeout: "10m",
        Agents: []AgentSpec{
            {Name: "processor", Image: "proc:v1"},
        },
    }
)

// Use in tests
It("processes multi-agent job", func() {
    job := NewTestJobWithSpec("test", fixtures.MultiAgentSpec)
    // ...
})
```

## Assertions

Use clear, specific assertions:

```go
// ✅ Good: Clear, specific
Expect(job.Status).To(Equal(agent.StatePending))
Expect(err.Error()).To(ContainSubstring("invalid tenant"))
Expect(count).To(BeNumerically(">", 0))

// ❌ Bad: Vague
Expect(result).To(BeTrue())
Expect(err).To(HaveOccurred())
Expect(list).NotTo(BeEmpty())
```

### Common Matchers

```go
// Equality
Expect(value).To(Equal(expected))
Expect(value).NotTo(Equal(unexpected))

// Errors
Expect(err).NotTo(HaveOccurred())
Expect(err).To(HaveOccurred())
Expect(err.Error()).To(ContainSubstring("message"))

// Strings
Expect(str).To(Equal("value"))
Expect(str).To(ContainSubstring("substring"))
Expect(str).To(MatchRegexp("pattern"))

// Collections
Expect(list).To(HaveLen(3))
Expect(list).To(BeEmpty())
Expect(list).To(ContainElement(item))

// Numeric
Expect(count).To(BeNumerically(">", 0))
Expect(value).To(BeNumerically("==", 42))

// Custom
Expect(obj.Name).To(Equal("expected"))
```

## Running Unit Tests

### Run All Unit Tests

```bash
make test-unit
```

### Run Specific Package

```bash
go test ./core/agent -v
```

### Run Single Test

```bash
go test ./core/agent -run TestValidate -v
```

### With Coverage

```bash
# Generate coverage report
go test ./... -coverprofile=coverage.out

# View in browser
go tool cover -html=coverage.out
```

### With Race Detector

Detect race conditions:

```bash
go test -race ./...
```

### Continuous Mode

Watch for changes and rerun:

```bash
# Install watcher
go install github.com/cosmtrek/air@latest

# Run in watch mode
air
```

## Best Practices Checklist

- [ ] One behavior per test
- [ ] Descriptive test names
- [ ] Mock external dependencies
- [ ] Test both success and failure
- [ ] Use table tests for variants
- [ ] Arrange-Act-Assert pattern
- [ ] Clean up in AfterEach
- [ ] Use test helpers for DRY code
- [ ] Test error messages
- [ ] Keep tests deterministic (no random data)

## Troubleshooting

### Test Fails Intermittently (Flaky)

**Causes:**
- Non-deterministic test data
- Missing synchronization
- Time-dependent logic

**Solutions:**
```go
// ✅ Good: Explicit waits
Eventually(func() bool {
    return job.Status == Completed
}).WithTimeout(5 * time.Second).Should(BeTrue())

// ❌ Bad: Sleep
time.Sleep(1 * time.Second)
if job.Status != Completed {
    // Test fails inconsistently
}
```

### Test Can't Find Import

```bash
# Make sure dependencies are installed
go mod tidy
go mod download

# Run test again
go test ./...
```

### Too Many Mocks

If you're mocking more than 2 dependencies, it's a sign the unit is too coupled:

- Break into smaller units
- Consider integration test instead
- Refactor to reduce dependencies

---

## Related Documentation

- [:octicons-book-24: **Testing Overview**](./overview.md) — Testing strategy
- [:octicons-book-24: **Integration Tests**](./integration-tests.md) — Component integration
- [:octicons-book-24: **Code Style Guide**](../development/style.md) — Code quality standards
- [:octicons-book-24: **Contributing Guide**](../development/contributing.md) — Contribution workflow

---

**Last Updated:** 2026-09-06
