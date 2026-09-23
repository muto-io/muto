// SPDX-License-Identifier: Apache-2.0
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var factory = promauto.With(ctrlmetrics.Registry)

var (
	// ReconciliationsTotal counts every Reconcile call, by reconciler and
	// outcome.
	ReconciliationsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "muto_reconciliations_total",
		Help: "Total number of reconcile calls, by reconciler and result.",
	}, []string{"reconciler", "result"})

	// ReconciliationDuration observes how long each Reconcile call takes.
	ReconciliationDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muto_reconciliation_duration_seconds",
		Help:    "Duration of reconcile calls in seconds, by reconciler.",
		Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10},
	}, []string{"reconciler"})

	// JobsTotal counts AgentJobs that reached a terminal state.
	JobsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "muto_jobs_total",
		Help: "Total number of AgentJobs that reached a terminal state, by tenant and status.",
	}, []string{"tenant", "status"})

	// JobDuration observes AgentJob wall-clock duration from start to
	// completion.
	JobDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muto_job_duration_seconds",
		Help:    "Duration of AgentJobs from start to completion in seconds, by tenant and status.",
		Buckets: []float64{1, 5, 15, 30, 60, 300, 900, 3600},
	}, []string{"tenant", "status"})

	// AgentsRunning is the current count of agent pods running, by tenant.
	AgentsRunning = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "muto_agents_running",
		Help: "Number of agent pods currently running, by tenant.",
	}, []string{"tenant"})

	// JobQueueDepth is the current count of AgentJobs in the Running
	// phase, by tenant. Despite the name, this does not include Pending
	// jobs (there is no separate queueing step in this reconciler - jobs
	// go straight from Pending to Running within one reconcile call).
	JobQueueDepth = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "muto_job_queue_depth",
		Help: "Number of AgentJobs currently in the Running phase, by tenant.",
	}, []string{"tenant"})
)

// ObserveReconcile runs fn, recording its duration and result on
// ReconciliationDuration and ReconciliationsTotal under the given
// reconciler label. result is "success" if fn returns a nil error, "error"
// otherwise.
func ObserveReconcile(reconciler string, fn func() (ctrl.Result, error)) (ctrl.Result, error) {
	start := time.Now()
	result, err := fn()
	ReconciliationDuration.WithLabelValues(reconciler).Observe(time.Since(start).Seconds())
	status := "success"
	if err != nil {
		status = "error"
	}
	ReconciliationsTotal.WithLabelValues(reconciler, status).Inc()
	return result, err
}

// JobStarted records that agentCount agent pods were just created for an
// AgentJob belonging to tenant.
func JobStarted(tenant string, agentCount int32) {
	AgentsRunning.WithLabelValues(tenant).Add(float64(agentCount))
	JobQueueDepth.WithLabelValues(tenant).Inc()
}

// JobFinished records that an AgentJob belonging to tenant reached a
// terminal state. status must be "succeeded" or "failed" (lowercase —
// distinct from the AgentJob CRD's own capitalized Phase field). startedAt
// and activeAgents are the job's values from immediately before this
// transition.
func JobFinished(tenant, status string, startedAt time.Time, activeAgents int32) {
	JobsTotal.WithLabelValues(tenant, status).Inc()
	JobDuration.WithLabelValues(tenant, status).Observe(time.Since(startedAt).Seconds())
	AgentsRunning.WithLabelValues(tenant).Sub(float64(activeAgents))
	JobQueueDepth.WithLabelValues(tenant).Dec()
}
