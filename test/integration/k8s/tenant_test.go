//go:build integration

package k8s_test

import (
	"context"
	"fmt"
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
		var (
			tenant     *v1alpha1.Tenant
			tenantName string
			agentsNS   string
		)

		BeforeEach(func() {
			// Tenant is cluster-scoped, so the name must be unique across
			// ginkgo -p processes, not just within this process.
			tenantName = fmt.Sprintf("integration-tenant-%d", GinkgoParallelProcess())
			agentsNS = fmt.Sprintf("integration-tenant-agents-%d", GinkgoParallelProcess())
			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: tenantName},
				Spec: v1alpha1.TenantSpec{
					Namespace:     agentsNS,
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
				err := k8sClient.Get(ctx, client.ObjectKey{Name: tenantName}, t)
				return err != nil
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(BeTrue())
		})

		It("creates the target namespace with muto.io/tenant label", func() {
			Eventually(func(g Gomega) {
				ns := &corev1.Namespace{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: agentsNS}, ns)).
					To(Succeed())
				g.Expect(ns.Labels["muto.io/tenant"]).To(Equal(tenantName))
			}).WithTimeout(15 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())
		})

		It("sets Tenant status.ready=true", func() {
			Eventually(func(g Gomega) {
				updated := &v1alpha1.Tenant{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tenantName}, updated)).
					To(Succeed())
				g.Expect(updated.Status.Ready).To(BeTrue())
			}).WithTimeout(15 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())
		})
	})
})
