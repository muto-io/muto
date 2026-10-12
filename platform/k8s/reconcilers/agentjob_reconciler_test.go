package reconcilers_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/platform/k8s/metrics"
	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

func TestAgentJobReconcilerPendingToRunning(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "acme"},
		Spec: v1alpha1.TenantSpec{
			Namespace:  "acme-agents",
			MessageBus: v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-1", Namespace: "acme-agents"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef:  "acme",
			Agents:     []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
			MessageBus: v1alpha1.JobBusSpec{Topic: "job-1-topic"},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "job-1", Namespace: "acme-agents"},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated := &v1alpha1.AgentJob{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "job-1", Namespace: "acme-agents"}, updated)
	if updated.Status.Phase != "Running" {
		t.Errorf("expected Running, got %q", updated.Status.Phase)
	}

	podList := &corev1.PodList{}
	if err := fakeClient.List(context.Background(), podList, client.InNamespace("acme-agents")); err != nil {
		t.Fatal(err)
	}
	if len(podList.Items) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(podList.Items))
	}
	envMap := map[string]string{}
	for _, e := range podList.Items[0].Spec.Containers[0].Env {
		envMap[e.Name] = e.Value
	}
	if envMap["MUTO_JOB_ID"] != "job-1" {
		t.Errorf("expected MUTO_JOB_ID=job-1 on job pod, got %q", envMap["MUTO_JOB_ID"])
	}
	if envMap["MUTO_MESSAGEBUS_TOPIC"] != "job-1-topic" {
		t.Errorf("expected MUTO_MESSAGEBUS_TOPIC=job-1-topic on job pod, got %q", envMap["MUTO_MESSAGEBUS_TOPIC"])
	}
}

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
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-a2a", Namespace: "a2a-ns"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "a2a-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "a2a-ns")}
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
	secretRefMap := map[string]*corev1.SecretKeySelector{}
	for _, e := range pod.Spec.Containers[0].Env {
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			secretRefMap[e.Name] = e.ValueFrom.SecretKeyRef
		} else {
			envMap[e.Name] = e.Value
		}
	}
	if envMap["MUTO_A2A_GATEWAY"] != "http://a2a-gateway.a2a-ns.svc.cluster.local:8080" {
		t.Errorf("unexpected MUTO_A2A_GATEWAY: %q", envMap["MUTO_A2A_GATEWAY"])
	}
	ref := secretRefMap["MUTO_A2A_TOKEN"]
	if ref == nil {
		t.Fatal("expected MUTO_A2A_TOKEN to use valueFrom.secretKeyRef, got plain value")
	}
	if ref.Name != "muto-a2a-token" || ref.Key != "token" {
		t.Errorf("unexpected secretKeyRef for MUTO_A2A_TOKEN: name=%q key=%q", ref.Name, ref.Key)
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

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "nats-ns")}
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

func TestAgentJobReconcilerRecordsMetricsOnCompletion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-job-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:  "metrics-job-agents",
			MessageBus: v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-job", Namespace: "metrics-job-agents"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "metrics-job-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "metrics-job-agents")}
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "metrics-job", Namespace: "metrics-job-agents"}}

	// Pending -> Running: creates the pod and should record JobStarted.
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues("metrics-job-tenant")); got != 1 {
		t.Errorf("AgentsRunning{metrics-job-tenant} = %v, want 1 after pod creation", got)
	}

	// Mark the pod Succeeded so the next reconcile sees the job as done.
	podList := &corev1.PodList{}
	if err := fakeClient.List(ctx, podList, client.InNamespace("metrics-job-agents")); err != nil {
		t.Fatal(err)
	}
	if len(podList.Items) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(podList.Items))
	}
	pod := podList.Items[0]
	pod.Status.Phase = corev1.PodSucceeded
	if err := fakeClient.Status().Update(ctx, &pod); err != nil {
		t.Fatal(err)
	}

	jobsTotalBefore := testutil.ToFloat64(metrics.JobsTotal.WithLabelValues("metrics-job-tenant", "succeeded"))

	// Running -> Succeeded: should record JobFinished.
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	if got := testutil.ToFloat64(metrics.JobsTotal.WithLabelValues("metrics-job-tenant", "succeeded")); got != jobsTotalBefore+1 {
		t.Errorf("JobsTotal{metrics-job-tenant,succeeded} = %v, want %v", got, jobsTotalBefore+1)
	}
	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues("metrics-job-tenant")); got != 0 {
		t.Errorf("AgentsRunning{metrics-job-tenant} = %v, want 0 after completion", got)
	}
}

func TestAgentJobReconcilerRecordsFailedJobMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "failed-job-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:  "failed-job-agents",
			MessageBus: v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "failed-job", Namespace: "failed-job-agents"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "failed-job-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "failed-job-agents")}
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "failed-job", Namespace: "failed-job-agents"}}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	podList := &corev1.PodList{}
	if err := fakeClient.List(ctx, podList, client.InNamespace("failed-job-agents")); err != nil {
		t.Fatal(err)
	}
	if len(podList.Items) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(podList.Items))
	}
	pod := podList.Items[0]
	pod.Status.Phase = corev1.PodFailed
	if err := fakeClient.Status().Update(ctx, &pod); err != nil {
		t.Fatal(err)
	}

	before := testutil.ToFloat64(metrics.JobsTotal.WithLabelValues("failed-job-tenant", "failed"))

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}

	if got := testutil.ToFloat64(metrics.JobsTotal.WithLabelValues("failed-job-tenant", "failed")); got != before+1 {
		t.Errorf("JobsTotal{failed-job-tenant,failed} = %v, want %v", got, before+1)
	}
}

