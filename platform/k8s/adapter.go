package k8s

import (
	"context"
	"fmt"

	"github.com/muto-io/muto/core/agent"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type K8sAdapter struct {
	client    client.Client
	namespace string
}

func NewK8sAdapter(c client.Client, namespace string) *K8sAdapter {
	return &K8sAdapter{client: c, namespace: namespace}
}

func (a *K8sAdapter) SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error) {
	if len(spec.Agents) == 0 {
		return "", fmt.Errorf("no agents in spec")
	}
	role := spec.Agents[0]
	podName := fmt.Sprintf("muto-%s-%s", spec.TenantRef, role.Role)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: a.namespace,
			Labels: map[string]string{
				"muto.io/tenant": spec.TenantRef,
				"muto.io/role":   role.Role,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  role.Role,
					Image: role.Image,
					Env: []corev1.EnvVar{
						{Name: "MUTO_TENANT", Value: spec.TenantRef},
						{Name: "MUTO_ROLE", Value: role.Role},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
	if err := a.client.Create(ctx, pod); err != nil {
		return "", fmt.Errorf("create pod: %w", err)
	}
	return podName, nil
}

func (a *K8sAdapter) TerminateAgent(ctx context.Context, agentID string) error {
	pod := &corev1.Pod{}
	pod.Name = agentID
	pod.Namespace = a.namespace
	return a.client.Delete(ctx, pod)
}

func (a *K8sAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 8)
	go func() {
		defer close(ch)
		pod := &corev1.Pod{}
		for {
			if err := a.client.Get(ctx, client.ObjectKey{Name: agentID, Namespace: a.namespace}, pod); err != nil {
				return
			}
			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				ch <- agent.Event{AgentID: agentID, Type: agent.EventCompleted}
				return
			case corev1.PodFailed:
				ch <- agent.Event{AgentID: agentID, Type: agent.EventFailed}
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return ch, nil
}
