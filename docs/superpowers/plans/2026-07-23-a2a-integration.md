# A2A Protocol Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Google's Agent2Agent (A2A) protocol as a second communication channel in Muto, provisioned via `type: a2a` in `Tenant.spec.messageBus`, following the same pattern as the existing Kafka configuration.

**Architecture:** A new `core/a2a/` package (no K8s imports) provides `BusTypeA2A`, `Config`, and `A2AClient`. The `TenantReconciler` gains a `reconcileA2AGateway` branch that provisions a Deployment + Service + Secret in the tenant namespace. The `AgentJobReconciler` fetches the parent Tenant and injects `MUTO_A2A_GATEWAY` + `MUTO_A2A_TOKEN` env vars into pods when the tenant uses A2A.

**Tech Stack:** Go 1.26, `net/http` stdlib (no new deps), `k8s.io/api`, `sigs.k8s.io/controller-runtime`, `sigs.k8s.io/controller-runtime/pkg/client/fake` (tests), `net/http/httptest` (tests).

---

## File Map

| File | Status | Responsibility |
|---|---|---|
| `core/a2a/a2a.go` | Create | `BusTypeA2A` const, `Config` struct |
| `core/a2a/client.go` | Create | `A2AClient`, `TaskResult`, `New()`, `SendTask()`, `GetTaskStatus()` |
| `core/a2a/client_test.go` | Create | Unit tests for `A2AClient` against `httptest.Server` |
| `platform/k8s/types/v1alpha1/tenant_types.go` | Modify | Add `a2a` to kubebuilder enum marker on `TenantBusSpec.Type` |
| `platform/k8s/reconcilers/tenant_reconciler.go` | Modify | Add `reconcileA2AGateway()`, bus-type switch, readiness gate |
| `platform/k8s/reconcilers/tenant_reconciler_test.go` | Modify | Add A2A provisioning test cases |
| `platform/k8s/reconcilers/agentjob_reconciler.go` | Modify | Fetch parent Tenant in `reconcilePending`, pass to `buildPod`, inject A2A env vars |
| `platform/k8s/reconcilers/agentjob_reconciler_test.go` | Modify | Add A2A env var assertion test |
| `deploy/crds/muto.io_tenants.yaml` | Regenerate | Run `make generate` to update enum |
| `test/integration/a2a_gateway_test.go` | Create | Integration test: A2A tenant → gateway provisioned → job pods have env vars |

---

## Task 1: `core/a2a/a2a.go` — BusType constant and Config

**Files:**
- Create: `core/a2a/a2a.go`

- [ ] **Step 1: Create the file**

```go
package a2a

import "github.com/muto-io/muto/core/messaging"

// BusTypeA2A is the bus type string for the A2A protocol.
// It uses messaging.BusType for consistency with nats and kafka constants.
const BusTypeA2A messaging.BusType = "a2a"

// Config holds the coordinates needed to connect to an A2A gateway.
type Config struct {
	GatewayURL string
	AuthToken  string
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./core/a2a/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add core/a2a/a2a.go
git commit -m "feat(a2a): add BusTypeA2A const and Config struct"
```

---

## Task 2: `core/a2a/client.go` — A2AClient

**Files:**
- Create: `core/a2a/client.go`

- [ ] **Step 1: Write the failing test first** (`core/a2a/client_test.go`)

```go
package a2a_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muto-io/muto/core/a2a"
)

func TestNewReturnsErrorOnEmptyURL(t *testing.T) {
	_, err := a2a.New(&a2a.Config{GatewayURL: ""})
	if err == nil {
		t.Fatal("expected error for empty GatewayURL, got nil")
	}
}

func TestSendTaskReturnsTaskID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks/send" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"taskId": "task-abc",
			"state":  "submitted",
			"output": nil,
		})
	}))
	defer srv.Close()

	client, err := a2a.New(&a2a.Config{GatewayURL: srv.URL, AuthToken: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SendTask(context.Background(), "agent-1", []byte(`{"hello":"world"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "task-abc" {
		t.Errorf("expected task-abc, got %q", result.TaskID)
	}
	if result.State != "submitted" {
		t.Errorf("expected submitted, got %q", result.State)
	}
}

