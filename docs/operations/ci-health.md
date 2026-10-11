# CI/CD Health

Health of the Muto CI/CD pipeline over the last 30 days, refreshed daily by the docs build. Only runs on `main`, scheduled runs and release tags are counted; pull-request runs are excluded.

<div id="ci-health-root" data-src="data.json">Loading…</div>

## What is measured

| Metric | Definition |
|---|---|
| Workflow success rate | Successful / (successful + failed + timed out) runs per workflow; cancelled and skipped runs are ignored |
| Test pass rate | Same calculation per test job (unit, integration, E2E); this is job-level, not individual spec results |
| Dependabot PRs | Pull requests opened and merged per ISO week, and median time to merge |
| Time to first review | From PR open to the first submitted review by someone other than the author (Dependabot PRs excluded) |

Failures on `main`, scheduled and release runs are also posted to Slack; see [Monitoring and Observability](monitoring-observability.md#cicd-health).
