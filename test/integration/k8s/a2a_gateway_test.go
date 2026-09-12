//go:build integration

package k8s_test

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

// Cleanup ceilings for the A2A Gateway Lifecycle specs. A passing cleanup
// returns as soon as the object is gone, so these only matter when something is stuck.
// AfterEach logs the actual durations
const (
	tenantCleanupTimeout = 60 * time.Second
	nsCleanupTimeout     = 60 * time.Second
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
		// Delete the Tenant first so its muto.io/tenant-cleanup finalizer removes
		// the gateway resources, then the namespace. Both phases are timed and
		// logged separately to show which one dominates cleanup.
		cleanupStart := time.Now()

		// --- Phase 1/2: Tenant deletion ----------------------------------
		// Background propagation: the finalizer already deletes the gateway
		// resources, and foreground would only add a wait on the garbage collector.
		By(fmt.Sprintf("[%s] Phase 1/2: deleting Tenant %q (timeout %s)", time.Now().Format(time.RFC3339Nano), tenantName, tenantCleanupTimeout))
		if tenant != nil {
			tenant := &v1alpha1.Tenant{ObjectMeta: metav1.ObjectMeta{Name: tenantName}}
			delOpts := []client.DeleteOption{
				client.PropagationPolicy(metav1.DeletePropagationBackground),
			}
			_ = k8sClient.Delete(ctx, tenant, delOpts...)
		}

		// Wait for the Tenant finalizer to finish and the Tenant to be deleted
		tenantDeleteStart := time.Now()
		Eventually(func() bool {
			t := &v1alpha1.Tenant{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: tenantName}, t)
			return err != nil
		}).WithTimeout(tenantCleanupTimeout).WithPolling(500 * time.Millisecond).Should(BeTrue())
		tenantDeleteDuration := time.Since(tenantDeleteStart)
		tenantTimeoutRatio := float64(tenantDeleteDuration) / float64(tenantCleanupTimeout) * 100
		GinkgoWriter.Printf(
			"[%s] Tenant deletion: %dms / %s (ratio of timeout: %.1f%%)\n",
			time.Now().Format(time.RFC3339Nano),
			tenantDeleteDuration.Milliseconds(),
			tenantCleanupTimeout,
			tenantTimeoutRatio,
		)
		By(fmt.Sprintf("[%s] Phase 1/2 complete: Tenant deleted in %s", time.Now().Format(time.RFC3339Nano), tenantDeleteDuration))

		// --- Phase 2/2: namespace deletion --------------------------------
		// Now delete the namespace — cascade-delete will clean up remaining resources
		By(fmt.Sprintf("[%s] Phase 2/2: deleting namespace %q (timeout %s)", time.Now().Format(time.RFC3339Nano), tenantNS, nsCleanupTimeout))
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tenantNS}}
		delOpts := []client.DeleteOption{
			client.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		_ = k8sClient.Delete(ctx, ns, delOpts...)
		nsDeleteStart := time.Now()

		// Force-delete remaining pods (grace-period=0) right away instead of
		// waiting out their graceful shutdown, which is not under test here.
		// The namespace is already terminating, so no controller can recreate
		// them.
		podsForceDeleted := 0
		podList := &corev1.PodList{}
		if listErr := k8sClient.List(ctx, podList, client.InNamespace(tenantNS)); listErr == nil {
			for i := range podList.Items {
				pod := &podList.Items[i]
				GinkgoWriter.Printf(
					"[%s] force-deleting pod %q/%q (grace-period=0)\n",
					time.Now().Format(time.RFC3339Nano), pod.Namespace, pod.Name,
				)
				if delErr := k8sClient.Delete(ctx, pod, client.GracePeriodSeconds(0)); client.IgnoreNotFound(delErr) == nil {
					podsForceDeleted++
				}
			}
		} else {
			GinkgoWriter.Printf(
				"[%s] force-delete: failed to list pods in namespace %q: %v\n",
				time.Now().Format(time.RFC3339Nano), tenantNS, listErr,
			)
		}

		Eventually(func() bool {
			n := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: tenantNS}, n)
			return err != nil
		}).WithTimeout(nsCleanupTimeout).WithPolling(500 * time.Millisecond).Should(BeTrue())
		nsDeleteDuration := time.Since(nsDeleteStart)
		nsTimeoutRatio := float64(nsDeleteDuration) / float64(nsCleanupTimeout) * 100
		GinkgoWriter.Printf(
			"[%s] Namespace deletion: %dms / %s (ratio of timeout: %.1f%%, pods force-deleted: %d)\n",
			time.Now().Format(time.RFC3339Nano),
			nsDeleteDuration.Milliseconds(),
			nsCleanupTimeout,
			nsTimeoutRatio,
			podsForceDeleted,
		)
		By(fmt.Sprintf("[%s] Phase 2/2 complete: namespace deleted in %s (pods force-deleted: %d)", time.Now().Format(time.RFC3339Nano), nsDeleteDuration, podsForceDeleted))

		// --- Bottleneck analysis ------------------------------------------
		totalDuration := time.Since(cleanupStart)
		bottleneckStep := "Tenant deletion"
		bottleneckDuration, otherDuration := tenantDeleteDuration, nsDeleteDuration
		if nsDeleteDuration > tenantDeleteDuration {
			bottleneckStep = "namespace deletion"
			bottleneckDuration, otherDuration = nsDeleteDuration, tenantDeleteDuration
		}
		stepRatio := float64(bottleneckDuration)
		if otherDuration > 0 {
			stepRatio = float64(bottleneckDuration) / float64(otherDuration)
		}
		GinkgoWriter.Printf(
			"[%s] Cleanup bottleneck: %s (%dms) is %.2fx the other step (%dms); total cleanup: %dms\n",
			time.Now().Format(time.RFC3339Nano),
			bottleneckStep,
			bottleneckDuration.Milliseconds(),
			stepRatio,
			otherDuration.Milliseconds(),
			totalDuration.Milliseconds(),
		)
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
				Trigger:   v1alpha1.TriggerSpec{Type: "event"},
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
