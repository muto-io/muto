// SPDX-License-Identifier: Apache-2.0
package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muto-io/muto/core/a2a"
	"github.com/muto-io/muto/core/agent"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	namespace := spec.Namespace
	if namespace == "" {
		namespace = a.namespace
	}
	podName := spec.Name
	if podName == "" {
		podName = fmt.Sprintf("muto-%s-%s", spec.TenantRef, role.Role)
	}
	labels := spec.Labels
	if labels == nil {
		labels = map[string]string{
			"muto.io/tenant": spec.TenantRef,
			"muto.io/role":   role.Role,
		}
	}

	envVars := []corev1.EnvVar{
		{Name: "MUTO_TENANT", Value: spec.TenantRef},
		{Name: "MUTO_ROLE", Value: role.Role},
	}
	if spec.MessageBus.Type == a2a.BusTypeA2A {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  "MUTO_A2A_GATEWAY",
				Value: fmt.Sprintf("http://a2a-gateway.%s.svc.cluster.local:8080", namespace),
			},
			corev1.EnvVar{
				Name: "MUTO_A2A_TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "muto-a2a-token"},
						Key:                  "token",
					},
				},
			},
		)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  role.Role,
					Image: role.Image,
					Env:   envVars,
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
	if err := a.client.Create(ctx, pod); err != nil && !errors.IsAlreadyExists(err) {
		return "", fmt.Errorf("create pod: %w", err)
	}
	return namespace + "/" + podName, nil
}

func (a *K8sAdapter) TerminateAgent(ctx context.Context, agentID string) error {
	namespace, name := a.splitAgentID(agentID)
	pod := &corev1.Pod{}
	pod.Name = name
	pod.Namespace = namespace
	if err := a.client.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func (a *K8sAdapter) AgentStatus(ctx context.Context, agentID string) (*agent.Event, error) {
	return a.checkPodStatus(ctx, agentID)
}

func (a *K8sAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 8)
	go func() {
		defer close(ch)
		for {
			ev, err := a.checkPodStatus(ctx, agentID)
			if err != nil {
				if ctx.Err() == nil {
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Second):
					}
					continue
				}
				return
			}
			if ev != nil {
				select {
				case ch <- *ev:
				case <-ctx.Done():
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	return ch, nil
}

// checkPodStatus fetches the pod identified by agentID and returns a
// terminal event once it has succeeded or failed (including having been
// deleted externally, which is treated as a failure), or nil while it is
// still active.
func (a *K8sAdapter) checkPodStatus(ctx context.Context, agentID string) (*agent.Event, error) {
	namespace, name := a.splitAgentID(agentID)
	pod := &corev1.Pod{}
	err := a.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			return &agent.Event{AgentID: agentID, Type: agent.EventFailed}, nil
		}
		return nil, err
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		return &agent.Event{AgentID: agentID, Type: agent.EventCompleted}, nil
	case corev1.PodFailed:
		return &agent.Event{AgentID: agentID, Type: agent.EventFailed}, nil
	}
	return nil, nil
}

// splitAgentID parses the "<namespace>/<name>" composite ID returned by
// SpawnAgent. An ID with no "/" falls back to the adapter's own namespace,
// for backward compatibility with any caller holding a pre-existing bare
// pod-name ID.
func (a *K8sAdapter) splitAgentID(agentID string) (namespace, name string) {
	if ns, n, ok := strings.Cut(agentID, "/"); ok {
		return ns, n
	}
	return a.namespace, agentID
}
