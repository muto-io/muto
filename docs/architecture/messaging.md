# Messaging: Inter-Agent Communication

Muto provides a message bus abstraction that enables agents to communicate reliably. Agents publish and subscribe to topics, with messages routed by the message bus implementation (NATS, Kafka, or custom).

## Messaging Architecture

```
┌────────────────────────────────────────────────────────────┐
│ Agent Job Execution                                        │
│                                                            │
│ Agent A          Message Bus         Agent B               │
│  (Running)      (NATS/Kafka)          (Running)           │
│    │                 │                   │                │
│    ├─ Process  ┌─────┴─────┐  Listen ├──┘                │
│    │  input    │ Topic:    │  for                          │
│    │           │ tenant-a/ │  messages                     │
│    └─────────► │ workflow  │ ─────┬──────────────┐        │
│               │ /a-done   │      │               │        │
│               └───────────┘      ▼               │        │
│                               ┌────────────┐    │        │
│                               │ Agent B    │    │        │
│                               │ receives   │    │        │
│                               │ notification    │        │
│                               │ Starts     │    │        │
│                               │ processing │    │        │
│                               └──────┬─────┘    │        │
│                                      │         │        │
│                               Publishes──────────┘        │
│                               results to topic           │
│                               tenant-a/                  │
│                               workflow/b-done            │
└────────────────────────────────────────────────────────────┘
```

## MessageBus Interface

All message bus implementations follow this interface:

<<<<<<< HEAD
=======
```go
type MessageBus interface {
    // Publish a message to a topic
    Publish(ctx context.Context, topic string, message *Message) error
    
    // Subscribe to a topic, receive messages via callback
    Subscribe(ctx context.Context, topic string, callback MessageCallback) error
    
    // Unsubscribe from a topic
    Unsubscribe(ctx context.Context, topic string) error
    
    // Health check
    HealthCheck(ctx context.Context) error
    
    // Close connection
    Close() error
}

type Message struct {
    Topic     string            // Topic name
    ID        string            // Unique message ID
    Timestamp time.Time         // When published
    Tenant    string            // Tenant ID for scoping
    Headers   map[string]string // Metadata
    Payload   []byte            // Message body (usually JSON)
}

type MessageCallback func(ctx context.Context, msg *Message) error
```
>>>>>>> a53175d (docs: write architecture/messaging.md - message bus abstraction and implementations)

## Topic Naming Convention

Topics are hierarchically scoped by tenant:

```
tenant-a/workflow/stage-complete
├─ tenant-a: Tenant ID (first segment for isolation)
├─ workflow: Domain/category
└─ stage-complete: Specific event
```

Examples:
```
tenant-a/data-pipeline/extracted
tenant-a/notifications/job-completed
tenant-a/errors/extraction-failed

tenant-b/models/training-done
tenant-b/metrics/updated
```

**Isolation:** Muto enforces that tenants can only publish/subscribe to their own topics (`tenant-<id>/*`).

## Message Format

Messages use JSON for structure:

```json
{
  "jobId": "k8s-default-data-pipeline-0",
  "jobName": "data-pipeline",
  "tenant": "tenant-a",
  "timestamp": "2026-09-03T10:30:45Z",
  "eventType": "extraction-complete",
  "data": {
    "recordsProcessed": 1500,
    "outputLocation": "s3://bucket/results/2026-09-03/extraction",
    "duration": "1m23s",
    "checksum": "abc123def456"
  },
  "metadata": {
    "correlationId": "req-12345",
    "sourceAgent": "extractor",
    "nextAgent": "transformer"
  }
}
```

## NATS Implementation

NATS is the default message bus for simplicity and performance.

### Architecture

```
Muto Operators (multiple replicas)
    │
    ├─ Agent Job Reconcilers
    ├─ Message Publishing (to NATS)
    │
    ▼
NATS Server Cluster
├─ NATS-1 (Primary)
├─ NATS-2 (Replica)
└─ NATS-3 (Replica)
    │
    ├─ Topics scoped by tenant
    ├─ Persistence: In-memory (or JetStream for persistence)
    │
    ▼
Agent Containers
├─ Agent A (subscribes to tenant-a/*)
├─ Agent B (subscribes to tenant-a/workflow/*)
└─ Agent C (subscribes to tenant-a/notifications/*)
```

### Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muto-config
  namespace: muto-system
data:
  messageBus: |
    type: nats
    nats:
      servers:
      - nats://nats-0.nats.muto-system.svc.cluster.local:4222
      - nats://nats-1.nats.muto-system.svc.cluster.local:4222
      - nats://nats-2.nats.muto-system.svc.cluster.local:4222
      
      # JetStream persistence (optional)
      jetstream:
        enabled: true
        storage: file
        maxBytes: 1073741824  # 1GB
        maxAge: 604800s       # 7 days
      
      # Credentials (optional)
      auth:
        username: muto-user
        passwordSecret: muto-nats-password
