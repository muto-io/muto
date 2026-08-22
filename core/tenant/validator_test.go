package tenant_test

import (
	"testing"
	"github.com/muto-io/muto/core/tenant"
)

func TestTopicPrefix(t *testing.T) {
	cases := []struct {
		tenantID string
		want     string
	}{
		{"acme", "tenant.acme."},
		{"foo-bar", "tenant.foo-bar."},
	}
	for _, c := range cases {
		got := tenant.TopicPrefix(c.tenantID)
		if got != c.want {
			t.Errorf("TopicPrefix(%q) = %q, want %q", c.tenantID, got, c.want)
		}
	}
}

func TestValidateTopic(t *testing.T) {
	if err := tenant.ValidateTopic("acme", "tenant.acme.job.1"); err != nil {
		t.Errorf("expected no error: %v", err)
	}
	if err := tenant.ValidateTopic("acme", "tenant.other.job.1"); err == nil {
		t.Error("expected cross-tenant error")
	}
}

func TestValidateTenant(t *testing.T) {
	valid := tenant.Tenant{
		ID:            "acme",
		Namespace:     "acme-agents",
		IsolationTier: tenant.TierShared,
		BusType:       "nats",
	}
	if err := tenant.Validate(valid); err != nil {
		t.Errorf("expected valid tenant: %v", err)
	}

	invalid := tenant.Tenant{ID: ""}
	if err := tenant.Validate(invalid); err == nil {
		t.Error("expected error for empty ID")
	}
}
