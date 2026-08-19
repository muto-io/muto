package a2a

// BusTypeA2A is the string identifier for the A2A message bus type used in
// Tenant CRDs. A2A uses request/reply HTTP semantics (SendTask / GetTaskStatus)
// rather than the pub/sub MessageBus interface used by NATS and Kafka — it is
// therefore NOT registered with core/messaging.NewBus. Use core/a2a.New(cfg)
// directly to create an A2AClient.
const BusTypeA2A = "a2a"

// Config holds the coordinates needed to connect to an A2A gateway.
type Config struct {
	GatewayURL string
	AuthToken  string
}