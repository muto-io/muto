package reconcilers_test

import (
	"context"
	"fmt"
	"testing"

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
)

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
			TenantRef: "acme",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant, job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}
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
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}
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
	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}
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

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}
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
