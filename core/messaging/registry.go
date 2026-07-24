package messaging

import "fmt"

// constructors holds pub/sub MessageBus implementations registered via init().
// Registered backends: nats (core/messaging/nats), kafka (core/messaging/kafka).
//
// A2A is intentionally absent — it uses request/reply HTTP (core/a2a.A2AClient),
// not the pub/sub MessageBus interface, and is wired directly by the reconcilers.
var constructors = map[BusType]func(*Config) (MessageBus, error){}

func Register(t BusType, fn func(*Config) (MessageBus, error)) {
	constructors[t] = fn
}

func NewBus(t BusType, cfg *Config) (MessageBus, error) {
	fn, ok := constructors[t]
	if !ok {
		return nil, fmt.Errorf("unknown bus type %q; registered types: %v", t, registeredTypes())
	}
	return fn(cfg)
}

func registeredTypes() []string {
	keys := make([]string, 0, len(constructors))
	for k := range constructors {
		keys = append(keys, k)
	}
	return keys
}
