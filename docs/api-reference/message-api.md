# Message API Reference

Specification for inter-agent messaging and message bus communication in Muto. This document defines the message format, headers, routing, and expected behavior for agents communicating via the message bus.

## Overview

Muto uses a publish/subscribe message bus (NATS or Kafka) for asynchronous inter-agent communication. Agents publish status updates, results, and events; other agents subscribe and react to messages.

**Key Concepts:**
- **Topics**: Hierarchical topic names control message routing
- **Tenant Isolation**: All topics are automatically prefixed with tenant ID
- **Message Schema**: Standardized JSON format with headers and payload
- **Ordering**: Messages are ordered within a topic (exactly-once semantics with proper configuration)
- **Persistence**: Configurable message retention (duration or count)

---

## Topic Structure

### Topic Naming Convention

All topic names follow a hierarchical pattern:

```
<tenant-prefix>/<workflow>/<stage>/<event-type>
```

### Components

**`<tenant-prefix>`** (automatically added by Muto)
- Format: `tenant.<tenantID>`
- Example: `tenant.tenant-a`
- Purpose: Ensures complete tenant isolation
- Automatically prepended by the operator; agents do NOT include this in topic strings

**`<workflow>`** (user-defined)
- Logical workflow name
- Example: `data-pipeline`, `alerts`, `coordination`
- Can contain alphanumerics and hyphens
- Recommended: Snake case

**`<stage>`** (user-defined)
- Processing stage or agent role
- Example: `extract`, `transform`, `load`
- Maps to agent role names in AgentJob spec
- Optional: If omitted, defaults to agent role

**`<event-type>`** (user-defined)
- Type of event/message
- Example: `complete`, `failure`, `status`, `request`
- Standard types: `complete`, `started`, `progress`, `error`, `result`

### Examples

#### Example 1: Data Pipeline Completion

```
tenant.tenant-a/data-pipeline/extract/complete
```

- Tenant: `tenant-a`
- Workflow: `data-pipeline`
- Stage: `extract` (agent role)
- Event: `complete` (agent finished successfully)

#### Example 2: Job Status Update

```
tenant.tenant-a/job-monitor/status/update
```

- Tenant: `tenant-a`
- Workflow: `job-monitor`
- Stage: (implied)
- Event: `status/update` (status changed)

#### Example 3: Broadcast Topic

```
tenant.tenant-a/broadcast/all/event
```

- Subscribing to `tenant.tenant-a/broadcast/*` captures all events

---

## Message Format

### Standard Message Structure

All messages on the message bus follow this JSON structure:

```json
{
  "id": "msg-unique-id-123",
  "timestamp": "2026-09-03T10:30:45.123Z",
  "tenantID": "tenant-a",
  "jobID": "job-12345",
  "sourceAgent": "extractor",
  "sourceRole": "extract",
  "correlationID": "corr-abc123",
  "type": "JobComplete",
  "version": "1.0",
  "headers": {
    "X-Custom-Header": "value",
    "X-Priority": "high"
  },
  "payload": {
    "status": "succeeded",
    "outputPath": "s3://bucket/results.json",
    "itemsProcessed": 1500,
    "duration": 45.3
  }
}
```

### Field Definitions

#### `id` (string)
- **Required**: yes
- **Format**: Unique identifier, typically UUID v4 or custom ID
- **Description**: Unique message ID for deduplication and tracing
- **Example**: `"msg-550e8400-e29b-41d4-a716-446655440000"`

#### `timestamp` (string)
- **Required**: yes
- **Format**: RFC3339 ISO 8601
- **Description**: When the message was created (UTC)
- **Example**: `"2026-09-03T10:30:45.123Z"`

#### `tenantID` (string)
- **Required**: yes
- **Description**: Tenant ID (for context; automatically added to topic at publish time)
- **Example**: `"tenant-a"`
- **Note**: Topic string in the message bus already includes this as `tenant.<tenantID>`