func TestGetTaskStatusReturnsState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks/task-xyz/status" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"taskId": "task-xyz",
			"state":  "completed",
			"output": []byte(`{"result":"ok"}`),
		})
	}))
	defer srv.Close()

	client, err := a2a.New(&a2a.Config{GatewayURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetTaskStatus(context.Background(), "task-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "completed" {
		t.Errorf("expected completed, got %q", result.State)
	}
}

func TestSendTaskReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := a2a.New(&a2a.Config{GatewayURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendTask(context.Background(), "agent-1", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./core/a2a/... -v
```

Expected: `FAIL` — `a2a.New` undefined.

- [ ] **Step 3: Implement `core/a2a/client.go`**

```go
package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// A2AClient sends tasks to agents via an A2A gateway.
type A2AClient struct {
	gatewayURL string
	authToken  string
	httpClient *http.Client
}

// TaskResult is the response from a SendTask or GetTaskStatus call.
type TaskResult struct {
	TaskID string
	State  string // submitted | working | completed | failed
	Output []byte
}

// New creates an A2AClient. Returns an error if GatewayURL is empty.
func New(cfg *Config) (*A2AClient, error) {
	if cfg.GatewayURL == "" {
		return nil, fmt.Errorf("a2a: GatewayURL must not be empty")
	}
	return &A2AClient{
		gatewayURL: cfg.GatewayURL,
		authToken:  cfg.AuthToken,
		httpClient: &http.Client{},
	}, nil
}

// SendTask submits a task payload to the named agent via the A2A gateway.
// The caller is responsible for retry logic.
func (c *A2AClient) SendTask(ctx context.Context, agentID string, payload []byte) (*TaskResult, error) {
	body := map[string]any{"agentId": agentID, "payload": payload}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("a2a: marshal send task: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gatewayURL+"/tasks/send", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("a2a: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	return c.doRequest(req)
}

// GetTaskStatus polls the current state of a task from the gateway.
func (c *A2AClient) GetTaskStatus(ctx context.Context, taskID string) (*TaskResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/tasks/%s/status", c.gatewayURL, taskID), nil)
	if err != nil {
		return nil, fmt.Errorf("a2a: create request: %w", err)
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	return c.doRequest(req)
}

type gatewayResponse struct {
	TaskID string          `json:"taskId"`
	State  string          `json:"state"`
	Output json.RawMessage `json:"output"`
}

func (c *A2AClient) doRequest(req *http.Request) (*TaskResult, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a2a: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("a2a: gateway returned %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("a2a: read response: %w", err)
	}
	var gr gatewayResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return nil, fmt.Errorf("a2a: unmarshal response: %w", err)
	}
	return &TaskResult{
		TaskID: gr.TaskID,
		State:  gr.State,
		Output: []byte(gr.Output),
	}, nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./core/a2a/... -v
```

Expected: all four tests PASS.

- [ ] **Step 5: Commit**

```bash
git add core/a2a/client.go core/a2a/client_test.go
git commit -m "feat(a2a): add A2AClient with SendTask and GetTaskStatus"
```

---

## Task 3: CRD type — add `a2a` enum value

**Files:**
- Modify: `platform/k8s/types/v1alpha1/tenant_types.go:35`

- [ ] **Step 1: Update the kubebuilder marker**

In `platform/k8s/types/v1alpha1/tenant_types.go`, change line 35 from:

```go
// +kubebuilder:validation:Enum=nats;kafka
Type string `json:"type"`
```

to:

```go
// +kubebuilder:validation:Enum=nats;kafka;a2a
Type string `json:"type"`
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./platform/k8s/...
```

Expected: no output.

- [ ] **Step 3: Regenerate CRDs**

```bash
make generate
```

Expected: `deploy/crds/muto.io_tenants.yaml` updated — the `type` field's enum in the CRD YAML now includes `a2a`.

Verify:
```bash
grep -A5 'type:$' deploy/crds/muto.io_tenants.yaml | grep 'a2a'
```

Expected: one line containing `- a2a`.

- [ ] **Step 4: Commit**

```bash
git add platform/k8s/types/v1alpha1/tenant_types.go deploy/crds/muto.io_tenants.yaml
git commit -m "feat(crd): add a2a to TenantBusSpec.Type enum"
```

---

## Task 4: TenantReconciler — `reconcileA2AGateway`

**Files:**
- Modify: `platform/k8s/reconcilers/tenant_reconciler.go`

**Background:** The current `TenantReconciler` only creates a Namespace and sets `Ready: true`. We're adding a bus-type switch and a new method `reconcileA2AGateway` that provisions Deployment + Service + Secret in the tenant namespace before setting Ready.

The gateway image is controlled by operator env var `MUTO_A2A_GATEWAY_IMAGE`. If unset, use a placeholder — **confirm the real image with your team before shipping**.

- [ ] **Step 1: Write the failing tests** (append to `platform/k8s/reconcilers/tenant_reconciler_test.go`)

```go
func TestTenantReconcilerA2AGatewayProvisioned(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "a2a-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:     "a2a-ns",
			IsolationTier: "dedicated",
			MessageBus:    v1alpha1.TenantBusSpec{Type: "a2a", Dedicated: true},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant).
		WithStatusSubresource(&v1alpha1.Tenant{}).Build()
	r := &reconcilers.TenantReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "a2a-tenant"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Deployment must exist
	dep := &appsv1.Deployment{}
	if err := fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "a2a-gateway", Namespace: "a2a-ns"}, dep); err != nil {
		t.Errorf("a2a-gateway Deployment not created: %v", err)
	}

	// Service must exist
	svc := &corev1.Service{}
	if err := fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "a2a-gateway", Namespace: "a2a-ns"}, svc); err != nil {
		t.Errorf("a2a-gateway Service not created: %v", err)
	}
	if svc.Spec.Ports[0].Port != 8080 {
		t.Errorf("expected port 8080, got %d", svc.Spec.Ports[0].Port)
	}

	// Secret must exist
	sec := &corev1.Secret{}
	if err := fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "muto-a2a-token", Namespace: "a2a-ns"}, sec); err != nil {
		t.Errorf("muto-a2a-token Secret not created: %v", err)
	}
	if len(sec.Data["token"]) == 0 {
		t.Error("expected non-empty token in Secret")
	}
}

