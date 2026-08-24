//go:build integration

package k8s_test

import (
	"context"
	"fmt"
	"time"
)

// K8sTestHelper provides common utilities for K8s e2e tests.
type K8sTestHelper struct {
	Counter int
}

// NewK8sTestHelper returns a new helper for K8s tests.
func NewK8sTestHelper() *K8sTestHelper {
	return &K8sTestHelper{Counter: 0}
}

// NextNamespace returns a unique namespace name for the current test.
func (h *K8sTestHelper) NextNamespace(prefix string) string {
	h.Counter++
	return fmt.Sprintf("%s-%d", prefix, h.Counter)
}

// WaitForJobPhase polls until the job reaches the desired phase or times out.
// Returns the phase reached or error.
func WaitForJobPhase(ctx context.Context, jobName, namespace, targetPhase string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for job %s/%s to reach phase %s", namespace, jobName, targetPhase)
}