#### `jobID` (string)
- **Required**: yes
- **Description**: Job this message relates to
- **Example**: `"data-pipeline-job-001"`

#### `sourceAgent` (string)
- **Required**: yes
- **Description**: Instance ID or name of the agent that published this message
- **Example**: `"extractor-pod-1", "transformer-instance-2"`

#### `sourceRole` (string)
- **Required**: yes
- **Description**: Role of the source agent (from AgentJob spec agents[].role)
- **Example**: `"extract", "transform", "load"`

#### `correlationID` (string)
- **Required**: no (recommended)
- **Format**: UUID or custom trace ID
- **Description**: Trace ID for correlating related messages across job
- **Example**: `"corr-550e8400-e29b-41d4-a716-446655440000"`
- **Benefit**: Allows tracking a request through multiple agents

#### `type` (string)
- **Required**: yes
- **Description**: Message type/classification
- **Standard Types**:
  - `JobStarted`: Job started executing
  - `AgentStarted`: Specific agent started
  - `JobProgress`: Progress update
  - `JobComplete`: Job completed successfully
  - `AgentComplete`: Specific agent completed
  - `JobError`: Job encountered error (may retry)
  - `AgentError`: Specific agent failed
  - `JobCancelled`: Job was cancelled
  - Custom types allowed (e.g., `DataValidationResult`)
- **Example**: `"JobComplete"`

#### `version` (string)
- **Required**: yes
- **Description**: Message schema version
- **Current**: `"1.0"`
- **Use**: For forward compatibility; consumers can check version

#### `headers` (object)
- **Required**: no
- **Description**: Custom headers for application use
- **Standard Headers**:
  - `X-Priority`: `low`, `normal`, `high`, `critical`
  - `X-Retry-Count`: Number of retries (for failed messages)
  - `X-Request-ID`: User-provided request ID
  - All custom headers start with `X-`
- **Example**:
  ```json
  {
    "X-Priority": "high",
    "X-Request-ID": "req-12345"
  }
  ```

#### `payload` (object)
- **Required**: yes
- **Description**: Message-specific data
- **Structure**: Arbitrary JSON object; shape depends on message type
- **Constraints**: Total message size < 1 MB (recommended < 100 KB)
- **Example**:
  ```json
  {
    "status": "succeeded",
    "outputPath": "s3://bucket/results.json",
    "itemsProcessed": 1500
  }
  ```

---

## Standard Message Types

### JobStarted

Emitted when a job transitions to Running phase.

```json
{
  "type": "JobStarted",
  "payload": {
    "jobID": "job-123",
    "scheduledTime": "2026-09-03T10:30:00Z",
    "agents": ["extract", "transform", "load"]
  }
}
```

### AgentStarted

Emitted when a specific agent starts executing.

```json
{
  "type": "AgentStarted",
  "sourceRole": "extract",
  "sourceAgent": "extract-pod-1",
  "payload": {
    "agentRole": "extract",
    "agentInstance": "extract-pod-1",
    "timestamp": "2026-09-03T10:30:15Z"
  }
}
```

### JobProgress

Progress update during execution.

```json
{
  "type": "JobProgress",
  "payload": {
    "jobID": "job-123",
    "completedAgents": ["extract"],
    "activeAgents": ["transform"],
    "pendingAgents": ["load"],
    "percentComplete": 33
  }
}
```

### JobComplete

Emitted when a job completes successfully.

```json
{
  "type": "JobComplete",
  "payload": {
    "jobID": "job-123",
    "phase": "Succeeded",
    "completedAt": "2026-09-03T10:45:30Z",
    "totalDuration": 915.5,
    "resultPath": "s3://bucket/job-123-results.json"
  }
}
```

### AgentComplete

Emitted when a specific agent completes.

