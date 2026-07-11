package k8s_test

import (
	"context"
	"testing"

	"github.com/muto-io/muto/core/agent"
	k8sadapter "github.com/muto-io/muto/platform/k8s"
	v1alpha1 "github.com/muto-io/muto/platform/k8s/types/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSpawnAgentCreatesPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	adapter := k8sadapter.NewK8sAdapter(fakeClient, "acme-agents")
	spec := &agent.Spec{
		TenantRef: "acme",
		Agents:    []agent.AgentRole{{Role: "worker", Image: "worker:latest", MaxReplicas: 1}},
	}
	id, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("expected non-empty agent ID")
	}
}
