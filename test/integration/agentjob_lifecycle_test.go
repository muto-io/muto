//go:build integration

package integration_test

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

var _ = Describe("AgentJob", func() {
	ctx := context.Background()

	// Use a unique namespace per It block via a counter so parallel/sequential
	// runs never collide on a Terminating namespace from a previous test.
	var testCounter int

	Describe("lifecycle", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
			job    *v1alpha1.AgentJob
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("lifecycle-test-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("test-%d", testCounter)},
				Spec: v1alpha1.TenantSpec{
					Namespace:     nsName,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

			job = &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "lifecycle-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef:          tenant.Name,
					Trigger:            v1alpha1.TriggerSpec{Type: "event"},
					Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.lifecycle-job", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, job)
			_ = k8sClient.Delete(ctx, tenant)
			_ = k8sClient.Delete(ctx, ns)
		})

		It("transitions to Running phase", func() {
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "lifecycle-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("creates agent pods with correct labels", func() {
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "lifecycle-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).Should(Succeed())

			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "lifecycle-job"},
			)).To(Succeed())
			Expect(podList.Items).NotTo(BeEmpty())
			Expect(podList.Items[0].Labels["muto.io/tenant"]).To(Equal(tenant.Name))
		})
	})
})


