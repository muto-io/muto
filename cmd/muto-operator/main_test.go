// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

// TestManagerServesHealthProbes guards the endpoints the Helm chart's
// liveness and readiness probes call. Without registered checks,
// controller-runtime does not mount them and the probes get a 404.
func TestManagerServesHealthProbes(t *testing.T) {
	probeAddr := freeAddr(t)

	// No controllers are registered, so the manager never contacts this API server.
	mgr, err := newManager(&rest.Config{Host: "http://127.0.0.1:1"}, "0", probeAddr)
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("manager exited with error: %v", err)
		}
	})

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			if got := getStatus(t, "http://"+probeAddr+path); got != http.StatusOK {
				t.Errorf("GET %s: status %d, want %d", path, got, http.StatusOK)
			}
		})
	}
}

// TestManagerServesMetrics guards the metrics bind address that
// MUTO_METRICS_BIND_ADDRESS (and, through it, the Helm chart's
// values.metrics.port) is supposed to control: newManager must bind the
// metrics server on the address it's given, not a hardcoded one.
func TestManagerServesMetrics(t *testing.T) {
	metricsAddr := freeAddr(t)

	// No controllers are registered, so the manager never contacts this API server.
	mgr, err := newManager(&rest.Config{Host: "http://127.0.0.1:1"}, metricsAddr, "0")
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("manager exited with error: %v", err)
		}
	})

	if got := getStatus(t, "http://"+metricsAddr+"/metrics"); got != http.StatusOK {
		t.Errorf("GET /metrics: status %d, want %d", got, http.StatusOK)
	}
}

// getStatus polls url until the server accepts connections and returns the
// HTTP status code of the first response.
func getStatus(t *testing.T, url string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			if err := resp.Body.Close(); err != nil {
				t.Fatalf("GET %s: closing body: %v", url, err)
			}
			return resp.StatusCode
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET %s: %v", url, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return addr
}
