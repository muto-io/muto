# Message Bus Setup and Configuration

What the NATS/Kafka message bus code actually does today, and what it doesn't.

## Overview

`core/messaging` defines one interface, `MessageBus` (`Publish`/`Subscribe`/`Close`), and two backends implement it: `core/messaging/nats` (via `nats.go`, using JetStream) and `core/messaging/kafka` (via `sarama`). Both exist as real, tested Go packages. Neither is wired into `muto-operator` or `muto-mcp` today - `messaging.NewBus` has no production call site anywhere in the repo, only in `core/messaging`'s own package tests. The one caller that takes a `MessageBus` as a dependency, `platform/k8s.NewEventWatcher`, is likewise only ever called from its own test.

In practice, Muto's actual job today is narrower than "runs a message bus": for each agent it spawns, the operator injects `MUTO_TENANT`, `MUTO_ROLE`, `MUTO_JOB_ID`, and `MUTO_MESSAGEBUS_TOPIC` as environment variables into that agent's container (see `platform/k8s/reconcilers/agentjob_reconciler.go`). What the agent does with those - including whether and how it connects to NATS or Kafka - is up to the agent's own code. There is no `MUTO_MESSAGE_BUS_TYPE`, `MUTO_NATS_URL`, or `MUTO_KAFKA_BROKERS` environment variable; none of this is configured via env vars anywhere in the code.

If you want to use `core/messaging`'s NATS/Kafka backends yourself (e.g. inside an agent, or in a future operator wiring), you select and configure a backend from Go code, not the environment:

```go
import "github.com/muto-io/muto/core/messaging"
import _ "github.com/muto-io/muto/core/messaging/nats" // registers "nats"

bus, err := messaging.NewBus(messaging.BusTypeNATS, &messaging.Config{
    URLs:     []string{"nats://nats.muto-system:4222"},
    Username: "user",     // optional
    Password: "password", // optional
})
```

`messaging.Config` has exactly three fields: `URLs []string`, `Username string`, `Password string`. That's the entire configuration surface of both backends - there is no TLS, SASL mechanism choice, connection pooling, reconnect tuning, compression, or partition/replication configuration anywhere in `core/messaging`, `core/messaging/nats`, or `core/messaging/kafka`. Specifically:

- **NATS**: `New` makes one `nats.go` connection with no reconnect/TLS options set beyond the library's own defaults, publishes/subscribes through JetStream. `Username`/`Password` map to basic `nats.UserInfo` auth; there's no JWT/NKey/TLS cert support.
- **Kafka**: `New` makes one `sarama` producer and one `sarama` consumer per bus. Setting `Username` turns on SASL, but only ever with sarama's default `PLAIN` mechanism - there's no SCRAM support, and no TLS config at all. `Subscribe` calls `ConsumePartition(topic, 0, ...)` directly - it always reads partition 0 and never joins a sarama consumer group, so despite the comparison tables in earlier drafts of this doc, there's no real consumer-group behavior on either backend yet.

## Topic Naming

`core/tenant.TopicPrefix(tenantID)` returns `tenant.<tenantID>.`, and `mcp/tools` uses it to build every scheduled job's topic as `tenant.<tenantID>.<jobID>`. `core/tenant.ValidateTopic` checks a topic string against that prefix (exercised by `test/integration/k8s/isolation_test.go`), but it's a plain library function - nothing in the NATS/Kafka backends calls it, so it's a naming convention Muto applies when building topic strings, not something enforced at the bus level. See [Multi-Tenant Setup](./multi-tenant-setup.md#message-bus-topic-isolation).

## Selecting a Backend Per Tenant

The `Tenant` CRD's `spec.messageBus.type` field accepts `nats`, `kafka`, or `a2a` (see [Multi-Tenant Setup](./multi-tenant-setup.md)). Today this value isn't read back out anywhere to actually construct a NATS or Kafka bus - only the `a2a` case does anything (provisioning a per-tenant A2A gateway). Setting `type: nats` or `type: kafka` on a `Tenant` is accepted by the schema but has no other effect yet.

## Running NATS or Kafka Yourself

Since Muto doesn't run or manage either broker, you deploy and operate them the normal way - e.g. the official Helm charts, or a Kubernetes `StatefulSet` you manage yourself. There's no Muto-specific setup beyond making sure whatever agent code you run knows how to reach the broker and construct a `messaging.Config` (or its own client) using the `MUTO_MESSAGEBUS_TOPIC`/`MUTO_TENANT`/`MUTO_ROLE` values Muto already injects.

## See Also

- [Environment Variables](./environment-variables.md) - the variables Muto actually injects into agent containers
- [Multi-Tenant Setup](./multi-tenant-setup.md) - how `messageBus.type` is set per tenant and what topic isolation really means
