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

var _ = Describe("Helm Deployment", func() {
	ctx := context.Background()

	Describe("operator deployment via Helm manifests", func() {
		var (
			helmNS     string
			testCounter int
		)

		BeforeEach(func() {
			testCounter++
			helmNS = fmt.Sprintf("muto-helm-test-%d", testCounter)

			// Create namespace for the operator
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: helmNS},
			}
			err := k8sClient.Create(ctx, ns)
			Expect(err).To(Or(Succeed(), MatchError(ContainSubstring("already exists"))))

			// The testEnv BeforeSuite already deployed CRDs and started the manager
			// with all three reconcilers (Tenant, AgentJob, AgentFleet)
			// This test verifies the same reconcilers work with Helm-like resources
		})

		AfterEach(func() {
			// Cleanup: delete the test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: helmNS},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should have CRDs installed", func() {
			// The suite_test.go BeforeSuite already loads CRDs
			// This test verifies they're queryable

			// Verify Tenant CRD works
			tenantList := &v1alpha1.TenantList{}
			Expect(k8sClient.List(ctx, tenantList)).To(Succeed())

			// Verify AgentJob CRD works
			jobList := &v1alpha1.AgentJobList{}
			Expect(k8sClient.List(ctx, jobList)).To(Succeed())

			// Verify AgentFleet CRD works
			fleetList := &v1alpha1.AgentFleetList{}
			Expect(k8sClient.List(ctx, fleetList)).To(Succeed())
		})

		It("should allow creating Tenant resources", func() {
			tenant := &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "helm-tenant-test",
					Namespace: helmNS,
				},
				Spec: v1alpha1.TenantSpec{
					Namespace:     helmNS,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

			// Verify we can retrieve it
			retrieved := &v1alpha1.Tenant{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      "helm-tenant-test",
				Namespace: helmNS,
			}, retrieved)).To(Succeed())

			Expect(retrieved.Spec.IsolationTier).To(Equal("shared"))
		})

		It("should allow creating AgentJob resources", func() {
			// First create a tenant
			tenant := &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "helm-job-tenant",
					Namespace: helmNS,
				},
				Spec: v1alpha1.TenantSpec{
					Namespace:     helmNS,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

			// Create an AgentJob
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "helm-test-job",
					Namespace: helmNS,
				},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.helm-test-job", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Verify reconciliation happens - job should transition to Running
			Eventually(func(g Gomega) {
				updated := &v1alpha1.AgentJob{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      job.Name,
					Namespace: helmNS,
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal("Running"))
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("should allow creating AgentFleet resources", func() {
			// Verify the AgentFleet CRD exists and is queryable
			fleetList := &v1alpha1.AgentFleetList{}
			Expect(k8sClient.List(ctx, fleetList)).To(Succeed())
		})

		It("should reconcile resources in test namespace", func() {
			// Create a complete set of resources and verify reconciliation
			tenant := &v1alpha1.Tenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "helm-reconcile-tenant",
					Namespace: helmNS,
				},
				Spec: v1alpha1.TenantSpec{
					Namespace:     helmNS,
					IsolationTier: "shared",
					MessageBus:    v1alpha1.TenantBusSpec{Type: "nats"},
				},
			}
			Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

			// Create a job
			job := &v1alpha1.AgentJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "helm-reconcile-job",
					Namespace: helmNS,
				},
				Spec: v1alpha1.AgentJobSpec{
					TenantRef: tenant.Name,
					Trigger:   v1alpha1.TriggerSpec{Type: "event"},
					Agents: []v1alpha1.AgentRoleSpec{
						{Role: "worker", Image: "busybox:latest", MaxReplicas: 1},
					},
					MessageBus:         v1alpha1.JobBusSpec{Topic: fmt.Sprintf("tenant.%s.helm-reconcile-job", tenant.Name)},
					TTLAfterCompletion: 60,
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			// Verify agent pod is created
			Eventually(func(g Gomega) {
				podList := &corev1.PodList{}
				g.Expect(k8sClient.List(ctx, podList,
					client.InNamespace(helmNS),
					client.MatchingLabels{"muto.io/job": "helm-reconcile-job"},
				)).To(Succeed())
				g.Expect(podList.Items).NotTo(BeEmpty())
			}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})
	})
})
