package reconcilers_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
