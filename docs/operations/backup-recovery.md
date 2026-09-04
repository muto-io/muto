# Backup and Recovery

Procedures for backing up Muto state and recovering from failures. Covers RTO/RPO, testing, and operational best practices.

## Overview

Muto maintains state in several locations:

- **Kubernetes etcd** — Job definitions, status, configurations
- **Message bus** — Inter-agent messages (persistent with Kafka)
- **Agent artifacts** — Output files, logs (external storage)
- **Configuration** — ConfigMaps, Secrets

This guide covers backing up and recovering each component.

## RTO/RPO Definition

**Recovery Time Objective (RTO):** How quickly can the system be restored to service?
- Muto RTO: < 5 minutes (operator restart from backup)
- Full system RTO: 15-30 minutes (including message bus)

**Recovery Point Objective (RPO):** How much data can be lost?
- RTO: < 5 minutes (etcd snapshot frequency)
- Message bus RPO: < 1 minute (Kafka replication)
- Agent artifacts RPO: Depends on external storage backup

## Backing Up Kubernetes etcd

### Automated Backup (Recommended)

Use Velero for automated, point-in-time backups:

```bash
# Install Velero
wget https://github.com/vmware-tanzu/velero/releases/download/v1.11.0/velero-v1.11.0-linux-amd64.tar.gz
tar -xzf velero-v1.11.0-linux-amd64.tar.gz

# Configure backup storage (e.g., AWS S3)
velero install \
  --provider aws \
  --plugins velero/velero-plugin-for-aws:v1.7.0 \
  --bucket muto-backups \
  --secret-file ./credentials-velero

# Create backup schedule
velero schedule create muto-daily \
  --schedule="0 2 * * *" \
  --include-namespaces muto-system,default \
  --ttl 720h
```

### Manual Backup

For manual point-in-time backups:

```bash
# Backup entire muto-system namespace
kubectl get all -n muto-system -o yaml > muto-system-backup.yaml

# Backup all CRDs and instances
kubectl get crd | grep muto | awk '{print $1}' | while read crd; do
  kubectl get $crd -A -o yaml >> muto-crd-backup.yaml
done

# Backup ConfigMaps and Secrets
kubectl get cm,secret -n muto-system -o yaml > muto-config-backup.yaml

# Backup etcd directly (requires cluster admin)
ETCD_POD=$(kubectl get pod -n kube-system -l component=etcd -o name | head -1)
kubectl exec -n kube-system $ETCD_POD -- \
  etcdctl snapshot save - | gzip > etcd-backup.db.gz

# Verify backup
ls -lh muto-*.yaml etcd-backup.db.gz
```

### Backup Verification

Regularly test your backups:

```bash
# List backup contents (Velero)
velero backup get
velero backup logs muto-daily-20260903

# Verify manual backups
gunzip -c etcd-backup.db.gz | head -20

# Check YAML validity
kubectl apply -f muto-system-backup.yaml --dry-run=client

# Validate CRD syntax
kubectl apply -f muto-crd-backup.yaml --dry-run=client
```

## Backing Up Message Bus

### NATS (Simple Setup)

NATS has limited persistence. Use JetStream for durable queues:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nats-config
data:
  nats.conf: |
    jetstream {
      store_dir: /data/jetstream
      max_memory_store: 1GB
      max_file_store: 10GB
    }
```

Backup JetStream store:

```bash
# Stop NATS (graceful shutdown)
kubectl delete pod -n message-bus nats-0

# Backup file store
kubectl cp message-bus/nats-0:/data/jetstream ./nats-backup/

# Restore to new cluster
kubectl cp ./nats-backup/jetstream message-bus/nats-0:/data/
```

### Kafka (Enterprise Setup)

Kafka has built-in replication. Additionally:

```bash
# Backup Kafka configuration
kafka-configs.sh --bootstrap-server localhost:9092 \
  --entity-type topics --describe > kafka-topics-backup.txt

# Backup ACLs
kafka-acls.sh --bootstrap-server localhost:9092 \
  --list > kafka-acls-backup.txt

# Topics are replicated across brokers automatically
# For disaster recovery, ensure replication.factor >= 2
```

### Message Retention

Configure appropriate retention policies:

**NATS JetStream:**
```yaml
retention: 7d           # Keep 7 days of messages
max_bytes: 10000000000  # Or 10GB, whichever comes first
```

**Kafka:**
```bash
kafka-configs.sh --bootstrap-server localhost:9092 \
  --entity-type topics \
  --entity-name muto-events \
  --alter \
  --add-config retention.ms=604800000  # 7 days
```

## Backing Up Agent Artifacts

Agent outputs are typically stored externally (S3, GCS, NFS, etc.).

### Backup Strategy

1. **Application Level:** Configure agents to backup outputs
2. **Storage Level:** Use storage-native backup tools
3. **Cloud Level:** Use managed backup services

### Example: S3 Backup

```bash
# Agents write to S3
aws s3 cp job-output.json s3://muto-artifacts/job-abc123/

