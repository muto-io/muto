# Production Deployment Checklist

Complete this checklist before deploying Muto to production.

## Pre-Deployment Planning

### [ ] Business Requirements

- [ ] Define service level objectives (SLOs)
  - Target availability: __ %
  - Maximum response time: __ ms
  - Maximum error rate: __ %
- [ ] Define RTO (Recovery Time Objective): __ hours
- [ ] Define RPO (Recovery Point Objective): __ hours
- [ ] Identify critical user journeys and features
- [ ] Document expected peak load (jobs/min): __
- [ ] Document expected data volume: __

### [ ] Capacity Planning

- [ ] Calculate required compute resources
  - Operator nodes: __
  - Worker nodes: __
  - Total CPU needed: __
  - Total memory needed: __
- [ ] Calculate storage requirements
  - State storage: __ GB
  - Log storage: __ GB
  - Backup storage: __ GB
- [ ] Plan network bandwidth
  - Ingress bandwidth needed: __ Mbps
  - Egress bandwidth needed: __ Mbps

### [ ] Team Preparation

- [ ] Assign platform owner/DRI
- [ ] Identify on-call rotation (primary, secondary, tertiary)
- [ ] Train team on Muto architecture
- [ ] Document escalation procedures
- [ ] Create runbooks for common operations
- [ ] Schedule deployment window with stakeholders

## Infrastructure Readiness

### Kubernetes Only

- [ ] Kubernetes cluster deployed and healthy
  - [ ] Control plane: __ nodes
  - [ ] Worker nodes: __ nodes
  - [ ] CNI plugin installed and working
  - [ ] Ingress controller deployed
  - [ ] Storage provisioner installed
- [ ] CRD support enabled (`kubectl api-resources | grep muto`)
- [ ] RBAC enabled (`kubectl auth can-i ...`)
- [ ] Network policies enabled
- [ ] Pod security policies configured
- [ ] Resource quotas defined

### CloudFoundry Only

- [ ] CloudFoundry API accessible
- [ ] Organization and spaces created
- [ ] Buildpacks available
  - [ ] Go buildpack version: __
  - [ ] Binary buildpack version: __
- [ ] Service brokers available (if needed)
- [ ] Network connectivity to all services

### Both Platforms

- [ ] DNS configured and resolving
- [ ] TLS certificates available (not self-signed for prod)
- [ ] Certificate rotation automated
- [ ] Firewall rules allowing required traffic
- [ ] Load balancer configured and healthy
- [ ] Backup storage accessible

## Message Bus Setup

### NATS

- [ ] NATS cluster deployed (3+ nodes for HA)
- [ ] NATS configured for persistence
- [ ] NATS JetStream enabled
- [ ] NATS authentication configured
  - [ ] Admin credentials secured
  - [ ] Client credentials rotated
- [ ] NATS monitoring enabled
- [ ] NATS cluster health verified
  - [ ] All nodes responding
  - [ ] Leader elected
  - [ ] Quorum healthy

### Kafka

- [ ] Kafka cluster deployed (3+ brokers for HA)
- [ ] Kafka topics created
  - [ ] Replication factor: __
  - [ ] Minimum ISR: __
- [ ] Kafka authentication configured (SASL)
- [ ] Kafka ACLs configured
- [ ] Kafka monitoring enabled
- [ ] Broker health verified
  - [ ] All brokers online
  - [ ] Leader election complete
  - [ ] Under-replicated partitions: 0

## Data Store Setup

- [ ] Database deployed (PostgreSQL recommended)
  - [ ] Version: __
  - [ ] HA configuration (primary + standby)
  - [ ] WAL archiving configured
- [ ] Connection pooling configured
- [ ] Database backups configured
  - [ ] Backup frequency: __
  - [ ] Retention period: __
  - [ ] Backup tested (verify restore works)
- [ ] Database monitoring enabled
  - [ ] Query performance insights
  - [ ] Replication lag monitoring
  - [ ] Disk usage monitoring

## Security Configuration

### Authentication & Authorization

- [ ] RBAC fully configured
- [ ] Service accounts created for all components
- [ ] API keys/tokens securely generated
- [ ] API keys rotated on schedule
- [ ] User groups defined for tenant separation
- [ ] Admin credentials managed in secrets manager

### Network Security

- [ ] TLS enabled for all communication
- [ ] Certificate pinning implemented (if applicable)
- [ ] Network policies enforced
  - [ ] Kubernetes: Network policies applied to all namespaces
  - [ ] CloudFoundry: Security groups configured
- [ ] Firewall rules audit-logged
- [ ] VPN/bastion host access for admin operations
- [ ] No public internet exposure of internal services

### Data Security

- [ ] Encryption at rest enabled
  - [ ] Database encryption: __ (AES-256 minimum)
  - [ ] Disk encryption: __ (LUKS/BitLocker)
