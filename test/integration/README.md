# Integration Tests

Comprehensive end-to-end (e2e) test infrastructure for muto, organized by platform with separate test suites for Kubernetes and CloudFoundry.

## Quick Start

### Run K8s E2E Tests
```bash
make test-integration-k8s
```

### Run CF E2E Tests
```bash
export CF_E2E_API_URL=https://api.cf.your-domain.com
export CF_E2E_USERNAME=admin
export CF_E2E_PASSWORD=password
make test-integration-cf
```

### Run Both
```bash
make test-e2e
```

---

## Directory Structure

```
test/integration/
├── k8s/                           # Kubernetes e2e tests (13 files)
│   ├── suite_test.go              # K8s cluster setup
│   ├── agentjob_lifecycle_test.go
│   ├── a2a_gateway_test.go
│   ├── failure_scenarios_test.go
│   ├── helm_deployment_test.go
│   ├── isolation_test.go
│   ├── mcp_roundtrip_test.go
│   ├── stress_testing_test.go
│   ├── tenant_test.go
│   ├── ttl_cleanup_test.go
│   ├── k8s_multiagent_coordination_test.go
│   ├── k8s_advanced_failure_test.go
│   └── k8s_stress_comprehensive_test.go
├── cf/                            # CloudFoundry e2e tests (11 files)
│   ├── suite_test.go              # CF test suite setup
│   ├── e2e_setup_test.go          # CF cluster initialization
│   ├── e2e_job_lifecycle_test.go
│   ├── e2e_multiagent_test.go
│   ├── e2e_failure_scenarios_test.go
│   ├── e2e_isolation_test.go
│   ├── e2e_stress_test.go
│   ├── e2e_cleanup_test.go
│   ├── mockserver_test.go
│   ├── mockclient_test.go
│   └── adapter_test.go
├── helpers.go                     # Shared helpers for both platforms
├── README.md                      # This file
├── E2E_TESTS_SUMMARY.md          # (archived - content below)
└── CONTRIBUTING.md               # (archived - content below)
```

---

## Kubernetes E2E Tests

Located in `test/integration/k8s/`

### Multi-Agent Coordination (`k8s_multiagent_coordination_test.go`)

Tests agent orchestration with multiple roles:

- **Coordinator and Worker Agents**: Verifies pods for both coordinator and worker roles are created and properly labeled
- **Replica Scaling**: Validates independent scaling of worker replicas
- **Message Bus Communication**: Ensures agents can communicate via configured message bus with proper environment variables
- **Role Mutual Exclusivity**: Confirms each pod has exactly one role label

### Advanced Failure Scenarios (`k8s_advanced_failure_test.go`)

Comprehensive failure handling tests:

- **Resource Constraints**: Memory-constrained agents, CPU-constrained agents, proper resource request/limit handling
- **Pod Lifecycle Failures**: Startup failures and recovery, long startup times, prevention of infinite restart loops
- **Concurrent Agent Failures**: Simultaneous pod failures, partial failures in multi-role jobs, job state preservation during failures

### Comprehensive Stress Testing (`k8s_stress_comprehensive_test.go`)

High-volume and concurrency tests:

- **High-Volume Job Creation**: Concurrent creation of 10+ jobs with validation
- **Sequential Job Creation**: Rapid sequential creation with minimal delays
- **High Replica Counts**: Jobs with up to 10 replicas per role
- **Mixed Replica Counts**: Multi-role jobs with varying replica counts
- **Rapid Job Churn**: Create/delete cycles under stress
- **Namespace Resource Exhaustion**: Graceful handling of resource limits

### Original K8s Tests