# Backup entire bucket to another region
aws s3 sync s3://muto-artifacts s3://muto-artifacts-backup --region us-east-1

# Set lifecycle policy to delete old artifacts
aws s3api put-bucket-lifecycle-configuration \
  --bucket muto-artifacts \
  --lifecycle-configuration '{
    "Rules": [{
      "Id": "delete-old-artifacts",
      "Status": "Enabled",
      "Expiration": {"Days": 90}
    }]
  }'
```

### Example: NFS Backup

```bash
# Mount NFS volume for agent artifacts
mount -t nfs nfs-server:/muto-artifacts /mnt/artifacts

# Backup with rsync
rsync -av /mnt/artifacts/ /backup/artifacts/

# Or with tar
tar -czf artifacts-backup-$(date +%Y%m%d).tar.gz /mnt/artifacts/
```

## Recovering from Failures

### Scenario 1: Operator Crash

**Symptoms:** Operator pod in CrashLoopBackOff, jobs stuck in Running

**Recovery steps:**

```bash
# 1. Check operator logs
kubectl logs -n muto-system deployment/muto-operator

# 2. Fix underlying issue (config, dependency, etc.)
# Example: update ConfigMap
kubectl edit configmap muto-config -n muto-system

# 3. Restart operator
kubectl rollout restart deployment/muto-operator -n muto-system

# 4. Verify recovery
kubectl wait --for=condition=ready pod \
  -l app=muto-operator -n muto-system --timeout=300s

# 5. Monitor reconciliation
kubectl logs -n muto-system deployment/muto-operator -f
```

### Scenario 2: etcd Corruption

**Symptoms:** Cannot list resources, etcd errors in logs

**Recovery steps:**

```bash
# 1. Stop operator
kubectl scale deployment/muto-operator -n muto-system --replicas=0

# 2. Restore etcd from backup (requires node access)
ssh <control-plane-node>
sudo systemctl stop kubelet
sudo systemctl stop kube-apiserver

# Restore from snapshot
sudo /usr/local/bin/etcd \
  --snapshot-restore /path/to/etcd-backup.db \
  --data-dir /var/lib/etcd.backup

# Replace corrupted data
sudo rm -rf /var/lib/etcd
sudo mv /var/lib/etcd.backup /var/lib/etcd

# Restart
sudo systemctl start kubelet
sudo systemctl start kube-apiserver

# 3. Restart operator
kubectl scale deployment/muto-operator -n muto-system --replicas=1
```

### Scenario 3: Message Bus Data Loss

**Symptoms:** Messages lost, agents cannot receive notifications

**Recovery steps:**

**For NATS:**
```bash
# 1. Stop NATS
kubectl delete statefulset nats -n message-bus

# 2. Restore JetStream data from backup
kubectl cp ./nats-backup/jetstream nats-0:/data/ \
  -n message-bus

# 3. Restart NATS
kubectl apply -f nats.yaml

# 4. Verify data
kubectl exec -it nats-0 -n message-bus -- \
  nats stream list
```

**For Kafka:**
```bash
# Kafka has built-in replication
# If all replicas lost, restore from backup topic configs

# 1. Identify topics
kafka-topics.sh --bootstrap-server localhost:9092 --list

# 2. Recreate topics from backup
while read line; do
  kafka-topics.sh --bootstrap-server localhost:9092 \
    --create --topic $line
done < kafka-topics-backup.txt
```

### Scenario 4: Tenant Namespace Deleted

**Symptoms:** Tenant jobs disappeared, namespace gone

**Recovery steps:**

```bash
# 1. Check if jobs backed up
kubectl get agentjobs -o yaml > jobs-backup.yaml  # Should have been done before

# 2. Recreate namespace
kubectl create namespace <tenant-namespace>

# 3. Restore RBAC and network policies
kubectl apply -f rbac-backup.yaml
kubectl apply -f network-policy-backup.yaml

# 4. Recreate tenant CRD
kubectl apply -f - <<EOF
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: <tenant-name>
spec:
  namespace: <tenant-namespace>
EOF

# 5. Restore jobs (if needed for audit trail)
kubectl apply -f jobs-backup.yaml
```

### Scenario 5: Partial Data Loss (Recovery Procedure)

For recovering to a specific point in time:

```bash
# 1. List available backups
velero backup get

# 2. Restore from specific backup
velero restore create \
  --from-backup muto-daily-20260903 \
  --namespace-mappings muto-system:muto-system-restore

# 3. Verify restoration
kubectl get all -n muto-system-restore

# 4. If good, swap namespaces
kubectl delete namespace muto-system
kubectl patch namespace muto-system-restore \
  --type merge -p '{"metadata":{"name":"muto-system"}}'

