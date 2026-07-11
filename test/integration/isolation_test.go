//go:build integration

package integration_test

import (
	"testing"

	"github.com/muto-io/muto/core/tenant"
)

func TestCrossTenantTopicBlocked(t *testing.T) {
	topicA := tenant.TopicPrefix("tenant-a") + "job.1"
	if err := tenant.ValidateTopic("tenant-b", topicA); err == nil {
		t.Error("expected cross-tenant topic validation to fail")
	}
}

func TestSameTenantTopicAllowed(t *testing.T) {
	topic := tenant.TopicPrefix("acme") + "job.42"
	if err := tenant.ValidateTopic("acme", topic); err != nil {
		t.Errorf("expected same-tenant topic to pass: %v", err)
	}
}
