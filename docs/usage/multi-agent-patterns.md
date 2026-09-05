# Multi-Agent Orchestration Patterns

Master common patterns for coordinating multiple agents to build complex workflows.

## Overview

Multi-agent workflows coordinate multiple agents to accomplish complex tasks. Muto supports several orchestration patterns, each suited for different use cases. This guide covers the patterns and shows how to implement them.

## Pattern 1: Sequential Execution

Run agents one after another, each depending on the previous one's output.

```
Agent A -> Agent B -> Agent C -> Done
  ↓        ↓        ↓
Output 1   Input: Output 1   Input: Output 2
           Output 2         Output 3
```

Use sequential when:
- Later stages depend on earlier results
- Must process data in order
- Reducing parallelism for resource constraints

### Sequential Execution Example

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: sequential-pipeline
spec:
  tenant: default
  agents:
    # Stage 1: Extract data
    - name: extractor
      image: myorg/extractor:v1
      env:
        - name: OUTPUT_FILE
          value: /tmp/extracted.json
    
    # Stage 2: Process extracted data
    - name: processor
      image: myorg/processor:v1
      dependsOn:
        - extractor          # Wait for extractor to complete
      env:
        - name: INPUT_FILE
          value: /tmp/extracted.json
        - name: OUTPUT_FILE
          value: /tmp/processed.json
    
    # Stage 3: Aggregate results
    - name: aggregator
      image: myorg/aggregator:v1
      dependsOn:
        - processor          # Wait for processor to complete
      env:
        - name: INPUT_FILE
          value: /tmp/processed.json
        - name: OUTPUT_FILE
          value: s3://bucket/final-result.json
```

### When to Use

- Data transformation pipelines
- Multi-stage processing workflows
- Where each stage significantly reduces data volume
- Waterfall-style processes

### Advantages

- Simple to reason about
- Easy to debug (clear stage boundaries)
- Natural for sequential algorithms
- Clear dependencies

### Disadvantages

- Slow (no parallelism)
- Early stage failures block everything
- Inefficient resource utilization
- Cannot run independent work in parallel

---

## Pattern 2: Parallel Execution

Run multiple agents concurrently, each processing independently.

```
        ┌─ Agent A ─┐
        │           │
 Input ─┤─ Agent B ─├─ Aggregator -> Output
        │           │
        └─ Agent C ─┘
```

Use parallel when:
- Agents work independently
- Same input -> different processing -> combine results
- Can utilize multiple cores/nodes

### Parallel Execution Example

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: parallel-processing
spec:
  tenant: default
  parallelism: 3          # Run up to 3 agents in parallel
  agents:
    # These three agents run simultaneously
    - name: processor-1
      image: myorg/processor:v1
      env:
        - name: WORKER_ID
          value: "1"
        - name: OUTPUT_FILE
          value: /tmp/result-1.json
    
    - name: processor-2
      image: myorg/processor:v1
      env:
        - name: WORKER_ID
          value: "2"
        - name: OUTPUT_FILE
          value: /tmp/result-2.json
    
    - name: processor-3
      image: myorg/processor:v1
      env:
        - name: WORKER_ID
          value: "3"
        - name: OUTPUT_FILE
          value: /tmp/result-3.json
    
    # This runs after all three complete
    - name: aggregator
      image: myorg/aggregator:v1
      dependsOn:
        - processor-1
        - processor-2
        - processor-3
      env:
        - name: INPUT_FILES
          value: "/tmp/result-1.json,/tmp/result-2.json,/tmp/result-3.json"
        - name: OUTPUT_FILE
          value: /tmp/aggregated.json
```

### When to Use

- Independent data processing (splits)
- Embarrassingly parallel problems
- Multiple API calls to combine
- Load testing and stress testing

### Advantages

- Fast (full parallelism)
- Scales with cluster size
- Good resource utilization
- Efficient for I/O-bound work

### Disadvantages

- More complex coordination
- Harder to debug across parallel paths
- Potential race conditions with shared storage
- Requires synchronization points

---

## Pattern 3: Fan-Out / Fan-In

Distribute work across many agents, then consolidate results.

