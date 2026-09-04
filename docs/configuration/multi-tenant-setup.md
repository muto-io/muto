# Multi-Tenant Setup and Configuration

Configure and manage Muto for multi-tenant environments with complete isolation.

## Overview

Multi-tenancy in Muto provides logical isolation of:
- **Compute**: Separate namespaces (K8s) or spaces (CloudFoundry)
- **Storage**: Isolated data and configuration per tenant
- **Messaging**: Tenant-scoped message bus topics
- **Access Control**: RBAC boundaries preventing cross-tenant access

This guide covers creating, configuring, and verifying tenants.

---

## Tenant Lifecycle

### Create a Tenant

#### On Kubernetes

**Step 1: Define Tenant Resource**

```yaml
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: customer-acme
  namespace: muto-system
spec:
  # Display name
  displayName: "ACME Corporation"
  
  # Email contact
  adminEmail: "ops@acme.com"
  
  # Platform configuration
  platform:
    type: kubernetes
    namespace: tenant-acme
    
  # Resource quotas
  quotas:
    cpu: "50"           # 50 cores
    memory: "100Gi"     # 100 GB
    jobs: "500"         # Max 500 concurrent jobs
    storage: "1Ti"      # 1 TB storage
    
  # Isolation level
  isolation:
    level: strong       # strong or moderate
    networkPolicy: true # Enable network policies
    rbac: true         # Enable RBAC
    
  # Message bus configuration
  messageBus:
    topicPrefix: "tenant-acme"
    consumerGroupPrefix: "tenant-acme"
    
  # Optional: Network policies
  networkPolicies:
    - name: deny-ingress
      policyTypes:
      - Ingress
      podSelector: {}
      ingress:
      - from:
        - namespaceSelector:
            matchLabels:
              name: tenant-acme
      
  # Annotations for custom metadata
  metadata:
    customerID: "cust-12345"
    tier: "premium"
    supportLevel: "24x7"
```

**Step 2: Apply Tenant**

```bash
kubectl apply -f tenant-acme.yaml
```

**Step 3: Verify Tenant Created**

```bash
# Check tenant status
kubectl get tenant customer-acme -n muto-system -o wide

# Expected output:
# NAME             DISPLAYNAME        STATUS   NAMESPACE     AGE
# customer-acme    ACME Corporation   Ready    tenant-acme   2m

# Check namespace was created
kubectl get namespace tenant-acme

# Check namespace labels
kubectl get namespace tenant-acme --show-labels
```

#### On CloudFoundry

**Step 1: Create Org and Space**

```bash
# Login to CloudFoundry
cf login -a https://api.cf.example.com

# Create organization
cf create-org acme-corp

# Create space within org
cf create-space production -o acme-corp

# Target the space
cf target -o acme-corp -s production
```

**Step 2: Create Service Bindings (Message Bus)**

For NATS:
```bash
# Create NATS service
cf create-service nats-service default nats-acme

# Create user-provided service for credentials
cf create-user-provided-service nats-config \
  -p '{"url":"nats://nats:4222","credentials":"path/to/creds"}'
```

For Kafka:
```bash
# Create Kafka service
cf create-service kafka-service default kafka-acme

# Configuration via environment
cf set-env muto-app MUTO_KAFKA_BROKERS kafka-broker:9092
```

**Step 3: Register with Muto (via API)**

```bash
# Call Muto MCP API to register tenant
curl -X POST http://muto-mcp:3000/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "name": "customer-acme",
    "displayName": "ACME Corporation",
    "platform": "cloudfoundry",
    "org": "acme-corp",
    "space": "production",
    "topicPrefix": "tenant-acme"
  }'
```

---

## Tenant Configuration

### Resource Quotas

Define resource limits per tenant to prevent one tenant from consuming all resources:

```yaml
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: customer-acme
spec:
  quotas:
    cpu: "50"           # Total CPU cores
    memory: "100Gi"     # Total memory
    jobs: "500"         # Max concurrent jobs
    storage: "1Ti"      # Max storage
    
    # Optional: Per-job limits
    jobDefaults:
      cpuRequest: "100m"
      cpuLimit: "2"
      memoryRequest: "256Mi"
      memoryLimit: "1Gi"
      timeout: "4h"
```