```json
{
  "type": "AgentComplete",
  "sourceRole": "extract",
  "sourceAgent": "extract-pod-1",
  "payload": {
    "agentRole": "extract",
    "agentInstance": "extract-pod-1",
    "status": "Succeeded",
    "duration": 125.3,
    "itemsProcessed": 1000,
    "outputPath": "s3://bucket/extract-output.json"
  }
}
```

### JobError

Emitted when a job encounters an error (may retry).

```json
{
  "type": "JobError",
  "payload": {
    "jobID": "job-123",
    "phase": "Failed",
    "failedAgent": "transform",
    "errorCode": "OUT_OF_MEMORY",
    "errorMessage": "Process exceeded memory limit",
    "retryAttempt": 1,
    "willRetry": true,
    "nextRetryTime": "2026-09-03T10:46:00Z"
  }
}
```

### AgentError

Emitted when a specific agent fails.

```json
{
  "type": "AgentError",
  "sourceRole": "transform",
  "sourceAgent": "transform-pod-1",
  "payload": {
    "agentRole": "transform",
    "agentInstance": "transform-pod-1",
    "status": "Failed",
    "errorCode": "VALIDATION_FAILED",
    "errorMessage": "Data validation failed on row 456",
    "retryAttempt": 0,
    "willRetry": true
  }
}
```

### JobCancelled

Emitted when a job is cancelled.

```json
{
  "type": "JobCancelled",
  "payload": {
    "jobID": "job-123",
    "cancelledAt": "2026-09-03T10:47:00Z",
    "cancellationReason": "User requested",
    "activeAgents": ["transform"],
    "note": "Active agents will be terminated within 30 seconds"
  }
}
```

---

## Publishing Messages

### From Agent Code

Agents typically publish messages to the message bus at key lifecycle points:

#### Go Example

<<<<<<< HEAD
=======
```go
import (
	"github.com/muto-io/muto/core/agent"
	"github.com/nats-io/nats.go"
)

func agentMain(nc *nats.Conn, tenantID string, jobID string) error {
	topic := fmt.Sprintf("tenant.%s/data-pipeline/extract/complete", tenantID)

	message := map[string]interface{}{
		"id":           "msg-12345",
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"tenantID":     tenantID,
		"jobID":        jobID,
		"sourceAgent":  "extract-pod-1",
		"sourceRole":   "extract",
		"type":         "JobComplete",
		"version":      "1.0",
		"correlationID": "corr-abc123",
		"payload": map[string]interface{}{
			"status":        "succeeded",
			"itemsProcessed": 1500,
			"outputPath":    "s3://bucket/results.json",
		},
	}

	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	return nc.Publish(topic, data)
}
```
>>>>>>> 630e387 (docs: add api-reference section with 4 files)

#### Python Example

```python
import json
import time
from datetime import datetime

def agent_main(nc, tenant_id, job_id):
    topic = f"tenant.{tenant_id}/data-pipeline/extract/complete"

    message = {
        "id": "msg-12345",
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "tenantID": tenant_id,
        "jobID": job_id,
        "sourceAgent": "extract-pod-1",
        "sourceRole": "extract",
        "type": "JobComplete",
        "version": "1.0",
        "correlationID": "corr-abc123",
        "payload": {
            "status": "succeeded",
            "itemsProcessed": 1500,
            "outputPath": "s3://bucket/results.json",
        }
    }

    nc.publish(topic, json.dumps(message).encode())
```

---

## Subscribing to Messages

### From Agent Code

Agents subscribe to topics and react to incoming messages:

#### Go Example

<<<<<<< HEAD
=======
```go
func handleMessages(nc *nats.Conn, tenantID string) error {
	topic := fmt.Sprintf("tenant.%s/data-pipeline/extract/>", tenantID)

	sub, err := nc.Subscribe(topic, func(msg *nats.Msg) {
		var message map[string]interface{}
		if err := json.Unmarshal(msg.Data, &message); err != nil {
			log.Printf("Failed to unmarshal: %v", err)
			return
		}

		messageType, ok := message["type"].(string)
		if !ok {
			return
		}

		switch messageType {
		case "JobComplete":
			handleJobComplete(message)
		case "JobError":
			handleJobError(message)
		}
	})

	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	select {}
}

func handleJobComplete(message map[string]interface{}) {
	payload := message["payload"].(map[string]interface{})
	outputPath := payload["outputPath"].(string)
	log.Printf("Processing output from: %s", outputPath)
}
```
>>>>>>> 630e387 (docs: add api-reference section with 4 files)

