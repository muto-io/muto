package messaging_test

import (
	"testing"
	"github.com/muto-io/muto/core/messaging"
)

func TestNewBusUnknownType(t *testing.T) {
	_, err := messaging.NewBus("unknown", nil)
	if err == nil {
		t.Error("expected error for unknown bus type")
	}
}

func TestBusTypeConstants(t *testing.T) {
	if messaging.BusTypeNATS != "nats" {
		t.Errorf("expected nats, got %s", messaging.BusTypeNATS)
	}
	if messaging.BusTypeKafka != "kafka" {
		t.Errorf("expected kafka, got %s", messaging.BusTypeKafka)
	}
}
