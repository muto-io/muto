//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("A2A Gateway Lifecycle", func() {
	ctx := context.Background()
	var testCounter int

	var (
		tenantName string
		tenantNS   string
		tenant     *v1alpha1.Tenant
	)

	BeforeEach(func() {
		testCounter++
		tenantName = fmt.Sprintf("a2a-tenant-%d", testCounter)
		tenantNS = fmt.Sprintf("a2a-ns-%d", testCounter)
	})

	AfterEach(func() {
		if tenant != nil {
			_ = k8sClient.Delete(ctx, tenant)
		}
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tenantNS}}
		_ = k8sClient.Delete(ctx, ns)
		Eventually(func() bool {
			n := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: tenantNS}, n)
			return err != nil
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(BeTrue())
	})

	It("provisions gateway Deployment, Service, and Secret for type:a2a dedicated tenant", func() {
		tenant = &v1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: tenantName},
			Spec: v1alpha1.TenantSpec{
				Namespace:     tenantNS,
				IsolationTier: "dedicated",
				MessageBus:    v1alpha1.TenantBusSpec{Type: "a2a", Dedicated: true},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		By("waiting for TenantStatus.Ready")
		Eventually(func(g Gomega) {
			t := &v1alpha1.Tenant{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tenantName}, t)).To(Succeed())
			g.Expect(t.Status.Ready).To(BeTrue())
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		By("checking Deployment exists")
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name: "a2a-gateway", Namespace: tenantNS,
		}, dep)).To(Succeed())

		By("checking Service exists on port 8080")
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name: "a2a-gateway", Namespace: tenantNS,
		}, svc)).To(Succeed())
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8080)))

		By("checking Secret exists with non-empty token")
		sec := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name: "muto-a2a-token", Namespace: tenantNS,
		}, sec)).To(Succeed())
		Expect(sec.Data["token"]).NotTo(BeEmpty())
	})

	It("injects MUTO_A2A_GATEWAY and MUTO_A2A_TOKEN env vars into AgentJob pods", func() {
		tenant = &v1alpha1.Tenant{
			ObjectMeta: metav1.ObjectMeta{Name: tenantName},
			Spec: v1alpha1.TenantSpec{
				Namespace:     tenantNS,
				IsolationTier: "dedicated",
				MessageBus:    v1alpha1.TenantBusSpec{Type: "a2a", Dedicated: true},
			},
		}
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())

		By("waiting for tenant ready")
		Eventually(func(g Gomega) {
			t := &v1alpha1.Tenant{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tenantName}, t)).To(Succeed())
			g.Expect(t.Status.Ready).To(BeTrue())
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		job := &v1alpha1.AgentJob{
			ObjectMeta: metav1.ObjectMeta{Name: "a2a-job", Namespace: tenantNS},
			Spec: v1alpha1.AgentJobSpec{
				TenantRef: tenantName,
				Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
			},
		}
		Expect(k8sClient.Create(ctx, job)).To(Succeed())

		By("waiting for pod to be created")
		Eventually(func(g Gomega) {
			podList := &corev1.PodList{}
			g.Expect(k8sClient.List(ctx, podList,
				client.InNamespace(tenantNS),
				client.MatchingLabels{"muto.io/job": "a2a-job"})).To(Succeed())
			g.Expect(podList.Items).NotTo(BeEmpty())
		}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		podList := &corev1.PodList{}
		Expect(k8sClient.List(ctx, podList,
			client.InNamespace(tenantNS),
			client.MatchingLabels{"muto.io/job": "a2a-job"})).To(Succeed())

		envMap := map[string]string{}
		secretRefMap := map[string]*corev1.SecretKeySelector{}
		for _, e := range podList.Items[0].Spec.Containers[0].Env {
			if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
				secretRefMap[e.Name] = e.ValueFrom.SecretKeyRef
			} else {
				envMap[e.Name] = e.Value
			}
		}
		Expect(envMap["MUTO_A2A_GATEWAY"]).To(Equal(
			"http://a2a-gateway." + tenantNS + ".svc.cluster.local:8080"))
		ref := secretRefMap["MUTO_A2A_TOKEN"]
		Expect(ref).NotTo(BeNil(), "expected MUTO_A2A_TOKEN to use valueFrom.secretKeyRef")
		Expect(ref.Name).To(Equal("muto-a2a-token"))
		Expect(ref.Key).To(Equal("token"))
	})
})
