# Security Model: Multi-Tenancy, Isolation, and Authentication

Muto is designed from the ground up for secure multi-tenant operation. This document describes the security model, isolation guarantees, and how to configure Muto securely.

## Security Principles

1. **Tenant Isolation**: Complete isolation between tenants — no data leakage
2. **Defense in Depth**: Multiple layers of security (RBAC, network policies, storage isolation)
3. **Zero Trust**: Every request authenticated and authorized
4. **Encryption in Transit**: TLS for all external communication
5. **Audit Trail**: All operations logged for compliance

## Multi-Tenancy Model

Muto uses **namespace-based isolation** for Kubernetes and **space-based isolation** for CloudFoundry:

```
Muto System (Single Cluster/Org)
│
├─ Tenant: tenant-a
│  ├─ K8s Namespace: tenant-a
│  ├─ CF Space: tenant-a-space
│  ├─ Message Topics: tenant-a/*
│  ├─ RBAC: tenant-a-role
│  └─ Data: Isolated from other tenants
│
├─ Tenant: tenant-b
│  ├─ K8s Namespace: tenant-b
│  ├─ CF Space: tenant-b-space
│  ├─ Message Topics: tenant-b/*
│  ├─ RBAC: tenant-b-role
│  └─ Data: Isolated from other tenants
│
└─ Tenant: tenant-c
   ├─ K8s Namespace: tenant-c
   ├─ CF Space: tenant-c-space
   ├─ Message Topics: tenant-c/*
   ├─ RBAC: tenant-c-role
   └─ Data: Isolated from other tenants
```

### Kubernetes-Based Isolation

Namespace provides hard boundary:

```yaml
# Create namespace per tenant
apiVersion: v1
kind: Namespace
metadata:
  name: tenant-a
  labels:
    muto.io/tenant: tenant-a

---
# RBAC: Only tenant-a users can access this namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tenant-a-user-role
  namespace: tenant-a
rules:
- apiGroups: ["muto.io"]
  resources: ["agentjobs"]
  verbs: ["get", "list", "create", "update"]
- apiGroups: [""]
  resources: ["pods", "pods/log"]
  verbs: ["get", "list"]

---
# Bind role to users
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tenant-a-user-binding
  namespace: tenant-a
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: tenant-a-user-role
subjects:
- kind: User
  name: alice@tenant-a.com
- kind: Group
  name: tenant-a-admins

---
# Network Policy: Deny cross-namespace traffic
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: tenant-isolation
  namespace: tenant-a
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: tenant-a
    - namespaceSelector:
        matchLabels:
          name: muto-system  # Allow muto-system for control plane

---
# Resource Quota: Limit tenant resource usage
apiVersion: v1
kind: ResourceQuota
metadata:
  name: tenant-a-quota
  namespace: tenant-a
spec:
  hard:
    pods: "100"
    requests.cpu: "10"
    requests.memory: "50Gi"
    limits.cpu: "20"
    limits.memory: "100Gi"
```

### CloudFoundry-Based Isolation

Space provides boundary with CF RBAC:

```bash
# Create space per tenant
cf create-space tenant-a -o muto-production

# Create users for tenant
cf create-user alice@tenant-a.com <password>
cf create-user bob@tenant-a.com <password>

# Assign to space
cf set-space-role alice@tenant-a.com muto-production tenant-a SpaceManager
cf set-space-role bob@tenant-a.com muto-production tenant-a SpaceDeveloper

# Create service instances for tenant (databases, secrets)
cf create-service postgresql standard tenant-a-db -c '{"tenant": "tenant-a"}'
```

CF isolates:
- Apps and tasks to space
- Service instances to space
- CredHub secrets with path namespacing
- Service credentials with tenant prefix

## RBAC (Role-Based Access Control)

Muto integrates with platform RBAC:

### Role Definitions

**Tenant Admin**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tenant-admin
  namespace: tenant-a
rules:
- apiGroups: ["muto.io"]
  resources: ["agentjobs"]
  verbs: ["*"]  # All operations
- apiGroups: [""]
  resources: ["pods", "pods/log", "pods/exec"]
  verbs: ["*"]
```

**Agent Developer**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: agent-developer
  namespace: tenant-a
rules:
- apiGroups: ["muto.io"]
  resources: ["agentjobs"]
  verbs: ["get", "list", "create", "update"]  # Can create/run jobs
- apiGroups: [""]
  resources: ["pods/log"]  # Read-only logs
  verbs: ["get"]
- apiGroups: ["muto.io"]
  resources: ["agentjobs/status"]
  verbs: ["get"]  # Monitor status
```

**Job Viewer**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: job-viewer
  namespace: tenant-a
