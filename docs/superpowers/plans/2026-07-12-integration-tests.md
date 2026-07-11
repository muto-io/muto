# Integration Tests — Ginkgo + Testcontainers-go Kind Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the existing envtest-based integration suite (which never wires reconcilers and times out) with a Ginkgo BDD suite backed by testcontainers-go kind, with all three reconcilers running in-process against a live cluster.

**Architecture:** `BeforeSuite` starts a kind cluster via testcontainers-go, connects to it with `envtest.Environment{UseExistingCluster: true}`, registers all reconcilers into a controller-runtime manager, and exposes `k8sClient` as a package var. All async assertions use `Eventually`/`Consistently` — no `time.Sleep`.

**Tech Stack:** Go 1.22+, Ginkgo v2, Gomega, testcontainers-go + kind module, controller-runtime v0.19.0

---

## File Map

```
go.mod                                          # add testcontainers-go + kind module
Makefile                                        # add test-integration-kind target, update test-integration
.github/workflows/ci.yml                        # replace setup-envtest with Docker-based kind
test/integration/suite_test.go                  # REPLACE: Ginkgo BeforeSuite/AfterSuite
test/integration/tenant_test.go                 # REPLACE: Ginkgo Describe/It with Eventually
test/integration/agentjob_lifecycle_test.go     # REPLACE: Ginkgo with Eventually
test/integration/ttl_cleanup_test.go            # REPLACE: Ginkgo with Eventually
test/integration/isolation_test.go              # REPLACE: Ginkgo (pure logic, no cluster)
test/integration/mcp_roundtrip_test.go          # REPLACE: Ginkgo with live K8sAdapter
```

---

### Task 1: Add testcontainers-go dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add testcontainers-go and kind module**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/k8s@latest
go mod tidy
```

Note: the kind provider in testcontainers-go is under `modules/k8s` (not `modules/kind`). Verify the exact module path after `go get`:
```bash
grep "testcontainers" go.mod
```

- [ ] **Step 2: Verify it resolves**

```bash
go build ./... 2>&1 | head -10
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add testcontainers-go with k8s/kind module"
```

---

### Task 2: Update Makefile and CI

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Update Makefile**

Replace the current `test-integration` target and add `test-integration-kind` in `Makefile`:

```makefile
test-integration-kind:
	go test ./test/integration/... -tags integration -v -timeout 15m

test-integration: test-integration-kind
```

The full updated Makefile (replace existing content):
```makefile
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.0
BINARY_DIR     := bin

.PHONY: generate build test-unit test-integration test-integration-kind kind-up kind-down docker-build

generate:
	$(CONTROLLER_GEN) crd paths="./platform/k8s/types/..." output:crd:artifacts:config=deploy/crds
	$(CONTROLLER_GEN) object paths="./platform/k8s/types/..."

build:
	mkdir -p $(BINARY_DIR)
	go build -o $(BINARY_DIR)/muto-operator ./cmd/muto-operator
	go build -o $(BINARY_DIR)/muto-mcp ./cmd/muto-mcp

test-unit:
	go test ./... -short -count=1 -coverprofile=coverage.out

test-integration-kind:
	go test ./test/integration/... -tags integration -v -timeout 15m

test-integration: test-integration-kind

kind-up:
	kind create cluster --config deploy/kind/kind-config.yaml --name muto-dev
	kubectl apply -f deploy/crds/

kind-down:
	kind delete cluster --name muto-dev

docker-build:
	docker build -t muto-operator:dev -f Dockerfile.operator .
	docker build -t muto-mcp:dev -f Dockerfile.mcp .
```

- [ ] **Step 2: Update CI integration-test job**

Replace the `integration-test` job in `.github/workflows/ci.yml` (Docker is pre-installed on GitHub-hosted ubuntu-latest runners):

```yaml
  integration-test:
    name: Integration Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: Run integration tests (kind via testcontainers)
        run: make test-integration-kind
        timeout-minutes: 15
