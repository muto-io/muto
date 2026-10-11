# Security Configuration

What Muto actually does for transport security and access control today, and what it deliberately doesn't do yet.

## Overview

Muto has no TLS or mTLS layer of its own, no admission webhooks, and no cert-manager integration - none of `MUTO_TLS_*`, `MUTO_MTLS_*`, `MUTO_RBAC_ENABLED`, or `MUTO_WEBHOOK_PORT` are read anywhere in the code, and the Helm chart (`deploy/helm/muto/`) ships no webhook service, no TLS secret mount, and no `Certificate`/`Issuer` resources. The security surface that does exist is narrower:

- **Kubernetes RBAC** - real, via static Helm-templated manifests.
- **Metrics endpoint** - plain HTTP, no TLS, no authentication.
- **CloudFoundry API calls** - whatever TLS the `CF_API_URL` endpoint itself provides; no app-level TLS configuration.
- **Message bus connections** - no TLS/SASL support implemented (see [Message Bus Setup](./message-bus-setup.md)).

If you need TLS between components, it has to come from outside Muto: a service mesh, an Ingress/Gateway TLS termination point, or NetworkPolicy-based network isolation.

## Kubernetes RBAC

The Helm chart creates a `ServiceAccount`, `ClusterRole`, and `ClusterRoleBinding` for the operator, controlled by two values:

```yaml
serviceAccount:
  create: true
rbac:
  create: true
```

The granted permissions (`deploy/helm/muto/templates/clusterrole.yaml`):

| API group | Resources | Verbs |
|---|---|---|
| `muto.io` | `tenants`, `agentjobs`, `agentfleets` | get, list, watch, create, update, patch, delete |
| `muto.io` | `tenants/status`, `agentjobs/status`, `agentfleets/status` | update, patch |
| `""` (core) | `namespaces` | get, list, watch, create, update, patch |
| `""` (core) | `pods` | get, list, watch, create, update, patch, delete |
| `""` (core) | `serviceaccounts` | get, list, watch, create, update, patch |
| `rbac.authorization.k8s.io` | `roles`, `rolebindings` | get, list, watch, create, update, patch, delete |
| `apps` | `statefulsets` | get, list, watch, create, update, patch, delete |

Set `rbac.create: false` only if you manage these bindings yourself outside the chart; the operator will fail to reconcile anything without equivalent permissions in place.

## Metrics Endpoint Exposure

The Prometheus `/metrics` endpoint (`MUTO_METRICS_BIND_ADDRESS`, default `:8080`) is plain HTTP with no authentication - this is a known limitation, not a configuration gap (see [Monitoring and Observability](../operations/monitoring-observability.md)). Restrict access with a `NetworkPolicy`:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: restrict-metrics
  namespace: muto-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: muto
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 8080
```

## Secrets

Muto reads CF credentials (`CF_API_URL`, `CF_USERNAME`, `CF_PASSWORD`) as plain environment variables - it doesn't fetch them from Vault or any secret backend itself. Source them from a Kubernetes `Secret` rather than inlining them in a Deployment manifest:

```bash
kubectl create secret generic muto-cf-credentials \
  --from-literal=username=muto-service \
  --from-literal=password='secure-password' \
  -n muto-system
```

```yaml
env:
- name: CF_USERNAME
  valueFrom:
    secretKeyRef:
      name: muto-cf-credentials
      key: username
- name: CF_PASSWORD
  valueFrom:
    secretKeyRef:
      name: muto-cf-credentials
      key: password
```

An external secret manager (Vault, External Secrets Operator, etc.) can populate that same `Secret` - Muto itself has no opinion on how the `Secret` gets its values, it just reads them as env vars like any other.

## Known Limitations

- No admission webhooks (validating or mutating) exist. Resource validation happens in the reconcilers, not at admission time.
- No mTLS between the operator and spawned agents, or between the operator and the message bus.
- No cert-manager integration or certificate rotation - there's nothing issuing certificates to rotate.
- The metrics endpoint has no TLS or authentication; network-level restriction is the only mitigation today.

## See Also

- [Monitoring and Observability](../operations/monitoring-observability.md) - metrics endpoint exposure and its known limitations
- [Environment Variables](./environment-variables.md) - Core and platform configuration variables
- [Message Bus Setup](./message-bus-setup.md) - NATS/Kafka connection configuration