# 5. Restart operator
kubectl rollout restart deployment/muto-operator -n muto-system
```

## Disaster Recovery Plan

### Tier 1: Single Component Failure (RTO: 5 min)

**What can fail:**
- Operator pod
- Single operator replica
- Agent pod

**Recovery:**
- Kubernetes automatically restarts failed pod
- Multi-replica operator provides HA
- Job reconciliation resumes after restart

**Testing:** Kill operator pod, verify recovery
```bash
kubectl delete pod -n muto-system -l app=muto-operator
kubectl wait --for=condition=ready pod -l app=muto-operator -n muto-system
```

### Tier 2: Component Cluster Failure (RTO: 15 min)

**What can fail:**
- etcd node
- Message bus node
- Single cluster zone

**Recovery:**
- Restore etcd from backup (minutes)
- Message bus replicates across zones
- Failover to warm standby cluster

**Testing:** 
```bash
# Simulate node failure
kubectl cordon <node>
kubectl drain <node> --ignore-daemonsets
# Verify workloads move to other nodes
# Restore node
kubectl uncordon <node>
```

### Tier 3: Full Cluster Failure (RTO: 30-60 min)

**What can fail:**
- All control plane nodes
- Entire region/availability zone

**Recovery:**
- Restore etcd to new cluster
- Deploy Muto from backup
- Restore message bus topics
- Resume job processing

**Testing:** 
```bash
# Quarterly disaster recovery drill
# 1. Create test cluster in different region
gcloud container clusters create muto-dr \
  --region us-west1 \
  --num-nodes 3

# 2. Restore from backup
velero restore create \
  --from-backup muto-daily-20260903

# 3. Test all functionality
kubectl get agentjobs
kubectl apply -f test-job.yaml
kubectl wait --for=condition=completed agentjob/test-job

# 4. Document any issues
# 5. Clean up test cluster
gcloud container clusters delete muto-dr
```

## Backup Schedule

**Recommended backup strategy:**

| Component | Frequency | Retention | Method |
|-----------|-----------|-----------|--------|
| etcd (full) | Daily | 30 days | Velero snapshot |
| etcd (hourly) | Hourly | 7 days | etcd snapshot |
| CRDs/jobs | Daily | 90 days | kubectl export |
| Message bus config | Daily | 30 days | Topic export |
| Agent artifacts | Continuous | 90 days | S3 lifecycle |

**Implementation:**

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: muto-backup
  namespace: muto-system
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: muto-backup
          containers:
          - name: backup
            image: muto-backup:latest
            env:
            - name: BACKUP_DEST
              value: s3://muto-backups
            volumeMounts:
            - name: backup-script
              mountPath: /scripts
            command: ["/scripts/backup.sh"]
          volumes:
          - name: backup-script
            configMap:
              name: muto-backup-script
          restartPolicy: OnFailure
```

**Backup script:**

```bash
#!/bin/bash
# backup.sh - Daily backup script

set -e
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR=/tmp/muto-backup-$TIMESTAMP

mkdir -p $BACKUP_DIR

# Backup CRDs
kubectl get agentjobs -A -o yaml > $BACKUP_DIR/agentjobs.yaml
kubectl get tenants -A -o yaml > $BACKUP_DIR/tenants.yaml

# Backup configs
kubectl get cm,secret -n muto-system -o yaml > $BACKUP_DIR/config.yaml

# Upload to S3
aws s3 sync $BACKUP_DIR s3://muto-backups/$TIMESTAMP

# Cleanup old backups (keep 30 days)
aws s3 rm s3://muto-backups --recursive --include "*" \
  --exclude "$(date -d '30 days ago' +%Y%m%d)*"

echo "Backup completed: $BACKUP_DIR"
```

## Testing and Validation

### Monthly Test Checklist

- [ ] Restore from most recent backup
- [ ] Verify all resources present
- [ ] Test operator functionality
- [ ] Run sample agent job
- [ ] Verify metrics are exported
- [ ] Check logs are collected
- [ ] Document any issues
- [ ] Update runbooks

### Backup Health Monitoring

```promql
# Alert if backup is stale
time() - muto_last_backup_timestamp_seconds > 86400

# Alert if backup fails
muto_backup_failures_total > 0
```

## Best Practices

1. **Automate backups** — Don't rely on manual backups
2. **Test recovery** — Monthly restore drills catch issues early
3. **Store backups separately** — Off-site or different region
4. **Encrypt backups** — Use encryption at rest and in transit
5. **Document procedures** — Keep runbooks updated
6. **Monitor backup health** — Alert on backup failures
7. **Version control configs** — GitOps for disaster recovery

---

**See Also:**
- [Monitoring and Observability](./monitoring-observability.md) — Monitoring backup success
- [Troubleshooting](./troubleshooting.md) — Recovery procedures
- [Deployment: Production Checklist](../deployment/production-checklist.md) — Pre-launch backup setup
