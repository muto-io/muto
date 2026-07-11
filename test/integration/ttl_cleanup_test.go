//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTTLCleanup(t *testing.T) {
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ttl-test"}}
	_ = k8sClient.Create(ctx, ns)
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, ns) })

	job := &v1alpha1.AgentJob{
		ObjectMeta: metav1.ObjectMeta{Name: "ttl-job", Namespace: "ttl-test"},
		Spec: v1alpha1.AgentJobSpec{
			TenantRef:          "test",
			Trigger:            v1alpha1.TriggerSpec{Type: "manual"},
			Agents:             []v1alpha1.AgentRoleSpec{{Role: "worker", Image: "busybox:latest", MaxReplicas: 1}},
			TTLAfterCompletion: 2,
		},
	}
	if err := k8sClient.Create(ctx, job); err != nil {
		t.Fatal(err)
	}

	time.Sleep(500 * time.Millisecond)
	now := metav1.Now()
	job.Status.Phase = "Succeeded"
	job.Status.CompletedAt = &now
	_ = k8sClient.Status().Update(ctx, job)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		check := &v1alpha1.AgentJob{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: "ttl-job", Namespace: "ttl-test"}, check)
		if errors.IsNotFound(err) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Error("AgentJob not deleted after TTL expiry")
}
