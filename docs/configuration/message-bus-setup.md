# Message Bus Setup and Configuration

Configure and optimize message bus for inter-agent communication.

## Overview

The message bus is the backbone of agent-to-agent communication in Muto. It provides publish/subscribe functionality for agents to coordinate and share results asynchronously.

### Message Bus Implementations

Muto supports two message bus implementations:

| Feature | NATS | Kafka |
|---------|------|-------|
| **Best For** | Development, small deployments | Production, high-throughput |
| **Persistence** | Memory (configurable TTL) | Disk-based | 
| **Clustering** | Built-in HA | Built-in HA |
| **Partitioning** | Topics only | Topics + Partitions |
| **Consumer Groups** | Supported | Supported |
| **Latency** | Sub-second | Single-digit ms |
| **Throughput** | 100k+ msgs/sec | 1M+ msgs/sec |
| **Storage** | None by default | Days to years |

---

## NATS Configuration

### NATS Server Setup

**Prerequisites:**
- NATS server 2.9+ running
- Connectivity from Muto operator to NATS server

**Quick Start (Docker):**
```bash
docker run -d \
  --name nats \
  -p 4222:4222 \
  -p 8222:8222 \
  nats:2.9-alpine
```

### Connection Configuration

**Basic Connection:**
```bash
export MUTO_MESSAGE_BUS_TYPE=nats
export MUTO_NATS_URL=nats://nats.muto-system:4222
```

**Single Server:**
```bash
export MUTO_NATS_URL=nats://nats-server.example.com:4222
```

**NATS Cluster (High Availability):**
```bash
export MUTO_NATS_URL=nats://nats-1:4222,nats://nats-2:4222,nats://nats-3:4222
```

### Connection Pooling

**NATS Connection Pool:**
```bash
# Number of connections in pool
export MUTO_NATS_POOL_SIZE=10

# Connections per tenant (in multi-tenant setup)
export MUTO_NATS_POOL_SIZE=50
```

**Tuning Guidelines:**
- 5-10: Development/testing
- 10-25: Small production (< 10 agents)
- 25-50: Medium production (10-100 agents)
- 50-100: Large production (100+ agents)

### Authentication

**No Auth (Development Only):**
```bash
export MUTO_NATS_URL=nats://nats:4222
```

**Username/Password:**
```bash
export MUTO_NATS_URL=nats://user:password@nats:4222
```

**JWT Credentials:**
```bash
export MUTO_NATS_CREDENTIALS_FILE=/etc/muto/nats-creds.txt
```

Create credentials file:
```bash
# Using NATS CLI
nats nkeys gen user -o ~/nats-user.nk
nats context add --user ~/nats-user.nk
```

### TLS Configuration

**Enable TLS:**
```bash
export MUTO_NATS_URL=nats+tls://nats:4222
export MUTO_NATS_TLS_CA=/etc/muto/nats-ca.crt
export MUTO_NATS_TLS_CERT=/etc/muto/nats-client.crt
export MUTO_NATS_TLS_KEY=/etc/muto/nats-client.key
```

**Verify TLS Certificate:**
```bash
# Check certificate validity
openssl x509 -in /etc/muto/nats-ca.crt -text -noout

# Test TLS connection
openssl s_client -connect nats:4222 -cert /etc/muto/nats-client.crt -key /etc/muto/nats-client.key -CAfile /etc/muto/nats-ca.crt
```

### Reconnection Settings

```bash
# Maximum reconnection attempts
export MUTO_NATS_MAX_RECONNECTS=5

# Wait between reconnection attempts
export MUTO_NATS_RECONNECT_WAIT=2s
```

**Tuning:**
```bash
# Aggressive reconnection (for unreliable networks)
export MUTO_NATS_MAX_RECONNECTS=15
export MUTO_NATS_RECONNECT_WAIT=1s

# Conservative (for stable networks)
export MUTO_NATS_MAX_RECONNECTS=3
export MUTO_NATS_RECONNECT_WAIT=5s
```

### NATS HA Setup (Kubernetes)

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nats-config
  namespace: muto-system
