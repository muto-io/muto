//go:build integration

package integration_test

import (
	"context"
	"testing"

	k8sadapter "github.com/muto-io/muto/platform/k8s"
	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/mcp/tools"
)

func TestMCPRoundTrip(t *testing.T) {
	ctx := context.Background()
	adapter := k8sadapter.NewK8sAdapter(k8sClient, "default")
	sched := scheduler.NewDefaultScheduler(adapter)
	h := tools.NewHandlers(sched)

	if err := h.ScheduleAgentJob(ctx, "mcp-test-job", "acme", "busybox:latest", "", 60); err != nil {
		t.Fatal(err)
	}

	st, err := h.GetJobStatus(ctx, "mcp-test-job")
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase != agent.PhaseRunning {
		t.Errorf("expected Running, got %s", st.Phase)
	}

	if err := h.CancelJob(ctx, "mcp-test-job"); err != nil {
		t.Fatal(err)
	}

	st, _ = h.GetJobStatus(ctx, "mcp-test-job")
	if st.Phase != agent.PhaseTerminating {
		t.Errorf("expected Terminating after cancel, got %s", st.Phase)
	}
}
