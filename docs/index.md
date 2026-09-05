# Muto Documentation Hub

Welcome to the **Muto** documentation! Muto is a multi-platform agent orchestration system for coordinating autonomous agents across Kubernetes and CloudFoundry environments.

> **M.U.T.O.** — Massive Unidentified Terrestrial Orchestrator: A system that consumes workloads and adapts to multi-tenant, multi-platform demands.

## What is Muto?

Muto provides a unified framework for:

- **Multi-platform support**: Deploy and manage agents across Kubernetes and CloudFoundry seamlessly
- **Agent coordination**: Implement complex orchestration patterns with role-based agents
- **Message-based communication**: Seamless inter-agent communication via message buses (NATS, RabbitMQ, Kafka)
- **Tenant isolation**: Secure multi-tenant environments with complete compute, network, and messaging isolation
- **Scalable architecture**: Handle high-volume agent deployments with built-in reconciliation and health management
- **Extensibility**: Custom reconcilers and plugins for domain-specific orchestration logic

---

## Getting Started Quickly

**First time here?** Start with these entry points:

1. **[What is Muto?](getting-started/overview.md)** — Understand the problems Muto solves
2. **[Quick Start](getting-started/quick-start.md)** — Deploy your first agent job in minutes
3. **[Installation Guide](getting-started/installation.md)** — Set up Muto in your environment
4. **[Key Concepts](getting-started/concepts.md)** — Learn core terminology
5. **[Contributing](../CONTRIBUTING.md)** — Join our community and help improve Muto

---

## Documentation Map

### 1. Getting Started

Start here if you're new to Muto. Learn the basics, understand concepts, and run your first example.

| File | Purpose | Audience |
|------|---------|----------|
| [Overview](getting-started/overview.md) | What Muto is, why it exists, and the problems it solves | Everyone |
| [Concepts](getting-started/concepts.md) | Core terminology: agents, reconcilers, message buses, tenants | Everyone |
| [Quick Start](getting-started/quick-start.md) | Deploy your first agent job in 10 minutes | Users |
| [Installation](getting-started/installation.md) | Install Muto for Kubernetes or CloudFoundry | Users |
| [Architecture Overview](getting-started/architecture-overview.md) | High-level architecture and design patterns | Engineers |

**Next steps:** After reading the overview, jump to [Quick Start](getting-started/quick-start.md) or [Installation](getting-started/installation.md) depending on your role.

---

### 2. Architecture

Deep dive into Muto's design. Understand messaging, reconciliation, multi-platform support, and security.

| File | Purpose | Audience |
|------|---------|----------|
| [Agent Lifecycle](architecture/agent-lifecycle.md) | Agent state machine and lifecycle transitions | Engineers |
| [Messaging](architecture/messaging.md) | Message bus design, communication patterns, guarantees | Engineers |
| [Reconcilers](architecture/reconcilers.md) | Reconciliation framework and custom reconciler development | Engineers |
| [Platform Design](architecture/platform-design.md) | Multi-platform abstraction layer (Kubernetes + CloudFoundry) | Engineers |
| [Security Model](architecture/security-model.md) | Tenant isolation, RBAC, encryption, audit logging | Architects |

**Next steps:** Start with [Platform Design](architecture/platform-design.md), then explore [Messaging](architecture/messaging.md), [Reconcilers](architecture/reconcilers.md), and [Security Model](architecture/security-model.md).

---

### 3. Deployment

Deploy Muto to production on Kubernetes or CloudFoundry. Includes installation guides, configuration, and pre-launch checklists.

| File | Purpose | Audience |
|------|---------|----------|
| **Kubernetes** | | |
| [Kubernetes Deployment](deployment/k8s.md) | Kubernetes deployment overview | Operators |
| [K8s Installation](deployment/kubernetes/install.md) | Step-by-step Kubernetes installation | Operators |
| [K8s Configuration](deployment/kubernetes/configuration.md) | Kubernetes-specific configuration | Operators |
| [Helm Chart](deployment/kubernetes/helm-chart.md) | Helm chart overview and usage | Operators |
| **CloudFoundry** | | |
| [CloudFoundry Deployment](deployment/cf.md) | CloudFoundry deployment overview | Operators |
| [CF Installation](deployment/cloudfoundry/install.md) | Step-by-step CloudFoundry installation | Operators |
| [CF Configuration](deployment/cloudfoundry/configuration.md) | CloudFoundry-specific configuration | Operators |
| **General** | | |
| [Production Checklist](deployment/production-checklist.md) | Pre-launch validation and readiness checklist | Operators |
| [Helm Chart Reference](deployment/helm.md) | General Helm chart reference | Operators |