```

### Topic Subscription

Agents subscribe to topics:

<<<<<<< HEAD
=======
```go
// Agent code subscribing to messages
type Agent struct {
    messageBus MessageBus
    logger     Logger
}

func (a *Agent) Start(ctx context.Context) error {
    tenant := os.Getenv("TENANT_ID")
    topic := fmt.Sprintf("%s/workflow/*", tenant)
    
    return a.messageBus.Subscribe(ctx, topic, func(ctx context.Context, msg *Message) error {
        a.logger.Info("Received message", "topic", msg.Topic, "data", string(msg.Payload))
        return a.processMessage(ctx, msg)
    })
}

func (a *Agent) processMessage(ctx context.Context, msg *Message) error {
    // Parse message
    var data map[string]interface{}
    json.Unmarshal(msg.Payload, &data)
    
    // Process
    result := a.transform(data)
    
    // Publish result
    resultMsg := &Message{
        Topic:   fmt.Sprintf("%s/workflow/transformed", msg.Tenant),
        Tenant:  msg.Tenant,
        Payload: marshal(result),
    }
    return a.messageBus.Publish(ctx, resultMsg.Topic, resultMsg)
}
```
>>>>>>> a53175d (docs: write architecture/messaging.md - message bus abstraction and implementations)

### Advantages

- **Simple**: Few moving parts, easy to operate
- **Fast**: In-memory pub/sub with nanosecond latency
- **Scalable**: Handles millions of messages per second
- **Reliable**: Can enable JetStream for persistence and replay
- **Multi-tenant**: Built-in topic hierarchy for isolation

### Limitations

- **In-memory**: Without JetStream, messages lost on server restart
- **Single datacenter**: Not ideal for cross-region deployments
- **Limited persistence**: JetStream requires significant resources

## Kafka Implementation

For enterprises requiring durable, replicated message storage.

### Architecture

```
Muto Operators
    │
    ├─ Publish to Kafka Topics
    │
    ▼
Kafka Cluster
├─ Broker-1 (Leader for partitions 0,1)
├─ Broker-2 (Leader for partitions 2,3)
└─ Broker-3 (Leader for partitions 4,5)
    │
    ├─ Topics: tenant-a-workflow, tenant-a-events, etc.
    ├─ Partitions: Distribute load
    ├─ Replication: 3x (durability)
    ├─ Retention: Configurable (days, bytes)
    │
    ▼
Consumer Groups
├─ Agent A subscribes to tenant-a topics
├─ Agent B subscribes to tenant-a topics
└─ Monitoring subscribes to all topics
```

### Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muto-config
data:
  messageBus: |
    type: kafka
    kafka:
      brokers:
      - kafka-0.kafka.muto-system.svc.cluster.local:9092
      - kafka-1.kafka.muto-system.svc.cluster.local:9092
      - kafka-2.kafka.muto-system.svc.cluster.local:9092
      
      # Consumer group for Muto operator
      consumerGroup: muto-operator
      
      # Topic configuration
      topicConfig:
        partitions: 3
        replicationFactor: 3
        retention:
          ms: 604800000  # 7 days
          bytes: 1073741824  # 1GB
      
      # SASL/SCRAM authentication (optional)
      auth:
        saslMechanism: SCRAM-SHA-512
        username: muto-user
        passwordSecret: muto-kafka-password
      
      # TLS (optional)
      tls:
        enabled: true
        caSecret: muto-kafka-ca
        certSecret: muto-kafka-cert
```

### Multi-Tenancy with Kafka

Topics are prefixed by tenant ID:

```
Kafka Cluster Topics:
├─ tenant-a-workflow (partition 0,1,2)
├─ tenant-a-events (partition 0,1,2)
├─ tenant-a-notifications (partition 0,1,2)
├─ tenant-b-workflow (partition 0,1,2)
├─ tenant-b-events (partition 0,1,2)
└─ muto-internal (for operator events)

Consumer Groups:
├─ agent-tenant-a (reads tenant-a-* topics)
├─ agent-tenant-b (reads tenant-b-* topics)
└─ muto-monitoring (reads all topics)
```

### Advantages

- **Durable**: Messages persisted to disk, survive restarts
- **Replicated**: Multiple brokers ensure high availability
- **Partitioned**: Distribute load across brokers
- **Replay**: Consume old messages by offset
- **Enterprise-ready**: Many organizations already use Kafka

### Limitations

- **Complex**: More infrastructure to manage
- **Resources**: Requires more CPU/memory than NATS
- **Operations**: Broker failover, rebalancing require expertise

