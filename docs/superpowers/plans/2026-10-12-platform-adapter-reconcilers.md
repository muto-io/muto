# Platform-Adapter-Backed AgentJob Reconciliation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route `AgentJobReconciler`'s agent spawn/status/terminate logic through `scheduler.PlatformAdapter` instead of hardcoded `corev1.Pod` CRUD, so `AgentJob` CRs work correctly on both `MUTO_PLATFORM=k8s` and `MUTO_PLATFORM=cf` (muto-io/muto#113).

**Architecture:** Extend `agent.Spec` with optional `Name`/`Namespace`/`Labels` hints and `MessageBus.Type`, and extend `scheduler.PlatformAdapter` with a synchronous `AgentStatus` poll method. `K8sAdapter` absorbs the pod-building logic (naming, A2A env injection, composite `"<namespace>/<name>"` IDs) that used to live in the reconciler; `CFAdapter` gets the matching `AgentStatus`. `AgentJobReconciler` persists the opaque IDs `SpawnAgent` returns into a new `AgentJob.Status.AgentIDs` field and uses them with `AgentStatus`/`TerminateAgent` instead of Kubernetes label selectors, keeping the reconciler stateless/restart-safe (same `RequeueAfter` polling cadence as today).

**Tech Stack:** Go, controller-runtime, Kubernetes CRDs (kubebuilder/controller-gen), standard library testing.

**Spec:** `docs/superpowers/specs/2026-10-12-platform-adapter-reconcilers-design.md`

## Global Constraints

- All new `agent.Spec` fields (`Name`, `Namespace`, `Labels`, `MessageBus.Type`) and the new `PlatformAdapter.AgentStatus` method are additive — every existing caller that doesn't set/use them must observe byte-for-byte identical behavior to today.
- `K8sAdapter.SpawnAgent` pod labels must include `muto.io/tenant`, `muto.io/job`, `muto.io/role` when the reconciler calls it (the existing `test/integration/k8s/*` suite asserts on exactly these three labels via `client.MatchingLabels`).
- No `OwnerReference` from spawned agents back to the `AgentJob` — cleanup is entirely via `AgentJobStatus.AgentIDs` + `TerminateAgent`, matching CF's existing (and now unified) behavior. This was confirmed with the user during design review.
- Every step that touches a file must leave `go build ./...` and `make test-unit` green before moving to the next task.

---

### Task 1: Extend `agent.Spec` and `PlatformAdapter` with `AgentStatus`

**Files:**
- Modify: `core/agent/types.go`
- Modify: `core/scheduler/scheduler.go`
- Modify: `core/tracing/scheduler.go`
- Modify: `core/scheduler/scheduler_test.go`
- Modify: `core/tracing/scheduler_test.go`

**Interfaces:**
- Produces: `agent.Spec{Name, Namespace, Labels}` fields; `agent.MessageBusConfig{Type}` field; `scheduler.PlatformAdapter.AgentStatus(ctx, agentID string) (*agent.Event, error)` — `nil, nil` means still running, a non-nil terminal `*agent.Event` (`EventCompleted`/`EventFailed`) means done.
- Consumes: nothing new (pure type/interface additions).

- [ ] **Step 1: Add the new `agent.Spec`/`MessageBusConfig` fields**

Edit `core/agent/types.go`, replacing:

```go
type MessageBusConfig struct {
	Topic string
}

type Spec struct {
	TenantRef          string
	Trigger            Trigger
	Agents             []AgentRole
	MessageBus         MessageBusConfig
	TTLAfterCompletion int32
}
```

with:

```go
type MessageBusConfig struct {
	Topic string
	Type  string // mirrors TenantBusSpec.Type, e.g. "a2a"; "" means no special handling
}

type Spec struct {
	TenantRef          string
	Trigger            Trigger
	Agents             []AgentRole
	MessageBus         MessageBusConfig
	TTLAfterCompletion int32
	Name               string            // unique instance hint, e.g. "<job>-<role>-<replica>"
	Namespace          string            // k8s target namespace hint; ignored by non-k8s adapters
	Labels             map[string]string // k8s pod labels hint; ignored by non-k8s adapters
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./core/...`
Expected: success (no callers set the new fields yet, so nothing else changes).

- [ ] **Step 3: Write the failing test for `tracedPlatformAdapter.AgentStatus`**

Add to `core/tracing/scheduler_test.go`, right after `TestWrapPlatformAdapterSpawnAgent` (keep everything else in the file unchanged):

```go
func TestWrapPlatformAdapterAgentStatus(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapPlatformAdapter(&stubAdapter{})
	ev, err := wrapped.AgentStatus(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("AgentStatus: unexpected error %v", err)
	}
	if ev == nil || ev.Type != agent.EventCompleted {
		t.Fatalf("AgentStatus: got %v, want a completed event", ev)
	}

	spans := sr.Ended()
	found := false
	for _, s := range spans {
		if s.Name() == "PlatformAdapter.AgentStatus" {
			found = true
		}
	}
	if !found {
		t.Error("expected a span named PlatformAdapter.AgentStatus")
	}
}
```

- [ ] **Step 4: Run it to confirm it fails to compile**

Run: `go test ./core/tracing/... -run TestWrapPlatformAdapterAgentStatus -v`
Expected: FAIL — `stubAdapter` does not implement `scheduler.PlatformAdapter` (missing `AgentStatus`), and `tracing.WrapPlatformAdapter`'s return value has no `AgentStatus` method either.

- [ ] **Step 5: Add `AgentStatus` to `stubAdapter`**

Add to `core/tracing/scheduler_test.go`, right after `stubAdapter.WatchAgent`:

