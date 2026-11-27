# CI/CD Health Monitoring and Dashboard — Design

Issue: muto-io/muto#16 — "Phase 4.1: Set Up Monitoring Dashboards"
Date: 2026-10-11
Status: Draft, awaiting review

## Goal

Maintainers can see CI/CD health at a glance and are told in Slack when
something breaks.

## Scope

In scope:

- Failure notifications to a single Slack channel via incoming webhook.
- Collection of five CI/CD health metrics over a rolling 30-day window.
- A static dashboard page published with the existing docs site.
- Documentation and unit tests for the collector.

Out of scope:

- Per-spec test analytics (job-level pass rate only).
- Grafana/Prometheus export of CI metrics.
- Alert routing beyond the single Slack channel.

## Acceptance criteria mapping

| Issue criterion | Delivered by |
|---|---|
| Dashboard shows CI/CD metrics | Component 3 (dashboard page) fed by Component 2 (collector) |
| Alerts are configured for failures | Component 1 (failure notification workflow) |
| Team can quickly identify issues | Slack message with failed jobs and run link; dashboard tiles and per-workflow table |

## Constraints discovered in the repo

- `pages.yml` builds the mkdocs site and deploys the *whole* site as one
  Pages artifact. A separate workflow deploying to Pages would overwrite
  the docs, so the dashboard must be produced inside the same build.
- Workflows today: `CI`, `E2E Tests`, `Release`, `Dependabot Auto-Merge`,
  `SLA Monitor`, `Build and Deploy GitHub Pages`.
- Test jobs: `unit-test`, `integration-test-k8s`, `integration-test-cf`
  (CI); `k8s-e2e`, `cf-e2e` (E2E Tests). No structured test reports are
  published, so test pass rate is derived from job conclusions.
- `sla-monitor.yaml` already tracks review SLA per PR and excludes
  Dependabot PRs; review-turnaround metrics follow the same exclusion.
- Dependabot runs weekly on Mondays (`.github/dependabot.yml`).

## Component 1: Failure notifications

File: `.github/workflows/ci-failure-notify.yaml`

- Trigger: `workflow_run` with `types: [completed]` for the workflows
  `CI`, `E2E Tests`, `Release` and `Build and Deploy GitHub Pages`.
- Fires only when the conclusion is `failure` or `timed_out` and the run
  is on `main`, scheduled, or a release tag. PR-branch failures are
  excluded; they are already visible on the PR.
- Slack delivery: POST to the `SLACK_WEBHOOK_URL` repository secret. The
  message contains workflow name, branch/ref, commit SHA and title,
  actor, list of failed jobs, and a link to the run.
- Recovery: when a run on the same workflow and branch succeeds and the
  immediately preceding completed run on that workflow and branch failed,
  send a single "recovered" message.
- Missing secret: log a notice and exit successfully, so forks and
  contributor setups never fail because of it.
- Permissions: `contents: read`, `actions: read` only. The webhook URL is
  passed through an environment variable, never interpolated into the
  script text.

## Component 2: Metrics collector

File: `scripts/ci-health/collect.py` (Python standard library only).

Runs as a step in `pages.yml` before `mkdocs build`, using the built-in
`GITHUB_TOKEN`. `pages.yml` also gains a daily `schedule` trigger and its
`paths` filter is extended to include `scripts/ci-health/**`. Output:
`docs/operations/ci-health/data.json` (gitignored, generated at build
time).

Window: rolling 30 days. Metrics:

| Metric | Source | Definition |
|---|---|---|
| Workflow success rate | Actions workflow runs API | Per workflow and overall: success / (success + failure + timed_out). Cancelled and skipped runs are excluded. |
| Test pass rate | Actions jobs API for `CI` and `E2E Tests` runs | Per test job (list above): success / completed non-cancelled executions. |
| Dependabot frequency | Pulls API, author `dependabot[bot]` | PRs opened and merged per ISO week; median open-to-merge time. |
| Review turnaround | Pulls and reviews APIs, non-Dependabot PRs | Time from PR open to first review and to merge; median and p90. |
| Failure alerts | n/a (Component 1) | Not a collected metric; covered by notifications. |

`data.json` also records `generated_at`, the window bounds, and sample
sizes so the dashboard can show "n = …" and flag thin data. The
collector paginates, tolerates API errors per metric (a failed metric is
emitted as `null` with an error note rather than failing the build), and
keeps aggregation functions pure so they can be tested without network
access.

## Component 3: Dashboard page

File: `docs/operations/ci-health.md`, linked from the operations section
of the mkdocs nav.

- Static page with a small vanilla-JS renderer loading `data.json`
  relative to the page. No new build dependencies.
- Summary tiles for the five metrics, a per-workflow success-rate table,
  a per-test-job pass-rate table, and a weekly Dependabot trend.
- Shows `generated_at` and a stale warning when data is older than 48
  hours.
- If `data.json` is missing or a metric is `null`, the page renders an
  explicit "unavailable" state rather than empty charts.

## Component 4: Documentation and tests

- Add a "CI/CD health" section to
  `docs/operations/monitoring-observability.md`: Slack secret setup,
  what each metric means, and how to read the dashboard.
- Unit tests for the collector's pure aggregation functions using
  fixture API responses (Python `unittest`), run in CI as a new job in
  `ci.yml`.
- Manual verification: because the notify workflow only reacts to
  `main`/scheduled/tag failures, it also supports `workflow_dispatch`
  with an input that posts a test message, so the Slack wiring can be
  verified without breaking `main`.

## Error handling summary

| Failure | Behaviour |
|---|---|
| Slack secret absent | Notice logged, workflow succeeds |
| Slack POST fails | Step fails visibly in notify workflow only; does not affect source workflow |
| GitHub API error in collector | Affected metric `null` with error note; build continues |
| `data.json` absent at runtime | Dashboard shows unavailable state |

## Open items

None. Slack setup (creating the webhook and `SLACK_WEBHOOK_URL` secret)
is a manual maintainer step, documented in Component 4.
