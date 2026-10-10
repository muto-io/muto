//go:build integration

package k8s_test

import (
	"context"
	"fmt"

	"github.com/muto-io/muto/core/scheduler"
	coretracing "github.com/muto-io/muto/core/tracing"
	"github.com/muto-io/muto/mcp/tools"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// findSpan returns the first ended span named name, failing the test if
// none is found.
func findSpan(g Gomega, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	for _, s := range spans {
		if s.Name() == name {
			return s
		}
	}
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name()
	}
	g.Expect(names).To(ContainElement(name), "span %q was not recorded", name)
	return nil
}

var _ = Describe("Tracing", func() {
	ctx := context.Background()

	It("produces one trace across the MCP tool call, the scheduler, and the platform adapter", func() {
		// Install a span recorder as the global TracerProvider for the
		// duration of this test. tracing.Wrap/WrapErr look up
		// otel.Tracer(...) fresh on every call (see their doc comments), so
		// this takes effect immediately for the wrapped scheduler/adapter
		// built below, without needing to restart anything.
		sr := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
		prev := otel.GetTracerProvider()
		otel.SetTracerProvider(tp)
		defer otel.SetTracerProvider(prev)

		// Mirrors the real wiring in cmd/muto-mcp/main.go: the K8s
		// platform adapter and the scheduler are both traced, and the MCP
		// server wraps each tool call in its own root span
		// (mcp/server.WrapToolHandler) - reproduced here via WrapErr since
		// this test drives mcp/tools.Handlers directly rather than going
		// through the mcp-go protocol layer, same shortcut
		// mcp_roundtrip_test.go takes.
		adapter := coretracing.WrapPlatformAdapter(k8sadapter.NewK8sAdapter(k8sClient, "default"))
		sched := coretracing.WrapScheduler(scheduler.NewDefaultScheduler(adapter))
		h := tools.NewHandlers(sched)

		jobID := fmt.Sprintf("tracing-test-job-%d", GinkgoParallelProcess())
		err := coretracing.WrapErr(ctx, "mcp.schedule_agent_job", func(ctx context.Context) error {
			return h.ScheduleAgentJob(ctx, jobID, "acme", "busybox:latest", "", 60)
		})
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			_ = sched.Cancel(context.Background(), jobID)
		})

		spans := sr.Ended()
		Expect(len(spans)).To(BeNumerically(">=", 3), "expected at least a root, Schedule, and SpawnAgent span")

		root := findSpan(Default, spans, "mcp.schedule_agent_job")
		scheduleSpan := findSpan(Default, spans, "Scheduler.Schedule")
		spawnSpan := findSpan(Default, spans, "PlatformAdapter.SpawnAgent")

		traceID := root.SpanContext().TraceID()
		Expect(scheduleSpan.SpanContext().TraceID()).To(Equal(traceID),
			"Scheduler.Schedule should be part of the same trace as the MCP tool call")
		Expect(spawnSpan.SpanContext().TraceID()).To(Equal(traceID),
			"PlatformAdapter.SpawnAgent should be part of the same trace as the MCP tool call")

		Expect(scheduleSpan.Parent().SpanID()).To(Equal(root.SpanContext().SpanID()),
			"Scheduler.Schedule should be a direct child of the MCP tool call span")
		Expect(spawnSpan.Parent().SpanID()).To(Equal(scheduleSpan.SpanContext().SpanID()),
			"PlatformAdapter.SpawnAgent should be a direct child of Scheduler.Schedule")

		// PlatformAdapter.WatchAgent runs against a detached
		// context.Background() (see DefaultScheduler.Schedule's comment:
		// "watch goroutines must outlive the request context"), so it is
		// never part of this trace. This is the one gap noted in
		// docs/operations/monitoring-observability.md's Known Limitations:
		// the watch loop that observes a job's outcome isn't causally
		// linked back to the call that scheduled it.
		for _, s := range spans {
			if s.Name() == "PlatformAdapter.WatchAgent" {
				Expect(s.SpanContext().TraceID()).NotTo(Equal(traceID),
					"PlatformAdapter.WatchAgent is expected to start its own trace, not join the scheduling one")
			}
		}
	})
})
