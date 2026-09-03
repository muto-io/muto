# CloudFoundry-Specific Configuration

Advanced CloudFoundry configuration options for Muto.

## Environment Variables

CloudFoundry environment variables are set in the manifest or via `cf set-env`.

### Core Configuration

```bash
# Operator settings
cf set-env muto-operator MUTO_PLATFORM cloudfoundry
cf set-env muto-operator MUTO_LOG_LEVEL info
cf set-env muto-operator MUTO_API_PORT 8080
cf set-env muto-operator MUTO_METRICS_PORT 9090

# Organization and space
cf set-env muto-operator CF_ORG muto-platform
cf set-env muto-operator CF_SPACE muto-system
cf set-env muto-operator CF_API_ENDPOINT https://api.cf.example.com

# Credentials
cf set-env muto-operator CF_USERNAME admin
cf set-env muto-operator CF_PASSWORD $(cf credentials)
```

### Message Bus Configuration

#### NATS Configuration

```bash
# NATS server URL
cf set-env muto-operator NATS_URL nats://nats.cf.example.com:4222

# NATS authentication
cf set-env muto-operator NATS_USERNAME nats-user
cf set-env muto-operator NATS_PASSWORD nats-password

# NATS connection settings
cf set-env muto-operator NATS_MAX_RECONNECT 10
cf set-env muto-operator NATS_RECONNECT_WAIT 1s
cf set-env muto-operator NATS_PING_INTERVAL 30s
```

#### Kafka Configuration

```bash
# Kafka brokers (comma-separated)
cf set-env muto-operator KAFKA_BROKERS kafka-0:9092,kafka-1:9092,kafka-2:9092

# Kafka authentication
cf set-env muto-operator KAFKA_AUTH_TYPE PLAIN
cf set-env muto-operator KAFKA_USERNAME kafka-user
cf set-env muto-operator KAFKA_PASSWORD kafka-password

# Kafka connection settings
cf set-env muto-operator KAFKA_COMPRESSION snappy
cf set-env muto-operator KAFKA_PARTITION_COUNT 3
cf set-env muto-operator KAFKA_REPLICATION_FACTOR 3
```

### Reconciler Configuration

```bash
# Worker thread configuration
cf set-env muto-operator RECONCILER_WORKERS 4
cf set-env muto-operator RECONCILER_MAX_CONCURRENT 20
cf set-env muto-operator RECONCILER_SYNC_PERIOD 10m

# Retry configuration
cf set-env muto-operator RECONCILER_RETRY_INITIAL_BACKOFF 100ms
cf set-env muto-operator RECONCILER_RETRY_MAX_BACKOFF 1000ms
cf set-env muto-operator RECONCILER_RETRY_MAX_RETRIES 15
```

### Logging Configuration

```bash
# Log format
cf set-env muto-operator LOG_FORMAT json

# Log level
cf set-env muto-operator LOG_LEVEL debug

# Structured logging
cf set-env muto-operator LOG_INCLUDE_TIMESTAMP true
cf set-env muto-operator LOG_INCLUDE_CALLER true
```

### Resource Management

```bash
# Agent resource limits
cf set-env muto-operator AGENT_DEFAULT_CPU 500m
cf set-env muto-operator AGENT_DEFAULT_MEMORY 256Mi
cf set-env muto-operator AGENT_MAX_CPU 2000m
cf set-env muto-operator AGENT_MAX_MEMORY 4Gi

# Task lifecycle
cf set-env muto-operator TASK_TIMEOUT 1h
cf set-env muto-operator TASK_DEFAULT_TIMEOUT 30m
cf set-env muto-operator TASK_GRACE_PERIOD 5m
```

## Manifest Configuration

### Basic Manifest

```yaml
---
version: 1
applications:
  - name: muto-operator
    memory: 512M
    disk: 1G
    instances: 1
    stack: cflinuxfs4
    buildpack: go_buildpack
    command: ./bin/muto-operator
    health-check-type: process
    timeout: 180
    
    env:
      MUTO_PLATFORM: cloudfoundry
      MUTO_LOG_LEVEL: info
      CF_API_ENDPOINT: https://api.cf.example.com
      
    services:
      - muto-nats
      - muto-postgres
```