**Verify Quotas Applied:**
```bash
# On Kubernetes
kubectl get resourcequota -n tenant-acme

# Check quota usage
kubectl describe resourcequota -n tenant-acme

# Example output:
# Name:          tenant-acme-quota
# Resource       Used     Hard
# --------       ---      ----
# cpu            5        50
# memory         8Gi      100Gi
# pods           12       500
```

### Access Control (RBAC)

Configure who can manage tenant resources:

#### Kubernetes RBAC

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: tenant-admin
  namespace: tenant-acme

---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: tenant-admin
  namespace: tenant-acme
rules:
# Agents can create jobs
- apiGroups: ["muto.io"]
  resources: ["agentjobs"]
  verbs: ["create", "list", "get", "watch"]
# Agents can read config
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list"]
# Agents can write logs
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tenant-admin-binding
  namespace: tenant-acme
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: tenant-admin
subjects:
- kind: ServiceAccount
  name: tenant-admin
  namespace: tenant-acme
- kind: Group
  name: acme-ops
  apiGroup: rbac.authorization.k8s.io
```

**Grant User Access:**
```bash
# Add user to tenant-admin role
kubectl create rolebinding tenant-admin:alice \
  --clusterrole=tenant-admin \
  --user=alice@acme.com \
  -n tenant-acme
```

#### CloudFoundry RBAC

```bash
# Add org manager
cf set-org-role alice@acme.com acme-corp OrgManager

# Add space developer
cf set-space-role alice@acme.com acme-corp production SpaceDeveloper

# Add space auditor (read-only)
cf set-space-role bob@acme.com acme-corp production SpaceAuditor
```

### Network Isolation

#### Kubernetes Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-cross-tenant
  namespace: tenant-acme
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow from within same namespace
  - from:
    - podSelector: {}
  # Allow from muto-system namespace
  - from:
    - namespaceSelector:
        matchLabels:
          name: muto-system
  egress:
  # Allow DNS
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow to message bus
  - to:
    - namespaceSelector:
        matchLabels:
          name: muto-system
      podSelector:
        matchLabels:
          app: nats
  # Allow external (with restrictions)
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
        except:
        - 169.254.169.254/32  # Metadata service
```

#### CloudFoundry Security Groups

```bash
# Create security group for tenant
cf create-security-group tenant-acme-sg \
  - protocol: tcp
    destination: 10.0.0.0/8
    ports: 443,4222,9092
  - protocol: udp
    destination: 8.8.8.8
    ports: 53

# Bind to space
cf bind-security-group tenant-acme-sg acme-corp production
```

### Message Bus Isolation

#### Topic Prefix Configuration

```yaml
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: customer-acme
spec:
  messageBus:
    topicPrefix: "tenant-acme"
    consumerGroupPrefix: "tenant-acme"
    # Message retention per tenant
    retentionHours: 168
    # Partition count (Kafka)
    partitionCount: 3
```

**Topic Naming Pattern:**
```
tenant-acme/workflow/job-123/started
tenant-acme/workflow/job-123/completed
tenant-acme/notifications/error
tenant-acme/system/status
```

**Verify Topic Isolation:**

For NATS:
```bash
# List all subscriptions
curl http://nats:8222/connz | jq '.conns[].subs'

# Should show tenant-specific subjects:
# tenant-acme.workflow.>
# tenant-acme.notifications.>
```

For Kafka:
```bash
# List topics (should be prefixed)
kafka-topics.sh --list --bootstrap-server kafka:9092 | grep tenant-acme

# Expected output:
# tenant-acme-workflow
# tenant-acme-notifications
# tenant-acme-system
```

---

## Tenant Monitoring and Verification

### Verify Tenant Isolation

#### Compute Isolation

```bash
# Verify namespace exists and is isolated
kubectl get ns tenant-acme --show-labels

# Check RBAC - user should NOT see other tenant namespaces
kubectl get ns -o name | grep -v tenant-acme
# Should return no results when accessed as tenant user

# Verify network policies
kubectl get networkpolicies -n tenant-acme

# Test network isolation
# From within tenant pod, try to reach another tenant
kubectl exec -it <pod> -n tenant-acme -- \
  curl http://pod-in-other-tenant
# Should timeout or be refused
```

