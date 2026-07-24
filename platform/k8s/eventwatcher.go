package k8s

import (
	"context"

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