func TestTenantReconcilerA2ANotDedicatedSkipsGateway(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "a2a-shared"},
		Spec: v1alpha1.TenantSpec{
			Namespace:     "a2a-shared-ns",
			IsolationTier: "shared",
			MessageBus:    v1alpha1.TenantBusSpec{Type: "a2a", Dedicated: false},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant).
		WithStatusSubresource(&v1alpha1.Tenant{}).Build()
	r := &reconcilers.TenantReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "a2a-shared"},
	})
	if err != nil {
		t.Fatal(err)
	}

	dep := &appsv1.Deployment{}
	err = fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "a2a-gateway", Namespace: "a2a-shared-ns"}, dep)
	if err == nil {
		t.Error("expected no Deployment for non-dedicated A2A tenant, but one was created")
	}
}
```

Add the required import for `appsv1` at the top of the test file:

```go
import (
    // existing imports ...
    appsv1 "k8s.io/api/apps/v1"
)
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./platform/k8s/reconcilers/... -v -run TestTenantReconcilerA2A
```

Expected: FAIL — `reconcileA2AGateway` undefined.

- [ ] **Step 3: Implement `reconcileA2AGateway` in `tenant_reconciler.go`**

Replace the entire file content:

```go
package reconcilers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/muto-io/muto/core/a2a"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	tenant := &v1alpha1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.ensureNamespace(ctx, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure namespace: %w", err)
	}

	switch tenant.Spec.MessageBus.Type {
	case a2a.BusTypeA2A:
		if tenant.Spec.MessageBus.Dedicated {
			if err := r.reconcileA2AGateway(ctx, tenant); err != nil {
				return ctrl.Result{}, fmt.Errorf("reconcile a2a gateway: %w", err)
			}
		}
	}

	tenant.Status.Ready = true
	if err := r.Status().Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TenantReconciler) reconcileA2AGateway(ctx context.Context, tenant *v1alpha1.Tenant) error {
	ns := tenant.Spec.Namespace
	ownerRef := *metav1.NewControllerRef(tenant, v1alpha1.GroupVersion.WithKind("Tenant"))

	if err := r.ensureA2ASecret(ctx, ns, ownerRef); err != nil {
		return err
	}
	if err := r.ensureA2ADeployment(ctx, ns, ownerRef); err != nil {
		return err
	}
	if err := r.ensureA2AService(ctx, ns, ownerRef); err != nil {
		return err
	}
	return nil
}