data:
  nats.conf: |
    port: 4222
    http_port: 8222
    
    server_name: nats-1
    
    # Clustering
    cluster {
      name: muto-cluster
      listen: 0.0.0.0:6222
      
      routes: [
        nats://nats-1:6222,
        nats://nats-2:6222,
        nats://nats-3:6222,
      ]
    }
    
    # Monitoring
    accounts {
      $SYS {
        users: [
          { user: "sys", password: "secret" }
        ]
      }
    }
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: nats
  namespace: muto-system
spec:
  serviceName: nats
  replicas: 3
  selector:
    matchLabels:
      app: nats
  template:
    metadata:
      labels:
        app: nats
    spec:
      containers:
      - name: nats
        image: nats:2.9-alpine
        ports:
        - containerPort: 4222
          name: client
        - containerPort: 6222
          name: cluster
        - containerPort: 8222
          name: monitor
        volumeMounts:
        - name: config
          mountPath: /etc/nats
        command:
        - nats-server
        - -c
        - /etc/nats/nats.conf
      volumes:
      - name: config
        configMap:
          name: nats-config
---
apiVersion: v1
kind: Service
metadata:
  name: nats
  namespace: muto-system
spec:
  clusterIP: None
  selector:
    app: nats
  ports:
  - port: 4222
    name: client
  - port: 6222
    name: cluster
  - port: 8222
    name: monitor
```

---

## Kafka Configuration

### Kafka Cluster Setup

**Prerequisites:**
- Kafka cluster 2.8+ running
- ZooKeeper or KRaft mode (Kafka 3.0+)
- Connectivity from Muto operator to Kafka brokers

**Quick Start (Docker Compose):**
```yaml
version: '3.8'
services:
  kafka:
    image: confluentinc/cp-kafka:7.0
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
    depends_on:
      - zookeeper
  zookeeper:
    image: confluentinc/cp-zookeeper:7.0
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181
```

### Connection Configuration

**Basic Connection:**
```bash
export MUTO_MESSAGE_BUS_TYPE=kafka
export MUTO_KAFKA_BROKERS=kafka:9092
```

**Multiple Brokers (Recommended):**
```bash
export MUTO_KAFKA_BROKERS=kafka-1:9092,kafka-2:9092,kafka-3:9092
```

**Custom Port:**
```bash
export MUTO_KAFKA_BROKERS=kafka.example.com:29092
```

### Authentication (SASL)

**Enable SASL:**
```bash
export MUTO_KAFKA_SASL_ENABLED=true
export MUTO_KAFKA_SASL_MECHANISM=PLAIN
export MUTO_KAFKA_SASL_USER=muto
export MUTO_KAFKA_SASL_PASSWORD=secret-password
```

**PLAIN Mechanism:**
```bash
export MUTO_KAFKA_SASL_MECHANISM=PLAIN
export MUTO_KAFKA_SASL_USER=muto-user
export MUTO_KAFKA_SASL_PASSWORD=password
```

**SCRAM-SHA-256:**
```bash
export MUTO_KAFKA_SASL_MECHANISM=SCRAM-SHA-256
export MUTO_KAFKA_SASL_USER=muto-user
export MUTO_KAFKA_SASL_PASSWORD=password
```

**SCRAM-SHA-512:**
```bash
export MUTO_KAFKA_SASL_MECHANISM=SCRAM-SHA-512
export MUTO_KAFKA_SASL_USER=muto-user
export MUTO_KAFKA_SASL_PASSWORD=password
```

### TLS Configuration

**Enable TLS:**
```bash
export MUTO_KAFKA_TLS_ENABLED=true
export MUTO_KAFKA_TLS_CA=/etc/muto/kafka-ca.crt
export MUTO_KAFKA_TLS_CERT=/etc/muto/kafka-client.crt
export MUTO_KAFKA_TLS_KEY=/etc/muto/kafka-client.key
```

**TLS + SASL:**
```bash
export MUTO_KAFKA_TLS_ENABLED=true
export MUTO_KAFKA_SASL_ENABLED=true
export MUTO_KAFKA_SASL_MECHANISM=SCRAM-SHA-256
export MUTO_KAFKA_TLS_CA=/etc/muto/kafka-ca.crt
export MUTO_KAFKA_SASL_USER=muto
export MUTO_KAFKA_SASL_PASSWORD=password
```

### Topic Configuration

**Consumer Group:**
```bash
export MUTO_KAFKA_CONSUMER_GROUP=muto-operator
```

**Replication Factor:**
```bash
# Development (single replica)
export MUTO_KAFKA_REPLICATION_FACTOR=1

