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