- **Agent Job Lifecycle** (`agentjob_lifecycle_test.go`): Job creation and lifecycle
- **A2A Gateway** (`a2a_gateway_test.go`): A2A protocol integration
- **Failure Scenarios** (`failure_scenarios_test.go`): Pod eviction and cleanup
- **Helm Deployment** (`helm_deployment_test.go`): Helm-based operator deployment
- **Isolation** (`isolation_test.go`): Namespace and tenant isolation
- **MCP Roundtrip** (`mcp_roundtrip_test.go`): MCP protocol testing
- **Stress Testing** (`stress_testing_test.go`): Original stress scenarios
- **Tenant Tests** (`tenant_test.go`): Tenant resource management
- **TTL Cleanup** (`ttl_cleanup_test.go`): TTL expiration and cleanup

---

## CloudFoundry E2E Tests

Located in `test/integration/cf/`

### Setup and Infrastructure (`e2e_setup_test.go`)

- **Real CF Cluster Support**: Connects to existing CF instance via `CF_E2E_API_URL` environment variable
- **kind-deployment Ready**: Framework prepared to integrate CloudFoundry's kind-deployment when needed
- **Test Organization Management**: Creates/uses test organization and spaces for all e2e tests

### Job Lifecycle (`e2e_job_lifecycle_test.go`)

Core job execution tests:

- **Successful Task Execution**: Create app, run task, verify SUCCEEDED state
- **Task Failure Handling**: Run failing tasks, verify FAILED state
- **Task Cancellation**: Cancel running tasks, verify terminal states
- **Concurrent Task Execution**: Run multiple tasks on same app concurrently

### Multi-Agent Coordination (`e2e_multiagent_test.go`)

Cross-role orchestration:

- **Coordinator-Worker Pattern**: Run coordinator and worker tasks, verify completion order
- **Worker Failure Gracefully**: Handle individual worker failures
- **Resource Constraints**: Tasks with memory and disk limits
- **Proper Scaling**: Create multiple instances of same role

### Failure Scenarios (`e2e_failure_scenarios_test.go`)

Comprehensive failure handling:

- **Task Timeout**: Handle long-running tasks and cancellation
- **Task Crashes**: Handle crashed tasks (kill -9 simulation)
- **Invalid Commands**: Non-existent command execution
- **OOM Scenarios**: Out-of-memory handling with tight resource limits
- **Concurrent Failure Storms**: Many concurrent task failures

### Tenant Isolation (`e2e_isolation_test.go`)

Multi-tenancy and security:

- **Task Isolation Between Tenants**: Verify tasks in different spaces are isolated
- **Prevention of Cross-Boundary Cancellation**: Prevent unauthorized task operations
- **Resource Quotas Per Tenant**: Enforce resource limits per tenant
- **Data Leakage Prevention**: Verify environment variables don't leak between tenants

### Stress Testing (`e2e_stress_test.go`)

High-volume and concurrency:

- **High Volume Concurrent Task Creation**: 20+ concurrent task creation
- **Rapid Submit and Cancel**: Create then immediately cancel tasks
- **Many Apps in Same Space**: Multiple concurrent app deployments
- **Failure Recovery**: Rapid failure scenario handling

### Resource Cleanup (`e2e_cleanup_test.go`)

Cleanup and TTL verification:

- **Completed Task Cleanup**: Verify task history is maintained
- **Task TTL Expiration**: Multiple task completion and cleanup cycles
- **Failed Task Cleanup**: Handle cleanup of failed tasks
- **Running Task Preservation**: Don't cleanup active tasks
- **Bulk Cleanup**: Handle many completed tasks

---

## Running Tests

### Kubernetes Tests (Local)

Run all Kubernetes e2e tests:
```bash
make test-integration-k8s
```

Or with ginkgo:
```bash
cd test/integration/k8s
ginkgo -v ./...
```

Run specific test suite:
```bash
ginkgo -v --focus="Multi-Agent" ./test/integration/k8s
ginkgo -v --focus="Failure" ./test/integration/k8s
ginkgo -v --focus="Stress" ./test/integration/k8s
```

### CloudFoundry Tests (Local)

#### Prerequisites

**Option A: Use existing CF instance**
```bash
export CF_E2E_API_URL=https://api.cf.your-domain.com
export CF_E2E_USERNAME=admin
export CF_E2E_PASSWORD=password

# Create test org and spaces:
cf create-org muto-e2e-test-org
cf create-space -o muto-e2e-test-org muto-test-1
cf create-space -o muto-e2e-test-org muto-test-2
# ... create additional spaces as needed
```