# Production (high availability)
export MUTO_KAFKA_REPLICATION_FACTOR=3
```

**Min In-Sync Replicas:**
```bash
# Strict durability (wait for all replicas)
export MUTO_KAFKA_MIN_IN_SYNC_REPLICAS=3

# Balanced
export MUTO_KAFKA_MIN_IN_SYNC_REPLICAS=2

# Fast but risky
export MUTO_KAFKA_MIN_IN_SYNC_REPLICAS=1
```

### Topic Auto-Creation

Enable Kafka to auto-create topics when Muto publishes to them:

```bash
# In broker config
auto.create.topics.enable=true
```

Or create topics manually:

```bash
kafka-topics --create \
  --topic muto-agent-messages \
  --partitions 3 \
  --replication-factor 3 \
  --bootstrap-server kafka-1:9092,kafka-2:9092,kafka-3:9092
```

### Kafka HA Setup (Kubernetes)

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kafka
  namespace: muto-system
spec:
  serviceName: kafka
  replicas: 3
  selector:
    matchLabels:
      app: kafka
  template:
    metadata:
      labels:
        app: kafka
    spec:
      containers:
      - name: kafka
        image: confluentinc/cp-kafka:7.0
        ports:
        - containerPort: 9092
          name: broker
        env:
        - name: KAFKA_BROKER_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.annotations['kafka.muto.io/broker-id']
        - name: KAFKA_ADVERTISED_LISTENERS
          value: PLAINTEXT://kafka-0.kafka.muto-system:9092,PLAINTEXT://kafka-1.kafka.muto-system:9092,PLAINTEXT://kafka-2.kafka.muto-system:9092
        - name: KAFKA_ZOOKEEPER_CONNECT
          value: zookeeper-0.zookeeper.muto-system:2181,zookeeper-1.zookeeper.muto-system:2181,zookeeper-2.zookeeper.muto-system:2181
        - name: KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR
          value: "3"
---
apiVersion: v1
kind: Service
metadata:
  name: kafka
  namespace: muto-system
spec:
  clusterIP: None
  selector:
    app: kafka
  ports:
  - port: 9092
    name: broker
```

---

## Message Bus Health Checks

### NATS Health Check

```bash
# Check NATS server connectivity
curl http://nats:8222/varz

# Expected response:
# {
#   "version": "2.9.0",
#   "proto": 1,
#   "go": "go1.19",
#   "host": "...",
#   "now": "...",
#   "uptime": "...",
#   "connections": 5,
#   "total_connections": 100,
#   "subscriptions": 1000,
#   ...
# }
```

### Kafka Health Check

```bash
# Check Kafka broker health
kafka-broker-api-versions.sh --bootstrap-server kafka:9092

# List topics
kafka-topics.sh --list --bootstrap-server kafka:9092

# Check consumer group
kafka-consumer-groups.sh --bootstrap-server kafka:9092 --group muto-operator --describe
```

### Muto Message Bus Health

```bash
# Check if message bus is connected
kubectl logs -n muto-system deployment/muto-operator | grep -i "message bus"

# Monitor message throughput
kubectl exec -n muto-system deployment/muto-operator -- curl localhost:8081/metrics | grep -i message
```

---

## Message Format and Schemas

### Standard Message Format

Muto messages have a consistent structure:

```json
{
  "version": "v1",
  "tenant": "tenant-a",
  "topic": "workflow/step-complete",
  "messageId": "msg-123456",
  "timestamp": "2026-09-03T10:30:45Z",
  "source": {
    "agent": "extractor",
    "jobId": "job-12345"
  },
  "payload": {
    "status": "completed",
    "recordsProcessed": 1000,
    "outputPath": "s3://bucket/results"
  },
  "headers": {
    "traceId": "trace-123",
    "spanId": "span-456"
  }
}
```

### Topic Naming Convention

```
<tenant>/<domain>/<subject>/<action>

Examples:
- tenant-a/workflow/extraction/complete
- tenant-a/workflow/processing/progress
- tenant-a/notifications/job/failure
- system/operator/startup
```

### Message Retention

**NATS Retention:**
```bash
# Time-based retention (24 hours)
export MUTO_NATS_RETENTION=24h

# Size-based retention (1GB)
export MUTO_NATS_RETENTION=1gb
```

