# Test Profiling

How to find the slowest integration tests, the current baseline, and the optimization targets derived from it.

## Profiling Script

`scripts/test-profile.sh` profiles the Ginkgo integration suites:

```bash
make test-profile                                    # both suites, top 5
scripts/test-profile.sh --suite k8s --top 10         # K8s only, top 10
scripts/test-profile.sh --suite cf --cpuprofile      # CF with a Go CPU profile
scripts/test-profile.sh --focus "A2A Gateway"        # only matching specs
scripts/test-profile.sh --report path/to/report.json # analyze an existing report
```

For each suite it:

1. Builds the test binary and times the build separately from the run.
2. Runs the binary with a Ginkgo JSON report (`-ginkgo.json-report`), which records the duration of every spec and of `BeforeSuite`/`AfterSuite`.
3. Records CPU time (user + sys) and peak RSS of the test process with GNU time. The K8s suite runs the controller manager in-process, so this includes the reconcilers.
4. For the K8s suite, samples CPU and memory of the k3s (testcontainers) or kind node containers with `docker stats`.
5. Writes a Markdown report with the slowest nodes, time per file, and the metrics above.

| Option | Default | Description |
|--------|---------|-------------|
| `-s`, `--suite` | `all` | `k8s`, `cf` or `all` |
| `-n`, `--top` | `5` | Number of slowest nodes to list |
| `-o`, `--out` | `test-results/profile` | Output directory |
| `-f`, `--focus` | | Only run specs matching the regex |
| `-r`, `--report` | | Analyze an existing Ginkgo JSON report instead of running (repeatable) |
| `--cpuprofile` | off | Record a Go CPU profile and list the top functions |
| `--interval` | `2` | Cluster sampling interval in seconds |
| `--timeout` | `20m` | Test binary timeout |

Suite environment variables (`MUTO_USE_EXISTING_CLUSTER`, `KUBECONFIG`, `KIND_DEPLOYMENT_PATH`, `CF_E2E_*`) are passed through unchanged.

**Requirements:** Go and `jq`. GNU time (`gtime` on macOS) and Docker are optional; without them the CPU/RSS and cluster metrics are skipped.

**Output** (in `test-results/profile/`):

| File | Content |
|------|---------|
| `profile.md` | Combined report |
| `<suite>/report.json` | Ginkgo JSON report |
| `<suite>/specs.tsv` | One row per executed node: suite, type, state, seconds, start, end, serial, ordered, location, name |
| `<suite>/test.log` | Verbose test output |
| `<suite>/rusage.txt` | Wall, user and sys time and peak RSS of the test process |
| `<suite>/cluster.tsv` | Cluster container samples: epoch, container, CPU %, memory MiB |
| `<suite>/cpu.pprof` | Go CPU profile (`--cpuprofile`) |

CPU percentages are relative to one core. The per-node cluster CPU is the average of the samples taken while the node ran. Nodes shorter than the sampling interval show `n/a`.

### Analyzing CI Runs

`make test-integration-k8s` and `make test-integration-cf` also write `test-results/<suite>/report.json`. The CI workflows upload that directory as the `k8s-test-results` and `cf-test-results` artifacts, so any run can be analyzed without rerunning it:

```bash
gh run download <run-id> --repo muto-io/muto -n k8s-test-results -D /tmp/k8s-results
scripts/test-profile.sh --report /tmp/k8s-results/report.json --top 10
```

## Baseline

### CI

Source: 5 runs of the E2E workflow on 2026-09-12 (GitHub-hosted `ubuntu-latest`; runs 34690087856, 34706898685, 34709558976, 34709644964, 34709964707). The spec durations varied by less than 1% between runs, except for the A2A Gateway specs.

| Job | Job duration | Go, Docker and kind setup | Build + module download | Suite |
|-----|--------------|---------------------------|-------------------------|-------|
| Kubernetes E2E | 2:32–2:52 | 45–63 s (kind cluster: 25–34 s) | 23–34 s | 68–77 s |
| CloudFoundry E2E | 3:52–4:05 | 20–31 s | 12–16 s | 191 s, 120 s of it in `BeforeSuite` |

The two jobs run in parallel, so the workflow takes as long as the CloudFoundry job (3:59–4:14). Both jobs download ~40 Go modules on every run (`go: downloading ...`), even though `setup-go` caching is enabled.

### Slowest Nodes

Mean over the 5 CI runs:

| # | Node | Suite | Mean | Range | Root cause | Follow-up |
|---|------|-------|------|-------|------------|-----------|
| 1 | `[BeforeSuite]` | CF | 120.35 s | 120.17–120.47 s | `waitForCFReady` polls the unresolvable `api.cf.local` until its 2-minute deadline, then the suite falls back to the mock server | [#87](https://github.com/muto-io/muto/issues/87) |
| 2 | CF Failure Scenarios … should handle concurrent task creation failures | CF | 13.04 s | 13.04–13.05 s | Creates 5 tasks and waits for each in turn; every mock task takes 2.61 s | [#88](https://github.com/muto-io/muto/issues/88) |
| 3 | A2A Gateway Lifecycle provisions gateway Deployment, Service, and Secret … | K8s | 8.70 s | 6.57–13.16 s | `AfterEach` blocks ~5.5 s on namespace deletion | [#89](https://github.com/muto-io/muto/issues/89) |
| 4 | A2A Gateway Lifecycle injects MUTO_A2A_GATEWAY and MUTO_A2A_TOKEN … | K8s | 7.94 s | 7.07–11.40 s | Same as #3 | [#89](https://github.com/muto-io/muto/issues/89) |
| 5 | Failure Scenarios Namespace Termination … | K8s | 6.22 s | 5.75–6.71 s | Waits for the namespace controller to remove the AgentJob | [#90](https://github.com/muto-io/muto/issues/90) |
| 6 | Stress Testing Scheduler Performance … | K8s | 6.06 s | 5.94–6.45 s | Creates 4 jobs and waits for each to reach `Running` in turn (0.5–2.0 s each) | [#91](https://github.com/muto-io/muto/issues/91) |

Further candidates, not yet tracked in their own issues:

| Node or step | Suite | Time | Cause |
|--------------|-------|------|-------|
| 17 specs of 2.61 s each | CF | 44 s per run | Fixed mock task lifecycle (500 ms to `RUNNING`, 2 s to done); covered by [#88](https://github.com/muto-io/muto/issues/88) |
| … should enforce resource quotas per tenant | CF | 5.22 s | Two mock tasks awaited in turn; covered by [#88](https://github.com/muto-io/muto/issues/88) |
| … should prevent infinite restart loops | K8s | 5.07 s | Fixed `time.Sleep(5 * time.Second)` in `k8s_advanced_failure_test.go` |
| CF Adapter E2E WatchAgent should watch task and return success event | CF | 5.00 s | `CFAdapter.WatchAgent` polls every 5 s (`platform/cf/adapter.go`) |
| Build + module download | K8s / CF E2E | 23–34 s / 12–16 s | Modules downloaded on every run |
| kind cluster creation | K8s E2E | 25–34 s | Infrastructure step |

### CPU and Memory

A local run of `scripts/test-profile.sh --suite all --cpuprofile` (Linux x86_64, 8 CPUs, Go 1.27, Docker 29.8). The K8s suite ran on k3s via testcontainers, the default without `MUTO_USE_EXISTING_CLUSTER`. The CF suite ran against the mock server, since `KIND_DEPLOYMENT_PATH` wasn't set.

| Metric | K8s | CF |
|--------|-----|----|
| Suite run time | 89.4 s (`BeforeSuite` 19.7 s, k3s startup) | 70.6 s |
| Test process CPU (user + sys) | 2.1 s + 0.6 s | 0.2 s + 0.1 s |
| Test process CPU utilization | 3% of one core | < 1% of one core |
| Test process peak RSS | 61 MiB | 23 MiB |
| Cluster CPU avg / peak | 167% / 535% of one core | n/a |
| Cluster memory peak | 566 MiB | n/a |

The local spec durations match CI: A2A Gateway 6.6 s and 7.1 s, Namespace Termination 6.7 s, Scheduler Performance 6.0 s, CF concurrent task creation failures 13.0 s.

**Both suites are bound by waiting, not CPU.** The test process is almost idle, and the Go CPU profile shows no hotspot worth optimizing. Only the cluster does real work, mostly in bursts while pods are created and deleted; the Namespace Termination spec averaged 378% of one core. Test time is spent in fixed sleeps, polling intervals, sequential create-and-wait loops, and cleanup that blocks on namespace deletion. Removing that waiting, or doing it concurrently, pays off; optimizing test code for CPU doesn't.

## Parallelization Opportunities

- **Jobs:** The K8s and CF E2E jobs already run in parallel. The CF job is the critical path. Once [#87](https://github.com/muto-io/muto/issues/87) is fixed it drops to ~2:00, and the K8s job (~2:45) becomes the critical path.
- **Within specs:** Several specs create resources one at a time and wait for each ([#88](https://github.com/muto-io/muto/issues/88), [#91](https://github.com/muto-io/muto/issues/91)). Creating everything first and then waiting is the cheapest form of parallelization.
- **CF suite with `ginkgo -p`:** Every Ginkgo process would start its own in-process mock server, so the mock-based specs can run in parallel. With a real CF instance, the per-process `CFTestHelper` counters would produce the same space names in every process. Fix [#88](https://github.com/muto-io/muto/issues/88) first; afterwards the remaining gain is small.
- **K8s suite with `ginkgo -p`:** No spec is marked `Serial` or `Ordered`, but the suite isn't parallel-safe yet:
    - Each process would run `BeforeSuite` and start its own controller manager against the same cluster, so several reconcilers would act on the same objects. One manager would have to be started in `SynchronizedBeforeSuite`.
    - Cluster-scoped Tenant names come from per-file counters (`tenant-stress-%d`, `tenant-multi-%d`, …) or are fixed (`integration-tenant`, `helm-tenant-test`). They'd collide across processes unless they include `GinkgoParallelProcess()` or a random suffix.
- **Suite split:** Splitting the K8s suite into focused suites that run as separate jobs ([#61](https://github.com/muto-io/muto/issues/61)) avoids the shared-manager problem, but each job pays for its own cluster setup (25–34 s for kind).

## Verifying an Optimization

1. Note the node's mean and range from the baseline above.
2. After the change, analyze the JSON reports of a few CI runs with `scripts/test-profile.sh --report …`.
3. Compare `specs.tsv` before and after. For CI-wide effects, compare the job durations too.

---

## Related Documentation

- [:octicons-book-24: **Running Tests Locally**](./running-locally.md) — Performance tips and local setup
- [:octicons-book-24: **E2E Tests**](./e2e-tests.md) — E2E test infrastructure
- [:octicons-book-24: **Integration Tests**](./integration-tests.md) — Integration testing guide

---
