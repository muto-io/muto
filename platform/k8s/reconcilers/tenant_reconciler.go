package reconcilers

import (
	"context"
	"fmt"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	tenant := &v1alpha1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.ensureNamespace(ctx, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure namespace: %w", err)
	}

	tenant.Status.Ready = true
	if err := r.Status().Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TenantReconciler) ensureNamespace(ctx context.Context, tenant *v1alpha1.Tenant) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: tenant.Spec.Namespace}, ns)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tenant.Spec.Namespace,
			Labels: map[string]string{
				"muto.io/tenant": tenant.Name,
			},
		},
	}
	return r.Create(ctx, ns)
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Tenant{}).
		Complete(r)
}
