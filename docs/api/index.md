# Muto API Documentation

Welcome to the Muto API Reference. This documentation covers all APIs for interacting with Muto: Kubernetes CRDs, messaging protocols, webhooks, and MCP tools for LLM integration.

---

## API Overview

Muto provides multiple APIs for different use cases:

| API | Purpose | Audience | Type |
|-----|---------|----------|------|
| **[CRD API](#crd-api)** | Define and manage agent jobs, tenants, and fleets | Kubernetes users | REST (via kubectl) |
| **[Message API](#message-api)** | Inter-agent communication and message bus protocol | Agent developers | Message-oriented |
| **[Webhook API](#webhook-api)** | Event notifications and job lifecycle hooks | Integration engineers | REST webhooks |
| **[MCP Tools](#mcp-tools)** | Schedule and monitor jobs from Claude and LLM clients | LLM applications | MCP protocol |

---

## CRD API

The CRD API defines Kubernetes Custom Resources for Muto. Use these to declare agent workloads, define tenants, and configure scheduling behavior.

### Core Resources

**Tenant** — A logical boundary with isolated compute, messaging, and RBAC
- **Kind**: `muto.io/v1alpha1/Tenant`
- **Scope**: Cluster
- **Purpose**: Define a scheduling tenant with isolation configuration
- **Example**: 
  ```yaml
  apiVersion: muto.io/v1alpha1
  kind: Tenant
  metadata:
    name: my-tenant
  spec:
    namespace: agents-my-tenant
    isolationTier: dedicated
    messageBus:
      type: nats
  ```

**AgentJob** — A unit of agent work to be scheduled
- **Kind**: `muto.io/v1alpha1/AgentJob`
- **Scope**: Namespaced
- **Purpose**: Trigger and manage one or more agents
- **Example**:
  ```yaml
  apiVersion: muto.io/v1alpha1
  kind: AgentJob
  metadata:
    name: example-job
  spec:
    tenantRef: my-tenant
    trigger:
      type: manual
    agents:
      - role: coordinator
        image: myregistry/coordinator:latest
        maxReplicas: 1
  ```

**AgentFleet** — Group related AgentJobs for coordinated operations
- **Kind**: `muto.io/v1alpha1/AgentFleet`
- **Scope**: Namespaced
- **Purpose**: Manage collections of agent jobs as a single unit
- **Example**:
  ```yaml
  apiVersion: muto.io/v1alpha1
  kind: AgentFleet
  metadata:
    name: my-fleet
  spec:
    tenantRef: my-tenant
    jobRefs:
      - job-1
      - job-2
  ```

### Key Concepts

**Isolation Tiers**
- `shared`: Tenant shares the controller's message bus infrastructure
- `dedicated`: Tenant gets its own message bus instance

**Trigger Types**
- `event`: Job triggered by incoming message on a topic
- `cron`: Job triggered on a schedule (source is a cron expression)
- `manual`: Job triggered explicitly via API or MCP tool

**Job Phases**
- `Pending`: Job created, agents not yet spawned
- `Running`: Agents are active
- `Succeeded`: All agents completed successfully
- `Failed`: One or more agents failed
- `Terminating`: Cleanup in progress, pods being deleted

### Full Reference

📖 **[CRD Type Reference](../api-reference/crd-types.md)** — Complete field specifications, validation rules, and examples

**Useful kubectl Commands**
```bash
# View CRD definitions
kubectl get crd | grep muto.io

# Explain resource fields
kubectl explain agentjob.spec
kubectl explain agentjob.spec.agents
kubectl explain tenant.spec.isolationTier

# Create resources
kubectl apply -f tenant.yaml
kubectl apply -f agentjob.yaml

# Query resources
kubectl get agentjobs -n agents-my-tenant
kubectl describe agentjob my-job -n agents-my-tenant
```

---

## Message API

The Message API defines inter-agent communication protocols and message bus semantics. Agents use this API to send and receive messages on the message bus.

### Protocol Overview

- **Transport**: NATS JetStream, Apache Kafka, or RabbitMQ
- **Message Format**: JSON with metadata headers
- **Routing**: Topic-based pub/sub with tenant isolation
- **Guarantees**: At-least-once delivery (with idempotency support)

### Message Structure

```json
{
  "messageId": "msg-uuid",
  "timestamp": "2026-09-05T12:00:00Z",
  "sender": "coordinator",
  "topic": "tenant.my-tenant.work-queue",
  "headers": {
    "correlation-id": "job-123",
    "priority": "high"
  },
  "payload": {
    "task": "process-data",
    "data": {...}
  }
}
```

### Message Bus Types

| Bus | Latency | Throughput | Durability | Use Case |
|-----|---------|-----------|-----------|----------|
| **NATS JetStream** | Very Low | Medium | Configurable | Low-latency, simple agent tasks |
| **Apache Kafka** | Low | Very High | High | Complex pipelines, high throughput |
| **RabbitMQ** | Low | Medium | High | Traditional queue patterns |

### Full Reference

📖 **[Message API Reference](../api-reference/message-api.md)** — Complete message format, routing rules, and examples

---

## Webhook API

The Webhook API sends event notifications when agent jobs change state. Configure webhooks to integrate Muto with external systems.

### Event Types

- **job.created**: AgentJob resource created
- **job.started**: Job transitioned to Running phase
- **job.completed**: Job reached terminal phase (Succeeded or Failed)
- **job.failed**: Job transitioned to Failed phase
- **fleet.updated**: Fleet composition or status changed

### Webhook Payload

```json
{
  "event": "job.completed",
  "timestamp": "2026-09-05T12:00:00Z",
  "jobId": "my-job",
  "jobNamespace": "agents-my-tenant",
  "tenantRef": "my-tenant",
  "phase": "Succeeded",
  "activeAgents": 0,
  "startedAt": "2026-09-05T11:50:00Z",
  "completedAt": "2026-09-05T12:00:00Z"
}
```

### Webhook Configuration

Define webhooks in your Muto controller deployment:

```yaml
MUTO_WEBHOOKS_ENABLED: "true"
MUTO_WEBHOOK_URLS: |
  https://your-system.com/muto-webhooks
MUTO_WEBHOOK_EVENTS: job.created,job.completed,job.failed
```

### Full Reference

📖 **[Webhook API Reference](../api-reference/webhook-api.md)** — Complete webhook specification, retry behavior, and integration examples

---

## MCP Tools

The MCP (Model Context Protocol) Tools API enables Claude and other LLM clients to interact with Muto. Use these tools in your Claude sessions to schedule jobs, check status, and manage workloads.

### Available Tools

**ScheduleJob** — Create and schedule a new agent job
```json
{
  "tool": "ScheduleJob",
  "parameters": {
    "tenantRef": "my-tenant",
    "trigger": "manual",
    "agents": [
      {
        "role": "coordinator",
        "image": "myregistry/coordinator:latest"
      }
    ]
  }
}
```

**GetJobStatus** — Retrieve current status of a job
```json
{
  "tool": "GetJobStatus",
  "parameters": {
    "jobId": "my-job",
    "jobNamespace": "agents-my-tenant"
  }
}
```

**ListJobs** — List active jobs for a tenant
```json
{
  "tool": "ListJobs",
  "parameters": {
    "tenantRef": "my-tenant",
    "phase": "Running"
  }
}
```

**CancelJob** — Cancel a running or pending job
```json
{
  "tool": "CancelJob",
  "parameters": {
    "jobId": "my-job",
    "jobNamespace": "agents-my-tenant"
  }
}
```

**DescribeTenant** — Get tenant configuration and status
```json
{
  "tool": "DescribeTenant",
  "parameters": {
    "tenantRef": "my-tenant"
  }
}
```

### MCP Server Setup

Install the Muto MCP server in your Claude or LLM environment:

```bash
# Via npm (recommended)
npm install @muto-io/mcp-server

# Via Claude Code
claude mcp add @muto-io/mcp-server --config "serverUrl=http://localhost:3000"
```

### Full Reference

📖 **[MCP Tools Reference](../api-reference/mcp-tools.md)** — Complete tool specifications, authentication, and integration examples

---

## API Reference Documents

For detailed specifications, use these reference documents:

| Document | Coverage | Best For |
|----------|----------|----------|
| [CRD Type Reference](../api-reference/crd-types.md) | Complete Kubernetes CRD schemas with all fields and validation | Defining resource manifests, understanding field constraints |
| [Message API Reference](../api-reference/message-api.md) | Message format, routing, headers, and protocol behavior | Building agents, implementing message handlers |
| [Webhook API Reference](../api-reference/webhook-api.md) | Webhook configuration, event types, retry logic | Setting up event notifications and integrations |
| [MCP Tools Reference](../api-reference/mcp-tools.md) | Tool specifications, parameters, and response formats | LLM integration, scheduling from Claude |

---

## Getting Started with APIs

### For Kubernetes Users

1. Review [Concepts](../getting-started/concepts.md) to understand terminology
2. Read [CRD Type Reference](../api-reference/crd-types.md) to learn resource fields
3. Check [Usage Examples](../usage/examples/) for real-world YAML manifests
4. Follow [Scheduling Agent Jobs](../usage/scheduling-agent-jobs.md) for practical guidance

### For Integration Engineers

1. Read [Webhook API Reference](../api-reference/webhook-api.md)
2. Configure webhook endpoints in your Muto deployment
3. Handle events in your external system
4. Use [Troubleshooting Guide](../operations/troubleshooting.md) if webhooks don't fire

### For LLM Application Developers

1. Install the [MCP Server](../api-reference/mcp-tools.md#mcp-server-setup)
2. Review [MCP Tools Reference](../api-reference/mcp-tools.md) for available tools
3. Use the tools in your Claude or LLM session to schedule and monitor jobs
4. See [Multi-Agent Patterns](../usage/multi-agent-patterns.md) for advanced coordination

### For Agent Developers

1. Study [Message API Reference](../api-reference/message-api.md)
2. Implement message handlers in your agent code
3. Review [Usage Examples](../usage/examples/) for reference implementations
4. Check [Architecture: Messaging](../architecture/messaging.md) for design patterns

---

## OpenAPI Specification

The Muto CRD API is fully described in OpenAPI 3.0 format for tooling integration:

📄 **[openapi.yaml](./openapi.yaml)** — Machine-readable OpenAPI spec for Muto CRDs

Use this spec with tools like:
- Swagger UI for interactive documentation
- Code generators (openapi-generator, kubebuilder)
- API mocking and testing frameworks

---

## API Versions & Stability

| API | Version | Status | Stability |
|-----|---------|--------|-----------|
| CRD API | `muto.io/v1alpha1` | Alpha | Unstable (breaking changes possible) |
| Message API | `v1` | Stable | Stable |
| Webhook API | `v1` | Stable | Stable |
| MCP Tools | `v1` | Stable | Stable |

The CRD API is in alpha and may change. Stable APIs (Message, Webhook, MCP) maintain backward compatibility.

---

## API Rate Limits & Quotas

| Resource | Limit | Notes |
|----------|-------|-------|
| Jobs per tenant | 1,000 | Configurable via MUTO_MAX_JOBS_PER_TENANT |
| Message rate | Unlimited | Limited by message bus capacity |
| Webhook retries | 5 | Configurable via MUTO_WEBHOOK_MAX_RETRIES |
| MCP tool calls | Unlimited | Limited by MCP server resources |

---

## Error Handling

All APIs use standard HTTP status codes:

| Code | Meaning | Example |
|------|---------|---------|
| 200 | Success | Job created, status retrieved |
| 201 | Created | New resource created |
| 400 | Bad Request | Invalid YAML, missing required fields |
| 401 | Unauthorized | Missing or invalid authentication |
| 403 | Forbidden | Insufficient permissions for resource |
| 404 | Not Found | Job or tenant does not exist |
| 409 | Conflict | Resource already exists or state conflict |
| 500 | Server Error | Internal server error |

---

## Support & Issues

**Having trouble with the APIs?**

- Check the [Troubleshooting Guide](../operations/troubleshooting.md)
- Review relevant [API Reference](../api-reference/)
- Search [GitHub Issues](https://github.com/muto-io/muto/issues)
- Ask in [GitHub Discussions](https://github.com/muto-io/muto/discussions)

---

## Next Steps

- 📖 Read the [Getting Started Guide](../getting-started/quick-start.md)
- 🏗️ Review [Architecture Documentation](../architecture/)
- 💡 Explore [Usage Examples](../usage/examples/)
- 🔧 Check [Configuration Reference](../configuration/)