#### Python Example

```python
def handle_messages(nc, tenant_id):
    topic = f"tenant.{tenant_id}/data-pipeline/extract/>"

    def message_handler(msg):
        try:
            message = json.loads(msg.data.decode())
            message_type = message.get("type")

            if message_type == "JobComplete":
                handle_job_complete(message)
            elif message_type == "JobError":
                handle_job_error(message)
        except json.JSONDecodeError:
            pass

    nc.subscribe(topic, cb=message_handler)
```

---

## Topic Wildcards & Subscriptions

NATS and Kafka support wildcard subscriptions:

### Single-Level Wildcard: `*`

```
tenant.tenant-a/data-pipeline/*/complete
```

Matches one level:
- ✅ `tenant.tenant-a/data-pipeline/extract/complete`
- ✅ `tenant.tenant-a/data-pipeline/transform/complete`
- ❌ `tenant.tenant-a/data-pipeline/extract/validate/complete` (too deep)

### Multi-Level Wildcard: `>`

```
tenant.tenant-a/data-pipeline/>
```

Matches all descendant levels:
- ✅ `tenant.tenant-a/data-pipeline/extract/complete`
- ✅ `tenant.tenant-a/data-pipeline/transform/error`
- ✅ `tenant.tenant-a/data-pipeline/deep/nested/topic`

### All Tenant Events

```
tenant.tenant-a/>
```

Matches all events in a tenant.

---

## Message Ordering & Exactly-Once Semantics

### Ordering Guarantees

**Within a single topic**: Messages published to the same topic are delivered in order (FIFO).

**Across topics**: No ordering guarantee between messages on different topics.

### Exactly-Once Semantics (For Idempotent Operations)

Muto uses message IDs to support idempotent processing:

1. Each message has a unique `id` field
2. Consumers can detect duplicate delivery by checking the message ID
3. Consumers should implement idempotent handlers (safe to process same message twice)

**Example: Idempotent Database Update**

<<<<<<< HEAD
=======
```go
func processMessage(msg map[string]interface{}, db *Database) error {
	messageID := msg["id"].(string)

	// Check if we've already processed this message
	if db.HasProcessedMessage(messageID) {
		log.Printf("Message %s already processed, skipping", messageID)
		return nil
	}

	// Process the message
	payload := msg["payload"].(map[string]interface{})
	if err := db.InsertResult(payload); err != nil {
		return err
	}

	// Mark as processed
	return db.RecordProcessedMessage(messageID)
}
```
>>>>>>> 630e387 (docs: add api-reference section with 4 files)

---

## Message Retention & Cleanup

### NATS Configuration

NATS JetStream allows configuring retention:

```yaml
# In stream configuration
retention:
  policy: "limits"
  maxAge: 604800  # 7 days
  maxMsgs: 1000000
```

### Kafka Configuration

Kafka allows per-topic retention:

```bash
kafka-topics --create --topic tenant.tenant-a.workflow \
  --config retention.ms=604800000  # 7 days
```

---

## Error Handling & Retry Logic

### Message Delivery Failures

If a message fails to publish:

<<<<<<< HEAD
=======
```go
err := nc.Publish(topic, data)
if err != nil {
	if err == nats.ErrTimeout {
		// Timeout: try again after backoff
		time.Sleep(1 * time.Second)
		nc.Publish(topic, data)
	} else if err == nats.ErrNoServers {
		// No servers: fatal, log and exit
		log.Fatalf("No NATS servers available: %v", err)
	}
}
```
>>>>>>> 630e387 (docs: add api-reference section with 4 files)