**Option B: Use kind-deployment (framework prepared)**
```bash
git clone https://github.com/cloudfoundry/kind-deployment
export KIND_DEPLOYMENT_PATH=/path/to/kind-deployment
# Deployment will initialize CF cluster automatically
```

#### Run CF Tests
```bash
make test-integration-cf
```

Or with ginkgo:
```bash
ginkgo -v --focus="Lifecycle" ./test/integration/cf
ginkgo -v --focus="Isolation" ./test/integration/cf
ginkgo -v --focus="Stress" ./test/integration/cf
```

### Run All E2E Tests
```bash
make test-e2e
```

---

## GitHub Actions CI/CD

E2E tests run automatically via `.github/workflows/e2e-tests.yml`:

### Triggers
- **Pull Requests** — When changes affect platform, core, or test code
- **Push to main** — On every push to main branch
- **Weekly Schedule** — Runs every Monday at 2 AM UTC

### PR Validation

For PRs:
- **K8s tests** — Always run (required to pass)
- **CF tests** — Run if `CF_E2E_API_URL` secret is configured (optional)

To enable CF testing in CI/CD, configure GitHub secrets:

Settings -> Secrets and Variables -> Actions

- `CF_API_URL` — CloudFoundry API endpoint
- `CF_USERNAME` — Admin username
- `CF_PASSWORD` — Admin password

### Workflow Configuration

The workflow:
1. Checks out latest code
2. Sets up Go 1.21 environment
3. Runs K8s e2e tests (always)
4. Runs CF e2e tests (if CF credentials available)
5. Uploads test results as artifacts (30-day retention)
6. Reports pass/fail status

---

## Makefile Targets

### New E2E Targets

```bash
make test-integration-k8s      # K8s e2e tests only (20m timeout)
make test-integration-cf       # CF e2e tests only (10m timeout)
make test-e2e                  # Both K8s and CF tests
```

### Existing Targets

```bash
make test-integration          # All integration tests (backward compatible)
make test-unit                 # Unit tests with coverage
```

---

## Test Coverage Matrix

| Scenario | K8s | CF | Notes |
|----------|-----|-----|-------|
| Agent Job Lifecycle | ✓ | ✓ | Create, run, complete |
| Multi-Agent Coordination | ✓ | ✓ | Multiple roles, communication |
| Failure Scenarios | ✓ | ✓ | Crashes, timeouts, OOM |
| Tenant Isolation | ✓ | ✓ | Data/resource separation |
| Resource Constraints | ✓ | ✓ | Memory/CPU limits |
| Concurrent Operations | ✓ | ✓ | High-volume testing |
| Stress Testing | ✓ | ✓ | Job churn, rapid cycling |
| Resource Cleanup | ✓ | ✓ | TTL, completion cleanup |

---

## Shared Test Helpers

**File:** `helpers.go`

Common utilities for both K8s and CF tests:

- `K8sTestHelper`: Generates unique namespaces and manages test counters
- `CFTestHelper`: Generates unique spaces, apps, and tenant names
- `WaitFor`: Generic polling function for async verification
- `WaitForJobPhase`: K8s-specific job phase waiter
- `WaitForTaskState`: CF-specific task state waiter

---

## Contributing New Tests

### Adding a Kubernetes E2E Test

Create a new file in `test/integration/k8s/` following this template:

```go
//go:build integration

package k8s_test

import (
	"context"
	"fmt"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("My Feature", func() {
	ctx := context.Background()
	var testCounter int

	Describe("my test suite", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("my-feature-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-%d", testCounter)},
				Spec: v1alpha1.TenantSpec{
					Namespace:     nsName,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should do something", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.test", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "test-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})
	})
})
```

#### Available K8s Globals (from `suite_test.go`)