#### Storage Isolation

```bash
# On Kubernetes - verify PVC/PV isolation
kubectl get pvc -n tenant-acme

# Check etcd keys are prefixed
etcdctl get --prefix /muto/tenant-acme/

# On CloudFoundry - verify service bindings
cf services -o acme-corp -s production
# Should only see services bound to this space
```

#### Messaging Isolation

```bash
# NATS - verify topic subscriptions
curl http://nats:8222/connz | \
  jq '.conns[] | select(.subs[] | startswith("tenant-acme")) | .subs'

# Kafka - verify consumer group
kafka-consumer-groups.sh \
  --bootstrap-server kafka:9092 \
  --group tenant-acme-operator \
  --describe
```

#### Access Control Verification

```bash
# As tenant user, verify they can:
# 1. List jobs in their namespace
kubectl get agentjobs -n tenant-acme

# 2. Create jobs in their namespace
kubectl apply -f agentjob.yaml -n tenant-acme

# 3. Cannot access other namespace
kubectl get agentjobs -n tenant-other
# Should be forbidden

# 4. Cannot create cluster-wide resources
kubectl apply -f clusterrole.yaml
# Should be forbidden
```

### Monitor Quota Usage

```bash
# Get quota status
kubectl describe resourcequota -n tenant-acme

# Example monitoring query (Prometheus)
container_memory_usage_bytes{namespace="tenant-acme"}

# Check CPU usage
kubectl top pods -n tenant-acme

# Check storage usage
kubectl exec -n tenant-acme <pod> -- du -sh /mnt/storage
```

---

## Tenant Scaling and Management

### Add More Capacity to Tenant

```yaml
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: customer-acme
spec:
  quotas:
    cpu: "100"    # Increased from 50
    memory: "200Gi"  # Increased from 100Gi
    jobs: "1000"  # Increased from 500
    storage: "2Ti"   # Increased from 1Ti
```

Apply the update:
```bash
kubectl apply -f tenant-updated.yaml

# Verify quota was updated
kubectl describe resourcequota -n tenant-acme
```

### Manage Tenant Keys/Credentials

#### CloudFoundry Service Credentials

```bash
# Get service credentials
cf service-key nats-acme muto-creds

# Rotate credentials
cf delete-service-key nats-acme muto-creds
cf create-service-key nats-acme muto-creds

# Update in Muto
cf set-env muto-app NATS_CREDENTIALS_FILE /path/to/new-creds
cf restage muto-app
```

#### Kubernetes Secrets

```bash
# Create secret for tenant credentials
kubectl create secret generic tenant-acme-creds \
  --from-literal=username=acme-user \
  --from-literal=password=secret-password \
  -n tenant-acme

# Reference in AgentJob
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: secure-job
  namespace: tenant-acme
spec:
  agents:
  - name: processor
    image: myorg/processor:v1
    env:
    - name: DB_USER
      valueFrom:
        secretKeyRef:
          name: tenant-acme-creds
          key: username
```

---

## Tenant Cleanup and Deletion

### Offboard a Tenant

**Step 1: Stop accepting new jobs**
```bash
kubectl patch tenant customer-acme \
  -p '{"spec":{"status":"suspended"}}' \
  -n muto-system
```

**Step 2: Wait for running jobs to complete**
```bash
# Monitor job completion
kubectl get agentjobs -n tenant-acme --watch

# Or force-terminate long-running jobs
kubectl delete agentjobs --all -n tenant-acme --grace-period=30
```

**Step 3: Export data (if needed)**
```bash
# Backup tenant configuration
kubectl get all -n tenant-acme -o yaml > tenant-acme-backup.yaml

# Export message bus data
# For NATS - configure backup
# For Kafka - run consumer to export topics
kafka-console-consumer.sh \
  --bootstrap-server kafka:9092 \
  --group tenant-acme-backup \
  --topic tenant-acme-workflow \
  --from-beginning > tenant-acme-messages.json
```

