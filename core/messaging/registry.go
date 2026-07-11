package messaging

import "fmt"

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
