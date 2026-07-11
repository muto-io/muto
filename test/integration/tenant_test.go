//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTenantCreatesNamespace(t *testing.T) {
	ctx := context.Background()

	tenant := &v1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "integration-tenant"},
		Spec: v1alpha1.TenantSpec{
			Namespace:     "integration-tenant-agents",
			IsolationTier: "shared",
			MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
		},
	}
	if err := k8sClient.Create(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, tenant) })

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		ns := &corev1.Namespace{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: "integration-tenant-agents"}, ns); err == nil {
			if ns.Labels["muto.io/tenant"] == "integration-tenant" {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("namespace integration-tenant-agents not created within 15s")
}
