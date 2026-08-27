// SPDX-License-Identifier: Apache-2.0
package reconcilers

import (
	"context"
	"fmt"
	"time"

	"github.com/muto-io/muto/core/a2a"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *AgentJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
}

func (r *AgentJobReconciler) reconcilePending(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	tenant := &v1alpha1.Tenant{}
	if err := r.Get(ctx, types.NamespacedName{Name: job.Spec.TenantRef}, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("get tenant %q: %w", job.Spec.TenantRef, err)
	}

	totalAgents := 0
	for _, roleSpec := range job.Spec.Agents {
		replicas := roleSpec.MaxReplicas
		if replicas == 0 {
			replicas = 1
		}
		totalAgents += int(replicas)

		for i := int32(0); i < replicas; i++ {
			pod := r.buildPod(job, roleSpec, tenant, i)
			if err := r.Create(ctx, pod); err != nil && !errors.IsAlreadyExists(err) {
				return ctrl.Result{}, fmt.Errorf("create pod: %w", err)
			}
		}
	}
	now := metav1.Now()
	job.Status.Phase = "Running"
	job.Status.ActiveAgents = int32(totalAgents)
	job.Status.StartedAt = &now
	return ctrl.Result{}, r.Status().Update(ctx, job)
}

func (r *AgentJobReconciler) reconcileRunning(ctx context.Context, job *v1alpha1.AgentJob) (ctrl.Result, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(job.Namespace),
		client.MatchingLabels{"muto.io/job": job.Name}); err != nil {
		return ctrl.Result{}, err
	}

	allDone, anyFailed := true, false
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			allDone = false
		}
		if pod.Status.Phase == corev1.PodFailed {
			anyFailed = true
		}
	}

	if !allDone {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	now := metav1.Now()
	job.Status.CompletedAt = &now
	job.Status.ActiveAgents = 0
	if anyFailed {
		job.Status.Phase = "Failed"
	} else {
		job.Status.Phase = "Succeeded"
	}
	return ctrl.Result{RequeueAfter: time.Duration(job.Spec.TTLAfterCompletion) * time.Second},
		r.Status().Update(ctx, job)
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
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(job.Namespace),
		client.MatchingLabels{"muto.io/job": job.Name}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list pods for terminating job: %w", err)
	}
	for i := range podList.Items {
		if err := r.Delete(ctx, &podList.Items[i]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete pod %s: %w", podList.Items[i].Name, err)
		}
	}
	if err := r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("delete agentjob: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *AgentJobReconciler) buildPod(
	job *v1alpha1.AgentJob,
	roleSpec v1alpha1.AgentRoleSpec,
	tenant *v1alpha1.Tenant,
	replicaIndex int32,
) *corev1.Pod {
	envVars := []corev1.EnvVar{
		{Name: "MUTO_TENANT", Value: job.Spec.TenantRef},
		{Name: "MUTO_ROLE", Value: roleSpec.Role},
		{Name: "MUTO_JOB_ID", Value: job.Name},
		{Name: "MUTO_MESSAGEBUS_TOPIC", Value: job.Spec.MessageBus.Topic},
	}
	if tenant.Spec.MessageBus.Type == a2a.BusTypeA2A {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "MUTO_A2A_GATEWAY",
				Value: "http://a2a-gateway." + job.Namespace + ".svc.cluster.local:8080",
			},
			corev1.EnvVar{
				Name: "MUTO_A2A_TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "muto-a2a-token"},
						Key:                  "token",
					},
				},
			},
		)
	}
	podName := fmt.Sprintf("%s-%s", job.Name, roleSpec.Role)
	replicas := roleSpec.MaxReplicas
	if replicas == 0 {
		replicas = 1
	}
	if replicas > 1 {
		podName = fmt.Sprintf("%s-%s-%d", job.Name, roleSpec.Role, replicaIndex)
	}

	container := corev1.Container{
		Name:  roleSpec.Role,
		Image: roleSpec.Image,
		Env:   envVars,
	}
	if roleSpec.Image == "busybox:latest" {
		container.Command = []string{"sh", "-c", "sleep 30"}
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: job.Namespace,
			Labels: map[string]string{
				"muto.io/tenant": job.Spec.TenantRef,
				"muto.io/job":    job.Name,
				"muto.io/role":   roleSpec.Role,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(job, v1alpha1.GroupVersion.WithKind("AgentJob")),
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{container},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
}

func (r *AgentJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentJob{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
