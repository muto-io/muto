package nats

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/messaging"
	natsgo "github.com/nats-io/nats.go"
)

func init() {
	messaging.Register(messaging.BusTypeNATS, func(cfg *messaging.Config) (messaging.MessageBus, error) {
		return New(cfg)
	})
}

type NATSBus struct {
	conn  *natsgo.Conn
	js    natsgo.JetStreamContext
	mu    sync.Mutex
	subs  map[string]*natsgo.Subscription
}

func New(cfg *messaging.Config) (*NATSBus, error) {
	url := strings.Join(cfg.URLs, ",")
	opts := []natsgo.Option{}
	if cfg.Username != "" {
		opts = append(opts, natsgo.UserInfo(cfg.Username, cfg.Password))
	}
	conn, err := natsgo.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("nats jetstream: %w", err)
	}
	return &NATSBus{
		conn: conn,
		js:   js,
		subs: make(map[string]*natsgo.Subscription),
	}, nil
}

func (b *NATSBus) Publish(ctx context.Context, topic string, msg []byte) error {
	_, err := b.js.Publish(topic, msg)
	return err
}

func (b *NATSBus) Subscribe(ctx context.Context, topic string, handler agent.MsgHandler) error {
	sub, err := b.js.Subscribe(topic, func(m *natsgo.Msg) {
		if err := handler(m.Subject, m.Data); err != nil {
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	})
	if err != nil {
		return err
	}

	// Track subscription for cleanup
	b.mu.Lock()
	b.subs[topic] = sub
	b.mu.Unlock()

	return nil
}

func (b *NATSBus) Close() error {
	// Unsubscribe all subscriptions first
	b.mu.Lock()
	for _, sub := range b.subs {
		_ = sub.Unsubscribe()
	}
	b.subs = make(map[string]*natsgo.Subscription)
	b.mu.Unlock()

	// Then close connection
	b.conn.Close()
	return nil
}
