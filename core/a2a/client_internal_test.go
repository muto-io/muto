// SPDX-License-Identifier: Apache-2.0
package a2a

import (
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func TestNewClientUsesOtelhttpTransport(t *testing.T) {
	c, err := New(&Config{GatewayURL: "http://example.invalid"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := c.httpClient.Transport.(*otelhttp.Transport); !ok {
		t.Errorf("httpClient.Transport = %T, want *otelhttp.Transport", c.httpClient.Transport)
	}
}