rules:
- apiGroups: ["muto.io"]
  resources: ["agentjobs"]
  verbs: ["get", "list"]  # Read-only
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]  # View logs
```

### Authorization Flow

```
User Request: kubectl get agentjobs
        │
        ▼
┌──────────────────────────────┐
│ Kubernetes API Server        │
└──────────────────┬───────────┘
                   │
                   ├─ 1. Authenticate
                   │    - Check user certificate
                   │    - Valid token?
                   │
                   ├─ 2. Authorize
                   │    - User: alice@tenant-a.com
                   │    - Namespace: tenant-a
                   │    - Action: get agentjobs
                   │    - Check RoleBinding
                   │    - Role has "get" on "agentjobs"? Yes ✓
                   │
                   ├─ 3. Audit
                   │    - Log access: alice@tenant-a.com accessed agentjobs
                   │
                   ▼
User receives: List of AgentJobs in tenant-a
```

## Authentication

Muto supports multiple authentication methods:

### Kubernetes: Bearer Tokens and Certificates

```bash
# 1. Service Account token (for programmatic access)
kubectl create serviceaccount muto-user -n tenant-a
kubectl create rolebinding muto-user-binding \
  --clusterrole=view \
  --serviceaccount=tenant-a:muto-user

# Get token
kubectl get secret <token-secret-name> -n tenant-a \
  -o jsonpath='{.data.token}' | base64 -d

# Use token
kubectl --token=$TOKEN --server=$API_SERVER get agentjobs
```

### CloudFoundry: OAuth2

```bash
# 1. Get OAuth2 token
CF_TOKEN=$(cf oauth-token)

# 2. Use token in API calls
curl -H "Authorization: bearer $CF_TOKEN" \
  https://api.run.pivotal.io/v3/spaces

# Token is valid for ~24 hours
```

### External OIDC (Enterprise)

Muto can integrate with enterprise OIDC providers:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muto-auth-config
data:
  auth: |
    type: oidc
    oidc:
      issuer: https://auth.company.com
      clientID: muto-client
      clientSecret: # from secret
      scopes:
        - openid
        - profile
        - email
      claimMappings:
        username: email
        tenant: tenant_id  # Custom claim for tenant
```

## Encryption in Transit

### TLS for Kubernetes Control Plane

K8s API communication is encrypted by default:

```
User kubectl
    │
    ├─ TLS 1.2+
    ├─ Verify server certificate (CA cert)
    ├─ Mutual TLS for client auth (client cert + key)
    │
    ▼
Kubernetes API Server (TLS port 6443)
    │
    ├─ Verify client certificate
    │
    ▼
Request processed securely
```

### mTLS Between Muto Components

Muto operator and webhooks use mTLS:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: muto-webhook-cert
  namespace: muto-system
spec:
  secretName: muto-webhook-tls
  issuerRef:
    name: muto-self-signed-issuer
    kind: Issuer

---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: agentjob-validation
webhooks:
- name: agentjob.muto.io
  clientConfig:
    service:
      name: muto-webhook
      namespace: muto-system
      path: "/validate"
    caBundle: <base64 encoded CA cert>  # Server certificate
  rules:
  - operations: ["CREATE", "UPDATE"]
    apiGroups: ["muto.io"]
    apiVersions: ["v1"]
    resources: ["agentjobs"]
```

### Message Bus TLS

NATS and Kafka connections are encrypted:

```yaml
# NATS with TLS
apiVersion: v1
kind: ConfigMap
metadata:
  name: muto-messaging-config
data:
  nats: |
    servers:
    - nats://nats-0.nats:4222
    tls:
      enabled: true
      caFile: /etc/nats/ca.crt
      certFile: /etc/nats/tls.crt
      keyFile: /etc/nats/tls.key
```

## Secrets Management

### CredHub (CloudFoundry)

Tenant-scoped secret storage:

```bash
# Store secret for tenant-a
credhub set --name=/muto/tenant-a/db-password \
  --type=password \
  --value='<password>'

