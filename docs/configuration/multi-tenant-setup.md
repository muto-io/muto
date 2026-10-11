# Multi-Tenant Setup and Configuration

What the `Tenant` custom resource actually provisions today, and what you still have to set up yourself.

## Overview

A `Tenant` is a cluster-scoped CRD (`muto.io/v1alpha1`, kind `Tenant`). Creating one gets you exactly two things from the `TenantReconciler`:

- a Kubernetes namespace (created if it doesn't exist, labeled `muto.io/tenant: <tenant-name>`)
- optionally, a dedicated A2A gateway (`Deployment` + `Service` + `Secret`) in that namespace, if the tenant opts into one

Everything else commonly associated with "multi-tenancy" - resource quotas, network policies, per-tenant RBAC, storage isolation - is **not** created or enforced by Muto. If you need those, you apply standard Kubernetes objects against the namespace yourself; see [What You Still Have to Set Up](#what-you-still-have-to-set-up-yourself) below.

## The Tenant Resource

The real `TenantSpec` (`platform/k8s/types/v1alpha1/tenant_types.go`) has three fields:

```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: customer-acme
spec:
  # Required. Namespace the reconciler creates/uses for this tenant's agent pods.
  namespace: tenant-acme

  # Required. shared: tenant uses the cluster's shared message bus / CF org.
  #            dedicated: tenant gets its own message bus instance / CF org.
  isolationTier: dedicated

  messageBus:
    # Required. nats | kafka | a2a
    type: a2a
    # Only meaningful when isolationTier is dedicated and type is a2a:
    # provisions a per-tenant A2A gateway. Ignored for nats/kafka today -
    # there's no per-tenant NATS/Kafka instance provisioning yet, despite
    # what TenantBusSpec's doc comment says is intended.
    dedicated: true
```

There is no `displayName`, `adminEmail`, `platform`, `quotas`, `isolation`, `networkPolicies`, or `metadata` block in the real spec - a manifest with those fields will be rejected by the CRD's OpenAPI schema (or silently ignored on fields that happen to overlap, like `metadata`, which is just the standard Kubernetes object metadata, not tenant-specific).

```bash
kubectl apply -f tenant-acme.yaml

# Check status - the only status field is `ready`
kubectl get tenant customer-acme -o jsonpath='{.status.ready}'

# Namespace gets created with a tenant label
kubectl get namespace tenant-acme --show-labels
```

## What the Reconciler Does on Create

1. Adds a finalizer (`muto.io/tenant-cleanup`) so gateway resources get cleaned up explicitly on delete.
2. Creates the namespace in `spec.namespace` if it doesn't already exist.
3. If `messageBus.type: a2a` and `messageBus.dedicated: true`, creates (idempotently) a `Secret` (`muto-a2a-token`, a random 32-byte hex token), a `Deployment` (`a2a-gateway`, image from the **operator's** `MUTO_A2A_GATEWAY_IMAGE` environment variable - see [Environment Variables](./environment-variables.md) - reconciliation fails if that's unset), and a `Service` (`a2a-gateway`, port 8080), all owned by the `Tenant` object.
4. Sets `status.ready: true`.

## What the Reconciler Does on Delete

`kubectl delete tenant customer-acme` does **not** cascade-delete the namespace, its pods, RBAC, or anything else in it. The finalizer only deletes the three A2A gateway resources above (each is a no-op if it was never created). The namespace and everything running in it are left behind - clean those up yourself if that's not what you want:

```bash
kubectl delete tenant customer-acme
kubectl delete namespace tenant-acme   # not automatic
```

## CloudFoundry Isolation

On CF, isolation is driven by the operator's `CF_ISOLATION_TIER` and `CF_SHARED_ORG` environment variables (not a per-`Tenant` field - the CF adapter doesn't read the `Tenant` CRD at all today):

- `CF_ISOLATION_TIER=dedicated`: each tenant gets its own CF **org**, named after the tenant ID, with a fixed `muto-agents` space.
- `CF_ISOLATION_TIER=shared`: all tenants share one org (`CF_SHARED_ORG`), each tenant gets its own **space** within it, named after the tenant ID.

The adapter resolves (and caches) the org/space GUIDs via the CF API - it does not create orgs or spaces for you. Create them yourself before a tenant's first job runs:

```bash
# dedicated tier
cf create-org customer-acme
cf create-space muto-agents -o customer-acme

# shared tier
cf create-space customer-acme -o acme-shared-org
```

## Message Bus Topic Isolation

`core/tenant.TopicPrefix(tenantID)` returns a fixed `tenant.<tenantID>.` prefix. `mcp/tools` uses it to build every job's topic as `tenant.<tenantID>.<jobID>` when a job is scheduled through the MCP server - this is automatic and not configurable (no `topicPrefix` field exists on `Tenant` or anywhere else).

`core/tenant.ValidateTopic(tenantID, topic)` checks a topic string against that prefix and errors if it doesn't match - this is exercised directly in `test/integration/k8s/isolation_test.go`, confirming the prefix format. It's a plain library function, though: nothing in the NATS/Kafka adapters currently calls it to reject a cross-tenant publish or subscribe at the bus level. Treat the prefix as a naming convention applied when Muto builds a topic string, not a hard boundary enforced by the message bus itself.

## What You Still Have to Set Up Yourself

None of the following are created by Muto. These are plain Kubernetes objects you apply against the namespace a `Tenant` creates, if you want them.

**Resource quotas:**

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: tenant-acme-quota
  namespace: tenant-acme
spec:
  hard:
    requests.cpu: "50"
    requests.memory: 100Gi
    pods: "500"
```

**Network policy:**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-cross-tenant
  namespace: tenant-acme
spec:
  podSelector: {}
  policyTypes: [Ingress]
  ingress:
  - from:
    - podSelector: {}
    - namespaceSelector:
        matchLabels:
          name: muto-system
```

**Per-tenant RBAC:**

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tenant-viewer
  namespace: tenant-acme
rules:
- apiGroups: ["muto.io"]
  resources: ["agentjobs"]
  verbs: ["get", "list", "watch"]
```

(For the RBAC the **operator itself** needs cluster-wide to manage all tenants, see [Security Configuration](./tls-security.md#kubernetes-rbac) - that's the chart-managed `ClusterRole`, a separate thing from the per-tenant `Role` above.)

## See Also

- [Environment Variables](./environment-variables.md) - `MUTO_A2A_GATEWAY_IMAGE`, `CF_ISOLATION_TIER`, `CF_SHARED_ORG`, and other configuration variables
- [Security Configuration](./tls-security.md) - what RBAC/TLS Muto itself manages vs. what's on you
- [Message Bus Setup](./message-bus-setup.md) - NATS/Kafka connection configuration
