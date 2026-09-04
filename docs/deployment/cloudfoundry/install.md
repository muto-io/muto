# Installing Muto on CloudFoundry

Deploy Muto to a CloudFoundry environment.

## Prerequisites

### CloudFoundry Environment

- **CF CLI:** Version 7+ or later
- **CloudFoundry instance:** Version v5.0 or later
- **Admin access:** Ability to create organizations and spaces
- **Buildpacks:** Go buildpack available in system buildpacks
- **Service broker:** Optional (for advanced integration)

### Required Tools

- **cf CLI:** Download from [CloudFoundry CLI](https://github.com/cloudfoundry/cli/wiki/V7-cli-installer-downloads)
<<<<<<< HEAD
- **Go:** Version 1.26+ (for local development/testing)
=======
- **Go:** Version 1.22+ (for local development/testing)
>>>>>>> 40719f9 (docs: write deployment/cloudfoundry/install.md - CF installation guide)
- **jq:** JSON query tool (optional, for script automation)

### Credentials and Access

- CloudFoundry API endpoint (e.g., `https://api.cf.example.com`)
- Admin username and password
- Org/space name where Muto will run

## Step 1: Prepare CloudFoundry Environment

### Log In to CloudFoundry

```bash
cf login -a https://api.cf.example.com --sso
# Or with credentials:
cf login -a https://api.cf.example.com -u admin -p password
```

Verify login:
```bash
cf whoami
cf api
```

### Create Organization and Space

Create a dedicated organization for Muto:

```bash
cf create-org muto-platform
cf create-space muto-system -o muto-platform
```

Target the space:
```bash
cf target -o muto-platform -s muto-system
```

Verify targeting:
```bash
cf target
# Output should show:
# api endpoint:   https://api.cf.example.com
# org:            muto-platform
# space:          muto-system
```

### Create Tenant Spaces

For each tenant, create a dedicated space:

```bash
cf create-space tenant-a -o muto-platform
cf create-space tenant-b -o muto-platform
```

## Step 2: Prepare Muto Application

### Clone and Build Muto

Get the source code:

```bash
git clone https://github.com/muto-io/muto.git
cd muto
```

Build the operators and applications:

```bash
make build
```

This creates:
- `bin/muto-operator` — CloudFoundry-compatible operator
- `bin/muto-mcp` — MCP server for Claude integration

### Create Manifest

Create a `manifest.yml` for CloudFoundry deployment:

```yaml
---
version: 1
applications:
  - name: muto-operator
    memory: 512M
    disk: 1G
    instances: 1
    buildpack: go_buildpack
    command: ./bin/muto-operator
    env:
      MUTO_PLATFORM: cloudfoundry
      MUTO_ORG: muto-platform
      MUTO_LOG_LEVEL: info
      MUTO_MESSAGEBUS_TYPE: nats
    services:
      - muto-nats
      - muto-credentials
```

### Alternative: Push via Buildpack

Let CloudFoundry build from source:

```yaml
---
version: 1
applications:
  - name: muto-operator
    memory: 512M
    disk: 1G
    instances: 1
    buildpack: go_buildpack
    stack: cflinuxfs4
    env:
      BP_GO_BUILD_FLAGS: "-ldflags '-s -w'"
      MUTO_PLATFORM: cloudfoundry
      MUTO_LOG_LEVEL: info
```

## Step 3: Set Up Services

### Create User-Provided Services

#### Message Bus Credentials

For NATS:

```bash
cf create-user-provided-service muto-nats -p '{"url":"nats://nats.example.com:4222"}'
```

For Kafka:

```bash
cf create-user-provided-service muto-kafka -p '{
  "brokers": ["kafka-0:9092", "kafka-1:9092"],
  "auth_type": "PLAIN",
  "username": "muto",
  "password": "secret"
}'
```

#### Credentials Service

Store operator credentials:

```bash
cf create-user-provided-service muto-credentials -p '{
  "admin_username": "admin",
  "admin_password": "secure-password",
  "api_key": "muto-api-key-123"
}'
```

### Service Registry Updates

Register services with VCAP_SERVICES:

```bash
cf env muto-operator
# View available services under VCAP_SERVICES
```

## Step 4: Deploy Muto Operator

### Push Application

Deploy to CloudFoundry:

```bash
cf target -o muto-platform -s muto-system
cf push -f manifest.yml
```

Monitor deployment:

```bash
cf logs muto-operator --recent
```

Expected output:
```
2026-09-03T10:30:45.123Z [APP/PROC/WEB/0] OUT INFO: muto-operator started version=0.1.0
2026-09-03T10:30:46.456Z [APP/PROC/WEB/0] OUT INFO: listening on :8080
```

### Verify Deployment

Check application status:

```bash
cf app muto-operator
# Expected: state: started, requested state: started, instances: 1/1 running
```

Check running logs:

```bash
cf logs muto-operator --recent
```

## Step 5: Deploy MCP Server (Optional)

For Claude/LLM integration:

Create `manifest-mcp.yml`:

```yaml
---
version: 1
applications:
  - name: muto-mcp
    memory: 256M
    disk: 512M
    instances: 1
    buildpack: go_buildpack
    command: ./bin/muto-mcp
    env:
      MUTO_PLATFORM: cloudfoundry
      MUTO_LOG_LEVEL: info
      MCP_PORT: 3000
```

Deploy:

```bash
cf push -f manifest-mcp.yml
```

## Step 6: Configure Tenants

### Create Tenant Application Manifests

For tenant-a:

```yaml
---
version: 1
applications:
  - name: tenant-a-worker
    memory: 256M
    disk: 512M
    instances: 1
    buildpack: go_buildpack
    command: ./bin/muto-agent
    env:
      TENANT_ID: tenant-a
      MUTO_OPERATOR: https://muto-operator.cf.example.com
      NATS_URL: nats://nats.example.com:4222
      NATS_CREDS: /etc/nats-credentials
    services:
      - muto-nats
```

Deploy to tenant space:

```bash
cf target -o muto-platform -s tenant-a
cf push -f manifest-tenant-a.yml
```

Verify:

```bash
cf apps
# Should show: tenant-a-worker with status "started"
```

## Step 7: Verify Installation

### Health Checks

Check operator endpoint:

```bash
OPERATOR_URL=$(cf app muto-operator | grep routes | awk '{print "https://"$2}')
curl -k $OPERATOR_URL/healthz
# Expected: "ok"
```

Check metrics endpoint:

```bash
curl -k $OPERATOR_URL/metrics | head -20
# Expected: Prometheus metrics output
```

### Test Tenant Connectivity

SSH into tenant app and verify connectivity:

```bash
cf ssh tenant-a-worker
# Inside container:
env | grep NATS_URL
telnet nats.example.com 4222
```

### Create Test Task

Create a simple agent task:

```bash
cf target -o muto-platform -s muto-system

cf run-task muto-operator \
  --command="echo 'Hello from Muto on CloudFoundry'" \
  --name test-task-1 \
  --memory=128

cf tasks muto-operator
```

### Success Indicators

- [ ] `cf app muto-operator` shows status "started"
- [ ] Health endpoint (`/healthz`) returns 200 OK
- [ ] Message bus connectivity verified
- [ ] Tenant apps are running
- [ ] Test task completes successfully

## Scaling Muto on CloudFoundry

### Horizontal Scaling

Increase instances:

```bash
cf scale muto-operator -i 3
```

Monitor:

```bash
cf app muto-operator
# Should show instances: 3/3 running
```

### Memory/Disk Adjustment

Update manifest and redeploy:

```yaml
applications:
  - name: muto-operator
    memory: 1G
    disk: 2G
```

Apply:

```bash
cf push -f manifest.yml
```

### Autoscaling (App Autoscaler)

If App Autoscaler is available:

```bash
cf create-autoscaling-rule muto-operator \
  --min-instances=1 \
  --max-instances=5 \
  --metric=memory \
  --threshold=80
```

## Monitoring and Logs

### View Live Logs

```bash
cf logs muto-operator
```

### Tail Recent Logs

```bash
cf logs muto-operator --recent
```

### Filter Logs

```bash
cf logs muto-operator --recent | grep ERROR
```

### Log Aggregation

Send logs to external system:

```bash
cf cups log-aggregator \
  -l syslog://logs.example.com:514

cf bind-service muto-operator log-aggregator
cf restage muto-operator
```

## Troubleshooting

### Application won't start

Check recent logs:
```bash
cf logs muto-operator --recent
```

Check system limits:
```bash
cf space-quota-usage
cf org-quota-usage
```

Check available buildpacks:
```bash
cf buildpacks | grep go
```

### Connectivity Issues

Verify network policies:
```bash
cf network-policies
```

Check security groups:
```bash
cf security-groups
cf running-security-groups
```

### Service binding failures

List available services:
```bash
cf services
```

Verify service credentials:
```bash
cf service-key muto-nats default
```

### Memory/CPU pressure

Check current usage:
```bash
cf app muto-operator
# Look at memory usage

cf app-events muto-operator
# Check for memory errors
```

Increase resources:
```bash
cf scale muto-operator -m 1G
```

## Uninstall Muto

### Delete Applications

```bash
cf delete muto-operator
cf delete muto-mcp
```

### Delete Services

```bash
cf delete-service muto-nats
cf delete-service muto-kafka
cf delete-service muto-credentials
```

### Clean Up Spaces

```bash
cf delete-space muto-system
cf delete-space tenant-a
cf delete-space tenant-b
```

### Delete Organization

```bash
cf delete-org muto-platform
```

## Next Steps

- **[CloudFoundry Configuration](./configuration.md)** — Advanced CF-specific settings
- **[Production Checklist](../production-checklist.md)** — Pre-launch verification
- **[Configuration Guide](../../configuration/)** — Environment variables and tuning

---

**Last Updated:** 2026-09-03