```go
func (s *stubAdapter) AgentStatus(ctx context.Context, agentID string) (*agent.Event, error) {
	return &agent.Event{AgentID: agentID, Type: agent.EventCompleted}, nil
}
```

- [ ] **Step 6: Add `AgentStatus` to the `PlatformAdapter` interface**

Edit `core/scheduler/scheduler.go`, replacing:

```go
type PlatformAdapter interface {
	SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error)
	TerminateAgent(ctx context.Context, agentID string) error
	WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error)
}
```

with:

```go
type PlatformAdapter interface {
	SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error)
	TerminateAgent(ctx context.Context, agentID string) error
	WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error)
	// AgentStatus returns nil, nil while the agent is still running, or a
	// terminal event (EventCompleted/EventFailed) once it has finished.
	AgentStatus(ctx context.Context, agentID string) (*agent.Event, error)
}
```

- [ ] **Step 7: Implement `tracedPlatformAdapter.AgentStatus`**

Add to `core/tracing/scheduler.go`, right after `tracedPlatformAdapter.WatchAgent`:

```go
func (t *tracedPlatformAdapter) AgentStatus(ctx context.Context, agentID string) (*agent.Event, error) {
	return Wrap(ctx, "PlatformAdapter.AgentStatus", func(ctx context.Context) (*agent.Event, error) {
		return t.inner.AgentStatus(ctx, agentID)
	})
}
```

- [ ] **Step 8: Run the new test again**

Run: `go test ./core/tracing/... -run TestWrapPlatformAdapterAgentStatus -v`
Expected: still FAILS to compile — `core/scheduler/scheduler_test.go`'s four fakes (`mockAdapter`, `completingAdapter`, `failingAdapter`, `blockingAdapter`) no longer satisfy `scheduler.PlatformAdapter`.

- [ ] **Step 9: Add `AgentStatus` stubs to the four fakes in `core/scheduler/scheduler_test.go`**

Add right after each fake's existing `WatchAgent` method:

```go
func (m *mockAdapter) AgentStatus(_ context.Context, _ string) (*agent.Event, error) {
	return nil, nil
}
```

```go
func (m *completingAdapter) AgentStatus(_ context.Context, agentID string) (*agent.Event, error) {
	return &agent.Event{AgentID: agentID, Type: agent.EventCompleted}, nil
}
```

```go
func (m *failingAdapter) AgentStatus(_ context.Context, agentID string) (*agent.Event, error) {
	return &agent.Event{AgentID: agentID, Type: agent.EventFailed}, nil
}
```

```go
func (m *blockingAdapter) AgentStatus(_ context.Context, _ string) (*agent.Event, error) {
	return nil, nil
}
```

- [ ] **Step 10: Run the full `core/...` test suite**

Run: `go test ./core/... -v`
Expected: PASS, including the new `TestWrapPlatformAdapterAgentStatus` and every pre-existing test in `core/scheduler` and `core/tracing`.

- [ ] **Step 11: Commit**

```bash
git add core/agent/types.go core/scheduler/scheduler.go core/scheduler/scheduler_test.go core/tracing/scheduler.go core/tracing/scheduler_test.go
git commit -m "feat: add AgentStatus to PlatformAdapter and Name/Namespace/Labels to agent.Spec"
```

---

### Task 2: `K8sAdapter` — honor Name/Namespace/Labels, composite IDs, `AgentStatus`, A2A injection

**Files:**
- Modify: `platform/k8s/adapter.go`
- Modify: `platform/k8s/adapter_test.go`

**Interfaces:**
- Consumes: `agent.Spec{Name, Namespace, Labels, MessageBus.Type}` (Task 1).
- Produces: `K8sAdapter.SpawnAgent` returns a composite `"<namespace>/<name>"` ID; `K8sAdapter.AgentStatus(ctx, agentID) (*agent.Event, error)`; `K8sAdapter.TerminateAgent` returns `nil` for an already-missing pod.

- [ ] **Step 1: Write the failing tests for Name/Namespace hints and composite IDs**

Add to `platform/k8s/adapter_test.go` (keep `TestSpawnAgentCreatesPod` as-is above these):

```go
func TestSpawnAgentUsesNameAndNamespaceHints(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "default-ns")
	spec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "worker:latest"}},
		Name:      "job-1-worker-0",
		Namespace: "acme-agents",
	}
	id, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if id != "acme-agents/job-1-worker-0" {
		t.Errorf("got id %q, want %q", id, "acme-agents/job-1-worker-0")
	}

	pod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "job-1-worker-0", Namespace: "acme-agents"}, pod); err != nil {
		t.Fatalf("pod not created in hinted namespace: %v", err)
	}
}

func TestSpawnAgentFallsBackToDefaultNamingAndNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	spec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "worker:latest"}},
	}
	id, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if id != "acme-agents/muto-acme-worker" {
		t.Errorf("got id %q, want %q", id, "acme-agents/muto-acme-worker")
	}
}
```

These new tests use `client.ObjectKey`, so add `"sigs.k8s.io/controller-runtime/pkg/client"` to the file's import block, next to the existing `"sigs.k8s.io/controller-runtime/pkg/client/fake"` import.

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./platform/k8s/... -run TestSpawnAgentUsesNameAndNamespaceHints -v`
Expected: FAIL — current `SpawnAgent` ignores `spec.Name`/`spec.Namespace` and returns a bare pod name, not a composite ID.

- [ ] **Step 3: Rewrite `platform/k8s/adapter.go`**

Replace the entire file with:

```go
// SPDX-License-Identifier: Apache-2.0
package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muto-io/muto/core/a2a"
	"github.com/muto-io/muto/core/agent"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type K8sAdapter struct {
	client    client.Client
	namespace string
}

