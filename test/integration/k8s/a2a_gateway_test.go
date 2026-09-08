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

// Cleanup timeout constants for the A2A Gateway Lifecycle test suite.
//
// This test is the only integration test in the suite that provisions a
// real, long-running Deployment (the a2a-gateway pod), so its AfterEach
// cleanup is slower and more variable than a plain Delete() — see
// docs/testing/a2a-gateway-cleanup.md for the full history and root-cause
// analysis. That history is a record of timeout *escalation* driven by
// guesswork: 30s->60s for the Tenant wait, then 60s->90s for the namespace
// wait, each time in response to an observed flake rather than a
// measurement. Recommendation #6 in that document calls for breaking that
// pattern by sizing timeouts from actual observed cleanup duration plus a
// margin, and documenting the reasoning inline so the next person to touch
// this file has data to check against instead of a reason to double a
// number again.
//
// Where the data comes from: AfterEach below logs the actual duration of
// each cleanup step via GinkgoWriter ("Tenant deletion: Xms / Ytimeout
// (ratio: Z%)" and the equivalent namespace line) — this was added as
// recommendation #2 (duration logging) specifically so timeout decisions
// could become measurement-based. To (re)validate or retune the constants
// below, run the suite against a real cluster (`make test-integration-k8s`
// or the k8s-e2e CI job) across several runs and collect those lines.
//
// The values below also reflect two mitigations already in place:
//   - The a2a-gateway pod's termination grace period is reduced to 5s in
//     test environments (see
//     TenantReconciler.a2aGatewayTerminationGracePeriodSeconds), down from
//     the Kubernetes default of 30s, so Tenant/pod cleanup is expected to
//     land in the low single-digit seconds under normal conditions rather
//     than the 20-30s implied by the old 30s default grace period.
//   - A force-delete fallback (nsForceDeleteGrace) forcibly removes any
//     pods still present partway through the namespace wait, capping the
//     worst case instead of passively hoping graceful termination finishes
//     before one large timeout expires.
//
// Expected ranges (initial estimates pending a larger sample of measured
// CI durations — see TODO below):
//
//	Tenant deletion:    typical ~1-5s,  est. p95 ~10-15s, timeout: 60s
//	Namespace deletion: typical ~1-5s,  est. p95 ~15-25s (bounded by the
//	                    20s force-delete grace plus its own teardown time),
//	                    timeout: 60s
//
// TODO(#59): once a statistically meaningful sample of the GinkgoWriter
// duration logs above has been collected from CI, replace the estimated
// ranges with observed p95 + ~20% margin, and note the sample size/date
// here — that keeps the next timeout change traceable to data rather than
// to "a flake happened."
const (
	// tenantCleanupTimeout bounds how long AfterEach waits for the Tenant
	// object (and, via foreground propagation, its owned
	// Deployment/Service/Secret) to be deleted.
	//
	// Unchanged at 60s from the prior escalation (30s -> 60s, commit
	// a253170). With the 5s test-only termination grace period now in
	// place, actual Tenant deletion is expected to be well under this —
	// the 60s ceiling is kept as a safety margin until CI duration data
	// confirms it can be tightened.
	tenantCleanupTimeout = 60 * time.Second

	// nsCleanupTimeout bounds how long AfterEach waits for the namespace
	// to be deleted after the Tenant is gone.
	//
	// Reduced from 90s to 60s now that nsForceDeleteGrace exists to unblock
	// stuck pod termination partway through the wait, rather than relying
	// on graceful shutdown alone to finish inside a single large timeout.
	nsCleanupTimeout = 60 * time.Second

	// nsForceDeleteGrace is how long graceful termination gets to finish
	// cleaning up namespace-scoped resources before AfterEach steps in and
	// force-deletes any pods still lingering (grace-period=0).
	//
	// Set well above the 5s test-only pod termination grace period so
	// graceful shutdown gets a real chance to complete first, but well
	// below nsCleanupTimeout so a stuck pod cannot consume the entire
	// namespace-deletion budget.
	nsForceDeleteGrace = 20 * time.Second
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
		// tenantCleanupTimeout, nsCleanupTimeout, and nsForceDeleteGrace are
		// defined as package-level constants above (with rationale and
		// expected-duration ranges) so they stay traceable to measured data
		// rather than being re-guessed here.

		// Expected cleanup sequence for a dedicated A2A tenant:
		//   1. Tenant deletion (foreground propagation) — the Tenant controller's
		//      finalizer tears down the gateway Deployment (including terminating
		//      its pod(s)), Service, and Secret before the Tenant object itself
		//      is removed.
		//   2. Namespace deletion (background propagation) — Kubernetes' own
		//      namespace controller cascades deletion of anything left in the
		//      namespace (e.g. AgentJob pods created during the test), with a
		//      force-delete fallback above if that takes too long.
		//
		// The two phases are timed independently below (and compared at the end)
		// so we can tell which one is the actual bottleneck rather than guessing.
		// This directly addresses recommendation #4 from the A2A Gateway cleanup
		// docs: is slow cleanup caused by the Tenant controller's own
		// reconciliation/finalizer logic, or by Kubernetes garbage-collecting the
		// namespace?
		cleanupStart := time.Now()

		// --- Phase 1/2: Tenant deletion ----------------------------------
		// Delete the Tenant explicitly first — this triggers cleanup of gateway resources
		By(fmt.Sprintf("[%s] Phase 1/2: deleting Tenant %q (timeout %s)", time.Now().Format(time.RFC3339Nano), tenantName, tenantCleanupTimeout))
		if tenant != nil {
			tenant := &v1alpha1.Tenant{ObjectMeta: metav1.ObjectMeta{Name: tenantName}}
			delOpts := []client.DeleteOption{
				client.PropagationPolicy(metav1.DeletePropagationForeground),
			}
			_ = k8sClient.Delete(ctx, tenant, delOpts...)
		}

		// Wait for Tenant to be deleted (with increased timeout for finalizers)
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

		// Wait for namespace to be deleted (allow more time for A2A resources).
		// If graceful termination hasn't finished within nsForceDeleteGrace,
		// force-delete any remaining pods (grace-period=0) to unblock the
		// namespace's terminating finalizers, then keep waiting for the
		// namespace to disappear within the overall nsCleanupTimeout.
		nsDeleteStart := time.Now()
		forceDeleteTriggered := false
		Eventually(func() bool {
			n := &corev1.Namespace{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: tenantNS}, n)
			if err != nil {
				return true // namespace gone
			}

			if !forceDeleteTriggered && time.Since(nsDeleteStart) >= nsForceDeleteGrace {
				forceDeleteTriggered = true
				GinkgoWriter.Printf(
					"[%s] force-delete triggered: namespace %q still terminating after grace period %s; force-deleting remaining pods\n",
					time.Now().Format(time.RFC3339Nano), tenantNS, nsForceDeleteGrace,
				)

				podList := &corev1.PodList{}
				if listErr := k8sClient.List(ctx, podList, client.InNamespace(tenantNS)); listErr == nil {
					forceOpts := []client.DeleteOption{
						client.GracePeriodSeconds(0),
					}
					for i := range podList.Items {
						pod := &podList.Items[i]
						GinkgoWriter.Printf(
							"[%s] force-deleting pod %q/%q (grace-period=0)\n",
							time.Now().Format(time.RFC3339Nano), pod.Namespace, pod.Name,
						)
						_ = k8sClient.Delete(ctx, pod, forceOpts...)
					}
				} else {
					GinkgoWriter.Printf(
						"[%s] force-delete: failed to list pods in namespace %q: %v\n",
						time.Now().Format(time.RFC3339Nano), tenantNS, listErr,
					)
				}
			}

			return false
		}).WithTimeout(nsCleanupTimeout).WithPolling(500 * time.Millisecond).Should(BeTrue())
		nsDeleteDuration := time.Since(nsDeleteStart)
		nsTimeoutRatio := float64(nsDeleteDuration) / float64(nsCleanupTimeout) * 100
		GinkgoWriter.Printf(
			"[%s] Namespace deletion: %dms / %s (ratio of timeout: %.1f%%, force-delete triggered: %t)\n",
			time.Now().Format(time.RFC3339Nano),
			nsDeleteDuration.Milliseconds(),
			nsCleanupTimeout,
			nsTimeoutRatio,
			forceDeleteTriggered,
		)
		By(fmt.Sprintf("[%s] Phase 2/2 complete: namespace deleted in %s (force-delete triggered: %t)", time.Now().Format(time.RFC3339Nano), nsDeleteDuration, forceDeleteTriggered))

		// --- Bottleneck analysis ------------------------------------------
		// Compare the two phases directly (independent of their differing
		// per-phase timeouts and independent of the timeout-based percentages
		// logged above) to identify which step actually dominates total
		// cleanup time, and by how much.
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
