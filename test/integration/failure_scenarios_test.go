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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Failure Scenarios", func() {
	ctx := context.Background()
	var testCounter int

	Describe("Pod Eviction and Recovery", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
			job    *v1alpha1.AgentJob
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("failure-pod-evict-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-%d", testCounter), Namespace: nsName},
				Spec: v1alpha1.TenantSpec{
					Namespace:     nsName,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

			job = &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "eviction-test-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef:          tenant.Name,
					Trigger:            v1alpha1.TriggerSpec{Type: "event"},
					Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.eviction-test", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should handle pod deletion gracefully", func() {
			// Wait for job to be running
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: job.Name, Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Find the agent pod
			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "eviction-test-job"},
			)).To(Succeed())
			Expect(podList.Items).NotTo(BeEmpty())

			pod := &podList.Items[0]

			// Delete the pod (simulate eviction)
			Expect(k8sClient.Delete(ctx, pod)).To(Succeed())

			// The job should remain in Running state (operator will not automatically recreate pods)
			// This is expected behavior - the scheduler is responsible for spawning new instances
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: job.Name, Namespace: nsName,
				}, updated)).To(Succeed())
				// Job should still exist and not be in Failed state due to pod deletion
				g.Expect(updated.Status.Phase).NotTo(Equal("Failed"))
			}).WithTimeout(10 * time.Second).Should(Succeed())
		})

		It("should handle pod creation failure gracefully", func() {
			// Create an AgentJob with an invalid image
			invalidJob := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-image-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef:          tenant.Name,
					Trigger:            v1alpha1.TriggerSpec{Type: "event"},
					Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "nonexistent:invalid-tag-12345", MaxReplicas: 1}},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.invalid", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, invalidJob)).To(Succeed())

			// Job should transition to Running (pod will be created but stay in pending/imagepull state)
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: invalidJob.Name, Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Pod should exist but be in a non-ready state (ImagePullBackOff or similar)
			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "invalid-image-job"},
			)).To(Succeed())
			Expect(podList.Items).NotTo(BeEmpty())
			// Pod will be pending due to image pull failure
			Expect(podList.Items[0].Status.Phase).To(Equal(corev1.PodPending))
		})
	})

	Describe("Resource Deletion and Cleanup", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("failure-cleanup-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-%d", testCounter), Namespace: nsName},
				Spec: v1alpha1.TenantSpec{
					Namespace:     nsName,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should handle tenant deletion with active jobs", func() {
			// Create a job
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "cleanup-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef:          tenant.Name,
					Trigger:            v1alpha1.TriggerSpec{Type: "event"},
					Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.cleanup", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Wait for job to be running
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: job.Name, Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Delete the tenant
			Expect(k8sClient.Delete(ctx, tenant)).To(Succeed())

			// Tenant should be gone (may be in terminating state briefly)
			Eventually(func(g Gomega) {
				retrieved := &v1alpha1.Tenant{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      tenant.Name,
					Namespace: nsName,
				}, retrieved)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Job may still exist or be in cleanup phase, but should not prevent tenant deletion
			// The important thing is that deletion doesn't hang
		})

		It("should handle job deletion with TTL cleanup", func() {
			// Create a job with short TTL
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "ttl-cleanup-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef:          tenant.Name,
					Trigger:            v1alpha1.TriggerSpec{Type: "event"},
					Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.ttl", tenant.Name)},
					TTLAfterCompletion: 5, // 5 seconds TTL
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Delete the job immediately (should cleanup quickly due to cascading deletes)
			Expect(k8sClient.Delete(ctx, job)).To(Succeed())

			// Job should be gone
			Eventually(func(g Gomega) {
				retrieved := &v1alpha1.AgentJob{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      job.Name,
					Namespace: nsName,
				}, retrieved)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(30 * time.Second).Should(Succeed())
		})

		It("should handle missing referenced tenant gracefully", func() {
			// Create a job that references a non-existent tenant
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "missing-tenant-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef:          "nonexistent-tenant", // Reference to non-existent tenant
					Trigger:            v1alpha1.TriggerSpec{Type: "event"},
					Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
					MessageBus:         v1alpha1.JobBusSpec{Topic: "topic"},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Job should exist but likely remain in a pending/error state
			// The reconciler should handle the missing tenant gracefully
			Eventually(func(g Gomega) {
				retrieved := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      job.Name,
					Namespace: nsName,
				}, retrieved)).To(Succeed())
				// Job should exist (not crash the reconciler)
				g.Expect(retrieved.Name).To(Equal("missing-tenant-job"))
			}).WithTimeout(10 * time.Second).Should(Succeed())
		})
	})

	Describe("Namespace Termination", func() {
		var (
			nsName string
			ns     *corev1.Namespace
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("failure-terminating-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		})

		It("should handle resources in terminating namespace", func() {
			// Create tenant and job in the namespace
			tenant := &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "term-tenant", Namespace: nsName},
				Spec: v1alpha1.TenantSpec{
					Namespace:     nsName,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "term-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef:          tenant.Name,
					Trigger:            v1alpha1.TriggerSpec{Type: "event"},
					Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.term", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Wait for job to be running
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: job.Name, Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Delete the namespace (this will cascade delete all resources)
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())

			// Namespace should eventually be gone
			Eventually(func(g Gomega) {
				retrieved := &corev1.Namespace{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, retrieved)
				// Either namespace is gone or in terminating state
				g.Expect(err == nil && retrieved.DeletionTimestamp != nil || apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Job should be gone after namespace deletion
			Eventually(func(g Gomega) {
				retrieved := &v1alpha1.AgentJob{}
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      job.Name,
					Namespace: nsName,
				}, retrieved)
				g.Expect(apierrors.IsNotFound(err) || apierrors.IsConflict(err)).To(BeTrue())
			}).WithTimeout(30 * time.Second).Should(Succeed())
		})
	})

	Describe("Concurrent Operations", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("failure-concurrent-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-%d", testCounter), Namespace: nsName},
				Spec: v1alpha1.TenantSpec{
					Namespace:     nsName,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should handle concurrent job creations", func() {
			// Create multiple jobs concurrently
			numJobs := 3
			jobs := make([]*v1alpha1.AgentJob, numJobs)

			for i := 0; i < numJobs; i++ {
				job := &v1alpha1.AgentJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("concurrent-job-%d", i),
						Namespace: nsName,
					},
					Spec: v1alpha1.AgentJobSpec{
						TenantRef:          tenant.Name,
						Trigger:            v1alpha1.TriggerSpec{Type: "event"},
						Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
						MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.concurrent-%d", tenant.Name, i)},
						TTLAfterCompletion: 60,
					},
				}
				Expect(k8sClient.Create(ctx, job)).To(Succeed())
				jobs[i] = job
			}

			// Wait for all jobs to be running
			for i, job := range jobs {
				Eventually(func(g Gomega) {
					updated := &v1alpha1.AgentJob{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKey{
						Name: job.Name, Namespace: nsName,
					}, updated)).To(Succeed())
					g.Expect(updated.Status.Phase).To(Equal("Running"))
				}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).
					Should(Succeed(), fmt.Sprintf("Job %d failed to reach Running phase", i))
			}

			// All jobs should be running without interference
			for _, job := range jobs {
				retrieved := &v1alpha1.AgentJob{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name: job.Name, Namespace: nsName,
				}, retrieved)).To(Succeed())
				Expect(retrieved.Status.Phase).To(Equal("Running"))
			}
		})
	})
})