**Next steps:** Choose your platform ([Kubernetes](deployment/kubernetes/install.md) or [CloudFoundry](deployment/cloudfoundry/install.md)), then review the [Production Checklist](deployment/production-checklist.md) before going live.

---

### 4. Configuration

Configure Muto for your environment. Set up message buses, define reconcilers, enable TLS, and configure multi-tenancy.

| File | Purpose | Audience |
|------|---------|----------|
| [Environment Variables](configuration/environment-variables.md) | Complete reference of all configuration variables | Operators |
| [Message Bus Setup](configuration/message-bus-setup.md) | Configure NATS, RabbitMQ, or Kafka | Operators |
| [Reconciler Configuration](configuration/reconciler-config.md) | Configure and extend reconcilers | Engineers |
| [TLS & Security](configuration/tls-security.md) | Enable encryption, mTLS, and secure communication | Operators |
| [Multi-Tenant Setup](configuration/multi-tenant-setup.md) | Configure tenant isolation and policies | Operators |

**Next steps:** Start with [Environment Variables](configuration/environment-variables.md), then configure [Message Bus](configuration/message-bus-setup.md) and [TLS](configuration/tls-security.md) based on your deployment model.

---

### 5. Usage

Learn how to use Muto once it's deployed. Includes examples, patterns, and best practices.

| File | Purpose | Audience |
|------|---------|----------|
| [Best Practices](usage/best-practices.md) | Design patterns and operational best practices | Users |
| [Scheduling Agent Jobs](usage/scheduling-agent-jobs.md) | Define and schedule agent workloads | Users |
| [Multi-Agent Patterns](usage/multi-agent-patterns.md) | Build complex multi-agent workflows | Users |
| **Examples** | | |
| [Simple Job](usage/examples/simple-job.md) | "Hello World" example—a basic agent job | Users |
| [Custom Reconciler](usage/examples/custom-reconciler.md) | Build a custom reconciler for domain logic | Engineers |
| [Multi-Agent Workflow](usage/examples/multi-agent-workflow.md) | Orchestrate multiple agents with message passing | Users |

**Next steps:** Read [Best Practices](usage/best-practices.md), then work through [Simple Job](usage/examples/simple-job.md), [Scheduling](usage/scheduling-agent-jobs.md), and [Multi-Agent Patterns](usage/multi-agent-patterns.md).

---

### 6. API Reference

Complete API documentation: CRD types, message API, webhook API, and MCP tools.

| File | Purpose | Audience |
|------|---------|----------|
| [API Documentation](api/index.md) | Complete API overview, guides, and integration examples | Everyone |
| [CRD Types](api-reference/crd-types.md) | Agent, Job, Reconciler CRD definitions and fields | Engineers |
| [Message API](api-reference/message-api.md) | Message bus protocol and message formats | Engineers |
| [Webhook API](api-reference/webhook-api.md) | Webhook endpoints and event notifications | Engineers |
| [MCP Tools](api-reference/mcp-tools.md) | Model Context Protocol tools and integration | Engineers |

**Next steps:** Start with [API Documentation](api/index.md) for an overview, then dive into [CRD Types](api-reference/crd-types.md) to understand the data model and explore [Message API](api-reference/message-api.md) and [Webhook API](api-reference/webhook-api.md) for communication patterns.

---

### 7. Development

Develop, test, and contribute to Muto. Includes setup, testing strategy, debugging, and contribution guidelines.

| File | Purpose | Audience |
|------|---------|----------|
| [Setup](development/setup.md) | Set up your development environment | Contributors |
| [Testing Strategy](development/testing-strategy.md) | Testing philosophy, structure, and best practices | Contributors |
| [Contributing](development/contributing.md) | Contribution guidelines, PR process, code review standards | Contributors |
| [Debugging](development/debugging.md) | Debug Muto locally and in production | Engineers |
| [Style Guide](development/style.md) | Code style, naming conventions, and standards | Contributors |

