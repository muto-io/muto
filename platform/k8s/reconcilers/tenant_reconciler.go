// SPDX-License-Identifier: Apache-2.0
package reconcilers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/go-logr/logr"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// tenantFinalizer is added to every Tenant so that gateway resources
// (Deployment, Service, Secret) provisioned in the tenant's namespace are
// explicitly cleaned up before the Tenant object is removed, rather than
// relying solely on Kubernetes garbage collection of owner-referenced
// objects. This makes teardown observable (via logs) and controllable
// (deletion blocks until cleanup succeeds).
const tenantFinalizer = "muto.io/tenant-cleanup"

type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("tenant", req.Name)

	tenant := &v1alpha1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !tenant.DeletionTimestamp.IsZero() {
		return r.finalizeTenant(ctx, tenant, logger)
	}

	if !controllerutil.ContainsFinalizer(tenant, tenantFinalizer) {
		logger.Info("adding tenant finalizer", "finalizer", tenantFinalizer)
		controllerutil.AddFinalizer(tenant, tenantFinalizer)
		if err := r.Update(ctx, tenant); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
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

// finalizeTenant runs when a Tenant has a DeletionTimestamp set. It
// explicitly deletes the gateway resources provisioned for the tenant
// (Deployment, Service, Secret) before removing the finalizer, allowing the
// Tenant object itself to be garbage collected. Deletion of each resource is
// logged for observability, and missing resources (already deleted, or never
// created because the tenant never opted into a dedicated gateway) are
// treated as success.
func (r *TenantReconciler) finalizeTenant(ctx context.Context, tenant *v1alpha1.Tenant, logger logr.Logger) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(tenant, tenantFinalizer) {
		// Finalizer already removed; nothing left to do.
		return ctrl.Result{}, nil
	}

	logger.Info("tenant marked for deletion, cleaning up gateway resources", "finalizer", tenantFinalizer)

	if err := r.deleteA2AGatewayResources(ctx, tenant.Spec.Namespace, logger); err != nil {
		logger.Error(err, "failed to clean up gateway resources for tenant")
		return ctrl.Result{}, fmt.Errorf("cleanup gateway resources: %w", err)
	}

	logger.Info("gateway resources cleaned up, removing tenant finalizer", "finalizer", tenantFinalizer)
	controllerutil.RemoveFinalizer(tenant, tenantFinalizer)
	if err := r.Update(ctx, tenant); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

// deleteA2AGatewayResources explicitly deletes the Deployment, Service, and
// Secret created by reconcileA2AGateway for the given tenant namespace. It is
// safe to call even if a dedicated A2A gateway was never provisioned for the
// tenant (each delete is a no-op when the resource does not exist).
func (r *TenantReconciler) deleteA2AGatewayResources(ctx context.Context, ns string, logger logr.Logger) error {
	if ns == "" {
		return nil
	}

	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "a2a-gateway", Namespace: ns}}
	if err := r.Delete(ctx, dep); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete a2a-gateway deployment: %w", err)
	} else if err == nil {
		logger.Info("deleted a2a-gateway deployment", "namespace", ns)
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "a2a-gateway", Namespace: ns}}
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete a2a-gateway service: %w", err)
	} else if err == nil {
		logger.Info("deleted a2a-gateway service", "namespace", ns)
	}

	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "muto-a2a-token", Namespace: ns}}
	if err := r.Delete(ctx, sec); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete muto-a2a-token secret: %w", err)
	} else if err == nil {
		logger.Info("deleted muto-a2a-token secret", "namespace", ns)
	}

	return nil
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
		return fmt.Errorf("MUTO_A2A_GATEWAY_IMAGE env var is required for A2A tenants but is not set")
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
					// TerminationGracePeriodSeconds is left at the Kubernetes default
					// (nil -> 30s) for production. Integration tests opt into a much
					// shorter grace period (see a2aGatewayTerminationGracePeriodSeconds)
					// to cut cleanup time, since graceful shutdown semantics aren't
					// under test and the pod is torn down at the end of every test run.
					TerminationGracePeriodSeconds: a2aGatewayTerminationGracePeriodSeconds(),
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

// a2aGatewayTerminationGracePeriodSeconds returns the pod termination grace
// period to use for the a2a-gateway Deployment. Production runs (muto-operator)
// never set MUTO_A2A_GATEWAY_TEST_GRACE_PERIOD, so this returns nil and the
// Deployment falls back to the standard Kubernetes default of 30 seconds.
//
// Test environments (see test/integration/k8s/suite_test.go) opt in by setting
// MUTO_A2A_GATEWAY_TEST_GRACE_PERIOD=true, reducing the grace period to 5
// seconds. This is safe there because the a2a-gateway test image does no
// meaningful graceful-shutdown work and tests do not depend on it — cutting
// the grace period from 30s to 5s saves 20-25s of pod termination time on
// every test namespace teardown. Production deployments are unaffected.
func a2aGatewayTerminationGracePeriodSeconds() *int64 {
	if os.Getenv("MUTO_A2A_GATEWAY_TEST_GRACE_PERIOD") != "true" {
		return nil
	}
	grace := int64(5)
	return &grace
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
