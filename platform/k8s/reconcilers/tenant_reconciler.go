package reconcilers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/muto-io/muto/core/a2a"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
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

	switch tenant.Spec.MessageBus.Type {
	case a2a.BusTypeA2A:
		if tenant.Spec.MessageBus.Dedicated {
			if err := r.reconcileA2AGateway(ctx, tenant); err != nil {
				return ctrl.Result{}, fmt.Errorf("reconcile a2a gateway: %w", err)
			}
		}
	}

	tenant.Status.Ready = true
	if err := r.Status().Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TenantReconciler) reconcileA2AGateway(ctx context.Context, tenant *v1alpha1.Tenant) error {
	ns := tenant.Spec.Namespace
	ownerRef := *metav1.NewControllerRef(tenant, v1alpha1.GroupVersion.WithKind("Tenant"))

	if err := r.ensureA2ASecret(ctx, ns, ownerRef); err != nil {
		return err
	}
	if err := r.ensureA2ADeployment(ctx, ns, ownerRef); err != nil {
		return err
	}
	if err := r.ensureA2AService(ctx, ns, ownerRef); err != nil {
		return err
	}
	return nil
}

func (r *TenantReconciler) ensureA2ASecret(ctx context.Context, ns string, owner metav1.OwnerReference) error {
	sec := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: "muto-a2a-token", Namespace: ns}, sec)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	token, err := generateToken()
	if err != nil {
		return fmt.Errorf("generate a2a token: %w", err)
	}
	sec = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "muto-a2a-token",
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data: map[string][]byte{"token": []byte(token)},
	}
	return r.Create(ctx, sec)
}

func (r *TenantReconciler) ensureA2ADeployment(ctx context.Context, ns string, owner metav1.OwnerReference) error {
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: "a2a-gateway", Namespace: ns}, dep)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	image := os.Getenv("MUTO_A2A_GATEWAY_IMAGE")
	if image == "" {
		image = "ghcr.io/a2aprotocol/a2a-gateway:latest" // TBD: confirm real image
	}
	replicas := int32(1)
	dep = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "a2a-gateway",
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"muto.io/component": "a2a-gateway"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"muto.io/component": "a2a-gateway"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "a2a-gateway",
						Image: image,
						Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
					}},
				},
			},
		},
	}
	return r.Create(ctx, dep)
}

func (r *TenantReconciler) ensureA2AService(ctx context.Context, ns string, owner metav1.OwnerReference) error {
	svc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: "a2a-gateway", Namespace: ns}, svc)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	svc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "a2a-gateway",
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"muto.io/component": "a2a-gateway"},
			Ports: []corev1.ServicePort{{
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}
	return r.Create(ctx, svc)
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

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