- [ ] Encryption in transit enabled
  - [ ] TLS version: 1.2 minimum
  - [ ] mTLS between components enabled
- [ ] Secrets management configured
  - [ ] Secrets stored in vault (not env vars)
  - [ ] Secrets rotation automated
  - [ ] Access to secrets logged and monitored
- [ ] Data retention policies implemented
- [ ] PII/sensitive data identified and protected
- [ ] Audit logging enabled for all access

### Vulnerability Management

- [ ] Base images scanned for vulnerabilities
- [ ] Dependencies scanned (SBOM generated)
- [ ] Container registry configured as private
- [ ] Image signing/verification enabled
- [ ] Admission controllers prevent unsigned images
- [ ] Regular vulnerability scans scheduled

## Monitoring and Observability

### Metrics Collection

- [ ] Prometheus/metrics endpoint enabled
- [ ] Metrics collected for:
  - [ ] Job success/failure rates
  - [ ] Job latency (p50, p95, p99)
  - [ ] Agent CPU/memory usage
  - [ ] Message bus throughput and latency
  - [ ] Database query performance
  - [ ] API response times
- [ ] Alert thresholds defined:
  - [ ] High error rate: > __ %
  - [ ] High latency: > __ ms
  - [ ] Message bus lag: > __ seconds
  - [ ] Database replication lag: > __ seconds

### Logging

- [ ] Centralized log aggregation configured
  - [ ] Tool: __ (ELK, Splunk, CloudWatch, etc.)
  - [ ] Retention: __ days
- [ ] Structured logging (JSON) configured
- [ ] Log levels appropriate for production
- [ ] Sensitive data masked in logs
- [ ] Log search and alerting configured
- [ ] Debug logging disabled in prod

### Distributed Tracing

- [ ] Tracing infrastructure deployed (if using)
  - [ ] Tool: __ (Jaeger, Zipkin, DataDog, etc.)
- [ ] Tracing configured for Muto operator
- [ ] Trace sampling rate set: __ %
- [ ] Traces accessible for debugging

### Dashboards

- [ ] Dashboard created for operator health
  - [ ] Pod health and restarts
  - [ ] Message bus health
  - [ ] Database health
- [ ] Dashboard created for jobs
  - [ ] Active jobs count
  - [ ] Success/failure rates
  - [ ] Job latency distribution
- [ ] Dashboard created for resource usage
  - [ ] CPU, memory, disk utilization
  - [ ] Network I/O
  - [ ] Database connections

### Alerting

- [ ] Alert rules defined for:
  - [ ] Operator crash/restart
  - [ ] High job failure rate
  - [ ] Message bus unavailable
  - [ ] Database replication lag > threshold
  - [ ] Disk usage > 80%
  - [ ] Certificate expiring < 30 days
- [ ] Alerting channels configured
  - [ ] Primary: __ (PagerDuty, etc.)
  - [ ] Secondary: __ (Slack, email, etc.)
- [ ] Alert routing to on-call engineers
- [ ] Alert testing performed

## High Availability

### Kubernetes

- [ ] Operator deployed as StatefulSet or Deployment
  - [ ] Replicas: __ (minimum 3)
- [ ] Pod Disruption Budgets configured
  - [ ] Operator: minAvailable=__
- [ ] Node affinity configured
  - [ ] Spread across availability zones
  - [ ] Anti-affinity rules for operator pods
- [ ] Pod priority classes defined
  - [ ] Muto critical pods: __ class
- [ ] Readiness probes configured
- [ ] Liveness probes configured

### CloudFoundry

- [ ] Application instances: __ (minimum 2)
- [ ] Health check configured
  - [ ] Type: __ (http, process)
  - [ ] Endpoint: __
- [ ] Route redundancy configured
- [ ] Service binding redundancy (multiple message bus instances)

### Database

- [ ] Primary + standby configured
- [ ] Automatic failover enabled
- [ ] Failover tested
- [ ] Backup and recovery tested

### Message Bus

- [ ] Cluster mode enabled (not standalone)
- [ ] Quorum size: __ (odd number, >= 3)
- [ ] Leader election tested
- [ ] Node failure scenarios tested

## Backup and Disaster Recovery

### Backup Strategy

- [ ] Backup targets identified
  - [ ] Database: backed up
  - [ ] Configuration/secrets: backed up
  - [ ] Job history (optional): backed up
- [ ] Backup frequency: __
- [ ] Backup retention: __ days
- [ ] Backup storage
  - [ ] Location: __ (geographically separated)
  - [ ] Encryption: enabled
  - [ ] Access restricted: yes

### Disaster Recovery Testing