```
       Input
         │
         ▼
   ┌─ Distributor ─┐
   │               │
   ▼               ▼
 Worker-1    Worker-2
   ▼               ▼
 Result-1    Result-2  (repeats for N workers)
   │               │
   └────┬──────────┘
        ▼
   Consolidator
        ▼
      Output
```

Use fan-out/fan-in when:
- Need to process large datasets in chunks
- Distribute compute-intensive work
- Need horizontal scaling
- Batch processing with many independent units

### Fan-Out/Fan-In Example

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: fan-out-fan-in
spec:
  tenant: default
  agents:
    # Distributor: split work into chunks
    - name: distributor
      image: myorg/distributor:v1
      env:
        - name: INPUT_DATA
          value: s3://bucket/large-dataset.csv
        - name: CHUNK_COUNT
          value: "10"
        - name: CHUNK_DIR
          value: /tmp/chunks
    
    # Fan-out: 10 workers process chunks in parallel
    - name: worker-0
      image: myorg/worker:v1
      dependsOn:
        - distributor
      env:
        - name: CHUNK_ID
          value: "0"
        - name: INPUT_FILE
          value: /tmp/chunks/chunk-0.csv
        - name: OUTPUT_FILE
          value: /tmp/results/result-0.json
    
    - name: worker-1
      image: myorg/worker:v1
      dependsOn:
        - distributor
      env:
        - name: CHUNK_ID
          value: "1"
        - name: INPUT_FILE
          value: /tmp/chunks/chunk-1.csv
        - name: OUTPUT_FILE
          value: /tmp/results/result-1.json
    
    # ... repeat for worker-2 through worker-9 ...
    
    - name: worker-9
      image: myorg/worker:v1
      dependsOn:
        - distributor
      env:
        - name: CHUNK_ID
          value: "9"
        - name: INPUT_FILE
          value: /tmp/chunks/chunk-9.csv
        - name: OUTPUT_FILE
          value: /tmp/results/result-9.json
    
    # Fan-in: consolidate all results
    - name: consolidator
      image: myorg/consolidator:v1
      dependsOn:
        - worker-0
        - worker-1
        - worker-2
        - worker-3
        - worker-4
        - worker-5
        - worker-6
        - worker-7
        - worker-8
        - worker-9
      env:
        - name: RESULTS_DIR
          value: /tmp/results
        - name: OUTPUT_FILE
          value: s3://bucket/final-result.json
```

### When to Use

- Large-scale data processing
- Map-reduce style workflows
- Batch processing jobs
- Embarrassingly parallel compute

### Advantages

- Scales to many workers
- Natural for distributed computing
- Can handle very large datasets
- Efficient use of cluster resources

### Disadvantages

- Requires careful chunking logic
- Complex dependency management
- Harder to reason about overall flow
- Requires shared storage or message coordination

---

## Pattern 4: Conditional Execution

Run different agents based on conditions.

```
       Input
         │
         ▼
    Decision Point
       /  |  \
      /   |   \
    ▼    ▼     ▼
  Path-A Path-B Path-C
    │     │     │
    └─────┬─────┘
          ▼
       Output
