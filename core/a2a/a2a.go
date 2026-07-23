package a2a

import "github.com/muto-io/muto/core/messaging"

// BusTypeA2A is the bus type string for the A2A protocol.
// It uses messaging.BusType for consistency with nats and kafka constants.
const BusTypeA2A messaging.BusType = "a2a"

// Config holds the coordinates needed to connect to an A2A gateway.
type Config struct {
	GatewayURL string
	AuthToken  string
}