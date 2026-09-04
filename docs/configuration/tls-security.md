# TLS and Security Configuration

Configure TLS, mTLS, and other security settings for production deployments.

## Overview

TLS (Transport Layer Security) encrypts communication between:
- Operator and webhooks
- Operator and message bus
- Agents and operator
- Clients and operator API
- CloudFoundry API calls

This guide covers certificate generation, configuration, and best practices.

---

## TLS Basics

### Self-Signed Certificates (Development)

**Generate self-signed certificate:**

```bash
# Generate private key
openssl genrsa -out tls.key 2048

# Generate certificate (valid for 365 days)
openssl req -new -x509 -key tls.key -out tls.crt -days 365 \
  -subj "/CN=muto-operator.muto-system.svc.cluster.local"

# Verify certificate
openssl x509 -in tls.crt -text -noout
```

### Certificate from CA (Production)

**Request signed certificate:**

```bash
# Generate private key
openssl genrsa -out tls.key 2048

# Generate certificate signing request (CSR)
openssl req -new -key tls.key -out tls.csr \
  -subj "/CN=muto-operator.muto-system.svc.cluster.local" \
  -addext "subjectAltName=DNS:muto-operator.muto-system.svc.cluster.local,DNS:muto-operator.muto-system,DNS:muto-operator"

# Submit CSR to your CA
# CA returns: tls.crt

# Verify certificate
openssl x509 -in tls.crt -text -noout
```

---

## Operator TLS Configuration

### Enable TLS for Operator

```bash
export MUTO_TLS_ENABLED=true
export MUTO_TLS_CERT_FILE=/etc/muto/certs/tls.crt
export MUTO_TLS_KEY_FILE=/etc/muto/certs/tls.key
```

### Create Kubernetes Secret

```bash
# Create namespace
kubectl create namespace muto-system

# Create TLS secret from certificate files
kubectl create secret tls muto-tls \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key \
  -n muto-system

# Verify secret
kubectl get secret muto-tls -n muto-system -o yaml
```

### Mount in Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muto-operator
  namespace: muto-system
spec:
  template:
    spec:
      containers:
      - name: operator
        image: muto-operator:v1
        env:
        - name: MUTO_TLS_ENABLED
          value: "true"
        - name: MUTO_TLS_CERT_FILE
          value: /etc/muto/certs/tls.crt
        - name: MUTO_TLS_KEY_FILE
          value: /etc/muto/certs/tls.key
        - name: MUTO_WEBHOOK_PORT
          value: "8443"
        volumeMounts:
        - name: tls-certs
          mountPath: /etc/muto/certs
          readOnly: true
        ports:
        - containerPort: 8443
          name: webhook
      volumes:
      - name: tls-certs
        secret:
          secretName: muto-tls
          defaultMode: 0400
```

---

## Webhook Configuration (Kubernetes)

### ValidatingWebhookConfiguration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: muto-validation
webhooks:
- name: agentjob.muto.io
  admissionReviewVersions: ["v1"]
  clientConfig:
    service:
      name: muto-operator
      namespace: muto-system
      path: "/validate/agentjob"
    caBundle: <base64-encoded-ca-cert>
  rules:
  - operations: ["CREATE", "UPDATE"]
    apiGroups: ["muto.io"]
    apiVersions: ["v1"]
    resources: ["agentjobs"]
  failurePolicy: Fail
  sideEffects: None
  namespaceSelector:
    matchExpressions:
    - key: muto-validation
      operator: In
      values: ["enabled"]
```

### MutatingWebhookConfiguration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: muto-mutation
webhooks:
- name: agentjob-defaults.muto.io
  admissionReviewVersions: ["v1"]
  clientConfig:
    service:
      name: muto-operator
      namespace: muto-system
      path: "/mutate/agentjob"
    caBundle: <base64-encoded-ca-cert>
  rules:
  - operations: ["CREATE"]
    apiGroups: ["muto.io"]
    apiVersions: ["v1"]
    resources: ["agentjobs"]
  failurePolicy: Ignore
  sideEffects: None
```

**Encode CA Certificate for caBundle:**

```bash
cat /path/to/ca.crt | base64 -w 0
```

---

## Mutual TLS (mTLS) Configuration

mTLS requires both client and server to authenticate with certificates.

### Enable mTLS

```bash
export MUTO_MTLS_ENABLED=true
export MUTO_MTLS_CA_FILE=/etc/muto/certs/ca.crt
export MUTO_MTLS_CLIENT_CERT_FILE=/etc/muto/certs/client.crt
export MUTO_MTLS_CLIENT_KEY_FILE=/etc/muto/certs/client.key
```

### Generate mTLS Certificates

**Create CA (Certificate Authority):**

```bash
# Create CA private key
openssl genrsa -out ca.key 2048