func NewK8sAdapter(c client.Client, namespace string) *K8sAdapter {
	return &K8sAdapter{client: c, namespace: namespace}
}

func (a *K8sAdapter) SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error) {
	if len(spec.Agents) == 0 {
		return "", fmt.Errorf("no agents in spec")
	}
	role := spec.Agents[0]

	namespace := spec.Namespace
	if namespace == "" {
		namespace = a.namespace
	}
	podName := spec.Name
	if podName == "" {
		podName = fmt.Sprintf("muto-%s-%s", spec.TenantRef, role.Role)
	}
	labels := spec.Labels
	if labels == nil {
		labels = map[string]string{
			"muto.io/tenant": spec.TenantRef,
			"muto.io/role":   role.Role,
		}
	}

	envVars := []corev1.EnvVar{
		{Name: "MUTO_TENANT", Value: spec.TenantRef},
		{Name: "MUTO_ROLE", Value: role.Role},
	}
	if spec.MessageBus.Type == a2a.BusTypeA2A {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "MUTO_A2A_GATEWAY",
				Value: fmt.Sprintf("http://a2a-gateway.%s.svc.cluster.local:8080", namespace),
			},
			corev1.EnvVar{
				Name: "MUTO_A2A_TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "muto-a2a-token"},
						Key:                  "token",
					},
				},
			},
		)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  role.Role,
					Image: role.Image,
					Env:   envVars,
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
	if err := a.client.Create(ctx, pod); err != nil {
		return "", fmt.Errorf("create pod: %w", err)
	}
	return namespace + "/" + podName, nil
}

func (a *K8sAdapter) TerminateAgent(ctx context.Context, agentID string) error {
	namespace, name := a.splitAgentID(agentID)
	pod := &corev1.Pod{}
	pod.Name = name
	pod.Namespace = namespace
	if err := a.client.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (a *K8sAdapter) AgentStatus(ctx context.Context, agentID string) (*agent.Event, error) {
	return a.checkPodStatus(ctx, agentID)
}

func (a *K8sAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 8)
	go func() {
		defer close(ch)
		for {
			ev, err := a.checkPodStatus(ctx, agentID)
			if err != nil {
				if ctx.Err() == nil {
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Second):
					}
					continue
				}
				return
			}
			if ev != nil {
				select {
				case ch <- *ev:
				case <-ctx.Done():
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	return ch, nil
}

// checkPodStatus fetches the pod identified by agentID and returns a
// terminal event once it has succeeded or failed (including having been
// deleted externally, which is treated as a failure), or nil while it is
// still active.
func (a *K8sAdapter) checkPodStatus(ctx context.Context, agentID string) (*agent.Event, error) {
	namespace, name := a.splitAgentID(agentID)
	pod := &corev1.Pod{}
	err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			return &agent.Event{AgentID: agentID, Type: agent.EventFailed}, nil
		}
		return nil, err
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		return &agent.Event{AgentID: agentID, Type: agent.EventCompleted}, nil
	case corev1.PodFailed:
		return &agent.Event{AgentID: agentID, Type: agent.EventFailed}, nil
	}
	return nil, nil
}

// splitAgentID parses the "<namespace>/<name>" composite ID returned by
// SpawnAgent. An ID with no "/" falls back to the adapter's own namespace,
// for backward compatibility with any caller holding a pre-existing bare
// pod-name ID.
func (a *K8sAdapter) splitAgentID(agentID string) (namespace, name string) {
	if ns, n, ok := strings.Cut(agentID, "/"); ok {
		return ns, n
	}
	return a.namespace, agentID
}
```

- [ ] **Step 4: Run the Name/Namespace tests again**

Run: `go test ./platform/k8s/... -run TestSpawnAgent -v`
Expected: PASS for `TestSpawnAgentCreatesPod`, `TestSpawnAgentUsesNameAndNamespaceHints`, `TestSpawnAgentFallsBackToDefaultNamingAndNamespace`.

- [ ] **Step 5: Write the failing tests for Labels hints**

Add to `platform/k8s/adapter_test.go`:

```go
func TestSpawnAgentUsesLabelsHint(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	spec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "worker:latest"}},
		Name:      "job-1-worker-0",
		Namespace: "acme-agents",
		Labels: map[string]string{
			"muto.io/tenant": "acme",
			"muto.io/job":    "job-1",
			"muto.io/role":   "worker",
		},
	}
	if _, err := adapter.SpawnAgent(context.Background(), spec); err != nil {
		t.Fatal(err)
	}

	pod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "job-1-worker-0", Namespace: "acme-agents"}, pod); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"muto.io/tenant": "acme", "muto.io/job": "job-1", "muto.io/role": "worker"}
	if len(pod.Labels) != len(want) {
		t.Fatalf("got labels %v, want exactly %v", pod.Labels, want)
	}
	for k, v := range want {
		if pod.Labels[k] != v {
			t.Errorf("label %q = %q, want %q", k, pod.Labels[k], v)
		}
	}
}

