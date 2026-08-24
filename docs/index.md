# Muto Documentation

Welcome to the Muto documentation! Muto is a multi-platform orchestration system for coordinating autonomous agents across Kubernetes and CloudFoundry environments.

## What is Muto?

Muto provides a unified framework for:

- **Multi-platform support**: Deploy and manage agents across Kubernetes and CloudFoundry
- **Agent coordination**: Implement complex orchestration patterns with role-based agents
- **Message-based communication**: Seamless inter-agent communication via message buses
- **Tenant isolation**: Secure multi-tenant environments with complete isolation
- **Scalable architecture**: Handle high-volume agent deployments

## Quick Links

- **[Getting Started](getting-started/overview.md)** — Start building with Muto
- **[Architecture](architecture/overview.md)** — Understand the design and concepts
- **[Testing](testing/overview.md)** — Learn about our testing infrastructure
- **[Deployment](deployment/k8s.md)** — Deploy to your platform
- **[Contributing](development/contributing.md)** — Join our community

## Key Features

### 🚀 Multi-Platform
Deploy the same agent orchestration logic to Kubernetes or CloudFoundry without code changes.

### 🔀 Flexible Coordination
Define complex agent coordination patterns with role-based agents and message bus communication.

### 🔒 Secure Isolation
Each tenant has complete isolation with dedicated namespaces and security boundaries.

### 📊 Observable
Built-in observability with structured logging and metrics for all agent interactions.

### ⚙️ Extensible
Custom reconcilers and plugins allow you to extend Muto for your specific needs.

## Platform Support

| Feature | Kubernetes | CloudFoundry |
|---------|:-----------:|:------------:|
| Agent Deployment | ✅ | ✅ |
| Multi-Agent Coordination | ✅ | ✅ |
| Message Bus Communication | ✅ | ✅ |
| Tenant Isolation | ✅ | ✅ |
| Auto-scaling | ✅ | ✅ |
| Health Monitoring | ✅ | ✅ |

## Getting Started

### For Users

1. **[Installation](getting-started/installation.md)** — Set up Muto in your environment
2. **[Quick Start](getting-started/quick-start.md)** — Deploy your first agent job
3. **[Architecture](architecture/overview.md)** — Understand how Muto works

### For Contributors

1. **[Setup](development/setup.md)** — Set up your development environment
2. **[Testing](testing/overview.md)** — Learn our testing strategy
3. **[Contributing](development/contributing.md)** — Contribute to Muto

## Community

- **GitHub**: [muto-io/muto](https://github.com/muto-io/muto)
- **Issues**: [Report bugs and request features](https://github.com/muto-io/muto/issues)
- **Discussions**: [Ask questions and share ideas](https://github.com/muto-io/muto/discussions)

## License

Muto is licensed under the Apache License 2.0. See [LICENSE](https://github.com/muto-io/muto/blob/main/LICENSE) for details.

---

**Last Updated**: 2024  
**Documentation Version**: 1.0.0
