# Test Profiling

How to find the slowest integration tests, and the baseline measured when the profiling script was added.

## Profiling Script

`scripts/test-profile.sh` profiles the Ginkgo integration suites (`--help` lists all options):

```bash
make test-profile                                    # both suites, top 5
scripts/test-profile.sh --suite k8s --top 10         # K8s only, top 10
scripts/test-profile.sh --suite cf --cpuprofile      # CF with a Go CPU profile
scripts/test-profile.sh --focus "A2A Gateway"        # only matching specs
scripts/test-profile.sh --report path/to/report.json # analyze an existing report
```

It runs each suite with a Ginkgo JSON report and lists the slowest nodes, including `BeforeSuite`/`AfterSuite`. It also records CPU time and peak RSS of the test process with GNU time and, for the K8s suite, CPU and memory of the cluster containers with `docker stats`. The report is written to `test-results/profile/profile.md`; the raw data per suite, such as `specs.tsv` with one row per node, is in `test-results/profile/<suite>/`.

**Requirements:** Go and `jq`. GNU time (`gtime` on macOS) and Docker are optional; without them the CPU/RSS and cluster metrics are skipped.

### Analyzing CI Runs

`make test-integration-k8s` and `make test-integration-cf` also write `test-results/<suite>/report.json`. The CI workflows upload that directory as the `k8s-test-results` and `cf-test-results` artifacts, so any run can be analyzed without rerunning it:

```bash
gh run download <run-id> --repo muto-io/muto -n k8s-test-results -D /tmp/k8s-results
scripts/test-profile.sh --report /tmp/k8s-results/report.json --top 10
```

## Baseline

Slowest nodes, mean over 5 E2E runs on 2026-09-12 (GitHub-hosted `ubuntu-latest`). The follow-up issues describe the root causes.

| Node | Suite | Mean | Follow-up |
|------|-------|------|-----------|
| `[BeforeSuite]` | CF | 120.4 s | [#87](https://github.com/muto-io/muto/issues/87), fixed by [#93](https://github.com/muto-io/muto/pull/93) |
| CF Failure Scenarios … should handle concurrent task creation failures | CF | 13.0 s | [#88](https://github.com/muto-io/muto/issues/88) |
| A2A Gateway Lifecycle provisions gateway Deployment, Service, and Secret … | K8s | 8.7 s | [#89](https://github.com/muto-io/muto/issues/89) |
| A2A Gateway Lifecycle injects MUTO_A2A_GATEWAY and MUTO_A2A_TOKEN … | K8s | 7.9 s | [#89](https://github.com/muto-io/muto/issues/89) |
| Failure Scenarios Namespace Termination … | K8s | 6.2 s | [#90](https://github.com/muto-io/muto/issues/90) |
| Stress Testing Scheduler Performance … | K8s | 6.1 s | [#91](https://github.com/muto-io/muto/issues/91) |

CPU and memory from a local run of `scripts/test-profile.sh --suite all --cpuprofile` (8 CPUs; K8s on k3s via testcontainers, CF against the mock server):

| Metric | K8s | CF |
|--------|-----|----|
| Suite run time | 89 s | 71 s |
| Test process CPU utilization | 3% of one core | < 1% of one core |
| Test process peak RSS | 61 MiB | 23 MiB |
| Cluster CPU avg / peak | 167% / 535% of one core | n/a |
| Cluster memory peak | 566 MiB | n/a |

**Both suites are bound by waiting, not CPU.** The test process is almost idle, and the Go CPU profile shows no hotspot worth optimizing. Test time goes into fixed sleeps, polling intervals, sequential create-and-wait loops, and cleanup that blocks on namespace deletion. Removing that waiting, or doing it concurrently, pays off; optimizing test code for CPU doesn't. Running the suites with `ginkgo -p` is tracked in [#97](https://github.com/muto-io/muto/issues/97).

## Verifying an Optimization

1. Note the node's time from the baseline above or from its issue.
2. After the change, analyze the JSON reports of a few CI runs with `scripts/test-profile.sh --report …`.
3. Compare `specs.tsv` before and after. For CI-wide effects, compare the job durations too.

---

## Related Documentation

- [:octicons-book-24: **Running Tests Locally**](./running-locally.md) — Performance tips and local setup
- [:octicons-book-24: **E2E Tests**](./e2e-tests.md) — E2E test infrastructure
- [:octicons-book-24: **Integration Tests**](./integration-tests.md) — Integration testing guide

---