# Create CA certificate
openssl req -new -x509 -days 365 -key ca.key -out ca.crt \
  -subj "/CN=Muto-CA"

# Verify CA
openssl x509 -in ca.crt -text -noout
```

**Create Agent Certificate:**

```bash
# Generate agent private key
openssl genrsa -out agent.key 2048

# Generate CSR for agent
openssl req -new -key agent.key -out agent.csr \
  -subj "/CN=muto-agent"

# Sign with CA
openssl x509 -req -days 365 -in agent.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out agent.crt

# Verify agent certificate
openssl x509 -in agent.crt -text -noout
```

**Create Operator Certificate:**

```bash
# Generate operator private key
openssl genrsa -out operator.key 2048

# Generate CSR for operator
openssl req -new -key operator.key -out operator.csr \
  -subj "/CN=muto-operator"

# Sign with CA
openssl x509 -req -days 365 -in operator.csr \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out operator.crt

# Verify operator certificate
openssl x509 -in operator.crt -text -noout
```

### Deploy mTLS in Kubernetes

```bash
# Create secret with CA
kubectl create secret generic muto-ca \
  --from-file=ca.crt=ca.crt \
  -n muto-system

# Create secret with agent certificates
kubectl create secret tls muto-agent \
  --cert=agent.crt \
  --key=agent.key \
  -n muto-system

# Create secret with operator certificates
kubectl create secret tls muto-operator \
  --cert=operator.crt \
  --key=operator.key \
  -n muto-system
```

### Distribute Client Certificates to Agents

**In Agent Container:**

```bash
# Mount CA certificate
volumeMounts:
- name: muto-ca
  mountPath: /etc/muto/ca
  readOnly: true
- name: agent-certs
  mountPath: /etc/muto/certs
  readOnly: true

# Environment for client
env:
- name: MUTO_MTLS_ENABLED
  value: "true"
- name: MUTO_MTLS_CA_FILE
  value: /etc/muto/ca/ca.crt
- name: MUTO_MTLS_CLIENT_CERT_FILE
  value: /etc/muto/certs/tls.crt
- name: MUTO_MTLS_CLIENT_KEY_FILE
  value: /etc/muto/certs/tls.key

volumes:
- name: muto-ca
  secret:
    secretName: muto-ca
- name: agent-certs
  secret:
    secretName: muto-agent
```

---

## Message Bus TLS Configuration

### NATS with TLS

**NATS Server Configuration:**

```yaml
# nats-server.conf
port: 4222
tls {
  cert_file: "/etc/nats/certs/server.crt"
  key_file:  "/etc/nats/certs/server.key"
  ca_file:   "/etc/nats/certs/ca.crt"
  verify:    true          # Require client certificates
}
```

**Muto Configuration:**

```bash
export MUTO_NATS_URL=nats+tls://nats:4222
export MUTO_NATS_TLS_CA=/etc/muto/certs/nats-ca.crt
export MUTO_NATS_TLS_CERT=/etc/muto/certs/nats-client.crt
export MUTO_NATS_TLS_KEY=/etc/muto/certs/nats-client.key
```

### Kafka with TLS

**Kafka Broker Configuration:**

```properties
# server.properties
listeners=PLAINTEXT://kafka:9092,SSL://kafka:9093
advertised.listeners=PLAINTEXT://kafka:9092,SSL://kafka:9093
ssl.keystore.location=/etc/kafka/secrets/kafka.server.keystore.jks
ssl.keystore.password=keystore-password
ssl.key.password=key-password
ssl.truststore.location=/etc/kafka/secrets/kafka.server.truststore.jks
ssl.truststore.password=truststore-password
ssl.client.auth=need              # Require client certificates
```

**Muto Configuration:**

```bash
export MUTO_KAFKA_BROKERS=kafka:9093
export MUTO_KAFKA_TLS_ENABLED=true
export MUTO_KAFKA_TLS_CA=/etc/muto/certs/kafka-ca.crt
export MUTO_KAFKA_TLS_CERT=/etc/muto/certs/kafka-client.crt
export MUTO_KAFKA_TLS_KEY=/etc/muto/certs/kafka-client.key
```

### Generate Kafka TLS Artifacts

```bash
# Create keystore for broker
keytool -genkey -alias kafka-broker \
  -keyalg RSA -keysize 2048 \
  -keystore kafka.server.keystore.jks \
  -validity 365

