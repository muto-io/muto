// SPDX-License-Identifier: Apache-2.0
package cf_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudfoundry/go-cfclient/v3/config"
	"github.com/muto-io/muto/platform/cf"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// newFakeCFAPIServer returns a test server that serves just enough of the CF
// API.
func newFakeCFAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"links": map[string]any{
				"login": map[string]any{"href": srv.URL},
				"uaa":   map[string]any{"href": srv.URL},
			},
		})
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "fake-access-token",
			"token_type":    "bearer",
			"refresh_token": "fake-refresh-token",
			"expires_in":    3600,
		})
	})
	return srv
}

func TestNewRealCFClientConstructsSuccessfully(t *testing.T) {
	srv := newFakeCFAPIServer(t)

	_, err := cf.NewRealCFClient(srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewRealCFClient: %v", err)
	}
}

func TestConfigHttpClientOptionCarriesOtelhttpTransport(t *testing.T) {
	srv := newFakeCFAPIServer(t)

	wrapped := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	cfg, err := config.New(srv.URL, config.UserPassword("user", "pass"),
		config.HttpClient(wrapped))
	if err != nil {
		t.Fatalf("config.New: %v", err)
	}
	if _, ok := cfg.HTTPClient().Transport.(*otelhttp.Transport); !ok {
		t.Errorf("cfg.HTTPClient().Transport = %T, want *otelhttp.Transport", cfg.HTTPClient().Transport)
	}
}

func TestNewRealCFClientRecordsOtelSpan(t *testing.T) {
	srv := newFakeCFAPIServer(t)

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prevProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevProvider)

	_, err := cf.NewRealCFClient(srv.URL, "user", "pass")
	if err != nil {
		t.Fatalf("NewRealCFClient: %v", err)
	}

	var foundClientSpan bool
	for _, s := range sr.Ended() {
		if s.SpanKind() == trace.SpanKindClient {
			foundClientSpan = true
			break
		}
	}
	if !foundClientSpan {
		t.Error("expected at least one client-kind span recorded via the otelhttp-wrapped transport during construction, got none - this would fail if client.go's Transport wrap were reverted")
	}
}
