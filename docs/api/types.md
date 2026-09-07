# API Types Reference

This page provides an overview of Muto API types and schemas. For detailed specifications, see the complete reference documentation.

## Type Categories

### Core Resource Types

Muto defines several Kubernetes Custom Resource Definitions (CRDs) for managing agent workloads:

- **Tenant** — A logical boundary with isolated compute, messaging, and RBAC
- **AgentJob** — A unit of agent work to be scheduled
- **AgentFleet** — Groups related AgentJobs for coordinated operations

### Type Structures

#### Tenant Type
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

**Fields:**
- `namespace`: Kubernetes namespace for this tenant's agents
- `isolationTier`: `shared` or `dedicated` message bus isolation
- `messageBus.type`: Message bus implementation (nats, kafka, rabbitmq)

#### AgentJob Type
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

**Fields:**
- `tenantRef`: Reference to the parent Tenant
- `trigger.type`: `event`, `cron`, or `manual`
- `agents`: List of agent specifications

#### AgentFleet Type
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

**Fields:**
- `tenantRef`: Reference to the parent Tenant
- `jobRefs`: List of AgentJob references

## Type Versions

| Type | Version | Status | Stability |
|------|---------|--------|-----------|
| Tenant | `muto.io/v1alpha1` | Alpha | Unstable |
| AgentJob | `muto.io/v1alpha1` | Alpha | Unstable |
| AgentFleet | `muto.io/v1alpha1` | Alpha | Unstable |

## Detailed Reference

For complete field specifications, validation rules, and detailed examples, see:

[:octicons-book-24: **CRD Type Reference**](../api-reference/crd-types.md) — Complete Kubernetes CRD schemas with all fields and validation rules

## Related Documentation

- **[Message API Types](../api-reference/message-api.md)** — Message structure and headers
- **[Webhook API Types](../api-reference/webhook-api.md)** — Event and webhook payload types
- **[Architecture Documentation](../architecture/)** — Design and concepts behind the types
