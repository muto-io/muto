package reconcilers_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAgentJobReconcilerPendingToRunning(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-1", Namespace: "acme-agents"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "acme",
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "img:1", MaxReplicas: 1}},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).
		WithStatusSubresource(&v1alpha1.AgentJob{}).Build()

	r := &reconcilers.AgentJobReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "job-1", Namespace: "acme-agents"},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated := &v1alpha1.AgentJob{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "job-1", Namespace: "acme-agents"}, updated)
	if updated.Status.Phase != "Running" {
		t.Errorf("expected Running, got %q", updated.Status.Phase)
	}
}
