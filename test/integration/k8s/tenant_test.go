//go:build integration

package k8s_test

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
			// Delete the tenant and wait for it to be fully removed
			_ = k8sClient.Delete(ctx, tenant)

			// Wait for the tenant to be fully deleted before the next test
			// This prevents the "already exists" error when BeforeEach tries to create it again
			Eventually(func() bool {
				t := &v1alpha1.Tenant{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "integration-tenant"}, t)
				return err != nil
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(BeTrue())
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
