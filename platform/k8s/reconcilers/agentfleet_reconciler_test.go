// SPDX-License-Identifier: Apache-2.0
package reconcilers_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/platform/k8s/metrics"
	"github.com/muto-io/muto/platform/k8s/reconcilers"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAgentFleetReconcilerAggregatesJobCounts(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "job-1", Namespace: "ns"},
		Status:     v1alpha1.AgentJobStatus{Phase: "Running"},
	}
	fleet := &v1alpha1.AgentFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-1", Namespace: "ns"},
		Spec:       v1alpha1.AgentFleetSpec{JobRefs: []string{"job-1"}},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job, fleet).
		WithStatusSubresource(&v1alpha1.AgentFleet{}).Build()

	r := &reconcilers.AgentFleetReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "fleet-1", Namespace: "ns"},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated := &v1alpha1.AgentFleet{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "fleet-1", Namespace: "ns"}, updated)
	if updated.Status.RunningJobs != 1 {
		t.Errorf("RunningJobs = %d, want 1", updated.Status.RunningJobs)
	}
}

func TestAgentFleetReconcilerRecordsMetrics(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	fleet := &v1alpha1.AgentFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-2", Namespace: "ns"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet).
		WithStatusSubresource(&v1alpha1.AgentFleet{}).Build()
	r := &reconcilers.AgentFleetReconciler{Client: fakeClient, Scheme: scheme}

	before := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues("agentfleet", "success"))

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "fleet-2", Namespace: "ns"},
	})
	if err != nil {
		t.Fatal(err)
	}

	after := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues("agentfleet", "success"))
	if after != before+1 {
		t.Errorf("ReconciliationsTotal{agentfleet,success} = %v, want %v", after, before+1)
	}
}
