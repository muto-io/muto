package kafka_test

import (
	"testing"
	"github.com/muto-io/muto/core/messaging"
	_ "github.com/muto-io/muto/core/messaging/kafka"
)

func TestKafkaRegistered(t *testing.T) {
	_, err := messaging.NewBus(messaging.BusTypeKafka, &messaging.Config{
		URLs: []string{"localhost:19092"},
	})
	if err != nil && len(err.Error()) >= 16 && err.Error()[:16] == "unknown bus type" {
		t.Errorf("kafka bus type not registered: %v", err)
	}
}
