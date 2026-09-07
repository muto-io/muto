# Integration Tests

Comprehensive guide to writing and running integration tests for Muto. Integration tests verify that components work correctly together using real platforms (Kubernetes or CloudFoundry).

## Overview

Integration tests sit between unit tests and E2E tests:

| Aspect | Unit | Integration | E2E |
|--------|------|-------------|-----|
| **Scope** | Single function | Multiple components | Full system |
| **Speed** | Fast (ms) | Medium (s) | Slow (min) |
| **Setup** | Minimal | Requires platform | Full cluster |
| **Value** | Catch logic errors | Catch integration issues | Catch workflow issues |
| **Coverage** | 90% | 70% | 50% |

Integration tests verify:
- Components communicate correctly
- State flows between systems
- Platform APIs behave as expected
- Error handling works end-to-end

## Structure

Integration tests live in `test/integration/`:

```
test/
├── integration/
│   ├── common/
│   │   ├── helpers.go          # Shared utilities
│   │   ├── fixtures.go         # Test data
│   │   └── environment.go       # Test environment setup
│   ├── k8s/
│   │   ├── suite_test.go       # Ginkgo suite setup
│   │   ├── agent_job_test.go   # Job lifecycle tests
│   │   ├── messaging_test.go   # Message bus tests
│   │   ├── tenant_test.go      # Multi-tenancy tests
│   │   ├── scaling_test.go     # Scaling tests
│   │   └── failure_test.go     # Failure handling
│   ├── cf/
│   │   ├── suite_test.go
│   │   ├── agent_job_test.go
│   │   ├── messaging_test.go
│   │   ├── tenant_test.go
│   │   └── failure_test.go
│   └── README.md               # Integration test guide
```

## Test Environment Setup

### Test Environment Class

All integration tests use a shared test environment:

```go
// test/integration/common/environment.go
package common

import (
    "context"
    "github.com/muto-io/muto/test/integration/k8s"
)

type TestEnvironment struct {
    Name      string
    Namespace string
    Cluster   *k8s.TestCluster
    // ... other fields
}

func NewTestEnvironment(name string) *TestEnvironment {
    return &TestEnvironment{
        Name:      name,
        Namespace: fmt.Sprintf("muto-test-%s", name),
    }
}

func (te *TestEnvironment) Setup(ctx context.Context) error {
    // Create cluster
    // Apply CRDs
    // Start operator
    // Wait for ready
    return nil
}

func (te *TestEnvironment) Teardown(ctx context.Context) error {
    // Delete namespace
    // Clean up resources
    return nil
}
```

### Setup and Teardown

```go
var _ = Describe("AgentJob", Label("integration"), func() {
    var ctx context.Context
    var env *TestEnvironment

    BeforeEach(func() {
        ctx = context.Background()
        env = NewTestEnvironment("test")
        
        err := env.Setup(ctx)
        Expect(err).NotTo(HaveOccurred())
    })

    AfterEach(func() {
        err := env.Teardown(ctx)
        Expect(err).NotTo(HaveOccurred())
    })

    It("creates a job", func() {
        // Test body
    })
})
```

**Key points:**
- Each test gets isolated namespace
- Resources cleaned up after test
- Operator runs in test cluster
- Message bus ready before tests start

## Kubernetes Integration Tests

### Agent Job Lifecycle

Test complete job lifecycle from creation to completion:

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