```go
var (
	cfg          *rest.Config              // Kubernetes REST config
	k8sClient    client.Client             // Kubernetes client
	testEnv      *envtest.Environment      // Test environment
	cancelMgr    context.CancelFunc        // Manager context cancellation
	k3sContainer *tck3s.K3sContainer       // k3s container instance
)

func waitForPhase(t GinkgoTInterface, ctx context.Context, name, namespace, phase string, timeout time.Duration)
```

#### K8s Test Best Practices

**Use unique namespaces** — Avoid test conflicts:
```go
testCounter++
nsName := fmt.Sprintf("my-test-%d", testCounter)
```

**Clean up resources** — Always delete in `AfterEach`:
```go
AfterEach(func() {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	_ = k8sClient.Delete(ctx, ns)
})
```

**Use Eventually for async checks** — Don't use `time.Sleep`:
```go
Eventually(func(g Gomega) {
	// Check condition
}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
```

**Label resources properly**:
```yaml
metadata:
  labels:
    muto.io/job: my-job
    muto.io/role: worker
    muto.io/tenant: my-tenant
```

---

### Adding a CloudFoundry E2E Test

Create a new file in `test/integration/cf/` following this template:

```go
//go:build integration

package cf_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/muto-io/muto/platform/cf"
)

var _ = Describe("My CF Feature", func() {
	Describe("my test suite", func() {
		var (
			spaceName  string
			spaceGUID  string
			tenantName string
		)

		BeforeEach(func() {
			if cfCluster == nil {
				Skip("CF cluster not available")
			}

			spaceName = cfHelper.NextSpace()
			tenantName = cfHelper.NextTenant()

			space, err := cfCluster.Client.GetSpaceByName(ctx, cfTestOrgGUID, spaceName)
			if err == nil {
				spaceGUID = space.GUID
			} else {
				Skip("test space " + spaceName + " not found")
			}
		})

		It("should do something", func() {
			appReq := cf.PushRequest{
				Name:        fmt.Sprintf("test-app-%s", tenantName),
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			taskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-task", tenantName),
				Command: "echo 'test'",
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
```

#### Available CF Globals (from `e2e_setup_test.go`)

```go
var (
	cfCluster      *CFCluster              // CF cluster instance
	cfTestOrgGUID  string                  // Test organization GUID
	cfHelper       *CFTestHelper           // Helper for test resources
	ctx            context.Context         // Test context
)

type CFTestHelper struct {
	Counter      int
	OrgName      string
	SpacePrefix  string
	AppPrefix    string
}

func (h *CFTestHelper) NextSpace() string      // Unique space name
func (h *CFTestHelper) NextTenant() string     // Unique tenant name

func WaitForTaskState(ctx context.Context, client cf.CFClient, taskGUID, targetState string, timeout time.Duration) error
```

#### CF Test Best Practices

**Skip if CF not available** — Handle missing CF gracefully:
```go
BeforeEach(func() {
	if cfCluster == nil {
		Skip("CF cluster not available")
	}
	// ... rest of setup
})
```

**Generate unique names** — Use the helper:
```go
spaceName := cfHelper.NextSpace()   // Generates "muto-test-1", "muto-test-2", etc.
tenantName := cfHelper.NextTenant() // Generates "tenant-1", "tenant-2", etc.
```

**Use Docker images** — Always specify images:
```go
DockerImage: "busybox:latest"
```

**Set resource constraints** — Test with realistic limits:
```go
taskReq := cf.TaskRequest{
	Name:       "constrained-task",
	Command:    "my-command",
	MemoryInMB: 128,
	DiskInMB:   512,
}
```

**Wait for async operations** — Use the wait helper:
```go
err := WaitForTaskState(ctx, cfCluster.Client, task.GUID, "SUCCEEDED", 30*time.Second)
Expect(err).NotTo(HaveOccurred())
```

---

## Test Naming Conventions

### File Names
- K8s: Descriptive names like `k8s_multiagent_coordination_test.go`
- CF: Prefixed with `e2e_` like `e2e_failure_scenarios_test.go`