- [ ] Recovery procedure documented
- [ ] RTO verified (can recover in __ minutes)
- [ ] RPO verified (data loss <= __ minutes)
- [ ] Full restore test completed
  - [ ] Date: __
  - [ ] Duration: __
  - [ ] Result: pass/fail
- [ ] Partial restore test completed
- [ ] Data integrity verified after restore

## Performance Tuning

### Operator Performance

- [ ] Reconciler workers tuned: __
- [ ] Max concurrent reconciles: __
- [ ] Sync period optimized: __
- [ ] Memory limits appropriate
- [ ] CPU limits not causing throttling

### Message Bus Performance

- [ ] Batch size tuned: __
- [ ] Compression enabled: yes/no
- [ ] Partition count optimized
- [ ] Replication factor set: __
- [ ] Consumer groups optimized

### Database Performance

- [ ] Indexes created for common queries
- [ ] Query plans reviewed and optimized
- [ ] Connection pooling configured
  - [ ] Min connections: __
  - [ ] Max connections: __
- [ ] Slow query log enabled
- [ ] Vacuum/analyze scheduled

### Load Testing

- [ ] Load test performed
  - [ ] Tool: __
  - [ ] Target load: __ jobs/min
  - [ ] Test duration: __
  - [ ] Results:
    - [ ] P50 latency: __
    - [ ] P95 latency: __
    - [ ] P99 latency: __
    - [ ] Error rate: __
- [ ] Bottlenecks identified and addressed
- [ ] Autoscaling tested

## Testing

### Unit Tests

- [ ] Unit test coverage: >= __ %
- [ ] All tests passing

### Integration Tests

- [ ] Integration test suite passing
- [ ] Database integration tested
- [ ] Message bus integration tested

### End-to-End Tests

- [ ] E2E test suite created
  - [ ] Test: Basic job scheduling
  - [ ] Test: Multi-agent workflow
  - [ ] Test: Tenant isolation
  - [ ] Test: Error recovery
- [ ] E2E tests passing in production environment

### Smoke Tests

- [ ] Smoke test suite created
- [ ] Smoke tests run after deployment
- [ ] Smoke tests automated in CI/CD

## Documentation

- [ ] Architecture documentation updated
- [ ] Deployment guide completed
- [ ] Configuration documentation completed
- [ ] Troubleshooting guide completed
- [ ] Runbooks created for:
  - [ ] How to scale operator
  - [ ] How to handle message bus failure
  - [ ] How to handle database failure
  - [ ] How to recover from backup
  - [ ] How to update Muto version
- [ ] API documentation updated
- [ ] Operational procedures documented
- [ ] Change management procedures documented

## Compliance and Governance

- [ ] Compliance requirements identified
  - [ ] Standards: __ (HIPAA, PCI-DSS, SOC2, etc.)
- [ ] Compliance verification completed
- [ ] Data residency requirements met
- [ ] Data retention policies implemented
- [ ] Audit logging enabled
- [ ] Access controls documented
- [ ] Change log maintained
- [ ] Security review completed

## Go-Live Preparation

### Deployment Plan

- [ ] Deployment steps documented
- [ ] Rollback procedure documented
- [ ] Go-live window scheduled
- [ ] Stakeholders notified
- [ ] On-call team on standby
- [ ] Deployment checklist reviewed

### Communication

- [ ] Status page updated (if public)
- [ ] Team notified of go-live
- [ ] Customer notification plan (if applicable)
- [ ] Incident escalation procedures shared

### Pre-Deployment Verification

- [ ] All checklist items completed
- [ ] No critical issues remaining
- [ ] Staging environment matches production
- [ ] Final sanity checks passed
- [ ] Sign-off obtained from:
  - [ ] Engineering lead
  - [ ] Operations lead
  - [ ] Product lead

## Post-Deployment

### Immediate (First Hour)

- [ ] Deployment completed successfully
- [ ] All pods/instances running
- [ ] Health checks passing
- [ ] Metrics flowing correctly
- [ ] Logs aggregating correctly
- [ ] Smoke tests passing
- [ ] Basic functionality verified

### Short-term (First Day)

- [ ] Monitor for errors and anomalies
- [ ] Performance metrics within expectations
- [ ] User feedback collected
- [ ] Issues documented and triaged
- [ ] Post-deployment review scheduled

### Medium-term (First Week)

- [ ] Post-deployment review completed
- [ ] Lessons learned documented
- [ ] Issues resolved or tracked
- [ ] Performance optimization completed
- [ ] Documentation updated with findings

## Sign-Off

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Engineering Lead | _____________ | _______ | __________ |
| Operations Lead | _____________ | _______ | __________ |
| Security Lead | _____________ | _______ | __________ |
| Product Lead | _____________ | _______ | __________ |

---

**Last Updated:** 2026-09-03

**Deployment Date:** __________

**Deployed By:** __________

**Approved By:** __________