# Agent retrieves secret
credhub get --name=/muto/tenant-a/db-password
```

Muto enforces path-based access:
- tenant-a can only read `/muto/tenant-a/*`
- tenant-b cannot access `/muto/tenant-a/*`

### Kubernetes Secrets

Tenant-scoped secret storage:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: db-credentials
  namespace: tenant-a  # Only accessible in tenant-a namespace
type: Opaque
data:
  username: YWxpY2U=     # base64: alice
  password: c2VjcmV0=    # base64: secret

---
# Pod references secret
apiVersion: v1
kind: Pod
metadata:
  name: agent-pod
  namespace: tenant-a
spec:
  containers:
  - name: agent
    env:
    - name: DB_USERNAME
      valueFrom:
        secretKeyRef:
          name: db-credentials
          key: username
    - name: DB_PASSWORD
      valueFrom:
        secretKeyRef:
          name: db-credentials
          key: password
```

Muto enforces:
- Secrets are namespace-scoped
- RBAC controls who can read secrets
- Audit logging tracks secret access

## Network Security

### Kubernetes Network Policies

Restrict network traffic:

```yaml
# Deny all traffic by default
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
  namespace: tenant-a
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress

---
# Allow traffic within tenant namespace
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-within-tenant
  namespace: tenant-a
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: tenant-a
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: tenant-a
  - to:
    - podSelector: {}  # DNS
    ports:
    - protocol: UDP
      port: 53

---
# Allow traffic to message bus
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-message-bus
  namespace: tenant-a
spec:
  podSelector: {}
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: muto-messaging
    ports:
    - protocol: TCP
      port: 4222  # NATS
```

### CloudFoundry Security Groups

CF Application Security Groups (ASGs):

```bash
# Create ASG for tenant
cf create-security-group tenant-a-sg - << 'EOF'
[
  {
    "protocol": "tcp",
    "destination": "10.0.0.0/8",  # Only internal traffic
    "ports": "1-65535"
  },
  {
    "protocol": "tcp",
    "destination": "nats.muto-system",  # Message bus
    "ports": "4222"
  }
]
EOF

# Bind to space
cf bind-security-group tenant-a-sg muto-production tenant-a
```

## Audit Logging

All operations are logged for compliance:

```json
{
  "timestamp": "2026-09-03T10:30:05.123Z",
  "audit_id": "req-12345",
  "user": "alice@tenant-a.com",
  "tenant": "tenant-a",
  "action": "created",
  "resource": "agentjob/data-pipeline",
  "result": "success",
  "details": {
    "image": "myorg/extractor:v1",
    "timeout": "5m",
    "resources": {
      "cpu": "500m",
      "memory": "512Mi"
    }
  }
}
```

Logs exported to:
- **Kubernetes**: CloudAudit logs → Cloud Logging
- **Files**: Local audit.log with log rotation
- **SIEM**: Forward to Splunk, ELK, or similar

## Compliance and Best Practices

### Least Privilege

Grant minimum required permissions:

```yaml
# ✓ Good: Specific permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: restricted-role
rules:
- apiGroups: ["muto.io"]
  resources: ["agentjobs"]
  verbs: ["create", "get"]  # Only what's needed

---
# ✗ Bad: Overly broad permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: too-permissive
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]  # Too broad!
```

### Secure Defaults

Enable security features:

```yaml
# Pod Security Policy
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: restricted
spec:
  privileged: false  # No privileged containers
  allowPrivilegeEscalation: false  # No privesc
  requiredDropCapabilities:
  - ALL  # Drop all linux capabilities
  runAsUser:
    rule: "MustRunAsNonRoot"  # No root containers
  fsGroup:
    rule: "MustRunAs"
    ranges:
    - min: 1000
      max: 65535
  readOnlyRootFilesystem: true  # Read-only root

---
# Network Policy: Deny all, allow specific
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: tenant-a
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
```

### Secret Rotation

Rotate secrets regularly:

```bash
# Rotate NATS credentials
kubectl patch secret nats-credentials \
  -p '{"data": {"password": "'$(echo -n $NEW_PASSWORD | base64)'"}}' \
  -n muto-system

# Rotate TLS certificates (cert-manager)
kubectl delete secret muto-webhook-tls -n muto-system
# cert-manager automatically regenerates
```

### Monitoring for Security Issues

Monitor for suspicious activity:

```yaml
# Alert on failed authentication attempts
apiVersion: v1
kind: PrometheusRule
metadata:
  name: muto-security-alerts
spec:
  groups:
  - name: muto.security
    rules:
    - alert: AuthenticationFailureRate
      expr: rate(muto_auth_failures_total[5m]) > 0.1
      annotations:
        summary: "High authentication failure rate"
    
    - alert: UnauthorizedAccess
      expr: increase(muto_rbac_denials_total[5m]) > 10
      annotations:
        summary: "Unusual RBAC denials detected"
    
    - alert: ServiceAccountTokenLeak
      expr: muto_service_account_token_created > 1
      annotations:
        summary: "Suspicious service account token creation"
```

---

## Next Steps

- **[Platform Design](./platform-design.md)** — How isolation is implemented per platform
- **[Agent Lifecycle](./agent-lifecycle.md)** — State transitions with security boundaries
- **[Messaging](./messaging.md)** — Topic-based isolation in message bus

**Last Updated:** 2026-09-03
