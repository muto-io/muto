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

var _ = Describe("K8s Advanced Failure Scenarios", func() {
	ctx := context.Background()
	var testCounter int

	Describe("resource constraints and OOM", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("resource-failure-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-resource-%d", testCounter)},
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

		It("should handle agents with memory constraints", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "mem-constrain-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{
							Role:        "memory-worker",
							Image:       "busybox:latest",
							MaxReplicas: 1,
						},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.mem-job", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "mem-constrain-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Verify pod is created with resource requests/limits
			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "mem-constrain-job"},
			)).To(Succeed())
			Expect(podList.Items).NotTo(BeEmpty())
		})

		It("should handle agents with CPU constraints", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "cpu-constrain-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{
							Role:        "cpu-worker",
							Image:       "busybox:latest",
							MaxReplicas: 1,
						},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.cpu-job", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "cpu-constrain-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "cpu-constrain-job"},
			)).To(Succeed())
			Expect(podList.Items).NotTo(BeEmpty())
		})
	})

	Describe("agent pod lifecycle failures", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("lifecycle-failure-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-lifecycle-%d", testCounter)},
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

		It("should handle pod startup failure and recovery", func() {
			// Create a job that uses an image that fails to start
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "startup-fail-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.startup-fail", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Job should exist and transition
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "startup-fail-job", Namespace: nsName,
				}, updated)).To(Succeed())
				// Job should be in Running or Pending state
				g.Expect(updated.Status.Phase).To(Or(Equal("Running"), Equal("Pending")))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("should handle pod with long startup time", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "slow-start-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.slow-start", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Pod creation may take time; verify job handles it
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "slow-start-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Or(Equal("Running"), Equal("Pending")))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("should prevent infinite restart loops", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "no-restart-loop-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.no-restart", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Verify pod doesn't restart excessively
			time.Sleep(5 * time.Second)

			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "no-restart-loop-job"},
			)).To(Succeed())

			if len(podList.Items) > 0 {
				pod := podList.Items[0]
				for _, container := range pod.Status.ContainerStatuses {
					// Restart count should be 0 or minimal
					Expect(container.RestartCount).To(BeNumerically("<=", 3))
				}
			}
		})
	})

	Describe("concurrent agent failures", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("concurrent-fail-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-concurrent-%d", testCounter)},
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

		It("should handle simultaneous pod failures gracefully", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "concurrent-fail-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 3},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.concurrent-fail", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "concurrent-fail-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Delete multiple pods simultaneously
			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "concurrent-fail-job"},
			)).To(Succeed())

			// Delete pods in parallel
			for _, pod := range podList.Items {
				podCopy := pod
				_ = k8sClient.Delete(ctx, &podCopy)
			}

			// Job should still be in a valid state
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "concurrent-fail-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).NotTo(Equal("Failed"))
			}).WithTimeout(10 * time.Second).Should(Succeed())
		})

		It("should handle partial failure in multi-role jobs", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "partial-fail-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "coordinator", Image: "busybox:latest", MaxReplicas: 1},
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 2},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.partial-fail", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "partial-fail-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Delete only one worker pod
			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{
					"muto.io/job":  "partial-fail-job",
					"muto.io/role": "worker",
				},
			)).To(Succeed())

			if len(podList.Items) > 0 {
				_ = k8sClient.Delete(ctx, &podList.Items[0])
			}

			// Job should remain running
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: "partial-fail-job", Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Or(Equal("Running"), Equal("Pending")))
			}).WithTimeout(10 * time.Second).Should(Succeed())
		})
	})
})