## Custom Message Bus Implementation

You can implement custom message buses for specific needs:

<<<<<<< HEAD
=======
```go
type CustomMessageBus struct {
    // Your implementation
}

func (c *CustomMessageBus) Publish(ctx context.Context, topic string, msg *Message) error {
    // Your logic: send to custom broker, API, webhook, etc.
    return nil
}

func (c *CustomMessageBus) Subscribe(ctx context.Context, topic string, callback MessageCallback) error {
    // Your logic: listen for messages and call callback
    return nil
}

func (c *CustomMessageBus) HealthCheck(ctx context.Context) error {
    // Verify connection to your system
    return nil
}

func (c *CustomMessageBus) Close() error {
    // Cleanup
    return nil
}

// Register with Muto
func init() {
    messageBus.Register("custom", func(config map[string]interface{}) (MessageBus, error) {
        return &CustomMessageBus{}, nil
    })
}
```
>>>>>>> a53175d (docs: write architecture/messaging.md - message bus abstraction and implementations)

Examples:
- **Google Cloud Pub/Sub**: Publish to GCP Pub/Sub topics
- **AWS SQS/SNS**: Route messages through SQS/SNS
- **Webhook**: Forward messages to HTTP webhooks
- **Database**: Store messages in PostgreSQL with LISTEN/NOTIFY
- **RabbitMQ**: Use RabbitMQ for advanced routing

## Message Lifecycle

### Publishing

```
Agent publishes message
    │
    ├─ Validate tenant scoping (message topic contains tenant ID)
    ├─ Add metadata (timestamp, message ID, correlation ID)
    ├─ Serialize to JSON
    │
    ▼
MessageBus.Publish()
    │
    ├─ Route to correct broker
    ├─ Write to durable storage (if enabled)
    ├─ Notify subscribers
    │
    ▼
Subscribers receive message
```

### Subscription and Consumption

```
Agent subscribes to topic
    │
    ├─ Verify tenant scoping (can only subscribe to tenant-a/*)
    ├─ Register callback with message bus
    │
    ▼
Message published to topic
    │
    ├─ MessageBus identifies subscribers
    ├─ Calls callback function for each subscriber
    │
    ▼
Callback processes message
    │
    ├─ If succeeds: Message considered consumed
    ├─ If fails: Retry (with backoff, if supported)
    │ If max retries exceeded: Move to dead-letter queue (if enabled)
```

## Guarantees and Tradeoffs

### Delivery Guarantees

| Feature | NATS | Kafka | Custom |
|---------|------|-------|--------|
| At-most-once | ✓ | ✓ | Configurable |
| At-least-once | ✓ (w/JetStream) | ✓ | Configurable |
| Exactly-once | ✗ | ✗ | Possible w/idempotency |
| Message ordering | ✓ | ✓ (per partition) | Configurable |
| Message durability | ✗ (✓ w/JetStream) | ✓ | Configurable |

### Performance Characteristics

```
Latency (median, p99):
NATS:  1ms, 5ms
Kafka: 5ms, 50ms

Throughput:
NATS:  1M+ msg/sec
Kafka: 100k-1M msg/sec (depends on replication)

Storage per message:
NATS:  ~100 bytes (in-memory)
Kafka: ~500 bytes (persisted)
```

## Monitoring Message Bus Health

Muto monitors message bus health:

<<<<<<< HEAD
=======
```go
// Regular health checks
ticker := time.NewTicker(30 * time.Second)
for range ticker.C {
    if err := messageBus.HealthCheck(ctx); err != nil {
        logger.Error(err, "message bus unhealthy")
        // Alert operators
        metrics.IncMessageBusErrors()
    }
}
```
>>>>>>> a53175d (docs: write architecture/messaging.md - message bus abstraction and implementations)

Exported metrics:

```
# Messages published
muto_messages_published_total{tenant="tenant-a"} 10523
muto_messages_published_bytes_total{tenant="tenant-a"} 5242880

# Messages received
muto_messages_received_total{tenant="tenant-a"} 10520
muto_messages_received_bytes_total{tenant="tenant-a"} 5242500

# Message latency
muto_message_latency_seconds{topic="tenant-a/workflow/done"} 0.001234

# Bus health
muto_message_bus_health{status="healthy"} 1
muto_message_bus_latency_ms{} 2.5
```

---

## Next Steps

- **[Security Model](./security-model.md)** — How multi-tenancy is enforced on message topics
- **[Platform Design](./platform-design.md)** — How agents are deployed
- **[Concepts (Message Bus)](../getting-started/concepts.md#message-bus)** — Return to concepts

**Last Updated:** 2026-09-03
