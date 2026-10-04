# Security Model

What Muto's actual tenant-isolation and access-control boundaries are today, and what's on you to add. This is the architectural summary; the authoritative detail (and the exact code/Helm references backing each claim) lives in [Security Configuration](../configuration/tls-security.md) and [Multi-Tenant Setup](../configuration/multi-tenant-setup.md) - this page links to those rather than duplicating their content, so it can't drift out of sync with them again.

## Tenant Isolation

- **Kubernetes**: each `Tenant` gets a namespace (created by `TenantReconciler` if it doesn't exist) labeled `muto.io/tenant: <tenant-name>`. Agent pods carry the same label. That's the boundary - no RBAC, NetworkPolicy, ResourceQuota, or pod security hardening is created automatically. See [Multi-Tenant Setup](../configuration/multi-tenant-setup.md) for what the reconciler actually does and what you have to add yourself.
- **CloudFoundry**: driven by the operator's `CF_ISOLATION_TIER`/`CF_SHARED_ORG` environment variables, not a per-`Tenant` field - `dedicated` gives each tenant its own CF org with a fixed `muto-agents` space; `shared` gives each tenant its own space within one shared org. Muto resolves/caches org and space GUIDs; it doesn't create them for you. See [Multi-Tenant Setup](../configuration/multi-tenant-setup.md#cloudfoundry-isolation).
- **Message bus topics**: `core/tenant.TopicPrefix` applies a `tenant.<tenantID>.` prefix when a job is scheduled through the MCP server. This is a naming convention, not an enforced boundary - nothing in the NATS/Kafka backends rejects a cross-tenant publish/subscribe. See [Message Bus Setup](../configuration/message-bus-setup.md#topic-naming).

## Access Control

Muto has no authentication or authorization layer of its own - no OIDC integration, no custom auth, no per-tenant user/role provisioning. Access control is entirely whatever the underlying platform already provides:

- **Kubernetes**: standard Kubernetes RBAC governs who can call the K8s API directly (`kubectl`, service account tokens, CI pipelines). Muto doesn't create per-tenant `Role`/`RoleBinding`/`ServiceAccount` objects - if you want tenant users scoped to their own namespace, you author those yourself (examples in [Multi-Tenant Setup](../configuration/multi-tenant-setup.md#what-you-still-have-to-set-up-yourself)). The operator's *own* permissions come from a static Helm-templated `ClusterRole` - see [Security Configuration](../configuration/tls-security.md#kubernetes-rbac) for the exact granted permissions.
- **CloudFoundry**: standard CF space roles (`SpaceManager`, `SpaceDeveloper`, etc.) via `cf set-space-role` - ordinary CF administration, not something Muto manages.
- **No audit trail**: Muto doesn't emit structured audit events (who did what, when). The operator's own structured logs (`MUTO_LOG_FORMAT`/`MUTO_LOG_LEVEL`, see [Environment Variables](../configuration/environment-variables.md)) record its own reconcile activity, not user-attributed access; for actual K8s API audit records, use the Kubernetes API server's own audit log.

## Encryption in Transit

- Kubernetes API traffic (`kubectl`, the operator's own client-go calls) gets whatever TLS `kube-apiserver` already terminates - unrelated to Muto, works the same with or without it installed.
- CloudFoundry API traffic gets whatever TLS the `CF_API_URL` endpoint provides; there's no app-level TLS configuration in the CF adapter.
- Muto itself ships no TLS or mTLS of its own, no admission webhooks, and no cert-manager integration. See [Security Configuration](../configuration/tls-security.md#known-limitations) for the full list and what to use instead (service mesh, ingress-level TLS termination, NetworkPolicy).
- Message bus connections (NATS/Kafka) have no TLS support implemented. See [Message Bus Setup](../configuration/message-bus-setup.md).

## Secrets

- CF credentials (`CF_API_URL`/`CF_USERNAME`/`CF_PASSWORD`) are plain environment variables with no built-in Vault/CredHub integration - see [Security Configuration](../configuration/tls-security.md#secrets) for sourcing them from a Kubernetes `Secret`.
- The one secret Muto generates itself is `muto-a2a-token`: a random 32-byte hex token created per A2A-enabled tenant, injected into agent pods via `secretKeyRef` (see [Multi-Tenant Setup](../configuration/multi-tenant-setup.md#what-the-reconciler-does-on-create)). There's no rotation - it's generated once and left in place.

## Pod Security

Agent pods get no `securityContext` from Muto - no `runAsNonRoot`, no dropped capabilities, no read-only root filesystem. If you need Pod Security Standards enforced, apply them at the cluster level (the built-in `PodSecurity` admission controller, or a policy engine like Kyverno/OPA Gatekeeper) - Muto doesn't set or require any particular security posture on the pods it creates.

## What's Not Here

Muto has no automatic resource quotas, network policies, OIDC/SSO integration, CredHub integration, audit logging, or secret rotation. If your deployment needs any of these, they come from the platform (Kubernetes/CloudFoundry) and your own manifests/policies, not from Muto. [Multi-Tenant Setup](../configuration/multi-tenant-setup.md#what-you-still-have-to-set-up-yourself) has concrete examples for the Kubernetes ones.

## Next Steps

- [Security Configuration](../configuration/tls-security.md) - RBAC detail, metrics exposure, secrets, known limitations
- [Multi-Tenant Setup](../configuration/multi-tenant-setup.md) - what the `Tenant` CRD provisions and what you add yourself
- [Platform Design](./platform-design.md) - how isolation is implemented per platform
- [Agent Lifecycle](./agent-lifecycle.md) - state transitions
- [Messaging](./messaging.md) - topic-based isolation in the message bus
