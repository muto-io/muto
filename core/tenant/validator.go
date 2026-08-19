package tenant

import (
	"fmt"
	"strings"
)

func TopicPrefix(tenantID string) string {
	return fmt.Sprintf("tenant.%s.", tenantID)
}

func ValidateTopic(tenantID, topic string) error {
	prefix := TopicPrefix(tenantID)
	if !strings.HasPrefix(topic, prefix) {
		return fmt.Errorf("topic %q does not belong to tenant %q (expected prefix %q)", topic, tenantID, prefix)
	}
	return nil
}

func Validate(t Tenant) error {
	if t.ID == "" {
		return fmt.Errorf("tenant ID must not be empty")
	}
	if t.Namespace == "" {
		return fmt.Errorf("tenant namespace must not be empty")
	}
	if t.IsolationTier != TierShared && t.IsolationTier != TierDedicated {
		return fmt.Errorf("unknown isolation tier %q", t.IsolationTier)
	}
	return nil
}