func TestAgentJobReconcilerDoesNotDoubleCountOnStatusUpdateFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "retry-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:  "retry-agents",
			MessageBus: v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "retry-job", Namespace: "retry-agents"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "retry-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}

	failNextUpdate := true
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()
	fakeClient := interceptor.NewClient(baseClient, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if failNextUpdate {
				failNextUpdate = false
				return fmt.Errorf("injected failure")
			}
			return c.Status().Update(ctx, obj, opts...)
		},
	})

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "retry-agents")}
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "retry-job", Namespace: "retry-agents"}}

	before := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues("retry-tenant"))

	// First reconcile: Status().Update fails, so JobStarted must NOT fire.
	if _, err := r.Reconcile(ctx, req); err == nil {
		t.Fatal("expected an error from the injected Status().Update failure")
	}
	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues("retry-tenant")); got != before {
		t.Errorf("AgentsRunning{retry-tenant} = %v after failed update, want unchanged %v", got, before)
	}

	// Second reconcile: succeeds, JobStarted fires exactly once.
	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues("retry-tenant")); got != before+1 {
		t.Errorf("AgentsRunning{retry-tenant} = %v after retry, want %v", got, before+1)
	}
}

func TestAgentJobReconcilerAddsFinalizerOnCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "finalizer-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:  "finalizer-tenant-ns",
			MessageBus: v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "finalizer-job", Namespace: "finalizer-tenant-ns"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "finalizer-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: k8sadapter.NewK8sAdapter(fakeClient, "finalizer-tenant-ns")}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "finalizer-job", Namespace: "finalizer-tenant-ns"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := &v1alpha1.AgentJob{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "finalizer-job", Namespace: "finalizer-tenant-ns"}, got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range got.Finalizers {
		if f == "muto.io/agentjob-cleanup" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected muto.io/agentjob-cleanup finalizer to be added, got finalizers: %v", got.Finalizers)
	}
}

func TestAgentJobReconcilerDeletionTerminatesAgentsAndRemovesFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "deleting-job",
			Namespace:  "deleting-ns",
			Finalizers: []string{"muto.io/agentjob-cleanup"},
		},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "deleting-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 2}},
		},
		Status: v1alpha1.AgentJobStatus{
			Phase:    "Running",
			AgentIDs: []string{"deleting-ns/agent-0", "deleting-ns/agent-1"},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	adapter := newFakeAdapter()
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: adapter}

	// Mark the AgentJob for deletion directly (not via the TTL->Terminating
	// path). Because the finalizer is present, the fake client (like a real
	// API server) keeps the object around with a DeletionTimestamp set
	// instead of deleting it outright.
	if err := fakeClient.Delete(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "deleting-job", Namespace: "deleting-ns"},
	}); err != nil {
		t.Fatal(err)
	}

	if len(adapter.terminated) != 2 {
		t.Fatalf("expected TerminateAgent called for 2 tracked agents, got %d: %v", len(adapter.terminated), adapter.terminated)
	}

	// Finalizer removed -> AgentJob object itself is now gone.
	remaining := &v1alpha1.AgentJob{}
	err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "deleting-job", Namespace: "deleting-ns"}, remaining)
	if err == nil {
		t.Errorf("expected AgentJob to be fully deleted once finalizer is removed, got finalizers: %v", remaining.Finalizers)
	}
}

func TestAgentJobReconcilerTerminatingDeletesWithoutDoubleTerminate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ttl-job",
			Namespace:  "ttl-ns",
			Finalizers: []string{"muto.io/agentjob-cleanup"},
		},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "ttl-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 2}},
		},
		Status: v1alpha1.AgentJobStatus{
			Phase:    "Terminating",
			AgentIDs: []string{"ttl-ns/agent-0", "ttl-ns/agent-1"},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	adapter := newFakeAdapter()
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: adapter}

	// The job is already in the TTL-driven Terminating phase (not a direct
	// delete), with the finalizer already present as every reconciled
	// AgentJob now carries it. A single reconcile must terminate each
	// tracked agent exactly once and fully delete the object - not merely
	// set a DeletionTimestamp that would bounce back through
	// finalizeAgentJob and terminate everything a second time.
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "ttl-job", Namespace: "ttl-ns"},
	}); err != nil {
		t.Fatal(err)
	}

	if len(adapter.terminated) != 2 {
		t.Fatalf("expected TerminateAgent called exactly once per tracked agent (2), got %d: %v", len(adapter.terminated), adapter.terminated)
	}

	remaining := &v1alpha1.AgentJob{}
	err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "ttl-job", Namespace: "ttl-ns"}, remaining)
	if !errors.IsNotFound(err) {
		t.Fatalf("expected AgentJob to be fully deleted in one pass, got err=%v remaining=%+v", err, remaining)
	}
}

func TestAgentJobReconcilerRunningWithNoTrackedAgentIDsRequeuesWithoutSucceeding(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "untracked-job", Namespace: "untracked-ns", Finalizers: []string{"muto.io/agentjob-cleanup"}},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "untracked-tenant",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
		Status: v1alpha1.AgentJobStatus{
			Phase:    "Running",
			AgentIDs: nil,
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	adapter := newFakeAdapter()
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme, Adapter: adapter}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "untracked-job", Namespace: "untracked-ns"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 5*time.Second {
		t.Errorf("expected RequeueAfter=5s, got %v", result.RequeueAfter)
	}

	updated := &v1alpha1.AgentJob{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "untracked-job", Namespace: "untracked-ns"}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != "Running" {
		t.Errorf("expected job to remain Running with untracked AgentIDs, got %q", updated.Status.Phase)
	}
}
