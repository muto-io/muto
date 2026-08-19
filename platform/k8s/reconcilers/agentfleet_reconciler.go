package reconcilers

import (
	"context"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AgentFleetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *AgentFleetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	fleet := &v1alpha1.AgentFleet{}
	if err := r.Get(ctx, req.NamespacedName, fleet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var total, running, completed int32
	for _, jobRef := range fleet.Spec.JobRefs {
		job := &v1alpha1.AgentJob{}
		if err := r.Get(ctx, client.ObjectKey{Name: jobRef, Namespace: req.Namespace}, job); err != nil {
			continue
		}
		total++
		switch job.Status.Phase {
		case "Running":
			running++
		case "Succeeded", "Failed":
			completed++
		}
	}

	fleet.Status.TotalJobs = total
	fleet.Status.RunningJobs = running
	fleet.Status.CompletedJobs = completed
	return ctrl.Result{}, r.Status().Update(ctx, fleet)
}

func (r *AgentFleetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AgentFleet{}).
		Complete(r)
}