```

Use conditional when:
- Different workflows based on input conditions
- A/B testing different approaches
- Error recovery paths
- Multiple possible outcomes

### Conditional Execution Example

Muto handles conditions through agent environment variables and exit codes:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: conditional-workflow
spec:
  tenant: default
  agents:
    # Analyze input and set decision variable
    - name: analyzer
      image: myorg/analyzer:v1
      env:
        - name: INPUT_FILE
          value: /tmp/input.json
        - name: DECISION_FILE
          value: /tmp/decision.txt
      command:
        - /bin/sh
        - -c
        - |
          # Simple example: decide based on file size
          SIZE=$(stat -f%z /tmp/input.json 2>/dev/null || stat -c%s /tmp/input.json)
          if [ $SIZE -gt 1000000 ]; then
            echo "large" > /tmp/decision.txt
          else
            echo "small" > /tmp/decision.txt
          fi
    
    # Path A: process large files
    - name: large-processor
      image: myorg/large-processor:v1
      dependsOn:
        - analyzer
      env:
        - name: INPUT_FILE
          value: /tmp/input.json
        - name: OUTPUT_FILE
          value: /tmp/result-large.json
      command:
        - /bin/sh
        - -c
        - |
          DECISION=$(cat /tmp/decision.txt)
          if [ "$DECISION" = "large" ]; then
            /app/process-large.sh
          else
            exit 0  # Skip if not large
          fi
    
    # Path B: process small files
    - name: small-processor
      image: myorg/small-processor:v1
      dependsOn:
        - analyzer
      env:
        - name: INPUT_FILE
          value: /tmp/input.json
        - name: OUTPUT_FILE
          value: /tmp/result-small.json
      command:
        - /bin/sh
        - -c
        - |
          DECISION=$(cat /tmp/decision.txt)
          if [ "$DECISION" = "small" ]; then
            /app/process-small.sh
          else
            exit 0  # Skip if not small
          fi
    
    # Consolidate results
    - name: finalizer
      image: myorg/finalizer:v1
      dependsOn:
        - large-processor
        - small-processor
      env:
        - name: RESULT_LARGE
          value: /tmp/result-large.json
        - name: RESULT_SMALL
          value: /tmp/result-small.json
        - name: OUTPUT_FILE
          value: s3://bucket/final.json
```

### When to Use

- Different processing based on data characteristics
- Error recovery with fallback paths
- A/B testing different algorithms
- Adaptive workflows

### Advantages

- Flexible execution paths
- Can optimize for different scenarios
- Natural error recovery
- Reduces wasted computation

### Disadvantages

- More complex job definitions
- Harder to test all paths
- Potential for inconsistent results
- Requires careful state management

---

## Pattern 5: Message-Driven Coordination

Agents coordinate via message bus rather than dependencies.

```
Agent A                    Message Bus                Agent B
  │                             │
  ├─ Process data ─────────────►│
  │                       (topic: a/complete)
  │                             │
  │                             ├─► Subscribe to a/complete
  │                             │
  │                             ├─► Process & publish to b/complete
  │                             │
  │◄──────────────── (topic: b/complete)
  │
  └─ Aggregate results
```

Use message-driven when:
- Agents are long-lived services (not batch jobs)
- Need loose coupling between agents
- Event-driven architectures
- Agents don't have fixed end time

### Message-Driven Example

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: message-driven-workflow
spec:
  tenant: acme-corp
  agents:
    # Data producer: generates data and publishes
    - name: data-producer
      image: myorg/producer:v1
      timeout: 10m
      env:
        - name: MESSAGE_BUS_URL
          value: "nats://nats-cluster:4222"
        - name: PUBLISH_TOPIC
          value: "acme-corp/data-pipeline/extract-complete"
        - name: DATA_SOURCE
          value: "s3://bucket/input"
      command:
        - /bin/sh
        - -c
        - |
          # Generate data
          /app/extract-data.sh
          # Publish completion message
          echo '{"status":"complete","outputPath":"s3://bucket/extracted"}' | \
            nats pub acme-corp/data-pipeline/extract-complete
    
    # Data processor: listens for producer completion
    - name: data-processor
      image: myorg/processor:v1
      timeout: 20m
      env:
        - name: MESSAGE_BUS_URL
          value: "nats://nats-cluster:4222"
        - name: SUBSCRIBE_TOPIC
          value: "acme-corp/data-pipeline/extract-complete"
        - name: PUBLISH_TOPIC
          value: "acme-corp/data-pipeline/process-complete"
      command:
        - /bin/sh
        - -c
        - |
          # Subscribe and wait for message
          nats sub acme-corp/data-pipeline/extract-complete &
          SUBSCRIBER_PID=$!
          
          # Wait for message (with timeout)
          for i in {1..120}; do
            if [ -f /tmp/message-received ]; then
              break
            fi
            sleep 1
          done
          
          # Process data
          /app/process-data.sh
          
          # Publish completion
          echo '{"status":"complete","outputPath":"s3://bucket/processed"}' | \
            nats pub acme-corp/data-pipeline/process-complete
    
    # Aggregator: listens for processor completion
    - name: aggregator
      image: myorg/aggregator:v1
      timeout: 15m
      env:
        - name: MESSAGE_BUS_URL
          value: "nats://nats-cluster:4222"
        - name: SUBSCRIBE_TOPIC
          value: "acme-corp/data-pipeline/process-complete"
      command:
        - /bin/sh
        - -c
        - |
          # Subscribe and wait
          nats sub acme-corp/data-pipeline/process-complete &
          
          # Wait for message
          for i in {1..120}; do
            if [ -f /tmp/message-received ]; then
              break
            fi
            sleep 1
          done
          
          # Aggregate results
          /app/aggregate.sh
