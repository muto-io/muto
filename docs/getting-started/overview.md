# What is Muto?

Muto is a Kubernetes-native agent scheduler and orchestrator for multi-agent AI workloads.

> The name comes from the Godzilla universe: M.U.T.O. (Massive Unidentified Terrestrial Organism) — a creature that consumes energy and adapts. Fitting for a scheduler that consumes workloads and adapts to multi-tenant demand.

## The Problem

Coordinating multiple AI agents across distributed platforms is complex:

- **Multi-platform headache**: You need to support both Kubernetes and CloudFoundry, but they have different APIs and operational models
- **Coordination complexity**: Agents need to communicate and coordinate, but building message-based systems is error-prone
- **Tenant isolation**: In multi-tenant environments, you need complete isolation—compute, network, storage, messaging
- **Operational burden**: Monitoring, scaling, and maintaining agent workloads across platforms is time-consuming
- **Integration friction**: Existing orchestration tools don't understand AI agent patterns and state management

## What Muto Solves

Muto provides a unified framework for:

- **Multi-platform support**: Deploy and manage agents across Kubernetes and CloudFoundry with identical code
- **Seamless coordination**: Use structured messaging for reliable inter-agent communication
- **Tenant isolation**: Complete isolation guarantees at compute, network, storage, and messaging layers
- **Operational efficiency**: Built-in observability, health checks, and automated reconciliation
- **Extensibility**: Pluggable reconcilers and message bus implementations for custom needs
- **Kubernetes-native**: CRD-based definitions, controller pattern, standard Kubernetes tooling

## Who Should Use Muto

| Role | Use Case |
|------|----------|
| **AI/ML Engineers** | Build multi-agent workflows without worrying about orchestration plumbing |
| **Platform Teams** | Provide unified agent execution across Kubernetes and CloudFoundry |
| **SREs/Operators** | Operate agent workloads with built-in monitoring, scaling, and health checks |
| **DevOps Engineers** | Integrate agents into existing infrastructure with familiar tools (kubectl, Helm, etc.) |

## Key Features

### Multi-Platform Agnostic

Deploy the same agent orchestration logic to Kubernetes or CloudFoundry without code changes. Define jobs once, run anywhere.

### Flexible Coordination

Define complex agent coordination patterns:
- Sequential workflows (Agent A → Agent B → Agent C)
- Parallel execution (run agents concurrently)
- Fan-out/fan-in patterns (distribute work, aggregate results)
- Message-driven coordination (agents communicate via message bus)

### Secure Multi-Tenancy

Each tenant has complete isolation:
- Separate compute namespaces (K8s namespaces or CF spaces)
- Isolated messaging (tenant-scoped topic prefixes)
- Network policies and RBAC boundaries
- No cross-tenant data leakage

### Observable by Default

- Structured JSON logging for all operations
- Prometheus metrics for job status, throughput, latency
- Distributed tracing support (OpenTelemetry)
- Built-in dashboards and alerting patterns

### Extensible Architecture

- Custom reconcilers for domain-specific logic
- Pluggable message bus (NATS, Kafka, or custom)
- Webhook validation and mutation
- MCP server for Claude/LLM integration

### Production-Ready

- Declarative state management (Kubernetes reconciliation pattern)
- Automatic retries and error handling
- Health monitoring and liveness checks
- Horizontal scaling and load distribution

## Platform Support

| Feature | Kubernetes | CloudFoundry |
|---------|:-----------:|:------------:|
| Agent Deployment | ✅ | ✅ |
| Multi-Agent Coordination | ✅ | ✅ |
| Message Bus Communication | ✅ | ✅ |
| Tenant Isolation | ✅ | ✅ |
| Auto-scaling | ✅ | ✅ |
| Health Monitoring | ✅ | ✅ |
| Helm Charts | ✅ | - |
| Metrics Export | ✅ | ✅ |

## Architecture at a Glance

```
┌──────────────────────────────────────────────────────────────┐
│ Users/Claude (via MCP)                                        │
└────────────────────┬─────────────────────────────────────────┘
                     │ Schedule Jobs, Monitor Status
┌────────────────────▼─────────────────────────────────────────┐
│ Muto Operator (Kubernetes-native controller)                 │
│ ├─ Scheduler (state machine, job lifecycle)                  │
│ ├─ Reconcilers (TenantReconciler, AgentJobReconciler, ...)   │
│ └─ Event Watchers (watch K8s/CF for events)                  │
└────────┬───────────────────────────────────────────┬─────────┘
         │                                           │
         ▼                                           ▼
┌────────────────────┐                   ┌──────────────────────┐
│ Kubernetes         │                   │ CloudFoundry         │
│ ├─ CRDs            │                   │ ├─ Tasks             │
│ ├─ Namespaces      │                   │ ├─ Spaces            │
│ └─ Event Stream    │                   │ └─ Event Stream      │
└────────────────────┘                   └──────────────────────┘
         │                                           │
         └───────────┬───────────────────────────────┘
                     │
                     ▼
            ┌────────────────────┐
            │ Message Bus         │
            │ (NATS/Kafka)       │
            │ Tenant-scoped      │
            │ topics             │
            └────────────────────┘
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
    ┌─────────┐            ┌──────────┐
    │ Agent A │            │ Agent B  │
    └─────────┘            └──────────┘
```

## Next Steps

1. **[Core Concepts](../getting-started/concepts.md)** — Understand key Muto concepts
2. **[Quick Start](../getting-started/quick-start.md)** — Get running in 5 minutes
3. **[Installation](../getting-started/installation.md)** — Detailed setup instructions
4. **Architecture Overview** — Deep dive into system design (coming in Phase 2)

---

**Last Updated:** 2026-09-03