### Production Manifest

```yaml
---
version: 1
applications:
  - name: muto-operator
    memory: 1G
    disk: 2G
    instances: 3
    stack: cflinuxfs4
    buildpack: go_buildpack
    command: ./bin/muto-operator
    
    health-check-type: http
    health-check-http-endpoint: /healthz
    health-check-timeout: 180
    health-check-interval: 10
    
    timeout: 180
    
    env:
      MUTO_PLATFORM: cloudfoundry
      MUTO_LOG_LEVEL: info
      MUTO_METRICS_PORT: 9090
      
      CF_API_ENDPOINT: https://api.cf.example.com
      CF_SKIP_SSL_VALIDATION: false
      
      NATS_URL: nats://nats.cf.example.com:4222
      NATS_USERNAME: {{ .nats_username }}
      NATS_PASSWORD: {{ .nats_password }}
      
      RECONCILER_WORKERS: 8
      RECONCILER_MAX_CONCURRENT: 40
      RECONCILER_SYNC_PERIOD: 5m
      
      LOG_FORMAT: json
      LOG_LEVEL: info
    
    services:
      - muto-nats
      - muto-postgres
      - muto-credentials
    
    metadata:
      labels:
        tier: platform
        component: orchestration
```

## Service Binding

### VCAP_SERVICES

CloudFoundry injects service credentials via `VCAP_SERVICES` environment variable.

Access in code:

```go
package main

import (
    "encoding/json"
    "os"
)

type ServiceCreds struct {
    Credentials map[string]interface{} `json:"credentials"`
    Name        string                 `json:"name"`
}

func getServiceCreds(serviceName string) ServiceCreds {
    vcap := os.Getenv("VCAP_SERVICES")
    var services map[string][]ServiceCreds
    json.Unmarshal([]byte(vcap), &services)
    
    for _, service := range services[serviceName] {
        if service.Name == serviceName {
            return service
        }
    }
    return ServiceCreds{}
}
```

### User-Provided Services

Create for external services:

```bash
# NATS cluster
cf create-user-provided-service muto-nats \
  -p '{"url": "nats://user:pass@nats.example.com:4222"}'

# PostgreSQL for state
cf create-user-provided-service muto-postgres \
  -p '{"host": "pg.example.com", "port": 5432, "username": "muto", "password": "secret", "database": "muto"}'

# Credentials
cf create-user-provided-service muto-credentials \
  -p '{"admin_api_key": "key-123", "admin_username": "admin"}'
```

### Managed Services

If using CF-managed services:

```bash
# Create Postgres instance
cf create-service postgres standard muto-db

# Create Redis cache
cf create-service redis standard muto-cache

# Bind to application
cf bind-service muto-operator muto-db
cf bind-service muto-operator muto-cache
```

## Organization and Space Management

### Multi-Tenant Setup

Create isolated spaces for each tenant:

```bash
# Create organization
cf create-org customer-a

# Create spaces
cf create-space production -o customer-a
cf create-space staging -o customer-a
cf create-space dev -o customer-a

# Set quotas
cf create-space-quota production-quota -o customer-a \
  -m 100G -r 5 -s 10

cf assign-space-quota production-quota production -o customer-a
```

### RBAC Configuration

#### Organization Role Assignment

```bash
# Org manager
cf set-org-role user@customer-a.com customer-a OrgManager

# Billing manager
cf set-org-role finance@customer-a.com customer-a BillingManager
```

#### Space Role Assignment

```bash
# Space developer
cf set-space-role dev@customer-a.com customer-a production SpaceDeveloper

# Space manager
cf set-space-role ops@customer-a.com customer-a production SpaceManager

# Space auditor
cf set-space-role audit@customer-a.com customer-a production SpaceAuditor
```

## Security Configuration

### Network Policies

Define allowed traffic between apps:

```bash
# Allow muto-operator to talk to muto-nats
cf add-network-policy muto-operator muto-nats --protocol tcp --port 4222

# Allow tenant-a-worker to talk to message bus
cf add-network-policy tenant-a-worker muto-nats --protocol tcp --port 4222

# View policies
cf network-policies
```

