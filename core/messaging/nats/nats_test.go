package nats_test

import (
	"testing"
	"github.com/muto-io/muto/core/messaging"
	_ "github.com/muto-io/muto/core/messaging/nats"
)

func TestNATSRegistered(t *testing.T) {
	_, err := messaging.NewBus(messaging.BusTypeNATS, &messaging.Config{
		URLs: []string{"nats://localhost:14222"},
	})
	if err != nil && len(err.Error()) >= 16 && err.Error()[:16] == "unknown bus type" {
		t.Errorf("nats bus type not registered: %v", err)
	}
}
