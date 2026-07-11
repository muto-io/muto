package nats

import (
	"context"
	"fmt"
	"strings"

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
	conn *natsgo.Conn
	js   natsgo.JetStreamContext
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
	return &NATSBus{conn: conn, js: js}, nil
}

func (b *NATSBus) Publish(ctx context.Context, topic string, msg []byte) error {
	_, err := b.js.Publish(topic, msg)
	return err
}

func (b *NATSBus) Subscribe(ctx context.Context, topic string, handler agent.MsgHandler) error {
	_, err := b.js.Subscribe(topic, func(m *natsgo.Msg) {
		if err := handler(m.Subject, m.Data); err != nil {
			m.Nak()
			return
		}
		m.Ack()
	})
	return err
}

func (b *NATSBus) Close() error {
	b.conn.Close()
	return nil
}