var _ = Describe("AgentJob Lifecycle", Label("integration", "k8s"), func() {
    var env *TestEnvironment

    BeforeEach(func() {
        env = integration.NewK8sEnvironment("lifecycle")
        Expect(env.Setup(context.Background())).To(Succeed())
    })

    AfterEach(func() {
        Expect(env.Teardown(context.Background())).To(Succeed())
    })

    It("runs a simple agent job to completion", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
        defer cancel()

        // Create job
        job := &agentv1.AgentJob{
            ObjectMeta: metav1.ObjectMeta{
                Name: "simple-job",
            },
            Spec: agentv1.AgentJobSpec{
                Agents: []agentv1.AgentSpec{
                    {
                        Name:    "worker",
                        Image:   "alpine:latest",
                        Command: []string{"echo"},
                        Args:    []string{"hello world"},
                    },
                },
            },
        }

        // Submit job
        err := env.CreateAgentJob(ctx, job)
        Expect(err).NotTo(HaveOccurred())

        // Wait for completion
        completedJob, err := env.WaitForJobCompletion(ctx, "simple-job", 30*time.Second)
        Expect(err).NotTo(HaveOccurred())
        Expect(completedJob.Status.Phase).To(Equal(agentv1.JobSucceeded))

        // Verify logs
        logs, err := env.GetAgentLogs(ctx, "simple-job", "worker")
        Expect(err).NotTo(HaveOccurred())
        Expect(logs).To(ContainSubstring("hello world"))
    })

    It("handles job timeout correctly", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
        defer cancel()

        job := &agentv1.AgentJob{
            ObjectMeta: metav1.ObjectMeta{
                Name: "timeout-job",
            },
            Spec: agentv1.AgentJobSpec{
                Timeout: "5s", // Very short timeout
                Agents: []agentv1.AgentSpec{
                    {
                        Name:    "sleeper",
                        Image:   "alpine:latest",
                        Command: []string{"sleep"},
                        Args:    []string{"100"}, // Sleeps 100 seconds
                    },
                },
            },
        }

        err := env.CreateAgentJob(ctx, job)
        Expect(err).NotTo(HaveOccurred())

        completedJob, err := env.WaitForJobCompletion(ctx, "timeout-job", 30*time.Second)
        Expect(err).NotTo(HaveOccurred())
        Expect(completedJob.Status.Phase).To(Equal(agentv1.JobFailed))
        Expect(completedJob.Status.Message).To(ContainSubstring("timeout"))
    })

    It("retries failed agents", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
        defer cancel()

        job := &agentv1.AgentJob{
            ObjectMeta: metav1.ObjectMeta{
                Name: "retry-job",
            },
            Spec: agentv1.AgentJobSpec{
                Agents: []agentv1.AgentSpec{
                    {
                        Name:       "flaky",
                        Image:      "alpine:latest",
                        Command:    []string{"/bin/sh"},
                        Args:       []string{"-c", "exit 1"}, // Always fails
                        RetryCount: 3,                         // Retry 3 times
                    },
                },
            },
        }

        err := env.CreateAgentJob(ctx, job)
        Expect(err).NotTo(HaveOccurred())

        completedJob, err := env.WaitForJobCompletion(ctx, "retry-job", 1*time.Minute)
        Expect(err).NotTo(HaveOccurred())
        Expect(completedJob.Status.Phase).To(Equal(agentv1.JobFailed))
        Expect(completedJob.Status.RetryCount).To(Equal(3))
    })
})
```

### Message Bus Communication

Test inter-agent communication:

```go
var _ = Describe("Agent Message Communication", Label("integration", "k8s"), func() {
    var env *TestEnvironment

    BeforeEach(func() {
        env = integration.NewK8sEnvironment("messaging")
        Expect(env.Setup(context.Background())).To(Succeed())
    })

    AfterEach(func() {
        Expect(env.Teardown(context.Background())).To(Succeed())
    })

    It("delivers messages between agents", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
        defer cancel()

        job := &agentv1.AgentJob{
            ObjectMeta: metav1.ObjectMeta{
                Name: "messaging-job",
            },
            Spec: agentv1.AgentJobSpec{
                Agents: []agentv1.AgentSpec{
                    {
                        Name:  "publisher",
                        Image: "myorg/publisher:v1", // Publishes message
                    },
                    {
                        Name:  "subscriber",
                        Image: "myorg/subscriber:v1", // Subscribes and processes
                    },
                },
            },
        }

        err := env.CreateAgentJob(ctx, job)
        Expect(err).NotTo(HaveOccurred())

        completedJob, err := env.WaitForJobCompletion(ctx, "messaging-job", 1*time.Minute)
        Expect(err).NotTo(HaveOccurred())
        Expect(completedJob.Status.Phase).To(Equal(agentv1.JobSucceeded))

        // Verify both agents ran
        Expect(completedJob.Status.CompletedAgents).To(Equal(2))
    })
})
```

### Multi-Tenancy Isolation

Test tenant isolation:

```go
var _ = Describe("Tenant Isolation", Label("integration", "k8s"), func() {
    var env *TestEnvironment

    BeforeEach(func() {
        env = integration.NewK8sEnvironment("tenants")
        Expect(env.Setup(context.Background())).To(Succeed())
    })

    AfterEach(func() {
        Expect(env.Teardown(context.Background())).To(Succeed())
    })

    It("isolates jobs by tenant", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
        defer cancel()

        // Create jobs in different tenants
        jobA := integration.NewTestAgentJob("job-a", "tenant-a")
        jobB := integration.NewTestAgentJob("job-b", "tenant-b")

        err := env.CreateAgentJob(ctx, jobA)
        Expect(err).NotTo(HaveOccurred())
        err = env.CreateAgentJob(ctx, jobB)
        Expect(err).NotTo(HaveOccurred())

        // Tenant A cannot see tenant B's job
        jobsA, err := env.ListJobsForTenant(ctx, "tenant-a")
        Expect(err).NotTo(HaveOccurred())
        Expect(jobsA).To(HaveLen(1))
        Expect(jobsA[0].Name).To(Equal("job-a"))

        // Tenant B cannot see tenant A's job
        jobsB, err := env.ListJobsForTenant(ctx, "tenant-b")
        Expect(err).NotTo(HaveOccurred())
        Expect(jobsB).To(HaveLen(1))
        Expect(jobsB[0].Name).To(Equal("job-b"))
    })

    It("enforces resource quotas per tenant", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
        defer cancel()

        // Set resource quota
        err := env.SetTenantQuota(ctx, "tenant-limited", agentv1.ResourceQuota{
            MaxPods:    1,
            MaxCPU:     "1",
            MaxMemory:  "1Gi",
        })
        Expect(err).NotTo(HaveOccurred())

        // Try to create 2 jobs (should fail on second)
        job1 := integration.NewTestAgentJob("job-1", "tenant-limited")
        err = env.CreateAgentJob(ctx, job1)
        Expect(err).NotTo(HaveOccurred())

        job2 := integration.NewTestAgentJob("job-2", "tenant-limited")
        err = env.CreateAgentJob(ctx, job2)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("quota"))
    })
})
```

## CloudFoundry Integration Tests

### Job Execution on CloudFoundry

Test CF task execution:

```go
// test/integration/cf/agent_job_test.go
package cf_test