func TestSpawnAgentDefaultLabelsWhenHintEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	spec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "worker:latest"}},
	}
	if _, err := adapter.SpawnAgent(context.Background(), spec); err != nil {
		t.Fatal(err)
	}

	pod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "muto-acme-worker", Namespace: "acme-agents"}, pod); err != nil {
		t.Fatal(err)
	}
	if pod.Labels["muto.io/tenant"] != "acme" || pod.Labels["muto.io/role"] != "worker" {
		t.Errorf("got default labels %v, want tenant=acme role=worker", pod.Labels)
	}
}
```

- [ ] **Step 6: Run them to confirm they pass**

Run: `go test ./platform/k8s/... -run TestSpawnAgent -v`
Expected: PASS (the adapter already implements the `Labels` fallback from Step 3).

- [ ] **Step 7: Write the failing A2A injection tests**

Add to `platform/k8s/adapter_test.go`:

```go
func TestSpawnAgentInjectsA2AEnvVars(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "default-ns")
	spec := &agent.Spec{
		TenantRef:  "acme",
		Agents:     []agent.AgentRole{{Role: "worker", Image: "worker:latest"}},
		Name:       "job-a2a-worker-0",
		Namespace:  "a2a-ns",
		MessageBus: agent.MessageBusConfig{Type: "a2a"},
	}
	if _, err := adapter.SpawnAgent(context.Background(), spec); err != nil {
		t.Fatal(err)
	}

	pod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "job-a2a-worker-0", Namespace: "a2a-ns"}, pod); err != nil {
		t.Fatal(err)
	}
	var gateway string
	var tokenRef *corev1.SecretKeySelector
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == "MUTO_A2A_GATEWAY" {
			gateway = e.Value
		}
		if e.Name == "MUTO_A2A_TOKEN" && e.ValueFrom != nil {
			tokenRef = e.ValueFrom.SecretKeyRef
		}
	}
	if gateway != "http://a2a-gateway.a2a-ns.svc.cluster.local:8080" {
		t.Errorf("unexpected MUTO_A2A_GATEWAY: %q", gateway)
	}
	if tokenRef == nil || tokenRef.Name != "muto-a2a-token" || tokenRef.Key != "token" {
		t.Errorf("unexpected MUTO_A2A_TOKEN secretKeyRef: %v", tokenRef)
	}
}

func TestSpawnAgentNoA2AEnvVarsForOtherBusType(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "default-ns")
	spec := &agent.Spec{
		TenantRef:  "acme",
		Agents:     []agent.AgentRole{{Role: "worker", Image: "worker:latest"}},
		Name:       "job-nats-worker-0",
		Namespace:  "nats-ns",
		MessageBus: agent.MessageBusConfig{Type: "nats"},
	}
	if _, err := adapter.SpawnAgent(context.Background(), spec); err != nil {
		t.Fatal(err)
	}

	pod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "job-nats-worker-0", Namespace: "nats-ns"}, pod); err != nil {
		t.Fatal(err)
	}
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == "MUTO_A2A_GATEWAY" || e.Name == "MUTO_A2A_TOKEN" {
			t.Errorf("unexpected env var %q for non-A2A bus type", e.Name)
		}
	}
}
```

- [ ] **Step 8: Run them to confirm they pass**

Run: `go test ./platform/k8s/... -run TestSpawnAgentInjectsA2AEnvVars -v && go test ./platform/k8s/... -run TestSpawnAgentNoA2AEnvVarsForOtherBusType -v`
Expected: PASS.

- [ ] **Step 9: Write the failing `AgentStatus`/`TerminateAgent` tests**

Add to `platform/k8s/adapter_test.go`:

```go
func TestAgentStatusTransitions(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	spec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "worker:latest"}},
		Name:      "job-1-worker-0",
		Namespace: "acme-agents",
	}
	id, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}

	ev, err := adapter.AgentStatus(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if ev != nil {
		t.Fatalf("expected nil event for a freshly-created pod, got %v", ev)
	}

	pod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "job-1-worker-0", Namespace: "acme-agents"}, pod); err != nil {
		t.Fatal(err)
	}
	pod.Status.Phase = corev1.PodSucceeded
	if err := fakeClient.Status().Update(context.Background(), pod); err != nil {
		t.Fatal(err)
	}

	ev, err = adapter.AgentStatus(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil || ev.Type != agent.EventCompleted {
		t.Fatalf("expected EventCompleted after PodSucceeded, got %v", ev)
	}
}

func TestAgentStatusPodNotFoundReturnsFailed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	ev, err := adapter.AgentStatus(context.Background(), "acme-agents/never-existed")
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil || ev.Type != agent.EventFailed {
		t.Fatalf("expected EventFailed for a missing pod, got %v", ev)
	}
}

func TestTerminateAgentNotFoundIsNil(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	if err := adapter.TerminateAgent(context.Background(), "acme-agents/never-existed"); err != nil {
		t.Errorf("expected nil error terminating a missing pod, got %v", err)
	}
}
```

- [ ] **Step 10: Run them to confirm they pass**

Run: `go test ./platform/k8s/... -v`
Expected: PASS for every test in the package (old and new).

- [ ] **Step 11: Commit**

```bash
git add platform/k8s/adapter.go platform/k8s/adapter_test.go
git commit -m "feat: route K8sAdapter.SpawnAgent through Name/Namespace/Labels hints, add AgentStatus"
```

---

### Task 3: `CFAdapter` — add `AgentStatus`

**Files:**
- Modify: `platform/cf/adapter.go`
- Modify: `platform/cf/adapter_test.go`

**Interfaces:**
- Consumes: nothing new from earlier tasks (CF ignores `Name`/`Namespace`/`Labels`).
- Produces: `CFAdapter.AgentStatus(ctx, agentID) (*agent.Event, error)`.

- [ ] **Step 1: Write the failing tests**

Add to `platform/cf/adapter_test.go`:

```go
func TestAgentStatusStillRunning(t *testing.T) {
	mock, adapter := setupMock()
	task := &resource.Task{State: "RUNNING"}
	task.GUID = "task-running-status"
	mock.tasks["task-running-status"] = task

	ev, err := adapter.AgentStatus(context.Background(), "task-running-status")
	if err != nil {
		t.Fatal(err)
	}
	if ev != nil {
		t.Fatalf("expected nil event for a running task, got %v", ev)
	}
}

