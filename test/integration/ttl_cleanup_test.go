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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AgentJob TTL cleanup", func() {
	ctx := context.Background()

	var testCounter int

	var (
		ns     *corev1.Namespace
		tenant *v1alpha1.Tenant
		job    *v1alpha1.AgentJob
	)

	BeforeEach(func() {
		testCounter++
		nsName := fmt.Sprintf("ttl-test-%d", testCounter)
		tenantName := fmt.Sprintf("ttl-tenant-%d", testCounter)

		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		tenant = &v1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: tenantName},
			Spec: v1alpha1.TenantSpec{
				Namespace:     nsName,
				IsolationTier: "shared",
				MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		job = &v1alpha1.AgentJob{
			ObjectMeta: metav1.ObjectMeta{Name: "ttl-job", Namespace: nsName},
			Spec: v1alpha1.AgentJobSpec{
				TenantRef:          tenantName,
				Trigger:            v1alpha1.TriggerSpec{Type: "manual"},
				Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
				TTLAfterCompletion: 2,
			},
		}
		Expect(k8sClient.Create(ctx, job)).To(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, tenant)
		_ = k8sClient.Delete(ctx, ns)
	})

	It("deletes the AgentJob after TTL expires", func() {
		now := metav1.Now()
		patch := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = "Succeeded"
		job.Status.CompletedAt = &now
		Expect(k8sClient.Status().Patch(ctx, job, patch)).To(Succeed())

		Eventually(func(g Gomega) {
			check := &v1alpha1.AgentJob{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "ttl-job", Namespace: job.Namespace}, check)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(20 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})
})