```

### When to Use

- Event-driven architectures
- Asynchronous workflows
- Long-running agents that don't have fixed end time
- Loose coupling between components
- Integration with external event streams

### Advantages

- Very loose coupling
- Agents can be independent services
- Natural for event-driven architectures
- Scales to many agents

### Disadvantages

- More complex error handling
- Requires message bus infrastructure
- Harder to debug message flow
- Need to handle message delivery guarantees

---

## Pattern Comparison

| Pattern | Parallelism | Coupling | Use Case | Complexity |
|---------|-------------|----------|----------|-----------|
| Sequential | None | Tight | Pipelines, waterfall | Low |
| Parallel | Full | Loose | Independent work | Medium |
| Fan-out/Fan-in | Full | Loose | Distributed compute | High |
| Conditional | None | Tight | Decision trees | Medium |
| Message-driven | Full | Very loose | Event-driven | High |

---

## Best Practices for Multi-Agent Workflows

### 1. Clear Dependency Specification

Always explicitly define dependencies:

```yaml
# Good: explicit dependencies
- name: stage-b
  dependsOn:
    - stage-a

# Avoid: implicit ordering based on sequence in YAML
- name: stage-a
  image: ...
- name: stage-b
  image: ...  # Don't rely on order!
```

### 2. Idempotent Operations

Make agents idempotent so they can be retried safely:

```bash
# Good: check if output exists before processing
if [ -f /tmp/output.json ]; then
  echo "Already processed"
else
  /app/process.sh
fi

# Bad: always overwrites, fails on second run
/app/process.sh > /tmp/output.json
```

### 3. Shared Storage Strategy

Choose a shared storage approach for intermediate results:

```yaml
# Option 1: Volume mounts (local storage)
volumeMounts:
  - name: shared
    mountPath: /shared
    
# Option 2: Cloud storage (S3, GCS)
env:
  - name: STAGING_BUCKET
    value: s3://my-staging
    
# Option 3: Message bus (loosest coupling)
env:
  - name: MESSAGE_BUS_URL
    value: nats://cluster
```

### 4. Error Handling

Plan for failures in multi-agent workflows:

```yaml
# Ensure stages can recover from failures
retryPolicy:
  maxRetries: 3
  backoffSeconds: 10

# Add timeouts to prevent hanging
timeout: 1h
activeDeadlineSeconds: 3600

# Use conditional logic for error paths
command:
  - /bin/sh
  - -c
  - |
    if ! /app/process.sh; then
      /app/recovery.sh
      exit 1
    fi
```

### 5. Resource Allocation

Allocate resources appropriate to each stage:

```yaml
agents:
  # Heavy compute
  - name: processor
    resources:
      requests:
        cpu: "4"
        memory: "8Gi"
      limits:
        cpu: "8"
        memory: "16Gi"
  
  # Light coordination
  - name: coordinator
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "512Mi"
```

### 6. Monitoring Workflow Progress

Add observability to track multi-agent workflows:

```bash
# Watch job progress
kubectl get agentjob workflow -o json | \
  jq '.status.agents[] | {name, phase, startTime, completionTime}'

# Check for slow stages
kubectl describe agentjob workflow | grep "Duration"

# Monitor resource usage
kubectl top pods -l job=workflow
```

---

## Next Steps

- **[Scheduling Agent Jobs](./scheduling-agent-jobs.md)** — Job configuration details
- **[Best Practices](./best-practices.md)** — Optimize workflow performance
- **[Examples](./examples/multi-agent-workflow.md)** — See complex workflow example
- **[Configuration](../configuration/multi-tenant-setup.md)** — Multi-tenant orchestration

---

**Last Updated:** 2026-09-03