func TestAgentStatusSucceeded(t *testing.T) {
	mock, adapter := setupMock()
	task := &resource.Task{State: "SUCCEEDED"}
	task.GUID = "task-status-done"
	mock.tasks["task-status-done"] = task

	ev, err := adapter.AgentStatus(context.Background(), "task-status-done")
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil || ev.Type != agent.EventCompleted {
		t.Fatalf("expected EventCompleted, got %v", ev)
	}
}

func TestAgentStatusFailed(t *testing.T) {
	mock, adapter := setupMock()
	task := &resource.Task{State: "FAILED"}
	task.GUID = "task-status-fail"
	mock.tasks["task-status-fail"] = task

	ev, err := adapter.AgentStatus(context.Background(), "task-status-fail")
	if err != nil {
		t.Fatal(err)
	}
	if ev == nil || ev.Type != agent.EventFailed {
		t.Fatalf("expected EventFailed, got %v", ev)
	}
}
```

- [ ] **Step 2: Run them to confirm they fail**

Run: `go test ./platform/cf/... -run TestAgentStatus -v`
Expected: FAIL — `*cf.CFAdapter` has no method `AgentStatus`.

- [ ] **Step 3: Extract `checkTaskStatus` and implement `AgentStatus`/`WatchAgent`**

In `platform/cf/adapter.go`, replace the existing `WatchAgent` method:

```go
// WatchAgent polls the CF task state and emits a single event when the task
// reaches a terminal state, then closes the channel. Cancelling ctx causes the
// channel to close without an event.
func (a *CFAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 4)
	go func() {
		defer close(ch)
		for {
			task, err := a.client.GetTask(ctx, agentID)
			if err != nil {
				// Don't report context cancellation as a failure
				if errors.Is(err, context.Canceled) {
					return
				}
				ch <- agent.Event{AgentID: agentID, Type: agent.EventFailed, Message: err.Error()}
				return
			}
			switch task.State {
			case "SUCCEEDED":
				ch <- agent.Event{AgentID: agentID, Type: agent.EventCompleted}
				return
			case "FAILED", "CANCELING":
				ch <- agent.Event{AgentID: agentID, Type: agent.EventFailed}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	return ch, nil
}
```

with:

```go
// AgentStatus returns nil, nil while the CF task is still running, or a
// terminal event once it reaches SUCCEEDED/FAILED/CANCELING.
func (a *CFAdapter) AgentStatus(ctx context.Context, agentID string) (*agent.Event, error) {
	return a.checkTaskStatus(ctx, agentID)
}

// checkTaskStatus fetches the CF task and returns a terminal event once it
// has finished, or nil while it is still running.
func (a *CFAdapter) checkTaskStatus(ctx context.Context, agentID string) (*agent.Event, error) {
	task, err := a.client.GetTask(ctx, agentID)
	if err != nil {
		return nil, err
	}
	switch task.State {
	case "SUCCEEDED":
		return &agent.Event{AgentID: agentID, Type: agent.EventCompleted}, nil
	case "FAILED", "CANCELING":
		return &agent.Event{AgentID: agentID, Type: agent.EventFailed}, nil
	}
	return nil, nil
}

// WatchAgent polls the CF task state and emits a single event when the task
// reaches a terminal state, then closes the channel. Cancelling ctx causes the
// channel to close without an event.
func (a *CFAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 4)
	go func() {
		defer close(ch)
		for {
			ev, err := a.checkTaskStatus(ctx, agentID)
			if err != nil {
				// Don't report context cancellation as a failure
				if errors.Is(err, context.Canceled) {
					return
				}
				ch <- agent.Event{AgentID: agentID, Type: agent.EventFailed, Message: err.Error()}
				return
			}
			if ev != nil {
				ch <- *ev
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	return ch, nil
}
```

- [ ] **Step 4: Run all CF adapter tests**

Run: `go test ./platform/cf/... -v`
Expected: PASS for every test in the package, including the three new ones and the pre-existing `TestWatchAgent*` tests (unchanged behavior).

- [ ] **Step 5: Commit**

```bash
git add platform/cf/adapter.go platform/cf/adapter_test.go
git commit -m "feat: add CFAdapter.AgentStatus, extracted from WatchAgent's poll loop"
```

---

### Task 4: Add `AgentJobStatus.AgentIDs`

**Files:**
- Modify: `platform/k8s/types/v1alpha1/agentjob_types.go`
- Modify (generated): `platform/k8s/types/v1alpha1/zz_generated.deepcopy.go`
- Modify (generated): `deploy/crds/muto.io_agentjobs.yaml`
- Modify (generated): `deploy/helm/muto/templates/crds/agentjobs.yaml`

**Interfaces:**
- Produces: `v1alpha1.AgentJobStatus.AgentIDs []string`.
- Consumes: nothing from earlier tasks.

- [ ] **Step 1: Add the field**

Edit `platform/k8s/types/v1alpha1/agentjob_types.go`, replacing:

```go
	// CompletedAt is the time the job reached a terminal phase.
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}
```

with:

```go
	// CompletedAt is the time the job reached a terminal phase.
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	// AgentIDs holds the opaque platform-adapter-returned identifier for
	// every agent instance spawned for this job (one per replica). The
	// reconciler uses these to poll status and terminate agents without
	// depending on any platform-specific lookup mechanism.
	AgentIDs []string `json:"agentIDs,omitempty"`
}
```

- [ ] **Step 2: Regenerate the deepcopy function and CRD manifests**

Run: `make generate`

- [ ] **Step 3: If `make generate` succeeded, verify the diff**

Run: `git diff --stat`
Expected: changes in `platform/k8s/types/v1alpha1/zz_generated.deepcopy.go`, `deploy/crds/muto.io_agentjobs.yaml`, and `deploy/helm/muto/templates/crds/agentjobs.yaml`, each adding an `agentIDs`/`AgentIDs` entry. Skip Step 4.

- [ ] **Step 4: If `make generate` failed (e.g. no network access to fetch controller-gen), apply the three edits by hand**

In `platform/k8s/types/v1alpha1/zz_generated.deepcopy.go`, replace the `AgentJobStatus.DeepCopyInto` method:

```go
// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentJobStatus) DeepCopyInto(out *AgentJobStatus) {
	*out = *in
	if in.StartedAt != nil {
		in, out := &in.StartedAt, &out.StartedAt
		*out = (*in).DeepCopy()
	}
	if in.CompletedAt != nil {
		in, out := &in.CompletedAt, &out.CompletedAt
		*out = (*in).DeepCopy()
	}
}
```

with:

```go
// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentJobStatus) DeepCopyInto(out *AgentJobStatus) {
	*out = *in
	if in.StartedAt != nil {
		in, out := &in.StartedAt, &out.StartedAt
		*out = (*in).DeepCopy()
	}
	if in.CompletedAt != nil {
		in, out := &in.CompletedAt, &out.CompletedAt
		*out = (*in).DeepCopy()
	}
	if in.AgentIDs != nil {
		in, out := &in.AgentIDs, &out.AgentIDs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}
```

In both `deploy/crds/muto.io_agentjobs.yaml` and `deploy/helm/muto/templates/crds/agentjobs.yaml`, find the `status.properties` block (search for `activeAgents:`) and insert, right after the `activeAgents` property and before `completedAt:`, at the same indentation as `activeAgents:`:

```yaml
              agentIDs:
                description: |-
                  AgentIDs holds the opaque platform-adapter-returned identifier for
                  every agent instance spawned for this job (one per replica). The
                  reconciler uses these to poll status and terminate agents without
                  depending on any platform-specific lookup mechanism.
                items:
                  type: string
                type: array
```

- [ ] **Step 5: Verify it compiles**

Run: `go build ./platform/k8s/...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add platform/k8s/types/v1alpha1/agentjob_types.go platform/k8s/types/v1alpha1/zz_generated.deepcopy.go deploy/crds/muto.io_agentjobs.yaml deploy/helm/muto/templates/crds/agentjobs.yaml
git commit -m "feat: add AgentJobStatus.AgentIDs to track platform-adapter-spawned agents"
```

---

### Task 5: Rewire `AgentJobReconciler` through `PlatformAdapter`

**Files:**
- Modify: `platform/k8s/reconcilers/agentjob_reconciler.go`
- Modify: `platform/k8s/reconcilers/agentjob_reconciler_test.go`

**Interfaces:**
- Consumes: `scheduler.PlatformAdapter` (Task 1/2/3), `v1alpha1.AgentJobStatus.AgentIDs` (Task 4).
- Produces: `AgentJobReconciler{Client, Scheme, Adapter}` — a new required `Adapter scheduler.PlatformAdapter` field.

- [ ] **Step 1: Write the failing platform-agnostic reconciler test**

Replace the existing import block at the top of `platform/k8s/reconcilers/agentjob_reconciler_test.go` with this one (adds `"sync"`, `"github.com/muto-io/muto/core/agent"`, and `k8sadapter "github.com/muto-io/muto/platform/k8s"` to the existing imports), then append the new type and test below the imports, ahead of the existing `TestAgentJobReconcilerPendingToRunning`:

```go
import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/platform/k8s/metrics"
	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	k8sadapter "github.com/muto-io/muto/platform/k8s"
)

