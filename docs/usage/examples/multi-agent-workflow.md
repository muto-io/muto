# Example: Multi-Agent Data Pipeline

A realistic example of a complex data processing workflow with multiple coordinated agents.

## Scenario

Build a data pipeline that:
1. **Extracts** data from multiple sources
2. **Validates** the extracted data
3. **Transforms** it into a standard format (parallel processing)
4. **Aggregates** results and publishes to storage

This is a common pattern for ETL (Extract, Transform, Load) operations.

## Architecture

```
Sources (S3, Database, API)
    │
    ▼
┌─────────────────┐
│    Extractor    │
│  (reads data)   │
└────────┬────────┘
         │ Output: /tmp/extracted.json
         ▼
┌─────────────────┐
│   Validator     │
│ (checks format) │
└────────┬────────┘
         │ Output: /tmp/validated.json
         │
         ├──────────────────────────────────┐
         │   (fan-out to parallel workers)  │
         │                                  │
         ▼                                  ▼
    ┌────────────┐                  ┌────────────┐
    │ Transformer│                  │ Transformer│
    │  (Process) │                  │  (Process) │
    └─────┬──────┘                  └──────┬─────┘
          │ /tmp/transform-0.json          │ /tmp/transform-1.json
          │                                │
          └────────────────┬───────────────┘
                           │ (fan-in)
                           ▼
                  ┌─────────────────┐
                  │   Aggregator    │
                  │  (combine data) │
                  └────────┬────────┘
                           │
                           ▼
                   S3://results/output.json
```

## Complete Job Definition

Save this as `data-pipeline.yaml`:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: data-pipeline-daily
  namespace: production
  labels:
    app: data-pipeline
    schedule: daily
  annotations:
    description: "Daily ETL data pipeline"
spec:
  # Job configuration
  tenant: acme-corp
  priority: 75
  timeout: 2h
  activeDeadlineSeconds: 7200
  ttlSecondsAfterFinished: 86400    # Keep job for 1 day
  
  # Resource requests
  retryPolicy:
    maxRetries: 2
    backoffSeconds: 30
    backoffMultiplier: 2.0
    maxBackoffSeconds: 300
  
  # Stage 1: Extract data from sources
  agents:
    - name: extractor
      image: myorg/data-extractor:v2.1.0
      timeout: 30m
      
      resources:
        requests:
          cpu: "500m"
          memory: "512Mi"
        limits:
          cpu: "2"
          memory: "2Gi"
      
      env:
        # Data sources
        - name: S3_BUCKET
          value: "s3://company-data/raw"
        - name: DB_HOST
          value: "postgres.production.svc.cluster.local"
        - name: DB_NAME
          value: "analytics"
        - name: DB_USER
          valueFrom:
            secretKeyRef:
              name: db-credentials
              key: username
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: db-credentials
              key: password
        
        # Output
        - name: OUTPUT_FILE
          value: "/tmp/extracted.json"
        - name: LOG_LEVEL
          value: "info"
      
      # Health checks
      livenessProbe:
        httpGet:
          path: /health
          port: 8080
        initialDelaySeconds: 10
        periodSeconds: 30
        timeoutSeconds: 5
      
      readinessProbe:
        httpGet:
          path: /ready
          port: 8080
        initialDelaySeconds: 5
        periodSeconds: 10
    
    # Stage 2: Validate extracted data
    - name: validator
      image: myorg/data-validator:v1.3.0
      timeout: 15m
      
      dependsOn:
        - extractor       # Wait for extractor to complete
      
      resources:
        requests:
          cpu: "250m"
          memory: "256Mi"
        limits:
          cpu: "1"
          memory: "1Gi"
      
      env:
        - name: INPUT_FILE
          value: "/tmp/extracted.json"
        - name: OUTPUT_FILE
          value: "/tmp/validated.json"
        - name: SCHEMA_URL
          value: "s3://company-data/schemas/input-schema.json"
        - name: STRICT_MODE
          value: "true"
        - name: LOG_LEVEL
          value: "info"
      
      retryPolicy:
        maxRetries: 1     # Validations are strict, minimal retries
    
    # Stage 3a: Transform using processor 1
    - name: transformer-1
      image: myorg/data-transformer:v1.5.0
      timeout: 45m
      
      dependsOn:
        - validator       # Wait for validator
      
      resources:
        requests:
          cpu: "1"
          memory: "1Gi"
        limits:
          cpu: "4"
          memory: "4Gi"
      
      env:
        - name: INPUT_FILE
          value: "/tmp/validated.json"
        - name: OUTPUT_FILE
          value: "/tmp/transform-0.json"
        - name: WORKER_ID
          value: "0"
        - name: TOTAL_WORKERS
          value: "2"
        - name: TRANSFORM_RULES_URL
          value: "s3://company-data/rules/transform-rules.yaml"
        - name: LOG_LEVEL
          value: "info"
    
    # Stage 3b: Transform using processor 2
    - name: transformer-2
      image: myorg/data-transformer:v1.5.0
      timeout: 45m
      
      dependsOn:
        - validator       # Wait for validator
      
      resources:
        requests:
          cpu: "1"
          memory: "1Gi"
        limits:
          cpu: "4"
          memory: "4Gi"
      
      env:
        - name: INPUT_FILE
          value: "/tmp/validated.json"
        - name: OUTPUT_FILE
          value: "/tmp/transform-1.json"
        - name: WORKER_ID
          value: "1"
        - name: TOTAL_WORKERS
          value: "2"
        - name: TRANSFORM_RULES_URL
          value: "s3://company-data/rules/transform-rules.yaml"
        - name: LOG_LEVEL
          value: "info"
    
    # Stage 4: Aggregate results
    - name: aggregator
      image: myorg/data-aggregator:v2.0.0
      timeout: 20m
      
      dependsOn:
        - transformer-1   # Wait for both transformers
        - transformer-2
      
      resources:
        requests:
          cpu: "500m"
          memory: "512Mi"
        limits:
          cpu: "2"
          memory: "2Gi"
      
      env:
        - name: INPUT_FILES
          value: "/tmp/transform-0.json,/tmp/transform-1.json"
        - name: OUTPUT_FILE
          value: "/tmp/aggregated.json"
        - name: S3_OUTPUT_BUCKET
          value: "s3://company-data/processed"
        - name: OUTPUT_FORMAT
          value: "parquet"
        - name: LOG_LEVEL
          value: "info"
      
      # Run notification on completion
      command:
        - /bin/sh
        - -c
        - |
          # Run aggregation
          /app/aggregate.sh
          STATUS=$?
          
          # Notify on completion
          if [ $STATUS -eq 0 ]; then
            curl -X POST http://monitoring/webhook \
              -H "Content-Type: application/json" \
              -d '{"job":"data-pipeline-daily","status":"success"}'
          else
            curl -X POST http://monitoring/webhook \
              -H "Content-Type: application/json" \
              -d '{"job":"data-pipeline-daily","status":"failed"}'
          fi
          
          exit $STATUS
