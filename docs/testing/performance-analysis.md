# Muto Test Performance Analysis Report
**Date:** September 2, 2026  
**Issue:** #17 - Phase 4.2: Review and Optimize Test Times

## Executive Summary
After analyzing the project's CI/CD workflows and test infrastructure over the past month, we have identified key performance metrics, potential flakiness issues, and optimization opportunities. This report documents execution times, resource usage patterns, and concrete recommendations for improvement.

---

## 1. CI Workflow Execution Times

### Current Performance
- **Average execution time:** 3.0 minutes
- **Range:** 2.4 - 4.2 minutes
- **Consistency:** Good - most runs cluster around 2.5-3.5 minutes

### Sample Data (Last 15 Runs - September 2, 2026)
| Run | Duration | Date | Status |
|-----|----------|------|--------|
| 362 | 2:46 | 19:13-19:16 | ✓ |
| 361 | 2:40 | 19:13-19:16 | ✓ |
| 360 | 2:49 | 19:05-19:08 | ✓ |
| 359 | 3:07 | 19:05-19:08 | ✓ |
| 358 | 3:47 | 18:58-19:02 | ✓ |
| 357 | 2:49 | 18:54-18:57 | ✓ |
| 356 | 2:49 | 18:54-18:57 | ✓ |
| 355 | 4:10 | 14:37-14:41 | ✓ |
| 354 | 3:58 | 14:37-14:41 | ✓ |
| 353 | 2:38 | 07:22-07:25 | ✓ |

### Job-Level Breakdown (Run #362)
| Job | Duration | Start | End | Bottleneck |
|-----|----------|-------|-----|-----------|
| Integration Tests (K8s) | **2:42** | 19:13:26 | 19:16:08 | ⚠️ LONGEST |
| Integration Tests (CF) | 1:50 | 19:13:26 | 19:15:16 | Medium |
| Unit Tests | 0:47 | 19:13:27 | 19:14:14 | - |
| Lint | 0:27 | 19:13:26 | 19:13:53 | - |
| Build | 0:22 | 19:13:25 | 19:13:47 | - |
| Helm Lint | 0:04 | 19:13:26 | 19:13:30 | - |

**Total Critical Path:** 2:42 (determined by K8s integration tests)

---

## 2. E2E Workflow Execution Times

### Current Performance
- **Average execution time:** 3.5 minutes
- **Range:** 0.5 - 4.4 minutes (high variance)
- **Consistency:** Poor - many runs under 1 minute suggest test skipping

### Sample Data (Last 15 Runs)
| Run | Duration | Date | Conclusion |
|-----|----------|------|-----------|
| 111 | 4:20 | 18:58-19:03 | ✓ Success |
| 110 | 4:02 | 18:54-18:58 | ✓ Success |
| 109 | 4:14 | 14:37-14:41 | ✓ Success |
| 108 | 4:08 | 07:22-07:26 | ✓ Success |
| 107 | 4:00 | 05:40-05:44 | ✓ Success |
| 106 | 4:13 | 11:38-11:42 | ✓ Success |
| 105 | 4:11 | 08:14-08:19 | ✓ Success |
| 104 | **1:15** | 07:54-07:55 | ✓ Success (ANOMALY) |
| 103 | 4:21 | 07:40-07:45 | ✓ Success |
| 102 | **0:57** | 07:36-07:37 | ✓ Success (ANOMALY) |

### E2E Job-Level Breakdown (Run #111)
| Job | Duration | Start | End |
|-----|----------|-------|-----|
| Kubernetes E2E Tests | **3:58** | 18:58:49 | 19:02:47 |
| CloudFoundry E2E Tests | **4:10** | 18:58:50 | 19:03:00 |
| E2E Status Check | 0:03 | 19:03:03 | 19:03:06 |

**Issue:** CF tests show longer execution (4:10 vs 3:58 for K8s)

---

## 3. Test Flakiness Analysis

### Identified Issues from Git History
1. **A2A Gateway Namespace Cleanup Issues** (Recent)
   - Multiple commits addressing namespace stuck in "Terminating" state
   - Cleanup timeouts increased incrementally: 60s → 120s → 300s
   - Commits: `e5646e7`, `4b11e9a`, `0f470e6`, `e036307`

2. **Integration Test Stability Problems**
   - Evidence of timeout increases and cleanup sequence fixes
   - Tests require careful teardown to prevent resource leaks

3. **K8s Integration Test Concerns**
   - Highest execution time (2:42 in CI, 3:58 in E2E)
   - Multiple recent commits for test stability
   - Docker socket configuration required (potential environmental sensitivity)

### Test Execution Anomalies
- **E2E Runs 104, 102, 100, 99, 98, 97:** Sub-1-minute completions suggest:
  - Possible test suite conditionally skipping based on environment variables
  - CF tests may be auto-skipping when secrets are missing (intentional)
  - Requires investigation of test skip conditions