// fakeAdapter is an in-memory scheduler.PlatformAdapter with no platform
// dependency at all (no Pods, no CF). It exists to prove that
// AgentJobReconciler no longer requires Kubernetes Pods to function.
type fakeAdapter struct {
	mu         sync.Mutex
	nextID     int
	states     map[string]agent.EventType // terminal state per agent ID; absent = still running
	terminated []string
}

func newFakeAdapter() *fakeAdapter {
	return &fakeAdapter{states: make(map[string]agent.EventType)}
}

func (f *fakeAdapter) SpawnAgent(_ context.Context, spec *agent.Spec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	return fmt.Sprintf("fake-%d-%s", f.nextID, spec.Name), nil
}

func (f *fakeAdapter) TerminateAgent(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminated = append(f.terminated, id)
	return nil
}

func (f *fakeAdapter) WatchAgent(_ context.Context, _ string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event)
	close(ch)
	return ch, nil
}

func (f *fakeAdapter) AgentStatus(_ context.Context, id string) (*agent.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.states[id]
	if !ok {
		return nil, nil
	}
	return &agent.Event{AgentID: id, Type: state}, nil
}

func (f *fakeAdapter) complete(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[id] = agent.EventCompleted
}

func TestAgentJobReconcilerUsesAdapterNotPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:  "fake-ns",
			MessageBus: v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-job", Namespace: "fake-ns"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "fake-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Command: "run.sh", MaxReplicas: 2}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	adapter := newFakeAdapter()
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: adapter}
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "fake-job", Namespace: "fake-ns"}}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	updated := &v1alpha1.AgentJob{}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "fake-job", Namespace: "fake-ns"}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != "Running" {
		t.Fatalf("expected Running, got %q", updated.Status.Phase)
	}
	if len(updated.Status.AgentIDs) != 2 {
		t.Fatalf("expected 2 agent IDs for MaxReplicas=2, got %d", len(updated.Status.AgentIDs))
	}

	podList := &corev1.PodList{}
	if err := fakeClient.List(ctx, podList); err != nil {
		t.Fatal(err)
	}
	if len(podList.Items) != 0 {
		t.Errorf("expected no Pods created via a non-K8s adapter, got %d", len(podList.Items))
	}

	for _, id := range updated.Status.AgentIDs {
		adapter.complete(id)
	}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "fake-job", Namespace: "fake-ns"}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != "Succeeded" {
		t.Fatalf("expected Succeeded, got %q", updated.Status.Phase)
	}

	updated.Status.Phase = "Terminating"
	if err := fakeClient.Status().Update(ctx, updated); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if len(adapter.terminated) != 2 {
		t.Fatalf("expected TerminateAgent called for 2 agents, got %d", len(adapter.terminated))
	}
}
```

- [ ] **Step 2: Run it to confirm it fails to compile**

Run: `go test ./platform/k8s/reconcilers/... -run TestAgentJobReconcilerUsesAdapterNotPods -v`
Expected: FAIL to compile — `reconcilers.AgentJobReconciler` has no field `Adapter`.

- [ ] **Step 3: Rewrite `platform/k8s/reconcilers/agentjob_reconciler.go`**

Replace the entire file with:

```go
// SPDX-License-Identifier: Apache-2.0
package reconcilers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/platform/k8s/metrics"
	"github.com/muto-io/muto/platform/k8s/tracing"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentJobReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Adapter scheduler.PlatformAdapter
}