# Export certificate from broker keystore
keytool -export -alias kafka-broker \
  -keystore kafka.server.keystore.jks \
  -file kafka-broker.crt

# Create truststore with CA
keytool -import -alias ca -file ca.crt \
  -keystore kafka.server.truststore.jks

# Create client keystore
keytool -genkey -alias kafka-client \
  -keyalg RSA -keysize 2048 \
  -keystore kafka.client.keystore.jks \
  -validity 365

# Export client certificate
keytool -export -alias kafka-client \
  -keystore kafka.client.keystore.jks \
  -file kafka-client.crt

# Sign client certificate with CA
openssl x509 -req -days 365 \
  -in kafka-client.csr \
  -CA ca.crt -CAkey ca.key \
  -CAcreateserial \
  -out kafka-client-signed.crt

# Import signed cert back into keystore
keytool -import -alias kafka-client \
  -file kafka-client-signed.crt \
  -keystore kafka.client.keystore.jks
```

---

## Certificate Rotation

### Automated Rotation (cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: muto-operator
  namespace: muto-system
spec:
  secretName: muto-tls
  issuerRef:
    name: muto-ca
    kind: Issuer
  commonName: muto-operator.muto-system.svc.cluster.local
  dnsNames:
  - muto-operator.muto-system.svc.cluster.local
  - muto-operator.muto-system
  - muto-operator
  duration: 2160h  # 90 days
  renewBefore: 360h  # 15 days
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: muto-ca
  namespace: muto-system
spec:
  selfsigned: {}
```

### Manual Rotation Process

**Step 1: Generate new certificate**
```bash
openssl genrsa -out tls-new.key 2048
openssl req -new -x509 -key tls-new.key -out tls-new.crt -days 365
```

**Step 2: Update Kubernetes secret**
```bash
kubectl create secret tls muto-tls-new \
  --cert=tls-new.crt \
  --key=tls-new.key \
  -n muto-system

# Update deployment to use new secret
kubectl patch deployment muto-operator -n muto-system \
  -p '{"spec":{"template":{"spec":{"volumes":[{"name":"tls-certs","secret":{"secretName":"muto-tls-new"}}]}}}}'

# Verify pods restarted
kubectl rollout status deployment/muto-operator -n muto-system
```

**Step 3: Update webhook configurations**
```bash
# Encode new CA certificate
NEW_CA=$(cat tls-new.crt | base64 -w 0)

# Update webhook caBundle
kubectl patch validatingwebhookconfigurations muto-validation \
  --type='json' -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'$NEW_CA'}]"
```

**Step 4: Cleanup old certificates**
```bash
# After verifying everything works
kubectl delete secret muto-tls -n muto-system
```

---

## Secret Management

### Kubernetes Secrets

**Store sensitive data in secrets:**

```bash
# Create secret with credentials
kubectl create secret generic muto-credentials \
  --from-literal=username=admin \
  --from-literal=password=secret \
  -n muto-system

# Reference in pod
env:
- name: ADMIN_PASSWORD
  valueFrom:
    secretKeyRef:
      name: muto-credentials
      key: password
```

**Encrypt etcd at rest:**

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
- resources:
  - secrets
  providers:
  - aescbc:
      keys:
      - name: key1
        secret: <base64-encoded-key>
  - identity: {}
```

### External Secret Management

**HashiCorp Vault Integration:**

```bash
# Fetch secret from Vault
vault kv get secret/muto/credentials

# Use in pod via external-secrets
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: vault-backend
  namespace: muto-system
spec:
  provider:
    vault:
      server: "https://vault.example.com"
      path: "secret"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "muto-operator"
---
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: muto-creds
  namespace: muto-system
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: muto-credentials
    creationPolicy: Owner
  data:
  - secretKey: password
    remoteRef:
      key: muto/credentials
      property: password
```

### CloudFoundry Credential Management

**Store in User-Provided Services:**

```bash
# Create user-provided service with credentials
cf create-user-provided-service muto-config \
  -p '{"tls_cert":"-----BEGIN CERT-----...","tls_key":"-----BEGIN KEY-----..."}'