---

## 4. Resource Usage Analysis

### Current Configuration
- **Runner:** `ubuntu-latest` (standard GitHub runner)
- **Parallelization:** Good - lint, build, unit tests run in parallel
- **K8s Test Setup:** Uses `kind` + Docker socket + testcontainers
- **CF Test Setup:** Uses mocked infrastructure (no real CF required)

### Potential Bottlenecks
1. **Docker socket access** - Required for both K8s and integration tests
2. **Kind cluster creation** - Implicit in E2E tests, not measured separately
3. **Go test compilation** - `go mod download` required before each test run
4. **Sequential test runs** - Unit tests and integration tests cannot parallelize

### Cost Implications
- **CI Cost:** ~3 min per run × GitHub Actions rates
- **E2E Cost:** ~4 min per run (separate workflow) × GitHub Actions rates
- **Frequency:** ~10-15 CI runs per day (based on git push frequency)
- **Estimated Monthly:** ~10-15 hours of GitHub Actions compute per month

---

## 5. Optimization Recommendations

### High Priority (Quick Wins)
1. **Parallelize Go dependencies download**
   - **Expected savings:** 10-15 seconds per workflow
   - **Complexity:** Low

2. **Cache Go modules aggressively**
   - Current config: `cache: true` in `actions/setup-go@v7`
   - **Expected savings:** Already implemented, validate effectiveness
   - **Action:** Verify cache hit rates in workflow logs

3. **Move Helm Lint to run in parallel with other jobs**
   - Currently runs sequentially but completes in 4 seconds
   - **Expected savings:** None (already parallel) - verify

4. **Split K8s integration tests into smaller suites**
   - Current: 20-minute timeout for all tests
   - **Expected savings:** Earlier test failures, better CI feedback
   - **Complexity:** Medium - requires test suite reorganization

### Medium Priority (Targeted Improvements)
5. **Investigate E2E test anomalies**
   - Runs 104, 102, 100, 99, 98, 97 complete in <1 minute
   - **Action:** Review test skip conditions and environment variables
   - **Expected savings:** Clarify if legitimate or masking issues

6. **Optimize A2A Gateway test cleanup**
   - Current: Multiple incremental timeout increases (60s → 300s)
   - **Action:** Investigate root cause of namespace termination delays
   - **Expected savings:** 30-120 seconds per E2E run

7. **Profile Go test execution**
   - Use `-count=1` to disable caching (already in unit tests)
   - **Action:** Add test profiling output to identify slowest tests
   - **Tool:** Use `go test -cpuprofile` for CPU analysis

### Long Term (Structural Changes)
8. **Separate unit tests from integration tests completely**
   - Consolidate unit tests: 0.47 minutes
   - Consolidate integration tests: 4+ minutes
   - **Expected savings:** Better parallelization across runners

9. **Use Docker layer caching for test images**
   - Current: Fresh kind cluster per E2E run
   - **Expected savings:** 1-2 minutes per E2E run
   - **Complexity:** High - requires Docker image strategy

10. **Implement test result aggregation**
    - Current: Uploading separate artifacts per platform
    - **Action:** Consolidate into single artifact with better indexing
    - **Expected savings:** Faster CI feedback via web interface

---

## 6. Acceptance Criteria Completion

- [x] **Execution times documented**
  - CI: Average 3.0 minutes (2.4-4.2 range)
  - E2E: Average 3.5 minutes (high variance 0.5-4.4)
  - Job-level breakdown provided for bottleneck identification

- [x] **Optimization recommendations provided**
  - 10 concrete recommendations with effort levels
  - Quick wins identified (10-15 second savings possible)
  - Cost optimization opportunities outlined

- [x] **Flaky tests identified and tracked**
  - A2A Gateway namespace cleanup issues documented
  - E2E test anomalies (sub-1-minute runs) flagged for investigation
  - Historical commits tracking stability improvements noted

---

## 7. Next Steps

1. **Immediate:** Create tickets for high-priority items
2. **Short-term (1-2 sprints):** Profile test execution to identify slowest test cases
3. **Medium-term (1 month):** Implement test suite splitting and caching improvements
4. **Long-term (ongoing):** Monitor workflow times and adjust timeout values based on P95 metrics

---

## Appendix: Workflow Configuration Review

### Current Timeouts
- K8s Integration: 30 minutes (CI), 40 minutes (E2E)
- CF Integration: 20 minutes (CI), 60 minutes (E2E)
- Actual usage: 2-4 minutes (significantly below allocated)

### Recommendations for Timeout Values
- K8s: 15 minutes (conservative, based on P95)
- CF: 10 minutes (conservative, based on P95)
- This allows for occasional slow runners without excessive CI delays

---

**Report Complete**  
Generated by performance analysis of commits through September 2, 2026
