//go:build integration

package k8s_test

import (
	"context"
	"time"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/mcp/tools"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MCP round-trip", func() {
	ctx := context.Background()

	It("schedules, checks status, and cancels a job", func() {
		adapter := k8sadapter.NewK8sAdapter(k8sClient, "default")
		sched := scheduler.NewDefaultScheduler(adapter)
		h := tools.NewHandlers(sched)

		Expect(h.ScheduleAgentJob(ctx, "mcp-test-job", "acme", "busybox:latest", "", 60)).
			To(Succeed())

		Eventually(func(g Gomega) {
			st, err := h.GetJobStatus(ctx, "mcp-test-job")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(st.Phase).To(Equal(agent.PhaseRunning))
		}).WithTimeout(10 * time.Second).WithPolling(200 * time.Millisecond).Should(Succeed())

		Expect(h.CancelJob(ctx, "mcp-test-job")).To(Succeed())

		Eventually(func(g Gomega) {
			st, err := h.GetJobStatus(ctx, "mcp-test-job")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(st.Phase).To(Equal(agent.PhaseTerminating))
		}).WithTimeout(5 * time.Second).WithPolling(200 * time.Millisecond).Should(Succeed())
	})
})