```

## Execution Walkthrough

### Step 1: Create the Job

```bash
kubectl apply -f data-pipeline.yaml
```

### Step 2: Monitor Extraction (Stage 1)

```bash
# Watch extractor run
kubectl get agentjob data-pipeline-daily -o json | \
  jq '.status.agents[] | select(.name == "extractor")'

# Follow logs
kubectl logs agentjob/data-pipeline-daily --container=extractor --follow
```

Expected behavior:
- Reads from S3 bucket and database
- Generates `/tmp/extracted.json` with combined data
- Takes ~10-15 minutes

### Step 3: Monitor Validation (Stage 2)

```bash
# Watch validator after extractor completes
kubectl get agentjob data-pipeline-daily --watch

# Check validator logs
kubectl logs agentjob/data-pipeline-daily --container=validator --follow
```

Expected behavior:
- Waits for extractor to complete
- Validates data against schema
- Generates `/tmp/validated.json`
- Takes ~5-10 minutes

### Step 4: Monitor Parallel Transformation (Stage 3)

```bash
# Watch both transformers run in parallel
kubectl get agentjob data-pipeline-daily -o json | \
  jq '.status.agents[] | select(.name | startswith("transformer"))'

# Check transformer logs
kubectl logs agentjob/data-pipeline-daily --container=transformer-1 --follow &
kubectl logs agentjob/data-pipeline-daily --container=transformer-2 --follow
```

Expected behavior:
- Both transformers start immediately after validator completes
- Process different partitions of validated data
- Run in parallel (faster than sequential)
- Each generates transform output file
- Takes ~30-45 minutes each (in parallel)

### Step 5: Monitor Aggregation (Stage 4)

```bash
# Watch aggregator after transformers complete
kubectl logs agentjob/data-pipeline-daily --container=aggregator --follow
```

Expected behavior:
- Waits for both transformers to complete
- Combines results
- Publishes to S3
- Sends webhook notification
- Takes ~15-20 minutes

### Step 6: Verify Completion

```bash
# Check final status
kubectl get agentjob data-pipeline-daily

# Get detailed status
kubectl describe agentjob data-pipeline-daily

# Check all logs
kubectl logs agentjob/data-pipeline-daily

# Verify output in S3
aws s3 ls s3://company-data/processed/
```

## Key Features Demonstrated

### 1. Dependencies

Agents specify what they depend on:

```yaml
# Validator depends on extractor
- name: validator
  dependsOn:
    - extractor

# Both transformers depend on validator
- name: transformer-1
  dependsOn:
    - validator
```

### 2. Parallel Execution

Transformers run in parallel (they both depend on validator, not on each other):

```yaml
# transformer-1 and transformer-2 both depend on validator
# But not on each other, so they run simultaneously
- name: transformer-1
  dependsOn: [validator]
- name: transformer-2
  dependsOn: [validator]
```

### 3. Resource Allocation

Each stage has appropriate resource requests:

```yaml
# Extractor: light (reading, network-bound)
resources:
  requests:
    cpu: "500m"
    memory: "512Mi"

# Transformer: heavy (CPU-bound processing)
resources:
  requests:
    cpu: "1"
    memory: "1Gi"
  limits:
    cpu: "4"
    memory: "4Gi"