```

- [ ] **Step 3: Commit**

```bash
git add Makefile .github/workflows/ci.yml
git commit -m "ci: switch integration tests to testcontainers-go kind (no setup-envtest)"
```

---

### Task 3: Rewrite suite_test.go

**Files:**
- Modify: `test/integration/suite_test.go`

- [ ] **Step 1: Check testcontainers-go k8s module API**

```bash
find $(go env GOMODCACHE) -path "*/testcontainers-go*/modules/k8s/*.go" 2>/dev/null | head -10
```
Read the exported functions to understand the exact API for creating a kind cluster. Look for `RunContainer`, `New`, or similar. The kubeconfig is typically retrieved via a method on the returned container.

- [ ] **Step 2: Write the new suite_test.go**

Replace the entire file with:

```go
//go:build integration

package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	// Import testcontainers kind module — adjust import path based on what go get resolved
	tck8s "github.com/testcontainers/testcontainers-go/modules/k8s"
)

var (
	cfg        *rest.Config
	k8sClient  client.Client
	testEnv    *envtest.Environment
	cancelMgr  context.CancelFunc
	kindCluster *tck8s.K8sContainer
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Muto Integration Suite")
}

var _ = BeforeSuite(func() {
	ctx := context.Background()

	// Start kind cluster via testcontainers
	var err error
	kindCluster, err = tck8s.Run(ctx, "kindest/node:v1.31.0")
	Expect(err).NotTo(HaveOccurred())

	// Write kubeconfig to temp file and load it
	kubeconfigBytes, err := kindCluster.GetKubeconfig(ctx)
	Expect(err).NotTo(HaveOccurred())
	kubeconfigPath := filepath.Join(os.TempDir(), "muto-integration-kubeconfig")
	Expect(os.WriteFile(kubeconfigPath, kubeconfigBytes, 0600)).To(Succeed())
	os.Setenv("KUBECONFIG", kubeconfigPath)

	// Connect via envtest using existing cluster
	crdPath, err := filepath.Abs("../../deploy/crds")
	Expect(err).NotTo(HaveOccurred())
	testEnv = &envtest.Environment{
		UseExistingCluster:    boolPtr(true),
		CRDDirectoryPaths:     []string{crdPath},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	// Build scheme
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

	// Create client
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	// Start controller-runtime manager with all reconcilers
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	Expect((&reconcilers.TenantReconciler{Client: mgr.GetClient(), Scheme: scheme}).
		SetupWithManager(mgr)).To(Succeed())
	Expect((&reconcilers.AgentJobReconciler{Client: mgr.GetClient(), Scheme: scheme}).
		SetupWithManager(mgr)).To(Succeed())
	Expect((&reconcilers.AgentFleetReconciler{Client: mgr.GetClient(), Scheme: scheme}).
		SetupWithManager(mgr)).To(Succeed())

	var mgrCtx context.Context
	mgrCtx, cancelMgr = context.WithCancel(context.Background())
	go func() {
		Expect(mgr.Start(mgrCtx)).To(Succeed())
	}()
})

var _ = AfterSuite(func() {
	cancelMgr()
	Expect(testEnv.Stop()).To(Succeed())
	if kindCluster != nil {
		Expect(kindCluster.Terminate(context.Background())).To(Succeed())
	}
})

func boolPtr(b bool) *bool { return &b }
```

**Important:** After writing the file, check the actual testcontainers-go k8s module API. The import path and function signatures above are based on the v0.x API — if the module was resolved to a different path (e.g. `modules/kind` instead of `modules/k8s`), update accordingly. Check with:
```bash
find $(go env GOMODCACHE) -path "*/testcontainers*/modules/k*" -name "*.go" -not -name "*_test.go" 2>/dev/null | head -5
```

- [ ] **Step 3: Verify it compiles**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler && go build -tags integration ./test/integration/... 2>&1
```
Expected: no errors. Fix any import path issues.

- [ ] **Step 4: Commit**

```bash
git add test/integration/suite_test.go
git commit -m "test(integration): replace envtest bootstrap with testcontainers-go kind suite"
```

---

### Task 4: Rewrite tenant_test.go

**Files:**
- Modify: `test/integration/tenant_test.go`

- [ ] **Step 1: Write the new tenant_test.go**

```go
//go:build integration

package integration_test

import (
	"context"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Tenant", func() {
	ctx := context.Background()

	Describe("creating a Tenant CR", func() {
		var tenant *v1alpha1.Tenant

		BeforeEach(func() {
			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "integration-tenant"},
				Spec: v1alpha1.TenantSpec{
					Namespace:     "integration-tenant-agents",
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, tenant)
		})

		It("creates the target namespace with muto.io/tenant label", func() {
			Eventually(func(g Gomega) {
				ns := &corev1.Namespace{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "integration-tenant-agents"}, ns)).
					To(Succeed())
				g.Expect(ns.Labels["muto.io/tenant"]).To(Equal("integration-tenant"))
			}).WithTimeout(15 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())
		})

		It("sets Tenant status.ready=true", func() {
			Eventually(func(g Gomega) {
				updated := &v1alpha1.Tenant{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "integration-tenant"}, updated)).
					To(Succeed())
				g.Expect(updated.Status.Ready).To(BeTrue())
			}).WithTimeout(15 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())
		})
	})
})
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler && go build -tags integration ./test/integration/... 2>&1
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add test/integration/tenant_test.go
git commit -m "test(integration): rewrite tenant test as Ginkgo spec with Eventually"
```

---

### Task 5: Rewrite agentjob_lifecycle_test.go

**Files:**
- Modify: `test/integration/agentjob_lifecycle_test.go`

- [ ] **Step 1: Write the new agentjob_lifecycle_test.go**

```go
//go:build integration

package integration_test

import (
	"context"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AgentJob", func() {
	ctx := context.Background()

	Describe("lifecycle", func() {
		var (
			ns  *corev1.Namespace
			job *v1alpha1.AgentJob
		)

		BeforeEach(func() {
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "lifecycle-test"}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			job = &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "lifecycle-job", Namespace: "lifecycle-test"},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: "test",
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
					MessageBus:         v1alpha1.JobBusSpec{Topic: "tenant.test.lifecycle-job"},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, job)
			_ = k8sClient.Delete(ctx, ns)
		})

		It("transitions to Running phase", func() {
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "lifecycle-job", Namespace: "lifecycle-test",
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("creates agent pods with correct labels", func() {
			// Wait for Running first
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "lifecycle-job", Namespace: "lifecycle-test",
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).Should(Succeed())

			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace("lifecycle-test"),
				client.MatchingLabels{"muto.io/job": "lifecycle-job"},
			)).To(Succeed())
			Expect(podList.Items).NotTo(BeEmpty())
			Expect(podList.Items[0].Labels["muto.io/tenant"]).To(Equal("test"))
		})
	})
})
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler && go build -tags integration ./test/integration/... 2>&1
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add test/integration/agentjob_lifecycle_test.go
git commit -m "test(integration): rewrite agentjob lifecycle test as Ginkgo spec"
```

---

### Task 6: Rewrite ttl_cleanup_test.go

**Files:**
- Modify: `test/integration/ttl_cleanup_test.go`

- [ ] **Step 1: Write the new ttl_cleanup_test.go**

```go
//go:build integration

package integration_test

import (
	"context"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AgentJob TTL cleanup", func() {
	ctx := context.Background()

	var (
		ns  *corev1.Namespace
		job *v1alpha1.AgentJob
	)

	BeforeEach(func() {
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ttl-test"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		job = &v1alpha1.AgentJob{
			ObjectMeta: metav1.ObjectMeta{Name: "ttl-job", Namespace: "ttl-test"},
			Spec: v1alpha1.AgentJobSpec{
				TenantRef:          "test",
				Trigger:            v1alpha1.TriggerSpec{Type: "manual"},
				Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
				TTLAfterCompletion: 2,
			},
		}
		Expect(k8sClient.Create(ctx, job)).To(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, ns)
	})

	It("deletes the AgentJob after TTL expires", func() {
		// Patch status to Succeeded with CompletedAt = now
		now := metav1.Now()
		patch := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = "Succeeded"
		job.Status.CompletedAt = &now
		Expect(k8sClient.Status().Patch(ctx, job, patch)).To(Succeed())

		// AgentJob should be deleted within TTL + reconcile time
		Eventually(func(g Gomega) {
			check := &v1alpha1.AgentJob{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "ttl-job", Namespace: "ttl-test"}, check)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(20 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})
})
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler && go build -tags integration ./test/integration/... 2>&1
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add test/integration/ttl_cleanup_test.go
git commit -m "test(integration): rewrite TTL cleanup test as Ginkgo spec"
```

---

### Task 7: Rewrite isolation_test.go

**Files:**
- Modify: `test/integration/isolation_test.go`

- [ ] **Step 1: Write the new isolation_test.go**

```go
//go:build integration

package integration_test

import (
	"github.com/muto-io/muto/core/tenant"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tenant isolation", func() {
	Describe("topic prefix enforcement", func() {
		It("blocks cross-tenant topic access", func() {
			topicA := tenant.TopicPrefix("tenant-a") + "job.1"
			Expect(tenant.ValidateTopic("tenant-b", topicA)).To(HaveOccurred())
		})

		It("allows same-tenant topic access", func() {
			topic := tenant.TopicPrefix("acme") + "job.42"
			Expect(tenant.ValidateTopic("acme", topic)).To(Succeed())
		})
	})
})
```

- [ ] **Step 2: Verify compilation**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler && go build -tags integration ./test/integration/... 2>&1
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add test/integration/isolation_test.go
git commit -m "test(integration): rewrite isolation test as Ginkgo spec"
```

---

### Task 8: Rewrite mcp_roundtrip_test.go

**Files:**
- Modify: `test/integration/mcp_roundtrip_test.go`

- [ ] **Step 1: Write the new mcp_roundtrip_test.go**

```go
//go:build integration

package integration_test

import (
	"context"
	"time"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	"github.com/muto-io/muto/mcp/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP round-trip", func() {
	ctx := context.Background()

	It("schedules, checks status, and cancels a job", func() {
		adapter := k8sadapter.NewK8sAdapter(k8sClient, "default")
		sched := scheduler.NewDefaultScheduler(adapter)
		h := tools.NewHandlers(sched)

		Expect(h.ScheduleAgentJob(ctx, "mcp-test-job", "acme", "busybox:latest", "", 60)).
			To(Succeed())

		Eventually(func(g Gomega) {
			st, err := h.GetJobStatus(ctx, "mcp-test-job")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(st.Phase).To(Equal(agent.PhaseRunning))
		}).WithTimeout(10 * time.Second).WithPolling(200 * time.Millisecond).Should(Succeed())

		Expect(h.CancelJob(ctx, "mcp-test-job")).To(Succeed())

		Eventually(func(g Gomega) {
			st, err := h.GetJobStatus(ctx, "mcp-test-job")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(st.Phase).To(Equal(agent.PhaseTerminating))
		}).WithTimeout(5 * time.Second).WithPolling(200 * time.Millisecond).Should(Succeed())
	})
})
```

- [ ] **Step 2: Verify all integration tests compile**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler && go build -tags integration ./test/integration/... 2>&1
```
Expected: no errors.

- [ ] **Step 3: Run unit tests to confirm no regressions**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler && make test-unit 2>&1 | tail -15
```
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add test/integration/mcp_roundtrip_test.go
git commit -m "test(integration): rewrite MCP round-trip test as Ginkgo spec"
```

---

## Self-Review

**Spec coverage:**
| Spec Requirement | Task |
|---|---|
| testcontainers-go kind dependency | Task 1 |
| Makefile test-integration-kind target | Task 2 |
| CI switch from setup-envtest to Docker | Task 2 |
| BeforeSuite: kind cluster + manager with all reconcilers | Task 3 |
| AfterSuite: cancel manager + terminate cluster | Task 3 |
| Tenant CR → namespace + label test | Task 4 |
| AgentJob Pending→Running + pod creation | Task 5 |
| TTL cleanup test | Task 6 |
| Isolation (pure logic) | Task 7 |
| MCP round-trip via tools.Handlers | Task 8 |
| All async assertions use Eventually (no time.Sleep) | Tasks 4-8 |

**No placeholders found.** All code blocks complete.

**Type consistency:** `k8sClient` (package var set in BeforeSuite), `v1alpha1.Tenant`, `v1alpha1.AgentJob`, `v1alpha1.AgentRoleSpec`, `v1alpha1.TriggerSpec`, `v1alpha1.JobBusSpec` — all consistent with existing types in `platform/k8s/types/v1alpha1/`.