**Kafka Retention:**
```bash
# Time-based (7 days default)
log.retention.hours=168

# Size-based (unlimited)
log.retention.bytes=-1

# Segment size (1GB)
log.segment.bytes=1073741824
```

---

## Performance Tuning

### Connection Pool Sizing

**NATS:**
```bash
# Calculate pool size: 
# num_agents × avg_subscriptions_per_agent / 10

# Example: 50 agents, 5 subscriptions each
# 50 × 5 / 10 = 25 connections
export MUTO_NATS_POOL_SIZE=25
```

### Batch Size and Buffering

```bash
# Message batch size (for bulk operations)
export MUTO_MESSAGE_BATCH_SIZE=100

# Buffer flush timeout
export MUTO_MESSAGE_BUFFER_TIMEOUT=100ms
```

### Throughput Optimization

**For NATS:**
```bash
# Increase pool size
export MUTO_NATS_POOL_SIZE=50

# Enable compression
export MUTO_NATS_COMPRESSION=true

# Reduce reconnect wait
export MUTO_NATS_RECONNECT_WAIT=1s
```

**For Kafka:**
```bash
# Increase partitions for parallel processing
export MUTO_KAFKA_NUM_PARTITIONS=12

# Enable compression
export MUTO_KAFKA_COMPRESSION_TYPE=snappy

# Tune batch settings
export MUTO_KAFKA_BATCH_SIZE=32768
export MUTO_KAFKA_LINGER_MS=100
```

### Latency Optimization

**NATS (ultra-low latency):**
```bash
# Reduce batch/linger times
export MUTO_MESSAGE_BUFFER_TIMEOUT=10ms

# Increase pool size for parallelism
export MUTO_NATS_POOL_SIZE=50
```

**Kafka:**
```bash
# Reduce linger time
export MUTO_KAFKA_LINGER_MS=10

# Lower compression for faster encode
export MUTO_KAFKA_COMPRESSION_TYPE=lz4
```

---

## Monitoring and Observability

### Key Metrics to Monitor

```promql
# Messages published per second
rate(muto_message_bus_publish_total[1m])

# Messages received per second
rate(muto_message_bus_receive_total[1m])

# Publish latency (p95)
histogram_quantile(0.95, muto_message_bus_publish_duration_seconds)

# Message bus connection count
muto_message_bus_connections

# Consumer lag (Kafka only)
muto_kafka_consumer_lag_bytes
```

### Common Issues and Solutions

**Problem: High message latency**

Solution:
```bash
# Increase pool size
export MUTO_NATS_POOL_SIZE=50

# Reduce buffering
export MUTO_MESSAGE_BUFFER_TIMEOUT=50ms

# Check broker health
kubectl describe pod nats-0 -n muto-system
```

**Problem: Messages not being delivered**

Solution:
```bash
# Check message bus connectivity
kubectl logs -n muto-system deployment/muto-operator | grep -i "connection"

# Verify topic subscriptions
# For NATS
curl http://nats:8222/connz | jq '.conns[].subs'

# Check consumer group lag
kafka-consumer-groups.sh --bootstrap-server kafka:9092 --group muto-operator --describe
```

**Problem: Message bus disk full (Kafka)**

Solution:
```bash
# Check disk usage
df -h /var/kafka

# Increase retention settings or add storage
# Edit Kafka broker configuration

# Clean up old topics
kafka-topics.sh --delete --topic old-topic --bootstrap-server kafka:9092
```

---

## Best Practices

1. **Use NATS for development**, Kafka for production
2. **Enable TLS in production** — never skip encryption
3. **Configure authentication** — use SASL or credentials
4. **Monitor consumer lag** — especially for Kafka
5. **Set appropriate retention** — balance between storage and replay ability
6. **Test failover scenarios** — ensure system handles broker failures
7. **Regular backups** — for Kafka, back up Zookeeper data
8. **Tune based on metrics** — don't over-tune without data

---

## See Also

- [Environment Variables](./environment-variables.md) — All message bus settings
- [Architecture: Messaging](../architecture/messaging.md) — How messaging works
- [Deployment: Production Checklist](../deployment/production-checklist.md) — Pre-launch verification
- [Multi-Tenant Setup](./multi-tenant-setup.md) — Tenant-scoped topics

---

**Last Updated:** 2026-09-03
