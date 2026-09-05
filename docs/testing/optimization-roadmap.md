# Test Performance Optimization Roadmap
**Status:** Ready for Implementation  
**Priority:** Phase 4.2 - Test Performance Optimization  
**Related Issue:** #17

---

## Quick Wins (1-2 Day Implementation)

### 1. Verify Go Module Caching
**Effort:** 15 minutes | **Expected Savings:** 10-15 seconds | **Risk:** Low

- Check GitHub Actions workflow logs for cache hit rates
- Ensure `go-version-file` is set (already present)
- Document baseline metrics before optimization
- Target: 95%+ cache hit rate on consecutive runs

### 2. Investigate E2E Test Anomalies
**Effort:** 30 minutes | **Expected Savings:** Test coverage clarity | **Risk:** Low

E2E Runs 104, 102, 100, 99, 98, 97 complete in <1 minute (anomalies).

**Actions:**
- Review test skip conditions in `test/integration/cf/`
- Check if CF tests skip gracefully when secrets missing
- Verify K8s tests always run
- Document findings

### 3. Document A2A Gateway Test Issues
**Effort:** 20 minutes | **Expected Savings:** Understanding | **Risk:** None

Recent commits show timeout increases: 60s → 120s → 300s

**Actions:**
- Catalog commits: `e5646e7`, `4b11e9a`, `0f470e6`, `e036307`
- Document current state and root causes
- Create summary for future reference

---

## Medium-Effort Improvements (3-5 Days)

### 4. Profile Slowest Tests
**Effort:** 2-3 hours | **Expected Savings:** 20-30% after optimizations | **Risk:** Low

- Identify which test cases consume most CPU/time
- Look for parallelization opportunities
- Create profiling script: `scripts/test-profile.sh`
- Create optimization tickets for top 5 slowest tests

### 5. Optimize A2A Gateway Test Cleanup
**Effort:** 3-4 hours | **Expected Savings:** 60-120 seconds per E2E | **Risk:** Medium

Current: Namespace stuck in "Terminating" state

**Actions:**
- Root cause analysis of cleanup delays
- Evaluate solutions (force delete vs operator improvement)
- Reduce timeout from 300s to 60s
- Validate with 5 consecutive E2E runs

### 6. Implement Job Timeout Adjustments
**Effort:** 1 hour | **Expected Savings:** Faster CI feedback | **Risk:** Low

Current timeouts are 10-15x actual runtime (30-40 min vs 3-4 min actual).

**Changes:**
- K8s Integration: 20 min → 10 min
- CF Integration: 20 min → 10 min
- E2E K8s: 40 min → 10 min
- E2E CF: 60 min → 15 min

Files:
- `.github/workflows/ci.yml`
- `.github/workflows/e2e-tests.yml`

---

## Long-Term Improvements (1-2 Weeks)

### 7. Parallelize Integration Tests
**Effort:** 4-6 hours | **Expected Savings:** ~15 seconds if parallelizable | **Risk:** Medium

- Analyze K8s/CF test dependencies
- Check for Docker socket conflicts
- Modify E2E workflow to run in parallel if possible
- Monitor first 10 runs for race conditions

### 8. Implement Test Suite Splitting
**Effort:** 8-10 hours | **Expected Savings:** 30-40% with better parallelization | **Risk:** Medium-High

Split K8s tests into focused suites:
- Operator tests (~5m)
- A2A Gateway tests (~5m)
- Tenant management tests (~5m)

Create separate Makefile targets and CI jobs for parallel execution.

---

## Implementation Schedule

| Phase | Tasks | Duration | Owner |
|-------|-------|----------|-------|
| **Week 1** | Cache verification, E2E anomalies, A2A docs | 1-2 hours | Solo |
| **Week 2** | Test profiling, A2A optimization, timeouts | 6-8 hours | Solo |
| **Week 3-4** | Parallelization, test splitting | 12-16 hours | Collaborative |

**Total Effort:** 20-30 hours over 4 weeks  
**Expected ROI:** 30-50% reduction = ~1.5-2 hours saved monthly

---

## Success Metrics

| Metric | Target | Current | Improvement |
|--------|--------|---------|-------------|
| Avg CI time | < 2.5m | 3.0m | ~17% |
| P95 CI time | < 3.0m | 4.2m | ~29% |
| Avg E2E time | < 3.0m | 3.5m | ~14% |
| Cache hit rate | > 95% | Unknown | TBD |
| Test variance | < 30% | ~40% | ~25% |

---

## Risks & Mitigation

| Risk | Mitigation |
|------|-----------|
| Timeout too low causes false failures | Use P95 + 50% buffer, monitor first week |
| Parallelization introduces race conditions | Run 10 consecutive E2E builds, add debugging |
| Test splitting breaks dependent tests | Keep all tests passing before splitting |
| A2A cleanup fix breaks other tests | Test locally, use feature branch, slow rollout |

---

## Monitoring Strategy

### Weekly Metrics to Track
- Mean/P95 CI execution time
- Mean/P95 E2E execution time
- Cache hit rates (Go modules/build)
- Test pass rate per job
- Timeout frequency

### Dashboard
- Create GitHub Pages dashboard or use existing CI integrations
- Show 4-week trends
- Alert on regressions (>10% increase)

---

**Status:** Ready for team review and implementation planning  
**Next Step:** Prioritize quick wins for immediate impact