### Subscription Failures

If subscription encounters errors, implement reconnection logic:

<<<<<<< HEAD
=======
```go
for {
	sub, err := nc.Subscribe(topic, handler)
	if err != nil {
		log.Printf("Subscribe failed: %v, retrying...", err)
		time.Sleep(5 * time.Second)
		continue
	}
	
	// Block until connection lost
	<-sub.Done()
	log.Printf("Subscription ended, reconnecting...")
}
```
>>>>>>> 630e387 (docs: add api-reference section with 4 files)

---

## Message Size Limits

### Recommended Limits

- **Single message**: < 1 MB
- **Typical message**: < 100 KB
- **Large payloads**: Store in object storage (S3, GCS) and reference via URL in message

### Large Payload Pattern

```json
{
  "type": "JobComplete",
  "payload": {
    "status": "succeeded",
    "itemsProcessed": 1500000,
    "resultPath": "s3://bucket/results/job-123-output.parquet",
    "resultSize": "512 MB",
    "note": "Large result stored in object storage; see resultPath"
  }
}
```

---

## Security Considerations

### Tenant Isolation

- All topics automatically prefixed with `tenant.<tenantID>`
- Access control enforced at subscription level
- One tenant cannot subscribe to another tenant's topics

### Message Authentication

- Message `id` should be unique to prevent replay attacks
- `sourceAgent` and `sourceRole` help identify message origin
- Consider signing messages with HMAC for extra security

### Sensitive Data

- Avoid storing sensitive data in messages (use references instead)
- Use TLS for message bus connections
- Consider encryption at rest for message persistence

---

## Examples: Common Patterns

### Pattern 1: Sequential Agent Execution

<<<<<<< HEAD
<<<<<<< HEAD
```
=======
```json
>>>>>>> 630e387 (docs: add api-reference section with 4 files)
=======
```
>>>>>>> aaf5a84 (fix: change message-api JSON blocks to plain text (contains comments))
// Agent A (extract) publishes completion
{
  "type": "AgentComplete",
  "sourceRole": "extract",
  "payload": { "outputPath": "s3://bucket/extract.json" }
}

// Agent B (transform) subscribes and starts processing
// Agent B then publishes completion
{
  "type": "AgentComplete",
  "sourceRole": "transform",
  "payload": { "outputPath": "s3://bucket/transform.json" }
}

// Agent C (load) subscribes and completes pipeline
{
  "type": "JobComplete",
  "payload": { "status": "succeeded" }
}
```

### Pattern 2: Fan-Out/Fan-In

<<<<<<< HEAD
<<<<<<< HEAD
```
=======
```json
>>>>>>> 630e387 (docs: add api-reference section with 4 files)
=======
```
>>>>>>> aaf5a84 (fix: change message-api JSON blocks to plain text (contains comments))
// Coordinator publishes fan-out request
{
  "type": "FanOutRequest",
  "sourceRole": "coordinator",
  "payload": {
    "workItems": 100,
    "itemsPerWorker": 10
  }
}

// Each worker processes and publishes partial result
{
  "type": "PartialResult",
  "sourceRole": "worker",
  "sourceAgent": "worker-pod-1",
  "payload": { "itemsProcessed": 10, "result": { /* data */ } }
}

// All workers complete, aggregator collects and publishes final result
{
  "type": "JobComplete",
  "sourceRole": "aggregator",
  "payload": { "status": "succeeded", "totalProcessed": 100 }
}
```

---

## Related Documentation

- **[Usage: Multi-Agent Patterns](../usage/multi-agent-patterns.md)** — Orchestration patterns using message bus
- **[CRD Types](./crd-types.md)** — AgentJob and Tenant specs
- **[MCP Tools](./mcp-tools.md)** — Scheduling and monitoring tools

---

**Last Updated:** 2026-09-03  
**API Version**: v1  
**Message Schema Version**: 1.0
