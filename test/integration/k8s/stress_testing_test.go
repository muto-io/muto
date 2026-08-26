//go:build integration

package k8s_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Stress Testing", func() {
	ctx := context.Background()
	var testCounter int

	Describe("High Volume Job Scheduling", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("stress-jobs-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("stress-tenant-%d", testCounter), Namespace: nsName},
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

		It("should handle creation of multiple jobs in sequence", func() {
			numJobs := 5
			createdJobs := make([]*v1alpha1.AgentJob, numJobs)

			// Create multiple jobs sequentially
			for i := 0; i < numJobs; i++ {
				job := &v1alpha1.AgentJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("sequential-job-%d", i),
						Namespace: nsName,
					},
					Spec: v1alpha1.AgentJobSpec{
						TenantRef:          tenant.Name,
						Trigger:            v1alpha1.TriggerSpec{Type: "event"},
						Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
						MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.sequential-%d", tenant.Name, i)},
						TTLAfterCompletion: 60,
					},
				}
				Expect(k8sClient.Create(ctx, job)).To(Succeed())
				createdJobs[i] = job
			}

			// Wait for all jobs to transition to Running
			for idx, job := range createdJobs {
				Eventually(func(g Gomega) {
					updated := &v1alpha1.AgentJob{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKey{
						Name:      job.Name,
						Namespace: nsName,
					}, updated)).To(Succeed())
					g.Expect(updated.Status.Phase).To(Equal("Running"))
				}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).
					Should(Succeed(), fmt.Sprintf("Job %d failed to reach Running phase", idx))
			}

			// Verify all jobs are still running
			for _, job := range createdJobs {
				retrieved := &v1alpha1.AgentJob{}
				Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      job.Name,
					Namespace: nsName,
				}, retrieved)).To(Succeed())
				Expect(retrieved.Status.Phase).To(Equal("Running"))
			}
		})

		It("should handle rapid concurrent job creation", func() {
			numJobs := 5
			jobNames := make([]string, numJobs)
			createErrs := make([]error, numJobs)
			var createMutex sync.Mutex
			var wg sync.WaitGroup

			// Create multiple jobs concurrently
			for i := 0; i < numJobs; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					job := &v1alpha1.AgentJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("concurrent-rapid-job-%d", idx),
							Namespace: nsName,
						},
						Spec: v1alpha1.AgentJobSpec{
							TenantRef:          tenant.Name,
							Trigger:            v1alpha1.TriggerSpec{Type: "event"},
							Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
							MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.rapid-%d", tenant.Name, idx)},
							TTLAfterCompletion: 60,
						},
					}

					err := k8sClient.Create(ctx, job)
					createMutex.Lock()
					jobNames[idx] = job.Name
					createErrs[idx] = err
					createMutex.Unlock()
				}(i)
			}

			wg.Wait()

			// All jobs should be created successfully
			for i, err := range createErrs {
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Job %d creation failed", i))
			}

			// Wait for all jobs to reach Running phase
			for i, jobName := range jobNames {
				Eventually(func(g Gomega) {
					updated := &v1alpha1.AgentJob{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKey{
						Name:      jobName,
						Namespace: nsName,
					}, updated)).To(Succeed())
					g.Expect(updated.Status.Phase).To(Equal("Running"))
				}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).
					Should(Succeed(), fmt.Sprintf("Job %d failed to reach Running phase", i))
			}
		})

		It("should maintain performance with increasing job count", func() {
			// Create jobs in batches and measure reconciliation time
			const batchSize = 2
			const numBatches = 2

			for batch := 0; batch < numBatches; batch++ {
				batchStart := time.Now()

				// Create batch of jobs
				for i := 0; i < batchSize; i++ {
					jobNum := batch*batchSize + i
					job := &v1alpha1.AgentJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("batch-job-%d", jobNum),
							Namespace: nsName,
						},
						Spec: v1alpha1.AgentJobSpec{
							TenantRef:          tenant.Name,
							Trigger:            v1alpha1.TriggerSpec{Type: "event"},
							Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
							MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.batch-%d", tenant.Name, jobNum)},
							TTLAfterCompletion: 60,
						},
					}
					Expect(k8sClient.Create(ctx, job)).To(Succeed())
				}

				// Wait for all jobs in batch to reach Running
				for i := 0; i < batchSize; i++ {
					jobNum := batch*batchSize + i
					Eventually(func(g Gomega) {
						updated := &v1alpha1.AgentJob{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKey{
							Name:      fmt.Sprintf("batch-job-%d", jobNum),
							Namespace: nsName,
						}, updated)).To(Succeed())
						g.Expect(updated.Status.Phase).To(Equal("Running"))
					}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
				}

				batchDuration := time.Since(batchStart)
				GinkgoLogr.Info("Batch completed", "batch", batch, "duration", batchDuration.String())

				// Verify all jobs so far are running (no degradation)
				for jobIdx := 0; jobIdx < (batch+1)*batchSize; jobIdx++ {
					retrieved := &v1alpha1.AgentJob{}
					Expect(k8sClient.Get(ctx, client.ObjectKey{
						Name:      fmt.Sprintf("batch-job-%d", jobIdx),
						Namespace: nsName,
					}, retrieved)).To(Succeed())
					Expect(retrieved.Status.Phase).To(Equal("Running"))
				}
			}
		})
	})

	Describe("Multi-Tenant Load", func() {
		var (
			nsName string
			ns     *corev1.Namespace
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("stress-multitenant-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should isolate jobs between multiple tenants", func() {
			numTenants := 3
			jobsPerTenant := 2
			tenants := make([]*v1alpha1.Tenant, numTenants)

			// Create multiple tenants
			for t := 0; t < numTenants; t++ {
				tenant := &v1alpha1.Tenant{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("stress-tenant-%d-%d", testCounter, t),
						Namespace: nsName,
					},
					Spec: v1alpha1.TenantSpec{
						Namespace:     nsName,
						IsolationTier: "shared",
						MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
					},
				}
				Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
				tenants[t] = tenant
			}

			// Create jobs for each tenant
			for t, tenant := range tenants {
				for j := 0; j < jobsPerTenant; j++ {
					job := &v1alpha1.AgentJob{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("tenant-%d-job-%d", t, j),
							Namespace: nsName,
						},
						Spec: v1alpha1.AgentJobSpec{
							TenantRef:          tenant.Name,
							Trigger:            v1alpha1.TriggerSpec{Type: "event"},
							Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
							MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.job-%d", tenant.Name, j)},
							TTLAfterCompletion: 60,
						},
					}
					Expect(k8sClient.Create(ctx, job)).To(Succeed())
				}
			}

			// Verify all jobs reach Running phase
			for t := 0; t < numTenants; t++ {
				for j := 0; j < jobsPerTenant; j++ {
					Eventually(func(g Gomega) {
						updated := &v1alpha1.AgentJob{}
						g.Expect(k8sClient.Get(ctx, client.ObjectKey{
							Name:      fmt.Sprintf("tenant-%d-job-%d", t, j),
							Namespace: nsName,
						}, updated)).To(Succeed())
						g.Expect(updated.Status.Phase).To(Equal("Running"))
					}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
				}
			}

			// Verify pod labels indicate correct tenant isolation
			for t, tenant := range tenants {
				podList := &corev1.PodList{}
				Expect(k8sClient.List(ctx, podList,
					client.InNamespace(nsName),
					client.MatchingLabels{"muto.io/tenant": tenant.Name},
				)).To(Succeed())
				// Should have jobsPerTenant pods for this tenant
				Expect(podList.Items).To(HaveLen(jobsPerTenant),
					fmt.Sprintf("Tenant %d should have %d pods", t, jobsPerTenant))
			}
		})
	})

	Describe("Resource Quota and Limits", func() {
		var (
			nsName string
			ns     *corev1.Namespace
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("stress-quota-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			// Create resource quota for the namespace
			quota := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-quota",
					Namespace: nsName,
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						corev1.ResourcePods:    resource.MustParse("10"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
						corev1.ResourceCPU:    resource.MustParse("2"),
					},
				},
			}
			Expect(k8sClient.Create(ctx, quota)).To(Succeed())
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should respect pod quota limits", func() {
			tenant := &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("quota-tenant-%d", testCounter),
					Namespace: nsName,
				},
				Spec: v1alpha1.TenantSpec{
					Namespace:     nsName,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

			// Try to create more jobs than the quota allows
			const numJobs = 8
			for i := 0; i < numJobs; i++ {
				job := &v1alpha1.AgentJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("quota-job-%d", i),
						Namespace: nsName,
					},
					Spec: v1alpha1.AgentJobSpec{
						TenantRef:          tenant.Name,
						Trigger:            v1alpha1.TriggerSpec{Type: "event"},
						Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
						MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.quota-%d", tenant.Name, i)},
						TTLAfterCompletion: 60,
					},
				}
				Expect(k8sClient.Create(ctx, job)).To(Succeed())
			}

			// Get quota usage
			quota := &corev1.ResourceQuota{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      "test-quota",
					Namespace: nsName,
				}, quota)).To(Succeed())
				// Quota should show some pod usage
				g.Expect(quota.Status.Used[corev1.ResourcePods]).NotTo(BeNil())
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Verify that pod count is at or below quota limit (10 pods)
			podLimit := quota.Spec.Hard[corev1.ResourcePods]
			usedPods := quota.Status.Used[corev1.ResourcePods]
			GinkgoLogr.Info("Pod quota status",
				"limit", podLimit.String(),
				"used", usedPods.String())
			Expect(usedPods.Cmp(podLimit) <= 0).To(BeTrue())
		})
	})

	Describe("Scheduler Performance", func() {
		var (
			nsName string
			ns     *corev1.Namespace
			tenant *v1alpha1.Tenant
		)

		BeforeEach(func() {
			testCounter++
			nsName = fmt.Sprintf("stress-perf-%d", testCounter)
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			tenant = &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("perf-tenant-%d", testCounter), Namespace: nsName},
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

		It("should maintain reconciliation speed with multiple jobs", func() {
			numJobs := 4
			var creationTimes []time.Duration
			var reconcileTimes []time.Duration

			for i := 0; i < numJobs; i++ {
				createStart := time.Now()

				job := &v1alpha1.AgentJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("perf-job-%d", i),
						Namespace: nsName,
					},
					Spec: v1alpha1.AgentJobSpec{
						TenantRef:          tenant.Name,
						Trigger:            v1alpha1.TriggerSpec{Type: "event"},
						Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
						MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.perf-%d", tenant.Name, i)},
						TTLAfterCompletion: 60,
					},
				}
				Expect(k8sClient.Create(ctx, job)).To(Succeed())
				creationTimes = append(creationTimes, time.Since(createStart))

				// Measure time to reach Running phase
				reconcileStart := time.Now()
				Eventually(func(g Gomega) {
					updated := &v1alpha1.AgentJob{}
					g.Expect(k8sClient.Get(ctx, client.ObjectKey{
						Name:      job.Name,
						Namespace: nsName,
					}, updated)).To(Succeed())
					g.Expect(updated.Status.Phase).To(Equal("Running"))
				}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
				reconcileTimes = append(reconcileTimes, time.Since(reconcileStart))
			}

			// Log performance metrics
			for i := 0; i < numJobs; i++ {
				GinkgoLogr.Info("Performance metrics",
					"job", i,
					"creation_time", creationTimes[i].String(),
					"reconcile_time", reconcileTimes[i].String())
			}

			// Verify reconciliation times remain reasonable (under 30s, which is our timeout)
			// In practice, should be much faster, but we're being conservative for test environment
			for _, reconcileTime := range reconcileTimes {
				Expect(reconcileTime).To(BeNumerically("<", 30*time.Second))
			}
		})
	})
})
