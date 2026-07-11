package messaging

import (
	"context"
	"github.com/muto-io/muto/core/agent"
)

type BusType = string

const (
	BusTypeNATS  BusType = "nats"
	BusTypeKafka BusType = "kafka"
)

type MessageBus interface {
	Publish(ctx context.Context, topic string, msg []byte) error
	Subscribe(ctx context.Context, topic string, handler agent.MsgHandler) error
	Close() error
}

type Config struct {
	URLs     []string
	Username string
	Password string
}
