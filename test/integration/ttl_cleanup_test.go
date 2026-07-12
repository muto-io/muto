//go:build integration

package integration_test

import (
	"context"
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

	var (
		ns  *corev1.Namespace
		job *v1alpha1.AgentJob
	)

	BeforeEach(func() {
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ttl-test"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		job = &v1alpha1.AgentJob{
			ObjectMeta: metav1.ObjectMeta{Name: "ttl-job", Namespace: "ttl-test"},
			Spec: v1alpha1.AgentJobSpec{
				TenantRef:          "test",
				Trigger:            v1alpha1.TriggerSpec{Type: "manual"},
				Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
				TTLAfterCompletion: 2,
			},
		}
		Expect(k8sClient.Create(ctx, job)).To(Succeed())
	})

	AfterEach(func() {
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
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "ttl-job", Namespace: "ttl-test"}, check)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}).WithTimeout(20 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})
})
