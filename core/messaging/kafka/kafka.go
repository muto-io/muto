package kafka

import (
	"context"
	"fmt"
	"sync"

	"github.com/IBM/sarama"
	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/messaging"
)

func init() {
	messaging.Register(messaging.BusTypeKafka, func(cfg *messaging.Config) (messaging.MessageBus, error) {
		return New(cfg)
	})
}

type KafkaBus struct {
	producer       sarama.SyncProducer
	consumer       sarama.Consumer
	brokers        []string
	mu             sync.Mutex
	subscriptions  map[string]sarama.PartitionConsumer
}

func New(cfg *messaging.Config) (*KafkaBus, error) {
	scfg := sarama.NewConfig()
	scfg.Producer.Return.Successes = true
	if cfg.Username != "" {
		scfg.Net.SASL.Enable = true
		scfg.Net.SASL.User = cfg.Username
		scfg.Net.SASL.Password = cfg.Password
	}
	producer, err := sarama.NewSyncProducer(cfg.URLs, scfg)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: %w", err)
	}
	consumer, err := sarama.NewConsumer(cfg.URLs, scfg)
	if err != nil {
		_ = producer.Close()
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	return &KafkaBus{
		producer:      producer,
		consumer:      consumer,
		brokers:       cfg.URLs,
		subscriptions: make(map[string]sarama.PartitionConsumer),
	}, nil
}

func (b *KafkaBus) Publish(ctx context.Context, topic string, msg []byte) error {
	_, _, err := b.producer.SendMessage(&sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(msg),
	})
	return err
}

func (b *KafkaBus) Subscribe(ctx context.Context, topic string, handler agent.MsgHandler) error {
	pc, err := b.consumer.ConsumePartition(topic, 0, sarama.OffsetNewest)
	if err != nil {
		return fmt.Errorf("kafka subscribe: %w", err)
	}

	// Track partition consumer for cleanup
	key := fmt.Sprintf("%s-0", topic)
	b.mu.Lock()
	b.subscriptions[key] = pc
	b.mu.Unlock()

	go func() {
		defer func() {
			b.mu.Lock()
			delete(b.subscriptions, key)
			b.mu.Unlock()
		}()

		for {
			select {
			case msg, ok := <-pc.Messages():
				if !ok {
					return
				}
				_ = handler(msg.Topic, msg.Value)
			case <-ctx.Done():
				_ = pc.Close()
				return
			}
		}
	}()
	return nil
}

func (b *KafkaBus) Close() error {
	var errs []error

	// Close all partition consumers first
	b.mu.Lock()
	for _, pc := range b.subscriptions {
		if err := pc.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	b.subscriptions = make(map[string]sarama.PartitionConsumer)
	b.mu.Unlock()

	// Then close main consumer and producer
	if err := b.producer.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := b.consumer.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("kafka close: %v", errs)
	}
	return nil
}
