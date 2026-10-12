// SPDX-License-Identifier: Apache-2.0
package reconcilers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/platform/k8s/metrics"
	"github.com/muto-io/muto/platform/k8s/tracing"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentJobReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Adapter scheduler.PlatformAdapter
}

func (r *AgentJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return metrics.ObserveReconcile("agentjob", func() (ctrl.Result, error) {
		job := &v1alpha1.AgentJob{}
		if err := r.Get(ctx, req.NamespacedName, job); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		switch job.Status.Phase {
		case "", "Pending":
			return r.reconcilePending(ctx, job)
		case "Running":
			return r.reconcileRunning(ctx, job)
		case "Succeeded", "Failed":
			return r.reconcileTerminal(ctx, job)
		case "Terminating":
			return r.reconcileTerminating(ctx, job)
		}
		return ctrl.Result{}, nil
	})
}

func (r *AgentJobReconciler) reconcilePending(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	tenant := &v1alpha1.Tenant{}
	if err := r.Get(ctx, types.NamespacedName{Name: job.Spec.TenantRef}, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("get tenant %q: %w", job.Spec.TenantRef, err)
	}

	var agentIDs []string
	totalAgents := 0
	for _, roleSpec := range job.Spec.Agents {
		replicas := roleSpec.MaxReplicas
		if replicas == 0 {
			replicas = 1
		}
		totalAgents += int(replicas)

		for i := int32(0); i < replicas; i++ {
			spec := r.buildAgentSpec(job, roleSpec, tenant, replicas, i)
			id, err := r.Adapter.SpawnAgent(ctx, spec)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("spawn agent: %w", err)
			}
			agentIDs = append(agentIDs, id)
		}
	}
	now := metav1.Now()
	job.Status.Phase = "Running"
	job.Status.ActiveAgents = int32(totalAgents)
	job.Status.StartedAt = &now
	job.Status.AgentIDs = agentIDs
	if err := r.Status().Update(ctx, job); err != nil {
		return ctrl.Result{}, err
	}
	metrics.JobStarted(tenant.Name, int32(totalAgents))
	return ctrl.Result{}, nil
}

func (r *AgentJobReconciler) reconcileRunning(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	allDone, anyFailed := true, false
	for _, id := range job.Status.AgentIDs {
		ev, err := r.Adapter.AgentStatus(ctx, id)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("agent status %s: %w", id, err)
		}
		if ev == nil {
			allDone = false
			continue
		}
		if ev.Type == agent.EventFailed {
			anyFailed = true
		}
	}

	if !allDone {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	phase := "Succeeded"
	if anyFailed {
		phase = "Failed"
	}

	var startedAt time.Time
	if job.Status.StartedAt != nil {
		startedAt = job.Status.StartedAt.Time
	}
	activeAgents := job.Status.ActiveAgents

	now := metav1.Now()
	job.Status.CompletedAt = &now
	job.Status.ActiveAgents = 0
	job.Status.Phase = phase
	if err := r.Status().Update(ctx, job); err != nil {
		return ctrl.Result{}, err
	}

	metrics.JobFinished(job.Spec.TenantRef, strings.ToLower(phase), startedAt, activeAgents)
	return ctrl.Result{RequeueAfter: time.Duration(job.Spec.TTLAfterCompletion) * time.Second}, nil
}

func (r *AgentJobReconciler) reconcileTerminal(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	if job.Spec.TTLAfterCompletion <= 0 {
		return ctrl.Result{}, nil
	}
	if job.Status.CompletedAt == nil {
		return ctrl.Result{}, nil
	}
	elapsed := time.Since(job.Status.CompletedAt.Time)
	ttl := time.Duration(job.Spec.TTLAfterCompletion) * time.Second
	if elapsed < ttl {
		return ctrl.Result{RequeueAfter: ttl - elapsed}, nil
	}
	job.Status.Phase = "Terminating"
	return ctrl.Result{RequeueAfter: 0}, r.Status().Update(ctx, job)
}

func (r *AgentJobReconciler) reconcileTerminating(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	for _, id := range job.Status.AgentIDs {
		if err := r.Adapter.TerminateAgent(ctx, id); err != nil {
			return ctrl.Result{}, fmt.Errorf("terminate agent %s: %w", id, err)
		}
	}
	if err := r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete agentjob: %w", err)
	}
	return ctrl.Result{}, nil
}

// buildAgentSpec translates one AgentJob role/replica into the generic
// agent.Spec the PlatformAdapter understands. The pod-naming scheme matches
// what the reconciler used to build directly: a plain "<job>-<role>" name
// when there is a single replica, "<job>-<role>-<index>" when there are
// several.
func (r *AgentJobReconciler) buildAgentSpec(
	job *v1alpha1.AgentJob,
	roleSpec v1alpha1.AgentRoleSpec,
	tenant *v1alpha1.Tenant,
	replicas int32,
	replicaIndex int32,
) *agent.Spec {
	name := fmt.Sprintf("%s-%s", job.Name, roleSpec.Role)
	if replicas > 1 {
		name = fmt.Sprintf("%s-%s-%d", job.Name, roleSpec.Role, replicaIndex)
	}
	return &agent.Spec{
		TenantRef: job.Spec.TenantRef,
		Agents: []agent.AgentRole{{
			Role:        roleSpec.Role,
			Image:       roleSpec.Image,
			Command:     roleSpec.Command,
			MaxReplicas: roleSpec.MaxReplicas,
		}},
		Name:      name,
		Namespace: job.Namespace,
		Labels: map[string]string{
			"muto.io/tenant": job.Spec.TenantRef,
			"muto.io/job":    job.Name,
			"muto.io/role":   roleSpec.Role,
		},
		MessageBus: agent.MessageBusConfig{
			Topic: job.Spec.MessageBus.Topic,
			Type:  tenant.Spec.MessageBus.Type,
		},
		TTLAfterCompletion: job.Spec.TTLAfterCompletion,
	}
}

func (r *AgentJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentJob{}).
		Complete(tracing.WrapReconciler("AgentJobReconciler.Reconcile", r))
}