### Security Groups

Create security group for external message bus:

```bash
cat > muto-messagebeus-sg.json <<'EOF'
[
  {
    "protocol": "tcp",
    "destination": "nats.example.com",
    "ports": "4222"
  },
  {
    "protocol": "tcp",
    "destination": "kafka.example.com",
    "ports": "9092"
  }
]
EOF

cf create-security-group muto-external muto-external-sg.json
cf bind-security-group muto-external muto-platform muto-system
```

### Route Security

Configure HTTPS for exposed routes:

```bash
# Map custom domain with SSL
cf map-route muto-operator example.com --hostname muto

# Create SSL certificate
cf create-shared-domain example.com --https-only

# Update app manifest
cf push -f manifest.yml
```

## Task Configuration

### Run Tasks Directly

Execute one-off tasks in CloudFoundry:

```bash
# Run a task
cf run-task muto-operator \
  --command="./bin/muto-migrate" \
  --name migration-task-1 \
  --memory 256M \
  --disk 512M

# Monitor task
cf tasks muto-operator
cf task muto-operator 1

# Stream task logs
cf logs muto-operator --recent
```

### Scheduled Tasks

Use Cloud Scheduler or similar for recurring tasks:

```bash
# Daily backup at 2 AM
cf create-scheduled-task muto-operator \
  --command="./bin/muto-backup" \
  --expression="0 2 * * *" \
  --name backup-daily
```

## Monitoring and Logging

### Application Metrics

CloudFoundry provides built-in metrics:

```bash
# Current metrics
cf app muto-operator

# Detailed metrics
cf app muto-operator --guid | xargs -I {} \
  curl https://api.cf.example.com/v2/apps/{}/stats
```

### Log Streaming

### Real-time logs

```bash
cf logs muto-operator
```

### Recent logs

```bash
cf logs muto-operator --recent
```

### Log Forwarding

Send logs to external system:

```bash
# Create log drain
cf create-user-provided-service logs-drain \
  -l syslog://logs.example.com:514

cf bind-service muto-operator logs-drain
cf restage muto-operator
```

### Application Performance Monitoring

Integration example with third-party APM:

```bash
cf set-env muto-operator APM_ENABLED true
cf set-env muto-operator APM_AGENT_PYTHON_VERSION 4.26.1
cf restage muto-operator
```

## Cost Management

### Quotas and Limits

Define organization quotas:

```bash
# Create quota
cf create-quota muto-quota \
  -m 500G \
  -i 1G \
  -r 100 \
  -s 50

# Apply quota
cf set-quota muto-platform muto-quota
```

### Space Quotas

```bash
# Per-space limits
cf create-space-quota muto-space-quota \
  -m 100G \
  -i 512M \
  -r 20 \
  -s 10 \
  -o muto-platform

cf assign-space-quota muto-space-quota muto-system -o muto-platform
```

## Backup and Disaster Recovery

### Backup Configuration

```bash
# Backup manifest
cf apps > backup-apps.txt

# Export org/space config
cf export-config --path exported-config.zip

# Backup service bindings
cf services > backup-services.txt
```

### Recovery Procedure

```bash
# Restore from exported config
cf import-config exported-config.zip

# Rebind services
cf bind-service muto-operator muto-nats

# Restage application
cf restage muto-operator
```

## Troubleshooting

### Application Crashes

Check crash logs:
```bash
cf logs muto-operator --recent | grep ERROR
cf app muto-operator
# Look for exit code in event history
```

### Out of Memory

Increase memory:
```bash
cf scale muto-operator -m 1G
```

Monitor memory:
```bash
cf app muto-operator | grep memory
```

### Startup Timeout

Increase timeout in manifest:
```yaml
timeout: 300
```

Or via command:
```bash
cf update-app-manifest muto-operator --startup-timeout 300
```

### Service Connection Issues

Verify service binding:
```bash
cf services
cf service-key muto-nats default
```

Check firewall/security groups:
```bash
cf security-groups
cf running-security-groups
```

---

**Last Updated:** 2026-09-03
