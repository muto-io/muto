package k8s_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/core/agent"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSpawnAgentCreatesPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	spec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "worker:latest", MaxReplicas: 1}},
	}
	id, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("expected non-empty agent ID")
	}
}

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

func TestSpawnAgentAddsSIGTERMTrapForBusyboxImage(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	spec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "busybox:latest"}},
		Name:      "job-1-worker-0",
		Namespace: "acme-agents",
	}
	if _, err := adapter.SpawnAgent(context.Background(), spec); err != nil {
		t.Fatal(err)
	}

	pod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "job-1-worker-0", Namespace: "acme-agents"}, pod); err != nil {
		t.Fatal(err)
	}
	want := []string{"sh", "-c", "trap 'exit 0' TERM; sleep 30 & wait"}
	got := pod.Spec.Containers[0].Command
	if len(got) != len(want) {
		t.Fatalf("got Command %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got Command %v, want %v", got, want)
		}
	}

	// A non-busybox image must not get the trap override.
	nonBusyboxSpec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "worker:latest"}},
		Name:      "job-1-worker-1",
		Namespace: "acme-agents",
	}
	if _, err := adapter.SpawnAgent(context.Background(), nonBusyboxSpec); err != nil {
		t.Fatal(err)
	}
	otherPod := &corev1.Pod{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: "job-1-worker-1", Namespace: "acme-agents"}, otherPod); err != nil {
		t.Fatal(err)
	}
	if len(otherPod.Spec.Containers[0].Command) != 0 {
		t.Errorf("expected no Command override for non-busybox image, got %v", otherPod.Spec.Containers[0].Command)
	}
}

func TestSpawnAgentToleratesAlreadyExists(t *testing.T) {
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

	firstID, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}

	// Re-spawning the same role/replica name (e.g. a reconciler retry after a
	// status-update failure that left the pod already created) must not
	// error, and must return the same composite ID.
	secondID, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatalf("expected re-spawn with same Name/Namespace to tolerate AlreadyExists, got error: %v", err)
	}
	if secondID != firstID {
		t.Errorf("got id %q on retry, want same id %q", secondID, firstID)
	}
}

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