### Test Descriptions
```go
Describe("Feature Category", func() {
	Describe("specific scenario", func() {
		It("should do something specific", func() {
			// test implementation
		})
	})
})
```

Example:
```
Describe("K8s Multi-Agent Coordination")
  Describe("multi-agent job orchestration")
    It("should coordinate coordinator and worker agents")
```

---

## Debugging Tests

### K8s Test Issues

**Test hangs on pod creation:**
```bash
# Check if k3s cluster is running
docker ps | grep k3s

# Increase timeout
ginkgo -timeout 30m ./test/integration/k8s
```

**Check pod status:**
```bash
Eventually(func(g Gomega) {
	podList := &corev1.PodList{}
	g.Expect(k8sClient.List(ctx, podList,
		client.InNamespace(nsName),
	)).To(Succeed())
	for _, pod := range podList.Items {
		Logf("Pod %s: %s", pod.Name, pod.Status.Phase)
	}
}).Should(Succeed())
```

### CF Test Issues

**Test skips due to missing CF:**
```bash
# Ensure environment variables are set
echo $CF_E2E_API_URL
echo $CF_E2E_USERNAME
echo $CF_E2E_PASSWORD

# Check CF connectivity
curl -k $CF_E2E_API_URL/v3/organizations
```

**Task never completes:**
```bash
taskState, err := cfCluster.Client.GetTask(ctx, task.GUID)
Expect(err).NotTo(HaveOccurred())
Logf("Task state: %s", taskState.State)
```

---

## Performance Baselines

Typical test execution times:
- **K8s suite** — 15-20 minutes (includes k3s startup)
- **CF suite** — 5-10 minutes (requires existing CF instance)
- **Full e2e** — 25-30 minutes

---

## Troubleshooting

### K8s Tests Fail

```bash
# Check if k3s cluster is running
docker ps | grep k3s

# Check test logs
make test-integration-k8s 2>&1 | tail -100

# Run single test
ginkgo -v --focus="lifecycle" ./test/integration/k8s
```

### CF Tests Skip

```bash
# Verify CF credentials
echo $CF_E2E_API_URL
echo $CF_E2E_USERNAME

# Create test org/spaces if missing
cf create-org muto-e2e-test-org
cf create-space -o muto-e2e-test-org muto-test-1
```

### Timeout Issues

Tests have default timeouts. To increase:
```bash
# K8s: Increase pod startup timeout
ginkgo -timeout 30m ./test/integration/k8s

# CF: Increase task completion timeout
ginkgo -timeout 15m ./test/integration/cf
```

---

## Submitting New Tests

1. **Create tests** — Add files to appropriate folder (k8s/ or cf/)
2. **Follow patterns** — Use existing tests as templates
3. **Test locally** — Verify tests pass before pushing
4. **Create PR** — GitHub Actions will validate automatically
5. **Address feedback** — Fix any issues flagged by CI

---

## Test Categories

When adding tests, consider which category they fit:

- **Lifecycle** — Job/task creation and completion
- **Multi-Agent** — Role coordination and communication
- **Failure** — Error handling and recovery
- **Isolation** — Tenant/platform separation
- **Stress** — High-volume and concurrency scenarios
- **Cleanup** — Resource cleanup and TTL

Reference existing tests in these categories for patterns.

---

## Environment Variables

### CF Tests Only
```
CF_E2E_API_URL          # CloudFoundry API endpoint (required for CF tests)
CF_E2E_USERNAME         # CF admin username
CF_E2E_PASSWORD         # CF admin password
KIND_DEPLOYMENT_PATH    # Path to kind-deployment repo (optional, for k8s-hosted CF)
```

### K8s Tests
- Uses existing `testcontainers` k3s setup from suite_test.go
- No additional environment variables required

---

## References

- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
- [Testcontainers Go](https://golang.testcontainers.org/)
- [CloudFoundry Go Client](https://github.com/cloudfoundry/go-cfclient)
- [Muto Platform Adapters](../../platform/)
