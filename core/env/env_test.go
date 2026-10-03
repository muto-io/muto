// SPDX-License-Identifier: Apache-2.0
package env

import "testing"

func TestOrDefault(t *testing.T) {
	t.Run("unset returns fallback", func(t *testing.T) {
		t.Setenv("MUTO_TEST_VAR", "")
		if got := OrDefault("MUTO_TEST_VAR", "fallback"); got != "fallback" {
			t.Errorf("OrDefault = %q, want %q", got, "fallback")
		}
	})

	t.Run("set to a regular value returns that value", func(t *testing.T) {
		t.Setenv("MUTO_TEST_VAR", "custom")
		if got := OrDefault("MUTO_TEST_VAR", "fallback"); got != "custom" {
			t.Errorf("OrDefault = %q, want %q", got, "custom")
		}
	})

	// Regression guard for MUTO_METRICS_BIND_ADDRESS=0, which disables the
	// metrics server (see cmd/muto-operator/main.go's newManager). "0" must
	// pass through unchanged rather than being treated as empty/falsy and
	// overridden by the default bind address.
	t.Run("set to zero passes through unchanged", func(t *testing.T) {
		t.Setenv("MUTO_TEST_VAR", "0")
		if got := OrDefault("MUTO_TEST_VAR", ":8080"); got != "0" {
			t.Errorf("OrDefault = %q, want %q", got, "0")
		}
	})
}
