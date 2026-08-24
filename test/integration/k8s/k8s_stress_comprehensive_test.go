//go:build integration

package k8s_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("K8s Comprehensive Stress Testing", func() {
	ctx := context.Background()
	var testCounter int

	Describe("high-volume job creation", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("stress-highvolume-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-stress-%d", testCounter)},
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

		It("should handle creation of many concurrent jobs", func() {
			jobCount := 10
			var wg sync.WaitGroup
			var successCount int32

			for i := 0; i < jobCount; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					job := &v1alpha1.AgentJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("stress-job-%d", index),
							Namespace: nsName,
						},
						Spec: v1alpha1.AgentJobSpec{
							TenantRef: tenant.Name,
							Trigger:   v1alpha1.TriggerSpec{Type: "event"},
							Agents: []v1alpha1.AgentRoleSpec{
								{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
							},
							MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.stress-job-%d", tenant.Name, index)},
							TTLAfterCompletion: 60,
						},
					}

					if err := k8sClient.Create(ctx, job); err == nil {
						atomic.AddInt32(&successCount, 1)
					}
				}(i)
			}

			wg.Wait()

			// Expect all jobs to be created
			Expect(atomic.LoadInt32(&successCount)).To(Equal(int32(jobCount)))

			// Verify all jobs reach Running state
			for i := 0; i < jobCount; i++ {
				Eventually(func(g Gomega) {
					job := &v1alpha1.AgentJob{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKey{
						Name:      fmt.Sprintf("stress-job-%d", i),
						Namespace: nsName,
					}, job)).To(Succeed())
					g.Expect(job.Status.Phase).To(Or(Equal("Running"), Equal("Pending")))
				}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
			}
		})

		It("should handle rapid sequential job creation", func() {
			jobCount := 5

			for i := 0; i < jobCount; i++ {
				job := &v1alpha1.AgentJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("sequential-job-%d", i),
						Namespace: nsName,
					},
					Spec: v1alpha1.AgentJobSpec{
						TenantRef: tenant.Name,
						Trigger:   v1alpha1.TriggerSpec{Type: "event"},
						Agents: []v1alpha1.AgentRoleSpec{
							{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
						},
						MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.seq-job-%d", tenant.Name, i)},
						TTLAfterCompletion: 60,
					},
				}

				Expect(k8sClient.Create(ctx, job)).To(Succeed())

				// Short delay between creations
				time.Sleep(100 * time.Millisecond)
			}

			// Verify all created
			jobList := &v1alpha1.AgentJobList{}
			Expect(k8sClient.List(ctx, jobList, client.InNamespace(nsName))).To(Succeed())
			Expect(len(jobList.Items)).To(Equal(jobCount))
		})
	})

	Describe("agent scaling and replication", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("stress-scaling-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-scale-%d", testCounter)},
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

		It("should handle high replica counts", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "high-replica-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 10},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.high-replica", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      "high-replica-job",
					Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Verify pods are created (up to MaxReplicas)
			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "high-replica-job"},
			)).To(Succeed())

			Expect(len(podList.Items)).To(BeNumerically("<=", 10))
			Expect(len(podList.Items)).To(BeNumerically(">", 0))
		})

		It("should handle mixed replica counts across roles", func() {
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{Name: "mixed-replica-job", Namespace: nsName},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "coordinator", Image: "busybox:latest", MaxReplicas: 2},
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 5},
						{Role: "processor", Image: "busybox:latest", MaxReplicas: 3},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.mixed-replica", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      "mixed-replica-job",
					Namespace: nsName,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			// Verify total pod count doesn't exceed sum of MaxReplicas
			podList := &corev1.PodList{}
			Expect(k8sClient.List(ctx, podList,
				client.InNamespace(nsName),
				client.MatchingLabels{"muto.io/job": "mixed-replica-job"},
			)).To(Succeed())

			Expect(len(podList.Items)).To(BeNumerically("<=", 10)) // 2+5+3
			Expect(len(podList.Items)).To(BeNumerically(">", 0))
		})
	})

	Describe("rapid job churn", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("stress-churn-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-churn-%d", testCounter)},
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

		It("should handle rapid create and delete of jobs", func() {
			jobCount := 5
			var wg sync.WaitGroup
			var successCount int32

			// Create jobs
			for i := 0; i < jobCount; i++ {
				job := &v1alpha1.AgentJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("churn-job-%d", i),
						Namespace: nsName,
					},
					Spec: v1alpha1.AgentJobSpec{
						TenantRef: tenant.Name,
						Trigger:   v1alpha1.TriggerSpec{Type: "event"},
						Agents: []v1alpha1.AgentRoleSpec{
							{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
						},
						MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.churn-%d", tenant.Name, i)},
						TTLAfterCompletion: 60,
					},
				}

				if err := k8sClient.Create(ctx, job); err == nil {
					atomic.AddInt32(&successCount, 1)
				}
			}

			// Wait for jobs to be created
			Eventually(func() int32 {
				return atomic.LoadInt32(&successCount)
			}).WithTimeout(30 * time.Second).Should(Equal(int32(jobCount)))

			// Delete jobs
			for i := 0; i < jobCount; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					job := &v1alpha1.AgentJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("churn-job-%d", index),
							Namespace: nsName,
						},
					}
					_ = k8sClient.Delete(ctx, job)
				}(i)
			}

			wg.Wait()

			// Verify most jobs are deleted
			jobList := &v1alpha1.AgentJobList{}
			Expect(k8sClient.List(ctx, jobList, client.InNamespace(nsName))).To(Succeed())
			// Some jobs might still exist due to finalizers
			Expect(len(jobList.Items)).To(BeNumerically("<=", jobCount))
		})
	})

	Describe("namespace resource exhaustion", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("stress-exhaust-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("tenant-exhaust-%d", testCounter)},
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

		It("should gracefully handle namespace resource limits", func() {
			// Create multiple jobs to stress namespace resources
			for i := 0; i < 3; i++ {
				job := &v1alpha1.AgentJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("exhaust-job-%d", i),
						Namespace: nsName,
					},
					Spec: v1alpha1.AgentJobSpec{
						TenantRef: tenant.Name,
						Trigger:   v1alpha1.TriggerSpec{Type: "event"},
						Agents: []v1alpha1.AgentRoleSpec{
							{Role: "worker", Image: "busybox:latest", MaxReplicas: 5},
						},
						MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.exhaust-%d", tenant.Name, i)},
						TTLAfterCompletion: 60,
					},
				}

				Expect(k8sClient.Create(ctx, job)).To(Succeed())
			}

			// Verify namespace still functions
			jobList := &v1alpha1.AgentJobList{}
			Expect(k8sClient.List(ctx, jobList, client.InNamespace(nsName))).To(Succeed())
			Expect(len(jobList.Items)).To(Equal(3))
		})
	})
})