**Related:** Testing files are organized under [Testing](#8-testing-3-files-787-lines).

**Next steps:** Read [Setup](development/setup.md), then review [Testing Strategy](development/testing-strategy.md) and [Contributing](development/contributing.md).

---

### 8. Operations

Run Muto in production. Includes monitoring, troubleshooting, performance tuning, backup/recovery, and FAQs.

| File | Purpose | Audience |
|------|---------|----------|
| [Monitoring & Observability](operations/monitoring-observability.md) | Metrics, logs, tracing, and health checks | Operators |
| [Troubleshooting](operations/troubleshooting.md) | Common issues, diagnostics, and solutions | Operators |
| [Performance Tuning](operations/performance-tuning.md) | Optimize throughput, latency, and resource usage | Operators |
| [Backup & Recovery](operations/backup-recovery.md) | Back up state, recover from failures | Operators |
| [FAQs](operations/faqs.md) | Frequently asked questions and quick answers | Everyone |

**Next steps:** After deployment, review [Monitoring & Observability](operations/monitoring-observability.md) and [Troubleshooting](operations/troubleshooting.md). Use [FAQs](operations/faqs.md) for quick answers.

---

### Bonus: Testing

Testing infrastructure and analysis. Part of the development workflow.

| File | Purpose | Audience |
|------|---------|----------|
| [E2E Tests](testing/e2e-tests.md) | End-to-end testing and validation | Contributors |
| [Performance Analysis](testing/performance-analysis.md) | Performance benchmarks and analysis | Engineers |
| [Optimization Roadmap](testing/optimization-roadmap.md) | Future testing improvements and roadmap | Contributors |

**Next steps:** See [Development/Setup](development/setup.md) for test environment setup, then review the [Optimization Roadmap](testing/optimization-roadmap.md).

---

## Documentation at a Glance

| Section | Files | Lines | Purpose | Start Here |
|---------|-------|-------|---------|-----------|
| **Getting Started** | 5 | 1,068 | Learn Muto basics | [Overview](getting-started/overview.md) |
| **Architecture** | 5 | 2,689 | Understand the design | [Platform Design](architecture/platform-design.md) |
| **Deployment** | 9 | 506 | Deploy to production | [K8s](deployment/kubernetes/install.md) / [CF](deployment/cloudfoundry/install.md) |
| **Configuration** | 5 | 3,726 | Configure your setup | [Environment Variables](configuration/environment-variables.md) |
| **Usage** | 6 | 2,123 | Use Muto | [Best Practices](usage/best-practices.md) |
| **API Reference** | 4 | 2,932 | API details | [CRD Types](api-reference/crd-types.md) |
| **Development** | 5 | 2,402 | Develop & contribute | [Setup](development/setup.md) |
| **Operations** | 5 | 2,941 | Run in production | [Monitoring](operations/monitoring-observability.md) |
| **Testing** | 3 | 787 | Test & validate | [Performance Analysis](testing/performance-analysis.md) |
| **TOTAL** | **47** | **19,174** | Complete Muto docs | — |

---

## Navigation by Audience

### I'm a User — New to Muto
Start here and work through in order:
1. [What is Muto?](getting-started/overview.md)
2. [Concepts](getting-started/concepts.md)
3. [Quick Start](getting-started/quick-start.md)
4. [Scheduling Agent Jobs](usage/scheduling-agent-jobs.md)
5. [Best Practices](usage/best-practices.md)
6. [Troubleshooting](operations/troubleshooting.md) (when needed)

**Typical path:** Getting Started -> Usage -> Operations

---

### I'm an Operator — Deploying Muto
Follow this path:
1. [Platform Design](architecture/platform-design.md) (understand the system)
2. **Choose your platform:**
   - [Kubernetes Installation](deployment/kubernetes/install.md) OR
   - [CloudFoundry Installation](deployment/cloudfoundry/install.md)
3. [Environment Variables](configuration/environment-variables.md)
4. [Message Bus Setup](configuration/message-bus-setup.md)
5. [TLS & Security](configuration/tls-security.md)
6. [Production Checklist](deployment/production-checklist.md)
7. [Monitoring & Observability](operations/monitoring-observability.md)

**Typical path:** Architecture -> Deployment -> Configuration -> Operations

---

### I'm an Engineer — Building with Muto
Follow this path:
1. [Platform Design](architecture/platform-design.md)
2. [Messaging](architecture/messaging.md)
3. [Reconcilers](architecture/reconcilers.md)
4. [API Reference](api-reference/crd-types.md)
5. [Usage Examples](usage/examples/) (start with simple-job.md)
6. [Custom Reconciler Example](usage/examples/custom-reconciler.md)

**Typical path:** Architecture -> API Reference -> Usage Examples -> Development/Debugging

---

### I'm a Contributor — Developing Muto
Follow this path:
1. [Development Setup](development/setup.md)
2. [Testing Strategy](development/testing-strategy.md)
3. [Contributing Guidelines](development/contributing.md)
4. [Platform Design](architecture/platform-design.md) (for context)
5. **Then:**
   - For feature work: [Relevant architecture docs](architecture/) + [Usage examples](usage/examples/)
   - For testing: [Performance Analysis](testing/performance-analysis.md)
   - For debugging: [Debugging Guide](development/debugging.md)
6. [Style Guide](development/style.md) (before submitting PRs)

**Typical path:** Setup -> Testing -> Contributing -> Development -> (Architecture/Usage as needed)

---

### I'm an Architect — Evaluating Muto
Read these to understand Muto's design and capabilities:
1. [What is Muto?](getting-started/overview.md) (the problem)
2. [Platform Design](architecture/platform-design.md) (the solution)
3. [Security Model](architecture/security-model.md) (isolation and security)
4. [Production Checklist](deployment/production-checklist.md) (production readiness)
5. [Performance Analysis](testing/performance-analysis.md) (scalability)

**Typical path:** Getting Started -> Architecture -> Operations

---

## Quick Reference

### Links to Key Resources

**Community & Support:**
- [Contributing Guidelines](../CONTRIBUTING.md) — How to contribute
- [GitHub Issues](https://github.com/muto-io/muto/issues) — Report bugs or request features
- [GitHub Discussions](https://github.com/muto-io/muto/discussions) — Ask questions and share ideas

**Important Docs:**
- [Production Checklist](deployment/production-checklist.md) — Before going live
- [Troubleshooting Guide](operations/troubleshooting.md) — When things go wrong
- [FAQs](operations/faqs.md) — Quick answers
- [Best Practices](usage/best-practices.md) — Design patterns and tips

**Configuration:**
- [All Environment Variables](configuration/environment-variables.md) — Complete config reference
- [Message Bus Setup](configuration/message-bus-setup.md) — Choose and configure a message bus
- [Security & TLS](configuration/tls-security.md) — Enable encryption

---

## Platform Support

| Feature | Kubernetes | CloudFoundry |
|---------|:-----------:|:------------:|
| Agent Deployment | ✅ | ✅ |
| Multi-Agent Coordination | ✅ | ✅ |
| Message Bus Communication | ✅ | ✅ |
| Tenant Isolation | ✅ | ✅ |
| Auto-scaling | ✅ | ✅ |
| Health Monitoring | ✅ | ✅ |
| Helm Chart | ✅ | ❌ |
| Custom Metrics | ✅ | ✅ |

---

## Documentation Statistics

| Metric | Value |
|--------|-------|
| Total Files | 47 |
| Total Lines | 19,174 |
| Main Sections | 8 |
| Code Examples | 3+ |
| Last Updated | 2026-09-03 |
| Version | 1.0.0 |

---

## How to Use This Documentation

1. **Find What You Need:** Use the [Documentation Map](#documentation-map) above to locate topics by section
2. **Follow Your Path:** Use [Navigation by Audience](#navigation-by-audience) to find a guided path for your role
3. **Search & Link:** All files link to related topics. Use your browser's find (Ctrl+F / Cmd+F) to search
4. **Ask for Help:** Visit [GitHub Issues](https://github.com/muto-io/muto/issues) or [Discussions](https://github.com/muto-io/muto/discussions)

---

## Contributing to Documentation

Found an error? Have a suggestion? Help us improve!

1. **Report Issues:** [Create a GitHub Issue](https://github.com/muto-io/muto/issues/new)
2. **Contribute:** See [CONTRIBUTING.md](../CONTRIBUTING.md) for how to submit changes
3. **Style Guide:** Follow [development/style.md](development/style.md) for consistency

---

## License & Attribution

Muto is licensed under the **Apache License 2.0**. See [LICENSE](https://github.com/muto-io/muto/blob/main/LICENSE) for details.

---

**Documentation Version:** 1.0.0  
**Last Updated:** 2026-09-03  
**Generated with:** Muto Documentation Hub Generator Phase 9

---

## Table of Contents

1. [Quick Links](#getting-started-quickly)
2. [Documentation Map](#documentation-map)
   - [Getting Started](#1-getting-started)
   - [Architecture](#2-architecture)
   - [Deployment](#3-deployment)
   - [Configuration](#4-configuration)
   - [Usage](#5-usage)
   - [API Reference](#6-api-reference)
   - [Development](#7-development)
   - [Operations](#8-operations)
   - [Testing](#bonus-testing)
3. [At a Glance](#documentation-at-a-glance)
4. [Navigation by Audience](#navigation-by-audience)
5. [Quick Reference](#quick-reference)
6. [Platform Support](#platform-support)
7. [How to Use](#how-to-use-this-documentation)
