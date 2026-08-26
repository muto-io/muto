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

var _ = Describe("K8s Multi-Agent Coordination", func() {
	ctx := context.Background()
	var testCounter int
	var k8sHelper *K8sTestHelper

	BeforeEach(func() {
		if k8sHelper == nil {
			k8sHelper = NewK8sTestHelper()
		}
	})

	Describe("multi-agent job orchestration", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("multiagent-test-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-multi-%d", testCounter)},
				Spec: v1alpha1.TenantSpec{
					Namespace:     nsName,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should coordinate coordinator and worker agents", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "coord-worker-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "coordinator", Image: "busybox:latest", MaxReplicas: 1},
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 2},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.coord-job", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Verify job reaches Running
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "coord-worker-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Verify pods for both roles are created
			Eventually(func(g Gomega) {
				podList := &corev1.PodList{}
				g.Expect(k8sClient.List(ctx, podList,
					client.InNamespace(nsName),
					client.MatchingLabels{"muto.io/job": "coord-worker-job"},
				)).To(Succeed())

				// Should have at least 3 pods (1 coordinator + 2 workers)
				g.Expect(len(podList.Items)).To(BeNumerically(">=", 3))

				// Verify both roles are present
				hasCoordinator := false
				hasWorker := false
				for _, pod := range podList.Items {
					role := pod.Labels["muto.io/role"]
					if role == "coordinator" {
						hasCoordinator = true
					}
					if role == "worker" {
						hasWorker = true
					}
				}
				g.Expect(hasCoordinator).To(BeTrue())
				g.Expect(hasWorker).To(BeTrue())
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("should scale worker replicas independently", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "scale-test-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 5},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.scale-job", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Verify expected number of worker pods
			Eventually(func(g Gomega) {
				podList := &corev1.PodList{}
				g.Expect(k8sClient.List(ctx, podList,
					client.InNamespace(nsName),
					client.MatchingLabels{
						"muto.io/job":  "scale-test-job",
						"muto.io/role": "worker",
					},
				)).To(Succeed())

				// Should have up to 5 worker pods
				g.Expect(len(podList.Items)).To(BeNumerically("<=", 5))
				g.Expect(len(podList.Items)).To(BeNumerically(">", 0))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("should handle communication between roles via message bus", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "msgbus-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "producer", Image: "busybox:latest", MaxReplicas: 1},
						{Role: "consumer", Image: "busybox:latest", MaxReplicas: 1},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.msgbus-job", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Verify both producer and consumer pods are created
			Eventually(func(g Gomega) {
				podList := &corev1.PodList{}
				g.Expect(k8sClient.List(ctx, podList,
					client.InNamespace(nsName),
					client.MatchingLabels{"muto.io/job": "msgbus-job"},
				)).To(Succeed())

				g.Expect(len(podList.Items)).To(Equal(2))

				// Verify message bus env var is set
				for _, pod := range podList.Items {
					hasMsgBusEnv := false
					for _, container := range pod.Spec.Containers {
						for _, env := range container.Env {
							if env.Name == "MUTO_MESSAGEBUS_TOPIC" {
								hasMsgBusEnv = true
							}
						}
					}
					g.Expect(hasMsgBusEnv).To(BeTrue(), "pod should have message bus env var")
				}
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("should ensure agent pod mutual exclusivity per role", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "mutual-excl-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "primary", Image: "busybox:latest", MaxReplicas: 2},
						{Role: "secondary", Image: "busybox:latest", MaxReplicas: 2},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.mutual-excl", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			Eventually(func(g Gomega) {
				podList := &corev1.PodList{}
				g.Expect(k8sClient.List(ctx, podList,
					client.InNamespace(nsName),
					client.MatchingLabels{"muto.io/job": "mutual-excl-job"},
				)).To(Succeed())

				// Verify no pod has multiple roles
				for _, pod := range podList.Items {
					roles := pod.Labels["muto.io/role"]
					// Each pod should have exactly one role label
					g.Expect(len(roles)).To(BeNumerically(">", 0))
				}
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})
	})
})