import (
    "context"
    "time"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/muto-io/muto/test/integration"
)

var _ = Describe("AgentJob Execution on CF", Label("integration", "cf"), func() {
    var env *TestEnvironment

    BeforeEach(func() {
        env = integration.NewCFEnvironment("cf-test")
        Expect(env.Setup(context.Background())).To(Succeed())
    })

    AfterEach(func() {
        Expect(env.Teardown(context.Background())).To(Succeed())
    })

    It("runs agent as CF Task", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
        defer cancel()

        job := integration.NewTestAgentJob("cf-job", "default")
        
        err := env.CreateAgentJob(ctx, job)
        Expect(err).NotTo(HaveOccurred())

        completedJob, err := env.WaitForJobCompletion(ctx, "cf-job", 1*time.Minute)
        Expect(err).NotTo(HaveOccurred())
        Expect(completedJob.Status.Phase).To(Equal(agentv1.JobSucceeded))
    })

    It("binds services for configuration", func(ctx SpecContext) {
        ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
        defer cancel()

        // Setup service bindings
        err := env.CreateServiceBinding(ctx, "nats-service", "nats")
        Expect(err).NotTo(HaveOccurred())

        job := integration.NewTestAgentJob("cf-job-services", "default")
        err = env.CreateAgentJob(ctx, job)
        Expect(err).NotTo(HaveOccurred())

        completedJob, err := env.WaitForJobCompletion(ctx, "cf-job-services", 1*time.Minute)
        Expect(err).NotTo(HaveOccurred())

        // Verify VCAP_SERVICES was injected
        logs, err := env.GetAgentLogs(ctx, "cf-job-services", "worker")
        Expect(err).NotTo(HaveOccurred())
        Expect(logs).To(ContainSubstring("VCAP_SERVICES"))
    })
})
```

## Test Helpers and Fixtures

### Common Helpers

```go
// test/integration/common/helpers.go
package common

import (
    "context"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "github.com/muto-io/muto/api/v1"
)

// Create test job
func NewTestAgentJob(name, tenant string) *v1.AgentJob {
    return &v1.AgentJob{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: "default",
        },
        Spec: v1.AgentJobSpec{
            Tenant: tenant,
            Agents: []v1.AgentSpec{
                {
                    Name:    "worker",
                    Image:   "alpine:latest",
                    Command: []string{"echo"},
                    Args:    []string{"test"},
                },
            },
        },
    }
}

// Create job with custom spec
func NewTestAgentJobWithSpec(name, tenant string, spec v1.AgentJobSpec) *v1.AgentJob {
    job := NewTestAgentJob(name, tenant)
    job.Spec = spec
    return job
}

// Wait for job completion with timeout
func (env *TestEnvironment) WaitForJobCompletion(
    ctx context.Context,
    jobName string,
    timeout time.Duration,
) (*v1.AgentJob, error) {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        job, err := env.GetAgentJob(ctx, jobName)
        if err != nil {
            return nil, err
        }
        if job.Status.Phase != v1.JobPending && job.Status.Phase != v1.JobRunning {
            return job, nil
        }
        time.Sleep(500 * time.Millisecond)
    }
    return nil, fmt.Errorf("job did not complete within %v", timeout)
}
```

### Test Fixtures

```go
// test/integration/common/fixtures.go
package common