func (r *TenantReconciler) ensureA2ASecret(ctx context.Context, ns string, owner metav1.OwnerReference) error {
	sec := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: "muto-a2a-token", Namespace: ns}, sec)
	if err == nil {
		return nil // already exists — token is stable across reconcile loops
	}
	if !errors.IsNotFound(err) {
		return err
	}
	token, err := generateToken()
	if err != nil {
		return fmt.Errorf("generate a2a token: %w", err)
	}
	sec = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "muto-a2a-token",
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data: map[string][]byte{"token": []byte(token)},
	}
	return r.Create(ctx, sec)
}

func (r *TenantReconciler) ensureA2ADeployment(ctx context.Context, ns string, owner metav1.OwnerReference) error {
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: "a2a-gateway", Namespace: ns}, dep)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	image := os.Getenv("MUTO_A2A_GATEWAY_IMAGE")
	if image == "" {
		image = "ghcr.io/a2aprotocol/a2a-gateway:latest" // TBD: confirm real image
	}
	replicas := int32(1)
	dep = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "a2a-gateway",
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"muto.io/component": "a2a-gateway"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"muto.io/component": "a2a-gateway"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "a2a-gateway",
						Image: image,
						Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
					}},
				},
			},
		},
	}
	return r.Create(ctx, dep)
}

func (r *TenantReconciler) ensureA2AService(ctx context.Context, ns string, owner metav1.OwnerReference) error {
	svc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: "a2a-gateway", Namespace: ns}, svc)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	svc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "a2a-gateway",
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"muto.io/component": "a2a-gateway"},
			Ports: []corev1.ServicePort{{
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	return r.Create(ctx, svc)
}

func (r *TenantReconciler) ensureNamespace(ctx context.Context, tenant *v1alpha1.Tenant) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: tenant.Spec.Namespace}, ns)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tenant.Spec.Namespace,
			Labels: map[string]string{
				"muto.io/tenant": tenant.Name,
			},
		},
	}
	return r.Create(ctx, ns)
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Tenant{}).
		Complete(r)
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run all tenant reconciler tests — expect PASS**

```bash
go test ./platform/k8s/reconcilers/... -v -run TestTenantReconciler
```

Expected: all three tests PASS (`TestTenantReconcilerCreatesNamespace`, `TestTenantReconcilerA2AGatewayProvisioned`, `TestTenantReconcilerA2ANotDedicatedSkipsGateway`).

- [ ] **Step 5: Commit**

```bash
git add platform/k8s/reconcilers/tenant_reconciler.go platform/k8s/reconcilers/tenant_reconciler_test.go
git commit -m "feat(reconciler): provision A2A gateway Deployment+Service+Secret for type:a2a tenants"
```

---

## Task 5: AgentJobReconciler — inject A2A env vars

**Files:**
- Modify: `platform/k8s/reconcilers/agentjob_reconciler.go`
- Modify: `platform/k8s/reconcilers/agentjob_reconciler_test.go`

**Background:** `buildPod()` currently receives only `job` and `roleSpec`. We need it to also receive the parent Tenant (fetched once in `reconcilePending`) so it can decide whether to inject A2A env vars. We also need to read the `muto-a2a-token` Secret value.

- [ ] **Step 1: Write the failing test** (append to `agentjob_reconciler_test.go`)