func (r *AgentJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return metrics.ObserveReconcile("agentjob", func() (ctrl.Result, error) {
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
	})
}

func (r *AgentJobReconciler) reconcilePending(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	tenant := &v1alpha1.Tenant{}
	if err := r.Get(ctx, types.NamespacedName{Name: job.Spec.TenantRef}, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("get tenant %q: %w", job.Spec.TenantRef, err)
	}

	var agentIDs []string
	totalAgents := 0
	for _, roleSpec := range job.Spec.Agents {
		replicas := roleSpec.MaxReplicas
		if replicas == 0 {
			replicas = 1
		}
		totalAgents += int(replicas)

		for i := int32(0); i < replicas; i++ {
			spec := r.buildAgentSpec(job, roleSpec, tenant, replicas, i)
			id, err := r.Adapter.SpawnAgent(ctx, spec)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("spawn agent: %w", err)
			}
			agentIDs = append(agentIDs, id)
		}
	}
	now := metav1.Now()
	job.Status.Phase = "Running"
	job.Status.ActiveAgents = int32(totalAgents)
	job.Status.StartedAt = &now
	job.Status.AgentIDs = agentIDs
	if err := r.Status().Update(ctx, job); err != nil {
		return ctrl.Result{}, err
	}
	metrics.JobStarted(tenant.Name, int32(totalAgents))
	return ctrl.Result{}, nil
}

func (r *AgentJobReconciler) reconcileRunning(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	allDone, anyFailed := true, false
	for _, id := range job.Status.AgentIDs {
		ev, err := r.Adapter.AgentStatus(ctx, id)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("agent status %s: %w", id, err)
		}
		if ev == nil {
			allDone = false
			continue
		}
		if ev.Type == agent.EventFailed {
			anyFailed = true
		}
	}

	if !allDone {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	phase := "Succeeded"
	if anyFailed {
		phase = "Failed"
	}

	var startedAt time.Time
	if job.Status.StartedAt != nil {
		startedAt = job.Status.StartedAt.Time
	}
	activeAgents := job.Status.ActiveAgents

	now := metav1.Now()
	job.Status.CompletedAt = &now
	job.Status.ActiveAgents = 0
	job.Status.Phase = phase
	if err := r.Status().Update(ctx, job); err != nil {
		return ctrl.Result{}, err
	}

	metrics.JobFinished(job.Spec.TenantRef, strings.ToLower(phase), startedAt, activeAgents)
	return ctrl.Result{RequeueAfter: time.Duration(job.Spec.TTLAfterCompletion) * time.Second}, nil
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
	return ctrl.Result{RequeueAfter: 0}, r.Status().Update(ctx, job)
}

func (r *AgentJobReconciler) reconcileTerminating(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	for _, id := range job.Status.AgentIDs {
		if err := r.Adapter.TerminateAgent(ctx, id); err != nil {
			return ctrl.Result{}, fmt.Errorf("terminate agent %s: %w", id, err)
		}
	}
	if err := r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete agentjob: %w", err)
	}
	return ctrl.Result{}, nil
}

// buildAgentSpec translates one AgentJob role/replica into the generic
// agent.Spec the PlatformAdapter understands. The pod-naming scheme matches
// what the reconciler used to build directly: a plain "<job>-<role>" name
// when there is a single replica, "<job>-<role>-<index>" when there are
// several.
func (r *AgentJobReconciler) buildAgentSpec(
	job *v1alpha1.AgentJob,
	roleSpec v1alpha1.AgentRoleSpec,
	tenant *v1alpha1.Tenant,
	replicas int32,
	replicaIndex int32,
) *agent.Spec {
	name := fmt.Sprintf("%s-%s", job.Name, roleSpec.Role)
	if replicas > 1 {
		name = fmt.Sprintf("%s-%s-%d", job.Name, roleSpec.Role, replicaIndex)
	}
	return &agent.Spec{
		TenantRef: job.Spec.TenantRef,
		Agents: []agent.AgentRole{{
			Role:        roleSpec.Role,
			Image:       roleSpec.Image,
			Command:     roleSpec.Command,
			MaxReplicas: roleSpec.MaxReplicas,
		}},
		Name:      name,
		Namespace: job.Namespace,
		Labels: map[string]string{
			"muto.io/tenant": job.Spec.TenantRef,
			"muto.io/job":    job.Name,
			"muto.io/role":   roleSpec.Role,
		},
		MessageBus: agent.MessageBusConfig{
			Topic: job.Spec.MessageBus.Topic,
			Type:  tenant.Spec.MessageBus.Type,
		},
		TTLAfterCompletion: job.Spec.TTLAfterCompletion,
	}
}

