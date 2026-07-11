package k8s

import (
	"context"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/messaging"
)

type EventHandler func(tenantID, topic string, data []byte) error

type EventWatcher struct {
	bus      messaging.MessageBus
	topic    string
	tenantID string
	handler  EventHandler
}

func NewEventWatcher(bus messaging.MessageBus, topic, tenantID string, handler EventHandler) *EventWatcher {
	return &EventWatcher{bus: bus, topic: topic, tenantID: tenantID, handler: handler}
}

func (w *EventWatcher) Start(ctx context.Context) error {
	return w.bus.Subscribe(ctx, w.topic, func(topic string, data []byte) error {
		return w.handler(w.tenantID, topic, data)
	})
}

// FakeEventBus is a test double that immediately delivers one message on Subscribe.
type FakeEventBus struct {
	payload []byte
}

func NewFakeEventBus(payload []byte) *FakeEventBus {
	return &FakeEventBus{payload: payload}
}

func (f *FakeEventBus) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (f *FakeEventBus) Subscribe(ctx context.Context, topic string, handler agent.MsgHandler) error {
	go func() {
		_ = handler(topic, f.payload)
	}()
	return nil
}

func (f *FakeEventBus) Close() error { return nil }
