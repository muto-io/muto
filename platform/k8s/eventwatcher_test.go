package k8s_test

import (
	"context"
	"testing"
	"time"

	k8sadapter "github.com/muto-io/muto/platform/k8s"
)

func TestEventWatcherCallsHandler(t *testing.T) {
	called := make(chan string, 1)
	handler := func(tenantID, topic string, data []byte) error {
		called <- tenantID
		return nil
	}

	bus := k8sadapter.NewFakeEventBus([]byte(`{"hello":"world"}`))
	watcher := k8sadapter.NewEventWatcher(bus, "tenant.acme.tasks", "acme", handler)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go watcher.Start(ctx)

	select {
	case tenantID := <-called:
		if tenantID != "acme" {
			t.Errorf("expected acme, got %s", tenantID)
		}
	case <-ctx.Done():
		t.Error("handler not called within timeout")
	}
}
