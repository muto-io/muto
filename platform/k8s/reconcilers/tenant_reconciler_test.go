package reconcilers_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestTenantReconcilerCreatesNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "acme"},
		Spec: v1alpha1.TenantSpec{
			Namespace:     "acme-agents",
			IsolationTier: "shared",
			MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tenant).
		WithStatusSubresource(&v1alpha1.Tenant{}).Build()
	r := &reconcilers.TenantReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "acme"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ns := &corev1.Namespace{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "acme-agents"}, ns); err != nil {
		t.Errorf("namespace acme-agents not created: %v", err)
	}
	if ns.Labels["muto.io/tenant"] != "acme" {
		t.Errorf("namespace missing muto.io/tenant label, got labels: %v", ns.Labels)
	}
}

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

	dep := &appsv1.Deployment{}
	if err := fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "a2a-gateway", Namespace: "a2a-ns"}, dep); err != nil {
		t.Errorf("a2a-gateway Deployment not created: %v", err)
	}

	svc := &corev1.Service{}
	if err := fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "a2a-gateway", Namespace: "a2a-ns"}, svc); err != nil {
		t.Errorf("a2a-gateway Service not created: %v", err)
	}
	if svc.Spec.Ports[0].Port != 8080 {
		t.Errorf("expected port 8080, got %d", svc.Spec.Ports[0].Port)
	}

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