```go
func TestAgentJobReconcilerInjectsA2AEnvVars(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "a2a-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:     "a2a-ns",
			IsolationTier: "dedicated",
			MessageBus:    v1alpha1.TenantBusSpec{Type: "a2a", Dedicated: true},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "muto-a2a-token", Namespace: "a2a-ns"},
		Data:       map[string][]byte{"token": []byte("test-token-value")},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-a2a", Namespace: "a2a-ns"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "a2a-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tenant, secret, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "job-a2a", Namespace: "a2a-ns"},
	})
	if err != nil {
		t.Fatal(err)
	}

	podList := &corev1.PodList{}
	_ = fakeClient.List(context.Background(), podList, client.InNamespace("a2a-ns"))
	if len(podList.Items) == 0 {
		t.Fatal("expected pod to be created")
	}
	pod := podList.Items[0]

	envMap := map[string]string{}
	for _, e := range pod.Spec.Containers[0].Env {
		envMap[e.Name] = e.Value
	}
	if envMap["MUTO_A2A_GATEWAY"] != "http://a2a-gateway.a2a-ns.svc.cluster.local:8080" {
		t.Errorf("unexpected MUTO_A2A_GATEWAY: %q", envMap["MUTO_A2A_GATEWAY"])
	}
	if envMap["MUTO_A2A_TOKEN"] != "test-token-value" {
		t.Errorf("unexpected MUTO_A2A_TOKEN: %q", envMap["MUTO_A2A_TOKEN"])
	}
}

func TestAgentJobReconcilerNoA2AEnvVarsForNATSTenant(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "nats-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:  "nats-ns",
			MessageBus: v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-nats", Namespace: "nats-ns"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "nats-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "job-nats", Namespace: "nats-ns"},
	})
	if err != nil {
		t.Fatal(err)
	}

	podList := &corev1.PodList{}
	_ = fakeClient.List(context.Background(), podList, client.InNamespace("nats-ns"))
	if len(podList.Items) == 0 {
		t.Fatal("expected pod to be created")
	}
	pod := podList.Items[0]

	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == "MUTO_A2A_GATEWAY" || e.Name == "MUTO_A2A_TOKEN" {
			t.Errorf("unexpected env var %q in non-A2A tenant pod", e.Name)
		}
	}
}
```

Add required import to test file:

```go
import (
    // existing imports ...
    "sigs.k8s.io/controller-runtime/pkg/client"
)
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./platform/k8s/reconcilers/... -v -run TestAgentJobReconcilerInjectsA2A
```

Expected: FAIL.

- [ ] **Step 3: Update `agentjob_reconciler.go`**

Replace the entire file:

```go
package reconcilers

import (
	"context"
	"fmt"
	"time"

	"github.com/muto-io/muto/core/a2a"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *AgentJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	job := &v1alpha1.AgentJob{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	switch job.Status.Phase {
	case "", "Pending":
		return r.reconcilePending(ctx, job)
	case "Running":
		return r.reconcileRunning(ctx, job)
	case "Succeeded", "Failed":
		return r.reconcileTerminal(ctx, job)
	case "Terminating":
		return r.reconcileTerminating(ctx, job)
	}
	return ctrl.Result{}, nil
}

func (r *AgentJobReconciler) reconcilePending(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	tenant := &v1alpha1.Tenant{}
	if err := r.Get(ctx, types.NamespacedName{Name: job.Spec.TenantRef}, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("get tenant %q: %w", job.Spec.TenantRef, err)
	}

	var a2aToken string
	if tenant.Spec.MessageBus.Type == a2a.BusTypeA2A {
		sec := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      "muto-a2a-token",
			Namespace: job.Namespace,
		}, sec); err != nil {
			return ctrl.Result{}, fmt.Errorf("get a2a token secret: %w", err)
		}
		a2aToken = string(sec.Data["token"])
	}

	for _, roleSpec := range job.Spec.Agents {
		pod := r.buildPod(job, roleSpec, tenant, a2aToken)
		if err := r.Create(ctx, pod); err != nil && !errors.IsAlreadyExists(err) {
			return ctrl.Result{}, fmt.Errorf("create pod: %w", err)
		}
	}
	now := metav1.Now()
	job.Status.Phase = "Running"
	job.Status.ActiveAgents = int32(len(job.Spec.Agents))
	job.Status.StartedAt = &now
	return ctrl.Result{}, r.Status().Update(ctx, job)
}

func (r *AgentJobReconciler) reconcileRunning(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(job.Namespace),
		client.MatchingLabels{"muto.io/job": job.Name}); err != nil {
		return ctrl.Result{}, err
	}

	allDone, anyFailed := true, false
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			allDone = false
		}
		if pod.Status.Phase == corev1.PodFailed {
			anyFailed = true
		}
	}

	if !allDone {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	now := metav1.Now()
	job.Status.CompletedAt = &now
	job.Status.ActiveAgents = 0
	if anyFailed {
		job.Status.Phase = "Failed"
	} else {
		job.Status.Phase = "Succeeded"
	}
	return ctrl.Result{RequeueAfter: time.Duration(job.Spec.TTLAfterCompletion) * time.Second},
		r.Status().Update(ctx, job)
}

func (r *AgentJobReconciler) reconcileTerminal(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	if job.Spec.TTLAfterCompletion <= 0 {
		return ctrl.Result{}, nil
	}
	if job.Status.CompletedAt == nil {
		return ctrl.Result{}, nil
	}
	elapsed := time.Since(job.Status.CompletedAt.Time)
	ttl := time.Duration(job.Spec.TTLAfterCompletion) * time.Second
	if elapsed < ttl {
		return ctrl.Result{RequeueAfter: ttl - elapsed}, nil
	}
	job.Status.Phase = "Terminating"
	return ctrl.Result{Requeue: true}, r.Status().Update(ctx, job)
}

func (r *AgentJobReconciler) reconcileTerminating(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(job.Namespace),
		client.MatchingLabels{"muto.io/job": job.Name}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list pods for terminating job: %w", err)
	}
	for i := range podList.Items {
		if err := r.Delete(ctx, &podList.Items[i]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete pod %s: %w", podList.Items[i].Name, err)
		}
	}
	if err := r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete agentjob: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *AgentJobReconciler) buildPod(
	job *v1alpha1.AgentJob,
	roleSpec v1alpha1.AgentRoleSpec,
	tenant *v1alpha1.Tenant,
	a2aToken string,
) *corev1.Pod {
	envVars := []corev1.EnvVar{
		{Name: "MUTO_TENANT", Value: job.Spec.TenantRef},
		{Name: "MUTO_ROLE", Value: roleSpec.Role},
		{Name: "MUTO_JOB_ID", Value: job.Name},
		{Name: "MUTO_BUS_TOPIC", Value: job.Spec.MessageBus.Topic},
	}
	if tenant.Spec.MessageBus.Type == a2a.BusTypeA2A {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "MUTO_A2A_GATEWAY",
				Value: "http://a2a-gateway." + job.Namespace + ".svc.cluster.local:8080",
			},
			corev1.EnvVar{
				Name:  "MUTO_A2A_TOKEN",
				Value: a2aToken,
			},
		)
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", job.Name, roleSpec.Role),
			Namespace: job.Namespace,
			Labels: map[string]string{
				"muto.io/tenant": job.Spec.TenantRef,
				"muto.io/job":    job.Name,
				"muto.io/role":   roleSpec.Role,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(job, v1alpha1.GroupVersion.WithKind("AgentJob")),
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  roleSpec.Role,
				Image: roleSpec.Image,
				Env:   envVars,
			}},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

func (r *AgentJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentJob{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
```