```

### 4. Timeouts

Stage-specific timeouts prevent hanging:

```yaml
- name: extractor
  timeout: 30m        # Reading data: 30 minutes

- name: transformer-1
  timeout: 45m        # Heavy processing: 45 minutes
```

### 5. Health Checks

Extractor includes health checks:

```yaml
livenessProbe:
  httpGet:
    path: /health     # Is it alive?
    port: 8080
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /ready      # Is it ready to work?
    port: 8080
```

### 6. Environment Configuration

Agents receive configuration via environment variables:

```yaml
env:
  - name: S3_BUCKET
    value: "s3://company-data/raw"
  
  - name: DB_PASSWORD
    valueFrom:
      secretKeyRef:
        name: db-credentials
        key: password
```

### 7. Scheduling

Job prioritization and timing:

```yaml
priority: 75                         # Higher priority = scheduled sooner
ttlSecondsAfterFinished: 86400      # Keep for 1 day, then auto-delete
activeDeadlineSeconds: 7200         # Hard limit: 2 hours max
```

## Running This Example

### Prepare Your Cluster

```bash
# 1. Ensure Muto is installed
kubectl get deployment -n muto-system muto-operator

# 2. Create namespace
kubectl create namespace production

# 3. Create secrets for database credentials
kubectl create secret generic db-credentials \
  --from-literal=username=analytics_user \
  --from-literal=password=secure-password \
  -n production

# 4. (Optional) Create ConfigMaps for rules and schemas
kubectl create configmap transform-rules \
  --from-file=transform-rules.yaml=./rules/transform-rules.yaml \
  -n production
```

### Deploy the Job

```bash
# Apply the job
kubectl apply -f data-pipeline.yaml

# Monitor progress
kubectl get agentjobs -n production --watch

# Get detailed status
kubectl describe agentjob data-pipeline-daily -n production

# View logs
kubectl logs agentjob/data-pipeline-daily -n production --all-containers=true
```

### Post-Processing

```bash
# After job completes, verify results
aws s3 ls s3://company-data/processed/

# Download results for analysis
aws s3 cp s3://company-data/processed/output.parquet ./local-results.parquet

# Archive logs
kubectl logs agentjob/data-pipeline-daily -n production > job-logs.txt
```

## Modifications for Your Use Case

### Change Data Sources

```yaml
# Use different data sources
env:
  - name: KAFKA_BROKERS
    value: "kafka-1:9092,kafka-2:9092"
  - name: KAFKA_TOPIC
    value: "events"
```

### Add More Stages

```yaml
# Add data quality checks before aggregation
- name: quality-check
  image: myorg/quality-checker:v1
  dependsOn:
    - transformer-1
    - transformer-2
  timeout: 15m
```

### Scale Parallel Processing

```yaml
# Instead of 2 transformers, use 8 for larger datasets
- name: transformer-{0..7}  # Note: manually expand to transformer-0 through transformer-7
  dependsOn:
    - validator
```

### Add Error Handling

```yaml
# Retry on transient failures
retryPolicy:
  maxRetries: 3
  backoffSeconds: 30

# Alert on failures
command:
  - /bin/sh
  - -c
  - |
    /app/aggregate.sh || {
      curl -X POST http://alerts/webhook \
        -d '{"alert":"Pipeline failed"}'
      exit 1
    }
```

## Monitoring and Debugging

### Check Job Progress

```bash
# Graphical progress (repeated every 2s)
watch kubectl get agentjob data-pipeline-daily -o wide

# JSON status
kubectl get agentjob data-pipeline-daily -o json | jq '.status'

# Agent-level status
kubectl get agentjob data-pipeline-daily -o json | \
  jq '.status.agents[] | {name, phase, duration}'
```

### Debug Failures

```bash
# Check which stage failed
kubectl describe agentjob data-pipeline-daily | grep -A 2 "Events:"

# View full logs
kubectl logs agentjob/data-pipeline-daily --all-containers=true

# Check specific agent logs
kubectl logs agentjob/data-pipeline-daily --container=transformer-1

# Check exit codes
kubectl get agentjob data-pipeline-daily -o json | \
  jq '.status.agents[] | {name, exitCode}'
```

### Performance Analysis

```bash
# Check execution timeline
kubectl get agentjob data-pipeline-daily -o json | \
  jq '.status | {startTime, completionTime, duration}'

# Check resource usage
kubectl top pods -l job=data-pipeline-daily

# Find slow stages
kubectl get agentjob data-pipeline-daily -o json | \
  jq '.status.agents[] | {name, duration}' | sort -k3 -rn
```

## Next Steps

- **[Scheduling Agent Jobs](../scheduling-agent-jobs.md)** — Learn all job options
- **[Multi-Agent Patterns](../multi-agent-patterns.md)** — Learn other orchestration patterns
- **[Best Practices](../best-practices.md)** — Optimize for production scale
- **[Custom Reconciler Example](./custom-reconciler.md)** — Write custom orchestration logic

---

**Last Updated:** 2026-09-03
