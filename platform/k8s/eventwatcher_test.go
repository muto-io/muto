package k8s

import (
	"context"
	"testing"
	"time"

	"github.com/muto-io/muto/core/agent"
)

type fakeEventBus struct {
	payload []byte
}

func newFakeEventBus(payload []byte) *fakeEventBus {
	return &fakeEventBus{payload: payload}
}

func (f *fakeEventBus) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (f *fakeEventBus) Subscribe(ctx context.Context, topic string, handler agent.MsgHandler) error {
	go func() {
		_ = handler(topic, f.payload)
	}()
	return nil
}

func (f *fakeEventBus) Close() error { return nil }

func TestEventWatcherCallsHandler(t *testing.T) {
	called := make(chan string, 1)
	handler := func(tenantID, topic string, data []byte) error {
		called <- tenantID
		return nil
	}

	bus := newFakeEventBus([]byte(`{"hello":"world"}`))
	watcher := NewEventWatcher(bus, "tenant.acme.tasks", "acme", handler)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = watcher.Start(ctx) }()

	select {
	case tenantID := <-called:
		if tenantID != "acme" {
			t.Errorf("expected acme, got %s", tenantID)
		}
	case <-ctx.Done():
		t.Error("handler not called within timeout")
	}
}