func (r *AgentJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentJob{}).
		Complete(tracing.WrapReconciler("AgentJobReconciler.Reconcile", r))
}
```

- [ ] **Step 4: Run the new test again**

Run: `go test ./platform/k8s/reconcilers/... -run TestAgentJobReconcilerUsesAdapterNotPods -v`
Expected: still FAILS to compile — the five pre-existing tests construct `&reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}` with no `Adapter`, which now means every `reconcilePending` call in those tests will nil-deref on `r.Adapter.SpawnAgent`.

- [ ] **Step 5: Wire a real `K8sAdapter` into the five pre-existing tests**

In `platform/k8s/reconcilers/agentjob_reconciler_test.go`, for each of the five existing `r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}` literals, add the `Adapter` field, using the same namespace as that test's `Tenant`/`AgentJob` objects:

`TestAgentJobReconcilerPendingToRunning`:
```go
r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")}
```

`TestAgentJobReconcilerInjectsA2AEnvVars`:
```go
r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "a2a-ns")}
```

`TestAgentJobReconcilerNoA2AEnvVarsForNATSTenant`:
```go
r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "nats-ns")}
```

`TestAgentJobReconcilerRecordsMetricsOnCompletion`:
```go
r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "metrics-job-agents")}
```

`TestAgentJobReconcilerRecordsFailedJobMetrics`:
```go
r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "failed-job-agents")}
```

`TestAgentJobReconcilerDoesNotDoubleCountOnStatusUpdateFailure` (note: this one must wrap the *interceptor-wrapped* `fakeClient` variable, not a second raw client, so that pod creation still goes through the same object the test later inspects):
```go
r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "retry-agents")}
```

- [ ] **Step 6: Run the full reconciler test package**

Run: `go test ./platform/k8s/reconcilers/... -v`
Expected: PASS for all eleven tests (six pre-existing, one new `TestAgentJobReconcilerUsesAdapterNotPods`, four already covered by Task 2/3 in other packages).

- [ ] **Step 7: Run the full unit suite**

Run: `make test-unit`
Expected: PASS, no regressions anywhere else in the repo.

- [ ] **Step 8: Commit**

```bash
git add platform/k8s/reconcilers/agentjob_reconciler.go platform/k8s/reconcilers/agentjob_reconciler_test.go
git commit -m "feat: route AgentJobReconciler through PlatformAdapter instead of direct Pod CRUD"
```

---

### Task 6: Wire the adapter into `cmd/muto-operator/main.go` and final verification

**Files:**
- Modify: `cmd/muto-operator/main.go`

**Interfaces:**
- Consumes: `reconcilers.AgentJobReconciler{Adapter}` (Task 5), `platformAdapter scheduler.PlatformAdapter` (already constructed earlier in `run()`).

- [ ] **Step 1: Pass the adapter into the reconciler and fix the stale comment**

In `cmd/muto-operator/main.go`, replace:

```go
	// platformAdapter is used by the DefaultScheduler for direct job scheduling
	// (e.g. via the MCP server). K8s reconcilers use mgr.GetClient() directly
	// and are platform-independent.
	log.Info("platform adapter initialized", "platform", platform, "type", fmt.Sprintf("%T", platformAdapter))

	if err := (&reconcilers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create TenantReconciler")
		return 1
	}

	if err := (&reconcilers.AgentJobReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create AgentJobReconciler")
		return 1
	}
```

with:

```go
	// AgentJobReconciler delegates agent spawn/status/terminate calls to
	// platformAdapter, so it is platform-independent regardless of which
	// adapter was selected above. The other reconcilers only manage CRD
	// state via mgr.GetClient() and never touch agent compute directly.
	log.Info("platform adapter initialized", "platform", platform, "type", fmt.Sprintf("%T", platformAdapter))

	if err := (&reconcilers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create TenantReconciler")
		return 1
	}

	if err := (&reconcilers.AgentJobReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Adapter: platformAdapter,
	}).SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to create AgentJobReconciler")
		return 1
	}
```

- [ ] **Step 2: Verify the operator binary builds**

Run: `go build ./cmd/...`
Expected: success.

- [ ] **Step 3: Run the full unit suite, vet, and format check**

Run: `make test-unit && go vet ./... && gofmt -l .`
Expected: `make test-unit` PASSes; `go vet` reports nothing; `gofmt -l .` prints no file names (everything is formatted).

- [ ] **Step 4: Commit**

```bash
git add cmd/muto-operator/main.go
git commit -m "feat: wire platformAdapter into AgentJobReconciler in muto-operator"
```

---

## Not covered by this plan

- `test/integration/k8s/*` and `test/integration/cf/*` (build-tagged `integration`, require a live k3s cluster / CF mock harness) are not run as part of this plan. They exercise the reconciler at the black-box CR/Pod level and should continue to pass unchanged because this design preserves the same `muto.io/job`/`muto.io/tenant`/`muto.io/role` labels and the same `Phase`/`ActiveAgents` status semantics — but this is not verified by this plan's steps. Running `make test-integration-k8s` against a real cluster (or at minimum reading through `agentjob_lifecycle_test.go`, `k8s_multiagent_coordination_test.go`, `a2a_gateway_test.go`, `ttl_cleanup_test.go`) before merging is recommended as a follow-up, outside this plan's scope.
- Per-tenant platform selection (`Tenant.spec.platform`) and the rest of `docs/architecture/platform-design.md`'s aspirational surface remain out of scope (see the spec's Non-goals).