- [ ] **Step 4: Run all reconciler tests — expect PASS**

```bash
go test ./platform/k8s/reconcilers/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Run full unit suite**

```bash
go test ./... -short
```

Expected: PASS with no failures.

- [ ] **Step 6: Commit**

```bash
git add platform/k8s/reconcilers/agentjob_reconciler.go platform/k8s/reconcilers/agentjob_reconciler_test.go
git commit -m "feat(reconciler): inject MUTO_A2A_GATEWAY and MUTO_A2A_TOKEN env vars for A2A tenant jobs"
```

---

## Task 6: Integration test — A2A gateway lifecycle

**Files:**
- Create: `test/integration/a2a_gateway_test.go`

**Background:** Integration tests run with build tag `integration`, require a live k8s cluster (kind), and use Ginkgo v2 + Gomega. Look at `test/integration/suite_test.go` to understand the suite setup, and `test/integration/agentjob_lifecycle_test.go` for the test pattern. The suite already starts the controller-manager in-process.

- [ ] **Step 1: Read the existing integration test suite setup**

```bash
cat test/integration/suite_test.go
cat test/integration/agentjob_lifecycle_test.go
```

Understand: how `k8sClient` is accessed, how Tenants and AgentJobs are created, what `Eventually` polling intervals are used.

- [ ] **Step 2: Write the integration test**

```go
//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("A2A Gateway Lifecycle", func() {
	ctx := context.Background()
	var testCounter int

	var (
		tenantName string
		tenantNS   string
		tenant     *v1alpha1.Tenant
	)

	BeforeEach(func() {
		testCounter++
		tenantName = fmt.Sprintf("a2a-tenant-%d", testCounter)
		tenantNS = fmt.Sprintf("a2a-ns-%d", testCounter)
	})

	AfterEach(func() {
		if tenant != nil {
			_ = k8sClient.Delete(ctx, tenant)
		}
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tenantNS}}
		_ = k8sClient.Delete(ctx, ns)
		// Wait for namespace to terminate to avoid collision on re-run
		Eventually(func() bool {
			n := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: tenantNS}, n)
			return err != nil // gone when Get returns error
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(BeTrue())
	})

	It("provisions gateway Deployment, Service, and Secret for type:a2a dedicated tenant", func() {
		tenant = &v1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: tenantName},
			Spec: v1alpha1.TenantSpec{
				Namespace:     tenantNS,
				IsolationTier: "dedicated",
				MessageBus:    v1alpha1.TenantBusSpec{Type: "a2a", Dedicated: true},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		By("waiting for TenantStatus.Ready")
		Eventually(func(g Gomega) {
			t := &v1alpha1.Tenant{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tenantName}, t)).To(Succeed())
			g.Expect(t.Status.Ready).To(BeTrue())
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		By("checking Deployment exists")
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name: "a2a-gateway", Namespace: tenantNS,
		}, dep)).To(Succeed())

		By("checking Service exists on port 8080")
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name: "a2a-gateway", Namespace: tenantNS,
		}, svc)).To(Succeed())
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8080)))

		By("checking Secret exists with non-empty token")
		sec := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name: "muto-a2a-token", Namespace: tenantNS,
		}, sec)).To(Succeed())
		Expect(sec.Data["token"]).NotTo(BeEmpty())
	})

	It("injects MUTO_A2A_GATEWAY and MUTO_A2A_TOKEN env vars into AgentJob pods", func() {
		tenant = &v1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: tenantName},
			Spec: v1alpha1.TenantSpec{
				Namespace:     tenantNS,
				IsolationTier: "dedicated",
				MessageBus:    v1alpha1.TenantBusSpec{Type: "a2a", Dedicated: true},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		By("waiting for tenant ready")
		Eventually(func(g Gomega) {
			t := &v1alpha1.Tenant{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tenantName}, t)).To(Succeed())
			g.Expect(t.Status.Ready).To(BeTrue())
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		job := &v1alpha1.AgentJob{
			ObjectMeta: metav1.ObjectMeta{Name: "a2a-job", Namespace: tenantNS},
			Spec: v1alpha1.AgentJobSpec{
				TenantRef: tenantName,
				Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
			},
		}
		Expect(k8sClient.Create(ctx, job)).To(Succeed())

		By("waiting for pod to be created")
		Eventually(func(g Gomega) {
			podList := &corev1.PodList{}
			g.Expect(k8sClient.List(ctx, podList,
				client.InNamespace(tenantNS),
				client.MatchingLabels{"muto.io/job": "a2a-job"})).To(Succeed())
			g.Expect(podList.Items).NotTo(BeEmpty())
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		podList := &corev1.PodList{}
		Expect(k8sClient.List(ctx, podList,
			client.InNamespace(tenantNS),
			client.MatchingLabels{"muto.io/job": "a2a-job"})).To(Succeed())

		envMap := map[string]string{}
		for _, e := range podList.Items[0].Spec.Containers[0].Env {
			envMap[e.Name] = e.Value
		}
		Expect(envMap["MUTO_A2A_GATEWAY"]).To(Equal(
			"http://a2a-gateway." + tenantNS + ".svc.cluster.local:8080"))
		Expect(envMap["MUTO_A2A_TOKEN"]).NotTo(BeEmpty())
	})
})
```

- [ ] **Step 3: Verify integration tests compile**

```bash
go build -tags integration ./test/integration/...
```

Expected: no output (success). Do not run the full integration suite unless you have a kind cluster available (`make kind-up`).

- [ ] **Step 4: Commit**

```bash
git add test/integration/a2a_gateway_test.go
git commit -m "test(integration): add A2A gateway lifecycle integration tests"
```

---

## Task 7: Final verification

- [ ] **Step 1: Run full unit suite**

```bash
go test ./... -short
```

Expected: PASS.

- [ ] **Step 2: Verify build**

```bash
make build
```

Expected: `bin/muto-operator` and `bin/muto-mcp` built without errors.

- [ ] **Step 3: Lint**

```bash
make lint
```

Expected: no new lint errors introduced.

- [ ] **Step 4: Final commit if any loose files**

```bash
git status
```

If clean, nothing to do. If any files are unstaged, stage and commit with an appropriate message.