var TestJobSpecs = map[string]AgentJobSpec{
    "simple": {
        Agents: []AgentSpec{
            {
                Name:    "worker",
                Image:   "alpine:latest",
                Command: []string{"echo"},
                Args:    []string{"hello"},
            },
        },
    },
    "multi-agent": {
        Agents: []AgentSpec{
            {
                Name:    "producer",
                Image:   "myorg/producer:v1",
                Command: []string{"/app/producer"},
            },
            {
                Name:    "consumer",
                Image:   "myorg/consumer:v1",
                Command: []string{"/app/consumer"},
            },
        },
    },
    "long-running": {
        Timeout: "5m",
        Agents: []AgentSpec{
            {
                Name:    "processor",
                Image:   "myorg/processor:v1",
                Command: []string{"/app/process"},
            },
        },
    },
}
```

## Running Integration Tests

### Run All Integration Tests

```bash
# Kubernetes
make test-integration-k8s

# CloudFoundry
make test-integration-cf

# Both
make test-integration
```

### Run Specific Tests

```bash
# Single test
go test ./test/integration/k8s/... -run TestAgentJobLifecycle -v

# Specific label
go test ./test/integration/k8s/... -run "(integration && k8s)" -v

# With custom timeout
go test ./test/integration/... -timeout 30m -v
```

### Debug Mode

```bash
# Verbose output
go test ./test/integration/k8s/... -v -ginkgo.v

# Show all logs
go test ./test/integration/k8s/... -v -ginkgo.show-node-events=all

# Stop on first failure
go test ./test/integration/k8s/... -ginkgo.fail-fast
```

## Best Practices

### Test Isolation

Each test must be independent:

```go
// ✅ Good: Isolated setup and cleanup
BeforeEach(func() {
    env = NewTestEnvironment("test-" + uniqueID())
    Expect(env.Setup(ctx)).To(Succeed())
})

AfterEach(func() {
    Expect(env.Teardown(ctx)).To(Succeed())
})

// ❌ Bad: Shared state
var sharedEnv *TestEnvironment
var _ = BeforeSuite(func() {
    sharedEnv = NewTestEnvironment("shared")
    // Tests interfere with each other
})
```

### Timeouts

Always set reasonable timeouts:

```go
// ✅ Good: Context with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// ❌ Bad: No timeout
ctx := context.Background()
env.WaitForCompletion(ctx, jobName) // Can hang forever
```

### Assertions

Use meaningful assertions:

```go
// ✅ Good: Clear failure message
Expect(completedJob.Status.Phase).To(Equal(JobSucceeded), "job should complete successfully")

// ❌ Bad: No context
Expect(completedJob.Status.Phase).To(Equal(JobSucceeded))
```

### Test Data

Use fixtures for reusable test data:

```go
// ✅ Good: Reusable fixtures
job := integration.NewTestAgentJobWithSpec("test", "tenant", TestJobSpecs["simple"])

// ❌ Bad: Inline test data
job := &AgentJob{
    Spec: AgentJobSpec{
        Agents: []AgentSpec{{Image: "alpine", ...}},
    },
}
```

## Troubleshooting

### Tests Timeout

```bash
# Increase timeout
go test ./test/integration/... -timeout 40m

# Check if operator is running
kubectl logs -n muto-system deployment/muto-operator

# Check cluster resources
kubectl top nodes
```

### Platform Not Available

```bash
# For Kubernetes
kind cluster-info
docker ps | grep kind

# For CloudFoundry
cf target
cf api
```

### Flaky Tests

1. Add explicit waits for resource readiness
2. Increase timeout allowance
3. Check for race conditions: `go test -race ./...`

### Memory Issues

```bash
# Increase Docker memory
docker run --memory 4g ...

# Run tests sequentially
go test ./test/integration/k8s/... -p 1
```

---

## Related Documentation

- [:octicons-book-24: **Testing Strategy**](./testing-strategy.md) — Overall testing approach
- [:octicons-book-24: **Code Style Guide**](../development/style.md) — Code quality standards
- [:octicons-book-24: **Contributing Guide**](../development/contributing.md) — Contribution workflow
- [:octicons-book-24: **Development Setup**](../development/setup.md) — Environment setup

---

**Last Updated:** 2026-09-06