**Step 4: Delete tenant**
```bash
# On Kubernetes
kubectl delete tenant customer-acme -n muto-system

# This triggers cascading delete of:
# - Tenant namespace
# - All resources in namespace
# - RBAC policies
# - Network policies
# - Quotas

# On CloudFoundry
cf delete-space production -o acme-corp
cf delete-org acme-corp
```

**Step 5: Verify cleanup**
```bash
# Confirm namespace is gone
kubectl get ns tenant-acme
# Should return "not found"

# Confirm message bus topics are cleaned
kafka-topics.sh --list --bootstrap-server kafka:9092 | grep tenant-acme
# Should return nothing (or topics can persist for audit)
```

---

## Multi-Tenant Patterns

### Pattern 1: Single Tenant per Cluster

Simplest deployment - entire cluster for one tenant.

**Pros:** Maximum isolation, simplest management
**Cons:** Higher infrastructure cost, underutilization possible

```bash
export MUTO_PLATFORM=kubernetes
export MUTO_K8S_NAMESPACE=muto-system
# Single tenant namespace
export MUTO_TENANT_NAMESPACE_PREFIX=production-
```

### Pattern 2: Multiple Tenants in One Cluster

Multiple tenants in a shared cluster with isolation via namespaces.

**Pros:** Cost-efficient, dynamic tenant onboarding
**Cons:** More complex isolation verification

```yaml
# Create multiple tenant namespaces
---
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: customer-acme
spec:
  quotas:
    cpu: "50"
    memory: "100Gi"
---
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: customer-globex
spec:
  quotas:
    cpu: "75"
    memory: "150Gi"
```

### Pattern 3: Shared Message Bus, Isolated Namespaces

Shared message bus infrastructure but isolated topics per tenant.

**Pros:** Simpler ops, efficient messaging infrastructure
**Cons:** Message bus becomes critical path

```bash
# All tenants use same NATS cluster
export MUTO_NATS_URL=nats://shared-nats:4222

# But use tenant-specific topic prefixes
# Configured per-tenant in Tenant resource
# tenant-acme/workflow/*
# tenant-globex/workflow/*
```

---

## Best Practices for Multi-Tenancy

1. **Start Simple**: Begin with single tenant, add multi-tenancy when needed

2. **Test Isolation**: Regularly verify no cross-tenant access

3. **Monitor per Tenant**: Track quota usage and enforce limits

4. **Document Access**: Keep clear records of who has access to what

5. **Rotate Credentials**: Regularly rotate tenant credentials and keys

6. **Plan Capacity**: Build buffer into quotas for growth

7. **Audit Access**: Log all tenant operations for compliance

8. **Backup per Tenant**: Separate backups for each tenant when possible

---

## Troubleshooting Multi-Tenancy

### Tenant Jobs Not Scheduling

```bash
# Check tenant status
kubectl get tenant <name> -n muto-system -o yaml

# Verify namespace exists
kubectl get ns tenant-<name>

# Check resource quotas
kubectl describe resourcequota -n tenant-<name>

# Check reconciler logs
kubectl logs -n muto-system deployment/muto-operator | grep <tenant-name>
```

### Cross-Tenant Access Detected

```bash
# Check network policies
kubectl get networkpolicies -n tenant-<name>

# Verify RBAC
kubectl get rolebindings -n tenant-<name>

# Check pod connectivity
kubectl exec -it <pod> -n tenant-a -- nc -zv <pod-ip-in-tenant-b> 5000
# Should fail if isolation is working
```

### Message Bus Topic Leakage

```bash
# NATS - check subject subscriptions
curl http://nats:8222/connz | jq '.conns[].subs | select(contains("cross-tenant"))'

# Kafka - verify consumer group
kafka-consumer-groups.sh --bootstrap-server kafka:9092 --group <group> --describe
```

---

## See Also

- [Environment Variables](./environment-variables.md) — Tenant configuration options
- [TLS and Security](./tls-security.md) — Tenant authentication and encryption
- [Deployment Production Checklist](../deployment/production-checklist.md) — Multi-tenancy verification
- [Architecture: Security Model](../architecture/security-model.md) — How isolation works

---

**Last Updated:** 2026-09-03
