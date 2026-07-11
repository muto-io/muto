//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAgentJobLifecycle(t *testing.T) {
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "lifecycle-test"}}
	_ = k8sClient.Create(ctx, ns)
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, ns) })

	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "lifecycle-job", Namespace: "lifecycle-test"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef: "test",
			Trigger:   v1alpha1.TriggerSpec{Type: "event"},
			Agents:    []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
			MessageBus: v1alpha1.JobBusSpec{Topic: "tenant.test.lifecycle-job"},
			TTLAfterCompletion: 5,
		},
	}
	if err := k8sClient.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, job) })

	waitForPhase(t, ctx, "lifecycle-job", "lifecycle-test", "Running", 30*time.Second)

	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList, client.InNamespace("lifecycle-test"),
		client.MatchingLabels{"muto.io/job": "lifecycle-job"}); err != nil {
		t.Fatal(err)
	}
	if len(podList.Items) == 0 {
		t.Error("expected at least one pod")
	}
}