# Reference in manifest
applications:
- name: muto-app
  services:
  - muto-config
  env:
    MUTO_TLS_ENABLED: true
```

---

## Security Best Practices

### 1. Always Use TLS in Production

```bash
# Enable TLS
export MUTO_TLS_ENABLED=true

# Never disable in production
# export MUTO_TLS_ENABLED=false  # BAD!
```

### 2. Use Strong Certificates

```bash
# Minimum: 2048-bit RSA
openssl genrsa -out tls.key 2048

# Better: 4096-bit RSA (slower but stronger)
openssl genrsa -out tls.key 4096

# Best: ECDSA (smaller, faster, secure)
openssl ecparam -name prime256v1 -genkey -noout -out tls.key
```

### 3. Rotate Certificates Before Expiry

```bash
# Check certificate expiry
openssl x509 -in tls.crt -noout -dates

# Example output:
# notBefore=Sep  3 10:00:00 2026 GMT
# notAfter=Sep  3 10:00:00 2027 GMT

# Set alert when 30 days before expiry
EXPIRY=$(date -d "$(openssl x509 -in tls.crt -noout -enddate | cut -d= -f2)" +%s)
NOW=$(date +%s)
DAYS_LEFT=$(( ($EXPIRY - $NOW) / 86400 ))
if [ $DAYS_LEFT -lt 30 ]; then
  echo "Certificate expires in $DAYS_LEFT days - rotate now!"
fi
```

### 4. Monitor Certificate Validity

**Prometheus Alert Rule:**

```yaml
groups:
- name: tls-certificates
  interval: 1h
  rules:
  - alert: CertificateExpiringSoon
    expr: |
      (ssl_cert_not_after - time()) / 86400 < 30
    for: 1h
    annotations:
      summary: "Certificate expiring soon ({{ $value }} days)"
```

### 5. Prevent Secret Leaks

```bash
# Never commit secrets to git
echo 'tls.key' >> .gitignore

# Use gitcrypt or sealed-secrets for encrypted secrets in git
sealed-secrets create-secret \
  --name muto-tls \
  --namespace muto-system \
  < tls-secret.yaml > tls-secret-sealed.yaml
```

### 6. Audit Secret Access

```bash
# Enable audit logging for secret access
kubectl audit webhook --config audit-policy.yaml

# Monitor secret access in logs
kubectl logs -n kube-system -l component=kube-apiserver | grep secret
```

### 7. Use Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: restrict-webhook-traffic
  namespace: muto-system
spec:
  podSelector:
    matchLabels:
      app: muto-operator
  policyTypes:
  - Ingress
  ingress:
  # Only allow from API server
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
      podSelector:
        matchLabels:
          component: kube-apiserver
    ports:
    - protocol: TCP
      port: 8443
```

---

## Troubleshooting TLS Issues

### Certificate Verification Failed

```bash
# Verify certificate chain
openssl verify -CAfile ca.crt tls.crt

# Check certificate details
openssl x509 -in tls.crt -text -noout

# Test TLS connection
openssl s_client -connect muto-operator:8443 -CAfile ca.crt
```

### Webhook Not Working

```bash
# Check webhook configuration
kubectl get validatingwebhookconfigurations -o yaml

# Verify caBundle is base64-encoded
kubectl get validatingwebhookconfigurations muto-validation -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d

# Check webhook service is accessible
kubectl get svc muto-operator -n muto-system

# Test webhook endpoint
kubectl port-forward svc/muto-operator 8443:8443 -n muto-system
curl -k https://localhost:8443/validate/agentjob
```

### mTLS Connection Refused

```bash
# Check client certificate validity
openssl x509 -in client.crt -text -noout

# Verify client cert is signed by CA
openssl verify -CAfile ca.crt client.crt

# Check server is requiring client certs
# In mTLS logs, should see client certificate validation

# Test with openssl
openssl s_client \
  -connect muto-operator:8443 \
  -cert client.crt \
  -key client.key \
  -CAfile ca.crt
```

---

## See Also

- [Environment Variables](./environment-variables.md) — TLS configuration options
- [Deployment Checklist](../deployment/production-checklist.md) — Security verification
- [Architecture: Security Model](../architecture/security-model.md) — How security works
- [Kubernetes Secrets Documentation](https://kubernetes.io/docs/concepts/configuration/secret/) — Official K8s secrets guide

---

**Last Updated:** 2026-09-03
