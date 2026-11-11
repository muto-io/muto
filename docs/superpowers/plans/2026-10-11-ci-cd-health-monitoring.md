# CI/CD Health Monitoring and Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give maintainers a CI/CD health dashboard on the docs site and Slack alerts when `main`, scheduled or release runs fail.

**Architecture:** A stdlib-only Python package in `scripts/ci-health/` has pure aggregation functions (`metrics.py`), a GitHub API client (`ghapi.py`), a collector that writes `data.json` (`collect.py`) and a Slack notifier (`notify.py`). The collector runs inside the existing `pages.yml` mkdocs build, so the dashboard ships in the same Pages deployment as the docs; a new `workflow_run` workflow runs the notifier.

**Tech Stack:** Python 3 standard library (`unittest`, `urllib`), GitHub Actions, mkdocs-material, vanilla JS.

**Spec:** `docs/superpowers/specs/2026-10-11-ci-cd-health-monitoring-design.md`

## Global Constraints

- Python is standard library only; no `pip install` for the scripts. Code must run on Python 3.10+ (CI uses 3.11).
- Metrics window is a rolling 30 days.
- Slack delivery uses an incoming webhook from the `SLACK_WEBHOOK_URL` repository secret, passed via an environment variable and never interpolated into script text.
- Notification workflow permissions are `contents: read` and `actions: read` only.
- Missing `SLACK_WEBHOOK_URL`: log a notice and exit successfully.
- Alerts and metrics cover runs on `main`, scheduled runs and release tags only; PR-branch failures are excluded.
- A failed API call nulls only the affected metric (with an `errors` note); the docs build must never fail because of the collector.
- Dashboard warns when data is older than 48 hours and shows an explicit "unavailable" state for missing data.
- `docs/operations/ci-health/data.json` is generated at build time and gitignored.
- Tracked workflows: `CI`, `E2E Tests`, `Release`, `Build and Deploy GitHub Pages`. Tracked test jobs: `Unit Tests`, `Integration Tests (K8s)`, `Integration Tests (Cloud Foundry)` (CI) and `Kubernetes E2E Tests`, `CloudFoundry E2E Tests` (E2E Tests).
- Commit messages follow the repo style (`feat:`, `docs:`, `ci:` prefix, capitalised subject). Do not add Claude/`Co-Authored-By` trailers (project memory rule).

## Review Focus

Failure modes the spec implies but does not spell out; each has a test in the owning task.

1. A fork PR from a branch literally named `main` (event `pull_request`, `head_branch == "main"`) must not count in metrics or trigger alerts. Tests: Task 1 `test_pull_request_event_is_excluded_even_from_a_branch_named_main`, Task 3 `test_fork_pr_from_branch_named_main_is_silent`.
2. Commit messages containing Slack control text (`<!channel>`, `<https://x|y>`, `&`, 500 characters, multi-line) must not ping, link or flood. Tests: Task 3 `test_commit_text_cannot_ping_or_link`, `test_long_title_is_truncated`.
3. Reviews that are pending (no `submitted_at`) or written by the PR author must not count as the first review. Test: Task 1 `test_pending_and_self_reviews_are_ignored`.
4. A repo or window with zero runs/PRs must give `null` rates (rendered "n/a"), never `0%` or `NaN`. Tests: Task 1 `test_no_runs_gives_null_rate_not_zero`, Task 2 `test_empty_repository_yields_null_rates_not_errors`.
5. The first run of a workflow ever, or a success preceded only by cancelled runs, must not produce a bogus "recovered" message; a success after a real failure (skipping cancelled runs and the run itself) must. Tests: Task 3 `test_first_ever_run_success_is_silent`, `test_recovery_skips_cancelled_runs_and_the_run_itself`.

---

## File Structure

| File | Responsibility |
|---|---|
| `scripts/ci-health/metrics.py` | Pure aggregation: run filtering, success rates, percentiles, Dependabot and review stats |
| `scripts/ci-health/ghapi.py` | Minimal GitHub REST client with pagination |
| `scripts/ci-health/collect.py` | Orchestrates fetching and writes `data.json` |
| `scripts/ci-health/notify.py` | Builds and posts Slack failure/recovery messages |
| `scripts/ci-health/test_*.py` | `unittest` suites, one per module |
| `.github/workflows/ci-failure-notify.yaml` | `workflow_run` trigger for the notifier |
| `.github/workflows/ci.yml` | Modify: add job running the script tests |
| `.github/workflows/pages.yml` | Modify: run collector, daily schedule, deploy on schedule |
| `docs/operations/ci-health.md` | Dashboard page |
| `docs/operations/ci-health/dashboard.js`, `dashboard.css` | Dashboard renderer and styles |
| `mkdocs.yml` | Modify: nav entry, `extra_javascript`, `extra_css` |
| `.gitignore` | Modify: ignore generated `data.json` |
| `docs/operations/monitoring-observability.md` | Modify: add CI/CD health section |

All commands run from the repository root.

### Task 0: Branch

- [ ] **Step 1: Create the implementation branch**

The spec and this plan are committed on `docs/ci-cd-health-monitoring-spec`; build on top of them.

```bash
git checkout -b feat/ci-cd-health-monitoring
git log --oneline -3
```

Expected: the spec (and plan) commits are at the top of the log.

---

### Task 1: Metrics aggregation

**Files:**
- Create: `scripts/ci-health/metrics.py`
- Test: `scripts/ci-health/test_metrics.py`

**Interfaces:**
- Produces (used by Tasks 2 and 3):
  - `DEFAULT_BRANCH`, `DEPENDABOT_LOGIN`, `TEST_JOBS: dict[str, tuple[str, ...]]`, `COUNTED_CONCLUSIONS: tuple[str, ...]`
  - `parse_ts(value: str) -> datetime`
  - `is_health_run(run: dict) -> bool`
  - `workflow_success_rates(runs) -> {"overall": Rate, "by_workflow": {name: Rate}}`
  - `test_job_pass_rates(jobs: Iterable[tuple[str, dict]]) -> {job_name: Rate}`
  - `dependabot_stats(prs, window_start) -> {"opened", "merged", "median_hours_to_merge", "weekly"}`
  - `review_turnaround(prs, window_start) -> {"sample", "first_review_hours", "merge_hours"}`
  - `Rate = {"success": int, "failed": int, "total": int, "rate": float | None}`

- [ ] **Step 1: Write the failing tests**

Create `scripts/ci-health/test_metrics.py`:

```python
import unittest
from datetime import datetime, timezone

import metrics

WINDOW_START = datetime(2026, 9, 11, tzinfo=timezone.utc)


def run(name, conclusion, branch="main", event="push"):
    return {
        "name": name,
        "conclusion": conclusion,
        "head_branch": branch,
        "event": event,
    }


class IsHealthRunTest(unittest.TestCase):
    def test_push_to_main_counts(self):
        self.assertTrue(metrics.is_health_run(run("CI", "success")))

    def test_scheduled_run_counts(self):
        self.assertTrue(metrics.is_health_run(run("E2E Tests", "failure", event="schedule")))

    def test_feature_branch_push_is_excluded(self):
        self.assertFalse(metrics.is_health_run(run("CI", "failure", branch="feat/x")))

    def test_pull_request_event_is_excluded_even_from_a_branch_named_main(self):
        self.assertFalse(
            metrics.is_health_run(run("CI", "failure", branch="main", event="pull_request"))
        )

    def test_release_tag_counts(self):
        self.assertTrue(metrics.is_health_run(run("Release", "failure", branch="v1.2.3")))

    def test_tag_like_branch_without_semver_is_excluded(self):
        self.assertFalse(metrics.is_health_run(run("CI", "failure", branch="v1-experiment")))


class WorkflowSuccessRatesTest(unittest.TestCase):
    def test_overall_and_per_workflow_rates(self):
        runs = [
            run("CI", "success"),
            run("CI", "success"),
            run("CI", "success"),
            run("CI", "failure"),
            run("CI", "cancelled"),
            run("CI", None),
            run("CI", "failure", branch="feat/x"),
            run("E2E Tests", "timed_out", event="schedule"),
        ]
        result = metrics.workflow_success_rates(runs)
        self.assertEqual(result["overall"]["success"], 3)
        self.assertEqual(result["overall"]["failed"], 2)
        self.assertAlmostEqual(result["overall"]["rate"], 0.6)
        self.assertAlmostEqual(result["by_workflow"]["CI"]["rate"], 0.75)
        self.assertEqual(result["by_workflow"]["E2E Tests"]["rate"], 0.0)

    def test_no_runs_gives_null_rate_not_zero(self):
        result = metrics.workflow_success_rates([])
        self.assertEqual(result["overall"]["total"], 0)
        self.assertIsNone(result["overall"]["rate"])
        self.assertEqual(result["by_workflow"], {})


class TestJobPassRatesTest(unittest.TestCase):
    def test_counts_only_tracked_jobs_of_their_workflow(self):
        jobs = [
            ("CI", {"name": "Unit Tests", "conclusion": "success"}),
            ("CI", {"name": "Unit Tests", "conclusion": "success"}),
            ("CI", {"name": "Unit Tests", "conclusion": "failure"}),
            ("CI", {"name": "Unit Tests", "conclusion": "skipped"}),
            ("CI", {"name": "Lint", "conclusion": "failure"}),
            ("E2E Tests", {"name": "Unit Tests", "conclusion": "failure"}),
            ("E2E Tests", {"name": "Kubernetes E2E Tests", "conclusion": "success"}),
        ]
        result = metrics.test_job_pass_rates(jobs)
        self.assertEqual(result["Unit Tests"]["success"], 2)
        self.assertEqual(result["Unit Tests"]["failed"], 1)
        self.assertAlmostEqual(result["Unit Tests"]["rate"], 2 / 3)
        self.assertEqual(result["Kubernetes E2E Tests"]["rate"], 1.0)
        self.assertNotIn("Lint", result)

    def test_every_tracked_job_is_present_with_null_rate_when_unseen(self):
        result = metrics.test_job_pass_rates([])
        self.assertEqual(
            set(result),
            {
                "Unit Tests",
                "Integration Tests (K8s)",
                "Integration Tests (Cloud Foundry)",
                "Kubernetes E2E Tests",
                "CloudFoundry E2E Tests",
            },
        )
        self.assertIsNone(result["Unit Tests"]["rate"])


class PercentileTest(unittest.TestCase):
    def test_nearest_rank(self):
        self.assertEqual(metrics.percentile([1, 2, 3, 4, 5, 6, 7, 8, 9, 10], 90), 9)
        self.assertEqual(metrics.percentile([5], 90), 5)

    def test_empty_is_none(self):
        self.assertIsNone(metrics.percentile([], 90))
        self.assertIsNone(metrics.summarize_hours([]))


def pr(login, created, merged=None, reviews=None):
    return {
        "user": {"login": login},
        "created_at": created,
        "merged_at": merged,
        "reviews": reviews or [],
    }


class DependabotStatsTest(unittest.TestCase):
    def test_counts_weekly_buckets_and_median_merge_time(self):
        prs = [
            pr("dependabot[bot]", "2026-10-05T03:00:00Z", "2026-10-05T09:00:00Z"),
            pr("dependabot[bot]", "2026-10-05T04:00:00Z", "2026-10-13T04:00:00Z"),
            pr("dependabot[bot]", "2026-10-05T04:00:00Z"),
            pr("dependabot[bot]", "2026-08-01T04:00:00Z", "2026-08-02T04:00:00Z"),
            pr("someone", "2026-10-06T04:00:00Z", "2026-10-06T05:00:00Z"),
        ]
        result = metrics.dependabot_stats(prs, WINDOW_START)
        self.assertEqual(result["opened"], 3)
        self.assertEqual(result["merged"], 2)
        # merge durations are 6h and 192h; median of two is their mean
        self.assertEqual(result["median_hours_to_merge"], 99.0)
        self.assertEqual(
            result["weekly"],
            [
                {"week": "2026-W41", "opened": 3, "merged": 1},
                {"week": "2026-W42", "opened": 0, "merged": 1},
            ],
        )

    def test_no_dependabot_prs(self):
        result = metrics.dependabot_stats([], WINDOW_START)
        self.assertEqual(result["opened"], 0)
        self.assertIsNone(result["median_hours_to_merge"])
        self.assertEqual(result["weekly"], [])


class ReviewTurnaroundTest(unittest.TestCase):
    def test_first_review_and_merge_times(self):
        prs = [
            pr(
                "alice",
                "2026-10-05T00:00:00Z",
                merged="2026-10-06T00:00:00Z",
                reviews=[
                    {"user": {"login": "bob"}, "submitted_at": "2026-10-05T04:00:00Z"},
                    {"user": {"login": "carol"}, "submitted_at": "2026-10-05T02:00:00Z"},
                ],
            ),
            pr(
                "alice",
                "2026-10-07T00:00:00Z",
                merged="2026-10-09T00:00:00Z",
                reviews=[{"user": {"login": "bob"}, "submitted_at": "2026-10-07T10:00:00Z"}],
            ),
            pr("dependabot[bot]", "2026-10-07T00:00:00Z", merged="2026-10-07T01:00:00Z"),
            pr("alice", "2026-08-01T00:00:00Z", merged="2026-08-02T00:00:00Z"),
        ]
        result = metrics.review_turnaround(prs, WINDOW_START)
        self.assertEqual(result["sample"], 2)
        self.assertEqual(result["first_review_hours"], {"n": 2, "median": 6.0, "p90": 10.0})
        self.assertEqual(result["merge_hours"], {"n": 2, "median": 36.0, "p90": 48.0})

    def test_pending_and_self_reviews_are_ignored(self):
        prs = [
            pr(
                "alice",
                "2026-10-05T00:00:00Z",
                reviews=[
                    {"user": {"login": "bob"}, "submitted_at": None},
                    {"user": {"login": "alice"}, "submitted_at": "2026-10-05T01:00:00Z"},
                ],
            )
        ]
        result = metrics.review_turnaround(prs, WINDOW_START)
        self.assertEqual(result["sample"], 1)
        self.assertIsNone(result["first_review_hours"])
        self.assertIsNone(result["merge_hours"])


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `python3 -m unittest discover -s scripts/ci-health -v`
Expected: FAIL / ERROR with `ModuleNotFoundError: No module named 'metrics'`.

- [ ] **Step 3: Write the implementation**

Create `scripts/ci-health/metrics.py`:

```python
"""Pure aggregation functions for CI/CD health metrics. No network access."""

from __future__ import annotations

import math
import re
import statistics
from datetime import datetime

DEFAULT_BRANCH = "main"
DEPENDABOT_LOGIN = "dependabot[bot]"

# Workflow name -> job names whose pass rate is tracked (the `name:` of each job).
TEST_JOBS = {
    "CI": (
        "Unit Tests",
        "Integration Tests (K8s)",
        "Integration Tests (Cloud Foundry)",
    ),
    "E2E Tests": (
        "Kubernetes E2E Tests",
        "CloudFoundry E2E Tests",
    ),
}

# Conclusions that count towards a success rate. cancelled/skipped/None
# (still running) say nothing about health and are excluded.
COUNTED_CONCLUSIONS = ("success", "failure", "timed_out")

_RELEASE_TAG = re.compile(r"^v\d+\.\d+\.\d+")


def parse_ts(value: str) -> datetime:
    """Parse a GitHub ISO-8601 timestamp (trailing Z) into an aware datetime."""
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def is_health_run(run: dict) -> bool:
    """True for runs that reflect the health of main.

    Counted: pushes to main, scheduled runs, manual runs on main and release
    tags. Pull-request runs are never counted, even when a fork's branch is
    itself called "main"; the PR already shows its own status.
    """
    event = run.get("event")
    branch = run.get("head_branch") or ""
    if event == "pull_request":
        return False
    if event == "schedule":
        return True
    if branch == DEFAULT_BRANCH:
        return True
    return event == "push" and bool(_RELEASE_TAG.match(branch))


def _rate(success: int, failed: int) -> dict:
    total = success + failed
    return {
        "success": success,
        "failed": failed,
        "total": total,
        "rate": (success / total) if total else None,
    }


def workflow_success_rates(runs) -> dict:
    """Overall and per-workflow success rate from workflow-run dicts."""
    counts: dict[str, list[int]] = {}
    for run in runs:
        if not is_health_run(run):
            continue
        conclusion = run.get("conclusion")
        if conclusion not in COUNTED_CONCLUSIONS:
            continue
        success_failed = counts.setdefault(run["name"], [0, 0])
        success_failed[0 if conclusion == "success" else 1] += 1
    overall_success = sum(c[0] for c in counts.values())
    overall_failed = sum(c[1] for c in counts.values())
    return {
        "overall": _rate(overall_success, overall_failed),
        "by_workflow": {name: _rate(*counts[name]) for name in sorted(counts)},
    }


def test_job_pass_rates(jobs) -> dict:
    """Pass rate per tracked test job.

    `jobs` is an iterable of (workflow_name, job_dict). Every tracked job is
    present in the result; jobs never seen get a null rate.
    """
    counts = {name: [0, 0] for names in TEST_JOBS.values() for name in names}
    for workflow, job in jobs:
        name = job.get("name")
        if name not in TEST_JOBS.get(workflow, ()):
            continue
        conclusion = job.get("conclusion")
        if conclusion not in COUNTED_CONCLUSIONS:
            continue
        counts[name][0 if conclusion == "success" else 1] += 1
    return {name: _rate(*c) for name, c in counts.items()}


def iso_week(moment: datetime) -> str:
    year, week, _ = moment.isocalendar()
    return f"{year}-W{week:02d}"


def hours_between(start: datetime, end: datetime) -> float:
    return max(0.0, (end - start).total_seconds() / 3600)


def percentile(values, pct: float):
    """Nearest-rank percentile; None for empty input."""
    if not values:
        return None
    ordered = sorted(values)
    rank = max(1, math.ceil(pct / 100 * len(ordered)))
    return ordered[rank - 1]


def summarize_hours(values):
    """{"n", "median", "p90"} in hours, or None when there is no data."""
    if not values:
        return None
    return {
        "n": len(values),
        "median": round(statistics.median(values), 1),
        "p90": round(percentile(values, 90), 1),
    }


def dependabot_stats(prs, window_start: datetime) -> dict:
    """Dependabot PR activity for PRs created inside the window.

    Opened PRs are bucketed by the ISO week they were created in, merged PRs
    by the week they were merged in.
    """
    weekly: dict[str, dict] = {}

    def bucket(moment: datetime) -> dict:
        week = iso_week(moment)
        return weekly.setdefault(week, {"week": week, "opened": 0, "merged": 0})

    opened = merged = 0
    merge_hours = []
    for pr in prs:
        if (pr.get("user") or {}).get("login") != DEPENDABOT_LOGIN:
            continue
        created = parse_ts(pr["created_at"])
        if created < window_start:
            continue
        opened += 1
        bucket(created)["opened"] += 1
        if pr.get("merged_at"):
            merged_at = parse_ts(pr["merged_at"])
            merged += 1
            bucket(merged_at)["merged"] += 1
            merge_hours.append(hours_between(created, merged_at))
    summary = summarize_hours(merge_hours)
    return {
        "opened": opened,
        "merged": merged,
        "median_hours_to_merge": summary["median"] if summary else None,
        "weekly": [weekly[week] for week in sorted(weekly)],
    }


def review_turnaround(prs, window_start: datetime) -> dict:
    """Review turnaround for non-Dependabot PRs created inside the window.

    Each PR dict carries a "reviews" list as returned by the reviews API.
    The first review is the earliest *submitted* review by someone other than
    the PR author; pending reviews (no submitted_at) and self-reviews are
    ignored.
    """
    sample = 0
    first_review_hours = []
    merge_hours = []
    for pr in prs:
        author = (pr.get("user") or {}).get("login")
        if author == DEPENDABOT_LOGIN:
            continue
        created = parse_ts(pr["created_at"])
        if created < window_start:
            continue
        sample += 1
        submitted = [
            parse_ts(review["submitted_at"])
            for review in pr.get("reviews") or []
            if review.get("submitted_at")
            and (review.get("user") or {}).get("login") != author
        ]
        if submitted:
            first_review_hours.append(hours_between(created, min(submitted)))
        if pr.get("merged_at"):
            merge_hours.append(hours_between(created, parse_ts(pr["merged_at"])))
    return {
        "sample": sample,
        "first_review_hours": summarize_hours(first_review_hours),
        "merge_hours": summarize_hours(merge_hours),
    }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `python3 -m unittest discover -s scripts/ci-health -v`
Expected: `Ran 16 tests` ... `OK`.

- [ ] **Step 5: Commit**

```bash
git add scripts/ci-health/metrics.py scripts/ci-health/test_metrics.py
git commit -m "feat: Add CI/CD health metrics aggregation (#16)"
```

---

### Task 2: GitHub client and collector

**Files:**
- Create: `scripts/ci-health/ghapi.py`, `scripts/ci-health/collect.py`
- Test: `scripts/ci-health/test_ghapi.py`, `scripts/ci-health/test_collect.py`

**Interfaces:**
- Consumes: everything listed under Task 1 "Produces".
- Produces (used by Task 3 and the workflows):
  - `ghapi.GitHubError(Exception)`
  - `ghapi.next_link(link_header: str | None) -> str | None`
  - `ghapi.GitHubClient(token=None, api_root=API_ROOT, timeout=30)` with `get(path, params=None) -> object` and `paginate(path, params=None, key=None, max_pages=20) -> Iterator`
  - `collect.collect(client, repo: str, now: datetime, window_days=30) -> dict` returning `{"generated_at", "window", "workflows", "test_jobs", "dependabot", "review", "errors"}` (any of the four metric sections may be `None` with a matching `errors` key)
  - CLI: `python3 scripts/ci-health/collect.py [--output PATH] [--days N]`, env `GITHUB_TOKEN`, `GITHUB_REPOSITORY`

- [ ] **Step 1: Write the failing tests**

Create `scripts/ci-health/test_ghapi.py`:

```python
import unittest

import ghapi


class NextLinkTest(unittest.TestCase):
    def test_extracts_next(self):
        header = (
            '<https://api.github.com/x?page=2>; rel="next", '
            '<https://api.github.com/x?page=5>; rel="last"'
        )
        self.assertEqual(ghapi.next_link(header), "https://api.github.com/x?page=2")

    def test_missing_or_last_page(self):
        self.assertIsNone(ghapi.next_link(None))
        self.assertIsNone(ghapi.next_link('<https://api.github.com/x?page=1>; rel="prev"'))


class FakeClient(ghapi.GitHubClient):
    def __init__(self, pages):
        super().__init__(token=None, api_root="https://example.test")
        self.pages = pages
        self.urls = []

    def _request(self, url):
        self.urls.append(url)
        body, link = self.pages[len(self.urls) - 1]
        return body, {"Link": link} if link else {}


class PaginateTest(unittest.TestCase):
    def test_follows_next_links_and_unwraps_key(self):
        client = FakeClient(
            [
                ({"runs": [1, 2]}, '<https://example.test/p2>; rel="next"'),
                ({"runs": [3]}, None),
            ]
        )
        self.assertEqual(list(client.paginate("/runs", {"status": "completed"}, key="runs")), [1, 2, 3])
        self.assertEqual(client.urls[0], "https://example.test/runs?per_page=100&status=completed")
        self.assertEqual(client.urls[1], "https://example.test/p2")

    def test_plain_list_response_without_key(self):
        client = FakeClient([([{"n": 1}], None)])
        self.assertEqual(list(client.paginate("/pulls")), [{"n": 1}])

    def test_stops_at_max_pages(self):
        link = '<https://example.test/again>; rel="next"'
        client = FakeClient([([1], link)] * 5)
        self.assertEqual(list(client.paginate("/x", max_pages=3)), [1, 1, 1])


if __name__ == "__main__":
    unittest.main()
```

Create `scripts/ci-health/test_collect.py`:

```python
import unittest
from datetime import datetime, timezone

import collect
import ghapi

NOW = datetime(2026, 10, 11, 12, 0, tzinfo=timezone.utc)


class FakeClient:
    """Serves canned API data by path; a path mapped to an Exception raises it."""

    def __init__(self, data):
        self.data = data
        self.calls = []

    def paginate(self, path, params=None, key=None, max_pages=20):
        self.calls.append(path)
        result = self.data[path]
        if isinstance(result, Exception):
            raise result
        return iter(result)


REPO = "muto-io/muto"
RUNS = [
    {"id": 1, "name": "CI", "conclusion": "success", "head_branch": "main", "event": "push"},
    {"id": 2, "name": "CI", "conclusion": "failure", "head_branch": "feat/x", "event": "push"},
    {"id": 3, "name": "Release", "conclusion": "failure", "head_branch": "main", "event": "push"},
]
PRS = [
    {
        "number": 7,
        "user": {"login": "alice"},
        "created_at": "2026-10-05T00:00:00Z",
        "merged_at": "2026-10-06T00:00:00Z",
    },
    {
        "number": 6,
        "user": {"login": "dependabot[bot]"},
        "created_at": "2026-10-04T00:00:00Z",
        "merged_at": "2026-10-04T06:00:00Z",
    },
    {
        "number": 1,
        "user": {"login": "alice"},
        "created_at": "2026-07-01T00:00:00Z",
        "merged_at": None,
    },
]


def data(**overrides):
    base = {
        f"/repos/{REPO}/actions/runs": RUNS,
        f"/repos/{REPO}/actions/runs/1/jobs": [
            {"name": "Unit Tests", "conclusion": "success"},
            {"name": "Lint", "conclusion": "success"},
        ],
        f"/repos/{REPO}/pulls": PRS,
        f"/repos/{REPO}/pulls/7/reviews": [
            {"user": {"login": "bob"}, "submitted_at": "2026-10-05T03:00:00Z"}
        ],
    }
    base.update(overrides)
    return base


class CollectTest(unittest.TestCase):
    def test_happy_path_builds_all_sections(self):
        client = FakeClient(data())
        result = collect.collect(client, REPO, NOW)
        self.assertEqual(result["errors"], {})
        self.assertEqual(result["generated_at"], "2026-10-11T12:00:00+00:00")
        self.assertEqual(result["window"]["days"], 30)
        self.assertEqual(result["workflows"]["overall"]["total"], 2)
        self.assertEqual(result["test_jobs"]["Unit Tests"]["rate"], 1.0)
        self.assertEqual(result["dependabot"]["opened"], 1)
        self.assertEqual(result["review"]["sample"], 1)
        self.assertEqual(result["review"]["first_review_hours"]["median"], 3.0)

    def test_only_tracked_health_runs_fetch_jobs(self):
        client = FakeClient(data())
        collect.collect(client, REPO, NOW)
        job_calls = [c for c in client.calls if c.endswith("/jobs")]
        self.assertEqual(job_calls, [f"/repos/{REPO}/actions/runs/1/jobs"])

    def test_stops_paging_pull_requests_at_window_start(self):
        client = FakeClient(data())
        prs = collect.fetch_prs(client, REPO, datetime(2026, 9, 11, tzinfo=timezone.utc))
        self.assertEqual([pr["number"] for pr in prs], [7, 6])

    def test_run_fetch_failure_nulls_workflow_and_test_job_sections_only(self):
        client = FakeClient(data(**{f"/repos/{REPO}/actions/runs": ghapi.GitHubError("HTTP 403")}))
        result = collect.collect(client, REPO, NOW)
        self.assertIsNone(result["workflows"])
        self.assertIsNone(result["test_jobs"])
        self.assertIn("HTTP 403", result["errors"]["workflows"])
        self.assertIn("test_jobs", result["errors"])
        self.assertIsNotNone(result["dependabot"])
        self.assertIsNotNone(result["review"])

    def test_review_fetch_failure_keeps_dependabot(self):
        client = FakeClient(data(**{f"/repos/{REPO}/pulls/7/reviews": ghapi.GitHubError("HTTP 500")}))
        result = collect.collect(client, REPO, NOW)
        self.assertIsNone(result["review"])
        self.assertIn("review", result["errors"])
        self.assertEqual(result["dependabot"]["opened"], 1)

    def test_empty_repository_yields_null_rates_not_errors(self):
        client = FakeClient(
            {f"/repos/{REPO}/actions/runs": [], f"/repos/{REPO}/pulls": []}
        )
        result = collect.collect(client, REPO, NOW)
        self.assertEqual(result["errors"], {})
        self.assertIsNone(result["workflows"]["overall"]["rate"])
        self.assertEqual(result["dependabot"]["opened"], 0)
        self.assertIsNone(result["review"]["first_review_hours"])


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `python3 -m unittest discover -s scripts/ci-health -v 2>&1 | tail -15`
Expected: ERROR for `test_ghapi` and `test_collect` with `ModuleNotFoundError: No module named 'ghapi'` / `'collect'`; Task 1 tests still pass.

- [ ] **Step 3: Write the implementation**

Create `scripts/ci-health/ghapi.py`:

```python
"""Minimal GitHub REST client (stdlib only) with Link-header pagination."""

from __future__ import annotations

import json
import re
import urllib.error
import urllib.parse
import urllib.request

API_ROOT = "https://api.github.com"
_NEXT = re.compile(r'<([^>]+)>;\s*rel="next"')


class GitHubError(Exception):
    """Any failure talking to the GitHub API."""


def next_link(link_header):
    """Return the rel="next" URL from a Link header, or None."""
    if not link_header:
        return None
    match = _NEXT.search(link_header)
    return match.group(1) if match else None


class GitHubClient:
    def __init__(self, token=None, api_root=API_ROOT, timeout=30):
        self.token = token
        self.api_root = api_root
        self.timeout = timeout

    def _request(self, url):
        """Fetch `url`; return (parsed_json, headers). The single network seam."""
        headers = {
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "muto-ci-health",
        }
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        request = urllib.request.Request(url, headers=headers)
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as response:
                return json.load(response), response.headers
        except urllib.error.HTTPError as exc:
            raise GitHubError(f"GET {url} failed: HTTP {exc.code}") from exc
        except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
            raise GitHubError(f"GET {url} failed: {exc}") from exc

    def _url(self, path, params):
        query = urllib.parse.urlencode(params or {})
        return f"{self.api_root}{path}" + (f"?{query}" if query else "")

    def get(self, path, params=None):
        body, _ = self._request(self._url(path, params))
        return body

    def paginate(self, path, params=None, key=None, max_pages=20):
        """Yield items across pages. `key` names the list inside object responses."""
        params = {"per_page": 100, **(params or {})}
        url = self._url(path, params)
        for _ in range(max_pages):
            body, headers = self._request(url)
            yield from (body[key] if key else body)
            url = next_link(headers.get("Link"))
            if not url:
                return
```

Create `scripts/ci-health/collect.py`:

```python
"""Collect CI/CD health metrics from the GitHub API into data.json.

Run from the Pages build (see .github/workflows/pages.yml). Environment:
GITHUB_TOKEN, GITHUB_REPOSITORY. A failing metric becomes null with an entry
under "errors"; the script still exits 0 so the docs build is never blocked.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

import metrics
from ghapi import GitHubClient, GitHubError

WINDOW_DAYS = 30
DEFAULT_OUTPUT = "docs/operations/ci-health/data.json"


def fetch_runs(client, repo, start):
    params = {"status": "completed", "created": f">={start:%Y-%m-%d}"}
    return list(client.paginate(f"/repos/{repo}/actions/runs", params, key="workflow_runs"))


def fetch_test_job_pairs(client, repo, runs):
    """(workflow_name, job) pairs for every counted run of a tracked workflow."""
    pairs = []
    for run in runs:
        if run.get("name") not in metrics.TEST_JOBS:
            continue
        if not metrics.is_health_run(run):
            continue
        if run.get("conclusion") not in metrics.COUNTED_CONCLUSIONS:
            continue
        jobs = client.paginate(
            f"/repos/{repo}/actions/runs/{run['id']}/jobs", {"filter": "latest"}, key="jobs"
        )
        pairs.extend((run["name"], job) for job in jobs)
    return pairs


def fetch_prs(client, repo, start):
    """PRs created on or after `start`, newest first (stops paging early)."""
    params = {"state": "all", "sort": "created", "direction": "desc"}
    prs = []
    for pr in client.paginate(f"/repos/{repo}/pulls", params):
        if metrics.parse_ts(pr["created_at"]) < start:
            break
        prs.append(pr)
    return prs


def attach_reviews(client, repo, prs):
    for pr in prs:
        if (pr.get("user") or {}).get("login") == metrics.DEPENDABOT_LOGIN:
            pr["reviews"] = []
            continue
        pr["reviews"] = list(client.paginate(f"/repos/{repo}/pulls/{pr['number']}/reviews"))
    return prs


def _section(errors, name, fn):
    try:
        return fn()
    except GitHubError as exc:
        errors[name] = str(exc)
        return None


def collect(client, repo, now, window_days=WINDOW_DAYS):
    start = now - timedelta(days=window_days)
    errors: dict[str, str] = {}

    runs = _section(errors, "workflows", lambda: fetch_runs(client, repo, start))
    workflows = metrics.workflow_success_rates(runs) if runs is not None else None

    test_jobs = None
    if runs is None:
        errors["test_jobs"] = "workflow runs unavailable"
    else:
        pairs = _section(errors, "test_jobs", lambda: fetch_test_job_pairs(client, repo, runs))
        if pairs is not None:
            test_jobs = metrics.test_job_pass_rates(pairs)

    prs = _section(errors, "dependabot", lambda: fetch_prs(client, repo, start))
    dependabot = metrics.dependabot_stats(prs, start) if prs is not None else None

    review = None
    if prs is None:
        errors["review"] = "pull requests unavailable"
    else:
        with_reviews = _section(errors, "review", lambda: attach_reviews(client, repo, prs))
        if with_reviews is not None:
            review = metrics.review_turnaround(with_reviews, start)

    return {
        "generated_at": now.isoformat(timespec="seconds"),
        "window": {
            "days": window_days,
            "start": start.isoformat(timespec="seconds"),
            "end": now.isoformat(timespec="seconds"),
        },
        "workflows": workflows,
        "test_jobs": test_jobs,
        "dependabot": dependabot,
        "review": review,
        "errors": errors,
    }


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", default=DEFAULT_OUTPUT)
    parser.add_argument("--days", type=int, default=WINDOW_DAYS)
    args = parser.parse_args(argv)

    repo = os.environ.get("GITHUB_REPOSITORY")
    if not repo:
        print("GITHUB_REPOSITORY is not set", file=sys.stderr)
        return 2

    client = GitHubClient(token=os.environ.get("GITHUB_TOKEN"))
    data = collect(client, repo, datetime.now(timezone.utc), args.days)

    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(data, indent=2) + "\n")
    for name, message in data["errors"].items():
        print(f"::warning::ci-health: {name} unavailable: {message}")
    print(f"Wrote {output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `python3 -m unittest discover -s scripts/ci-health -v 2>&1 | tail -8`
Expected: `Ran 27 tests` ... `OK`.

- [ ] **Step 5: Smoke-run the collector against the real repo**

```bash
GITHUB_TOKEN="$(gh auth token)" GITHUB_REPOSITORY=muto-io/muto \
  python3 scripts/ci-health/collect.py --output /tmp/ci-health-smoke.json
python3 -c "import json; d=json.load(open('/tmp/ci-health-smoke.json')); print(d['errors']); print(d['workflows']['overall'])"
```

Expected: `Wrote /tmp/ci-health-smoke.json`, `errors` is `{}` (or lists only API failures you can explain), and `overall` has non-zero `total`. Do not commit this file.

- [ ] **Step 6: Commit**

```bash
git add scripts/ci-health/ghapi.py scripts/ci-health/collect.py scripts/ci-health/test_ghapi.py scripts/ci-health/test_collect.py
git commit -m "feat: Add CI/CD health metrics collector (#16)"
```

---

### Task 3: Slack notifier

**Files:**
- Create: `scripts/ci-health/notify.py`
- Test: `scripts/ci-health/test_notify.py`

**Interfaces:**
- Consumes: `metrics.is_health_run`, `metrics.COUNTED_CONCLUSIONS`, `ghapi.GitHubClient`, `ghapi.GitHubError`.
- Produces (used by the notify workflow):
  - `notify.decide_message(event_name: str, event: dict, client, repo: str) -> str | None`
  - `notify.post_slack(webhook_url: str, text: str, opener=urllib.request.urlopen) -> None`
  - CLI `python3 scripts/ci-health/notify.py`; env `SLACK_WEBHOOK_URL`, `GITHUB_TOKEN`, `GITHUB_REPOSITORY`, `GITHUB_EVENT_NAME`, `GITHUB_EVENT_PATH`

- [ ] **Step 1: Write the failing tests**

Create `scripts/ci-health/test_notify.py`:

```python
import unittest

import ghapi
import notify

REPO = "muto-io/muto"


def run(**overrides):
    base = {
        "id": 100,
        "workflow_id": 7,
        "name": "CI",
        "conclusion": "failure",
        "head_branch": "main",
        "head_sha": "abcdef1234567890",
        "event": "push",
        "html_url": "https://github.com/muto-io/muto/actions/runs/100",
        "created_at": "2026-10-11T10:00:00Z",
        "actor": {"login": "alice"},
        "head_commit": {"message": "fix: handle nil spec\n\nlong body"},
    }
    base.update(overrides)
    return base


class FakeClient:
    def __init__(self, jobs=None, previous_runs=None, jobs_error=None):
        self.jobs = jobs or []
        self.previous_runs = previous_runs or []
        self.jobs_error = jobs_error

    def paginate(self, path, params=None, key=None, max_pages=20):
        if self.jobs_error:
            raise self.jobs_error
        return iter(self.jobs)

    def get(self, path, params=None):
        return {"workflow_runs": self.previous_runs}


class MessageTest(unittest.TestCase):
    def test_failure_message_contents(self):
        text = notify.build_failure_message(run(), ["Unit Tests", "Lint"])
        self.assertIn("*CI* failed on `main`", text)
        self.assertIn("<https://github.com/muto-io/muto/actions/runs/100|View run>", text)
        self.assertIn("`abcdef1` fix: handle nil spec", text)
        self.assertNotIn("long body", text)
        self.assertIn("by alice", text)
        self.assertIn("Failed jobs: Unit Tests, Lint", text)

    def test_timeout_wording_and_scheduled_trigger(self):
        text = notify.build_failure_message(
            run(conclusion="timed_out", event="schedule"), []
        )
        self.assertIn("timed out", text)
        self.assertIn("scheduled run", text)
        self.assertNotIn("Failed jobs", text)

    def test_commit_text_cannot_ping_or_link(self):
        evil = run(head_commit={"message": "<!channel> <https://evil.test|click> & more"})
        text = notify.build_failure_message(evil, [])
        self.assertNotIn("<!channel>", text)
        self.assertNotIn("<https://evil.test", text)
        self.assertIn("&lt;!channel&gt;", text)
        self.assertIn("&amp; more", text)

    def test_long_title_is_truncated(self):
        long_run = run(head_commit={"message": "x" * 500})
        title = notify.commit_title(long_run)
        self.assertEqual(len(title), notify.MAX_TITLE)
        self.assertTrue(title.endswith("…"))

    def test_missing_commit_and_actor_do_not_crash(self):
        text = notify.build_failure_message(run(head_commit=None, actor=None), [])
        self.assertIn("by unknown", text)

    def test_recovery_message(self):
        text = notify.build_recovery_message(run(conclusion="success"))
        self.assertIn("*CI* recovered on `main`", text)


class DecideMessageTest(unittest.TestCase):
    def decide(self, event_name, event, client):
        return notify.decide_message(event_name, event, client, REPO)

    def test_failure_on_main_alerts_with_failed_jobs(self):
        client = FakeClient(
            jobs=[
                {"name": "Unit Tests", "conclusion": "failure"},
                {"name": "Build", "conclusion": "success"},
            ]
        )
        text = self.decide("workflow_run", {"workflow_run": run()}, client)
        self.assertIn("Failed jobs: Unit Tests", text)

    def test_failure_on_pr_branch_is_silent(self):
        event = {"workflow_run": run(head_branch="feat/x", event="pull_request")}
        self.assertIsNone(self.decide("workflow_run", event, FakeClient()))

    def test_fork_pr_from_branch_named_main_is_silent(self):
        event = {"workflow_run": run(head_branch="main", event="pull_request")}
        self.assertIsNone(self.decide("workflow_run", event, FakeClient()))

    def test_job_lookup_failure_still_alerts(self):
        client = FakeClient(jobs_error=ghapi.GitHubError("HTTP 502"))
        text = self.decide("workflow_run", {"workflow_run": run()}, client)
        self.assertIn("*CI* failed", text)

    def test_success_after_failure_sends_recovery(self):
        previous = [
            {"id": 99, "conclusion": "failure", "created_at": "2026-10-11T09:00:00Z"},
        ]
        event = {"workflow_run": run(conclusion="success")}
        text = self.decide("workflow_run", event, FakeClient(previous_runs=previous))
        self.assertIn("recovered", text)

    def test_success_after_success_is_silent(self):
        previous = [{"id": 99, "conclusion": "success", "created_at": "2026-10-11T09:00:00Z"}]
        event = {"workflow_run": run(conclusion="success")}
        self.assertIsNone(self.decide("workflow_run", event, FakeClient(previous_runs=previous)))

    def test_first_ever_run_success_is_silent(self):
        event = {"workflow_run": run(conclusion="success")}
        self.assertIsNone(self.decide("workflow_run", event, FakeClient(previous_runs=[])))

    def test_recovery_skips_cancelled_runs_and_the_run_itself(self):
        previous = [
            {"id": 100, "conclusion": "success", "created_at": "2026-10-11T10:00:00Z"},
            {"id": 99, "conclusion": "cancelled", "created_at": "2026-10-11T09:30:00Z"},
            {"id": 98, "conclusion": "failure", "created_at": "2026-10-11T09:00:00Z"},
        ]
        event = {"workflow_run": run(conclusion="success")}
        text = self.decide("workflow_run", event, FakeClient(previous_runs=previous))
        self.assertIn("recovered", text)

    def test_cancelled_run_is_silent(self):
        event = {"workflow_run": run(conclusion="cancelled")}
        self.assertIsNone(self.decide("workflow_run", event, FakeClient()))

    def test_workflow_dispatch_sends_test_message(self):
        text = self.decide("workflow_dispatch", {"inputs": {"message": "hello"}}, FakeClient())
        self.assertEqual(text, "hello")
        default = self.decide("workflow_dispatch", {}, FakeClient())
        self.assertEqual(default, notify.DEFAULT_TEST_MESSAGE)


class PostSlackTest(unittest.TestCase):
    def test_posts_json_body(self):
        seen = {}

        class Response:
            status = 200

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

        def opener(request, timeout):
            seen["url"] = request.full_url
            seen["body"] = request.data
            seen["type"] = request.get_header("Content-type")
            return Response()

        notify.post_slack("https://hooks.test/abc", "hi", opener=opener)
        self.assertEqual(seen["url"], "https://hooks.test/abc")
        self.assertEqual(seen["body"], b'{"text": "hi"}')
        self.assertEqual(seen["type"], "application/json")


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `python3 -m unittest scripts/ci-health/test_notify.py 2>&1 | tail -5` — if the path form errors, use `python3 -m unittest discover -s scripts/ci-health -p 'test_notify.py'`.
Expected: ERROR `ModuleNotFoundError: No module named 'notify'`.

- [ ] **Step 3: Write the implementation**

Create `scripts/ci-health/notify.py`:

```python
"""Post CI/CD failure and recovery notifications to Slack.

Run by .github/workflows/ci-failure-notify.yaml. Environment: GITHUB_TOKEN,
GITHUB_REPOSITORY, GITHUB_EVENT_NAME, GITHUB_EVENT_PATH and the optional
SLACK_WEBHOOK_URL. Untrusted values (commit messages, branch names) are read
from the event payload file, never interpolated into shell.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.request

import metrics
from ghapi import GitHubClient, GitHubError

FAILURE_CONCLUSIONS = ("failure", "timed_out")
DEFAULT_TEST_MESSAGE = "CI/CD health notifications: test message"
MAX_TITLE = 100


def slack_escape(text: str) -> str:
    """Escape Slack control characters so commit text cannot ping or link."""
    return text.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def commit_title(run: dict) -> str:
    message = (run.get("head_commit") or {}).get("message") or ""
    title = message.splitlines()[0] if message else ""
    if len(title) > MAX_TITLE:
        title = title[: MAX_TITLE - 1] + "…"
    return slack_escape(title)


def _context_line(run: dict) -> str:
    sha = (run.get("head_sha") or "")[:7]
    actor = slack_escape((run.get("actor") or {}).get("login") or "unknown")
    trigger = "scheduled run" if run.get("event") == "schedule" else "commit"
    title = commit_title(run)
    commit = f"`{sha}` {title}".strip()
    return f"<{run['html_url']}|View run> · {trigger} {commit} · by {actor}"


def build_failure_message(run: dict, failed_jobs) -> str:
    name = slack_escape(run["name"])
    ref = slack_escape(run.get("head_branch") or "unknown")
    outcome = "timed out" if run.get("conclusion") == "timed_out" else "failed"
    lines = [f":red_circle: *{name}* {outcome} on `{ref}`", _context_line(run)]
    if failed_jobs:
        lines.append("Failed jobs: " + ", ".join(slack_escape(j) for j in failed_jobs))
    return "\n".join(lines)


def build_recovery_message(run: dict) -> str:
    name = slack_escape(run["name"])
    ref = slack_escape(run.get("head_branch") or "unknown")
    return "\n".join(
        [f":large_green_circle: *{name}* recovered on `{ref}`", _context_line(run)]
    )


def failed_job_names(jobs):
    return [j["name"] for j in jobs if j.get("conclusion") in FAILURE_CONCLUSIONS]


def previous_conclusion(client, repo: str, run: dict):
    """Conclusion of the latest earlier counted run of the same workflow and branch."""
    body = client.get(
        f"/repos/{repo}/actions/workflows/{run['workflow_id']}/runs",
        {"branch": run["head_branch"], "status": "completed", "per_page": 20},
    )
    for candidate in body.get("workflow_runs", []):
        if candidate["id"] == run["id"]:
            continue
        if candidate["created_at"] >= run["created_at"]:
            continue
        if candidate.get("conclusion") in metrics.COUNTED_CONCLUSIONS:
            return candidate["conclusion"]
    return None


def decide_message(event_name: str, event: dict, client, repo: str):
    """Return the Slack text to send for this event, or None to stay quiet."""
    if event_name == "workflow_dispatch":
        return (event.get("inputs") or {}).get("message") or DEFAULT_TEST_MESSAGE

    run = event.get("workflow_run")
    if not run or not metrics.is_health_run(run):
        return None

    conclusion = run.get("conclusion")
    if conclusion in FAILURE_CONCLUSIONS:
        try:
            jobs = client.paginate(
                f"/repos/{repo}/actions/runs/{run['id']}/jobs", {"filter": "latest"}, key="jobs"
            )
            failed = failed_job_names(jobs)
        except GitHubError as exc:
            print(f"::warning::could not list failed jobs: {exc}")
            failed = []
        return build_failure_message(run, failed)

    if conclusion == "success":
        try:
            previous = previous_conclusion(client, repo, run)
        except GitHubError as exc:
            print(f"::warning::could not look up previous run: {exc}")
            return None
        if previous in FAILURE_CONCLUSIONS:
            return build_recovery_message(run)
    return None


def post_slack(webhook_url: str, text: str, opener=urllib.request.urlopen) -> None:
    request = urllib.request.Request(
        webhook_url,
        data=json.dumps({"text": text}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with opener(request, timeout=15) as response:
        if response.status >= 300:
            raise RuntimeError(f"Slack webhook returned HTTP {response.status}")


def main() -> int:
    webhook = os.environ.get("SLACK_WEBHOOK_URL", "")
    if not webhook:
        print("::notice::SLACK_WEBHOOK_URL is not set; skipping Slack notification")
        return 0

    with open(os.environ["GITHUB_EVENT_PATH"], encoding="utf-8") as handle:
        event = json.load(handle)
    client = GitHubClient(token=os.environ.get("GITHUB_TOKEN"))
    text = decide_message(
        os.environ["GITHUB_EVENT_NAME"], event, client, os.environ["GITHUB_REPOSITORY"]
    )
    if text is None:
        print("Nothing to report")
        return 0
    post_slack(webhook, text)
    print("Slack notification sent")
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 4: Run the whole suite**

Run: `python3 -m unittest discover -s scripts/ci-health -v 2>&1 | tail -8`
Expected: `Ran 44 tests` ... `OK` (a `::warning::could not list failed jobs` line is expected output of one test).

- [ ] **Step 5: Verify the missing-secret path**

```bash
env -u SLACK_WEBHOOK_URL python3 scripts/ci-health/notify.py; echo "exit=$?"
```

Expected: `::notice::SLACK_WEBHOOK_URL is not set; skipping Slack notification` and `exit=0`.

- [ ] **Step 6: Commit**

```bash
git add scripts/ci-health/notify.py scripts/ci-health/test_notify.py
git commit -m "feat: Add Slack notifier for CI/CD failures and recoveries (#16)"
```

---

### Task 4: Dashboard page

**Files:**
- Create: `docs/operations/ci-health.md`, `docs/operations/ci-health/dashboard.js`, `docs/operations/ci-health/dashboard.css`
- Modify: `mkdocs.yml` (nav, add `extra_css`/`extra_javascript`), `.gitignore`

**Interfaces:**
- Consumes: the `data.json` shape produced by `collect.collect` (Task 2) at `docs/operations/ci-health/data.json`, fetched relative to the page URL (`/operations/ci-health/`).
- Produces: the page `operations/ci-health/` on the docs site.

- [ ] **Step 1: Create the renderer**

Create `docs/operations/ci-health/dashboard.js`:

```javascript
/* CI/CD health dashboard: renders data.json (see scripts/ci-health/collect.py)
 * into #ci-health-root. Uses textContent only; no HTML is built from data. */
(function () {
  "use strict";

  var STALE_HOURS = 48;

  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined && text !== null) node.textContent = text;
    return node;
  }

  function isNum(value) {
    return typeof value === "number" && isFinite(value);
  }

  function pct(rate) {
    return isNum(rate) ? (rate * 100).toFixed(1) + "%" : "n/a";
  }

  function hrs(value) {
    return isNum(value) ? value.toFixed(1) + " h" : "n/a";
  }

  function tile(label, value, note) {
    var node = el("div", "ci-health-tile");
    node.appendChild(el("div", "ci-health-tile-label", label));
    node.appendChild(el("div", "ci-health-tile-value", value));
    node.appendChild(el("div", "ci-health-tile-note", note || ""));
    return node;
  }

  function table(headers, rows) {
    var t = el("table");
    var head = el("tr");
    headers.forEach(function (h) { head.appendChild(el("th", null, h)); });
    t.appendChild(el("thead")).appendChild(head);
    var body = el("tbody");
    rows.forEach(function (cells) {
      var tr = el("tr");
      cells.forEach(function (cell) { tr.appendChild(el("td", null, cell)); });
      body.appendChild(tr);
    });
    t.appendChild(body);
    return t;
  }

  function section(title, content) {
    var wrap = el("section", "ci-health-section");
    wrap.appendChild(el("h2", null, title));
    wrap.appendChild(content);
    return wrap;
  }

  function unavailable(text) {
    return el("p", "ci-health-unavailable", text || "Unavailable");
  }

  function ratesTable(firstHeader, rates) {
    var rows = Object.keys(rates).map(function (name) {
      var r = rates[name];
      return [name, pct(r.rate), String(r.success), String(r.failed)];
    });
    return table([firstHeader, "Success rate", "Passed", "Failed"], rows);
  }

  function aggregate(rates) {
    var success = 0, failed = 0;
    Object.keys(rates).forEach(function (name) {
      success += rates[name].success;
      failed += rates[name].failed;
    });
    var total = success + failed;
    return { rate: total ? success / total : null, total: total };
  }

  function render(root, data) {
    root.textContent = "";
    var errors = data.errors || {};

    var generated = new Date(data.generated_at);
    var ageHours = (Date.now() - generated.getTime()) / 36e5;
    var meta = "Last updated " + generated.toUTCString() + " · window: last " +
      (data.window ? data.window.days : "?") + " days";
    root.appendChild(el("p", "ci-health-meta", meta));
    if (isNum(ageHours) && ageHours > STALE_HOURS) {
      root.appendChild(el("p", "ci-health-stale",
        "This data is more than " + STALE_HOURS + " hours old; the daily refresh may be failing."));
    }

    var tiles = el("div", "ci-health-tiles");
    var wf = data.workflows;
    tiles.appendChild(wf
      ? tile("Workflow success rate", pct(wf.overall.rate), wf.overall.total + " runs")
      : tile("Workflow success rate", "n/a", errors.workflows || "unavailable"));

    var tj = data.test_jobs;
    if (tj) {
      var agg = aggregate(tj);
      tiles.appendChild(tile("Test pass rate", pct(agg.rate), agg.total + " job runs"));
    } else {
      tiles.appendChild(tile("Test pass rate", "n/a", errors.test_jobs || "unavailable"));
    }

    var dep = data.dependabot;
    tiles.appendChild(dep
      ? tile("Dependabot PRs", dep.opened + " opened / " + dep.merged + " merged",
          "median time to merge " + hrs(dep.median_hours_to_merge))
      : tile("Dependabot PRs", "n/a", errors.dependabot || "unavailable"));

    var rev = data.review;
    var first = rev ? rev.first_review_hours : null;
    if (rev) {
      tiles.appendChild(tile("Time to first review",
        first ? hrs(first.median) : "n/a",
        first ? "p90 " + hrs(first.p90) + " · n = " + first.n : "n = " + rev.sample + " PRs"));
    } else {
      tiles.appendChild(tile("Time to first review", "n/a", errors.review || "unavailable"));
    }
    root.appendChild(tiles);

    root.appendChild(section("Workflows",
      wf ? ratesTable("Workflow", wf.by_workflow) : unavailable(errors.workflows)));
    root.appendChild(section("Test jobs",
      tj ? ratesTable("Job", tj) : unavailable(errors.test_jobs)));

    if (dep) {
      var weekly = dep.weekly.map(function (w) {
        return [w.week, String(w.opened), String(w.merged)];
      });
      root.appendChild(section("Dependabot per week",
        weekly.length ? table(["Week", "Opened", "Merged"], weekly)
                      : unavailable("No Dependabot activity in this window")));
    } else {
      root.appendChild(section("Dependabot per week", unavailable(errors.dependabot)));
    }

    if (rev) {
      var m = rev.merge_hours;
      root.appendChild(section("Review turnaround",
        table(["Measure", "Median", "p90", "n"], [
          ["Open to first review",
            first ? hrs(first.median) : "n/a", first ? hrs(first.p90) : "n/a", first ? String(first.n) : "0"],
          ["Open to merge",
            m ? hrs(m.median) : "n/a", m ? hrs(m.p90) : "n/a", m ? String(m.n) : "0"]
        ])));
    } else {
      root.appendChild(section("Review turnaround", unavailable(errors.review)));
    }
  }

  function load(root) {
    root.textContent = "Loading…";
    fetch(root.getAttribute("data-src") || "data.json", { cache: "no-store" })
      .then(function (response) {
        if (!response.ok) throw new Error("HTTP " + response.status);
        return response.json();
      })
      .then(function (data) { render(root, data); })
      .catch(function () {
        root.textContent = "";
        root.appendChild(unavailable(
          "CI/CD health data is unavailable. It is generated by the docs build; " +
          "it will not exist when serving the docs locally."));
      });
  }

  function init() {
    var root = document.getElementById("ci-health-root");
    if (root) load(root);
  }

  // Material's instant navigation swaps pages without reloading scripts.
  if (typeof document$ !== "undefined") {
    document$.subscribe(init);
  } else if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
```

- [ ] **Step 2: Create the styles**

Create `docs/operations/ci-health/dashboard.css`:

```css
.ci-health-tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
  gap: 0.8rem;
  margin: 1rem 0 1.5rem;
}

.ci-health-tile {
  border: 0.05rem solid var(--md-default-fg-color--lightest);
  border-radius: 0.2rem;
  padding: 0.8rem 1rem;
}

.ci-health-tile-label {
  font-size: 0.7rem;
  color: var(--md-default-fg-color--light);
  text-transform: uppercase;
}

.ci-health-tile-value {
  font-size: 1.4rem;
  font-weight: 700;
  margin: 0.2rem 0;
}

.ci-health-tile-note,
.ci-health-meta,
.ci-health-unavailable {
  font-size: 0.75rem;
  color: var(--md-default-fg-color--light);
}

.ci-health-stale {
  border-left: 0.2rem solid var(--md-code-hl-color, #ffb300);
  padding-left: 0.6rem;
}
```

- [ ] **Step 3: Create the page**

Create `docs/operations/ci-health.md`:

```markdown
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
```

- [ ] **Step 4: Wire it into mkdocs and ignore generated data**

In `mkdocs.yml`, add an Operations nav section between `Configuration` and `Development`:

```yaml
  - Configuration:
      - Environment Variables: configuration/env-vars.md
      - Reconcilers: configuration/reconcilers.md
  - Operations:
      - CI/CD Health: operations/ci-health.md
  - Development:
```

Add these top-level keys immediately before `markdown_extensions:`:

```yaml
extra_css:
  - operations/ci-health/dashboard.css

extra_javascript:
  - operations/ci-health/dashboard.js

```

Append to `.gitignore` (after the `# Claude` block):

```
# Generated CI/CD health data (built by scripts/ci-health/collect.py)
docs/operations/ci-health/data.json
```

- [ ] **Step 5: Verify the renderer and the docs build**

```bash
node --check docs/operations/ci-health/dashboard.js
python3 -m venv /tmp/muto-docs-venv && /tmp/muto-docs-venv/bin/pip install -q -r docs-requirements.txt
GITHUB_TOKEN="$(gh auth token)" GITHUB_REPOSITORY=muto-io/muto python3 scripts/ci-health/collect.py
/tmp/muto-docs-venv/bin/mkdocs build -q
ls site/operations/ci-health/ site/operations/ci-health/index.html
git status --short
```

Expected: `node --check` prints nothing; the build succeeds; `site/operations/ci-health/` contains `index.html`, `dashboard.js`, `dashboard.css` and `data.json`; `git status` does not list `data.json` or `site/` (both ignored — if `site/` shows up, add it to `.gitignore` too).

Then view it in a browser:

```bash
python3 -m http.server 8000 --directory site
```

Open `http://localhost:8000/operations/ci-health/`. Expected: four tiles, tables with real numbers, no console errors, and "n/a" (not `NaN`) for any metric without data. Stop the server. Repeat once with `rm site/operations/ci-health/data.json` and reload: the page shows the "unavailable" message.

- [ ] **Step 6: Commit**

```bash
git add docs/operations/ci-health.md docs/operations/ci-health/dashboard.js docs/operations/ci-health/dashboard.css mkdocs.yml .gitignore
git commit -m "feat: Add CI/CD health dashboard page (#16)"
```

---

### Task 5: Workflows

**Files:**
- Create: `.github/workflows/ci-failure-notify.yaml`
- Modify: `.github/workflows/ci.yml` (append a job), `.github/workflows/pages.yml`

**Interfaces:**
- Consumes: `scripts/ci-health/notify.py` CLI (Task 3), `scripts/ci-health/collect.py` CLI (Task 2), `python -m unittest discover -s scripts/ci-health`.
- Produces: the Slack notification workflow, a CI job for the script tests, and a daily docs build that refreshes the dashboard.

- [ ] **Step 1: Create the notify workflow**

Create `.github/workflows/ci-failure-notify.yaml`:

```yaml
name: CI Failure Notify

on:
  workflow_run:
    workflows:
      - CI
      - E2E Tests
      - Release
      - Build and Deploy GitHub Pages
    types: [completed]
  workflow_dispatch:
    inputs:
      message:
        description: Test message to post to Slack (verifies the webhook wiring)
        required: false
        default: "CI/CD health notifications: test message"

permissions:
  contents: read
  actions: read

jobs:
  notify:
    name: Notify Slack
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - name: Send notification
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK_URL }}
        run: python3 scripts/ci-health/notify.py
```

`GITHUB_REPOSITORY`, `GITHUB_EVENT_NAME` and `GITHUB_EVENT_PATH` are provided by the runner. The script reads the commit message and branch from the event file, so no untrusted value reaches the shell.

- [ ] **Step 2: Add the script-test job to `ci.yml`**

Append after the `helm-lint` job at the end of `.github/workflows/ci.yml`:

```yaml

  ci-health-test:
    name: CI Health Script Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-python@v7
        with:
          python-version: '3.11'
      - name: Run unit tests
        run: python -m unittest discover -s scripts/ci-health -v
```

- [ ] **Step 3: Update `pages.yml`**

(a) Triggers — replace the `on:` block and add a daily schedule, manual trigger and the script path filter:

```yaml
on:
  push:
    branches: [main]
    paths:
      - 'docs/**'
      - 'scripts/ci-health/**'
      - '.github/workflows/pages.yml'
      - 'README.md'
      - 'CONTRIBUTING.md'
  pull_request:
    branches: [main]
    paths:
      - 'docs/**'
      - 'scripts/ci-health/**'
      - '.github/workflows/pages.yml'
  schedule:
    # Daily refresh of the CI/CD health dashboard
    - cron: '0 6 * * *'
  workflow_dispatch:
```

(b) Permissions — the collector reads workflow runs and pull requests:

```yaml
permissions:
  contents: read
  actions: read
  pull-requests: read
  pages: write
  id-token: write
```

(c) In the `build` job, insert this step between "Install dependencies" and "Build documentation":

```yaml
      - name: Collect CI/CD health metrics
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: python scripts/ci-health/collect.py
```

(d) In the `deploy` job, replace the `if:` so scheduled and manual runs on `main` deploy too:

```yaml
    if: github.ref == 'refs/heads/main' && github.event_name != 'pull_request'
```

- [ ] **Step 4: Validate the YAML**

```bash
python3 - <<'EOF'
import sys
try:
    import yaml
except ImportError:
    sys.exit("PyYAML not installed; use: /tmp/muto-docs-venv/bin/pip install pyyaml and rerun with that python")
for f in ["ci-failure-notify.yaml", "ci.yml", "pages.yml"]:
    d = yaml.safe_load(open(f".github/workflows/{f}"))
    print(f, "ok", list(d["jobs"]))
EOF
```

Expected: three `ok` lines (`ci-failure-notify.yaml ok ['notify']`, `ci.yml ok [... 'ci-health-test']`, `pages.yml ok ['build', 'deploy']`). If PyYAML is missing, run the same snippet with `/tmp/muto-docs-venv/bin/python` after `pip install pyyaml`.

Also confirm the script tests still pass the exact way CI runs them:

```bash
python3 -m unittest discover -s scripts/ci-health -v 2>&1 | tail -4
```

Expected: `OK`.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci-failure-notify.yaml .github/workflows/ci.yml .github/workflows/pages.yml
git commit -m "ci: Add Slack failure notifications and daily CI/CD health refresh (#16)"
```

---

### Task 6: Documentation and end-to-end verification

**Files:**
- Modify: `docs/operations/monitoring-observability.md` (insert a section before `## Known Limitations`)

- [ ] **Step 1: Add the CI/CD health section**

Insert immediately before the `## Known Limitations` heading in `docs/operations/monitoring-observability.md`:

````markdown
## CI/CD Health

Pipeline health is tracked separately from the operator's runtime metrics. The [CI/CD Health dashboard](ci-health.md) shows workflow success rate, test job pass rate, Dependabot activity and review turnaround over the last 30 days. It is regenerated daily (and on every docs build) by `scripts/ci-health/collect.py`.

Failures are posted to Slack by the `CI Failure Notify` workflow (`.github/workflows/ci-failure-notify.yaml`). It reacts to failed or timed-out runs of `CI`, `E2E Tests`, `Release` and the Pages build on `main`, scheduled runs and release tags (pull-request failures are visible on the PR and are not posted), and sends one "recovered" message when a failing workflow goes green again.

To enable notifications:

1. Create a Slack incoming webhook for the target channel.
2. Add it as the `SLACK_WEBHOOK_URL` repository secret (Settings → Secrets and variables → Actions).
3. Verify the wiring: Actions → `CI Failure Notify` → Run workflow. A test message should appear in the channel.

Without the secret the workflow logs a notice and succeeds, so forks are unaffected. The workflow is only triggered by other workflows' runs once it exists on `main`.
````

- [ ] **Step 2: Verify the anchor and the build**

```bash
/tmp/muto-docs-venv/bin/mkdocs build -q
grep -c 'id="cicd-health"' site/operations/monitoring-observability/index.html
```

Expected: build succeeds; the grep prints `1` (the anchor `monitoring-observability.md#cicd-health` used by the dashboard page resolves). If it prints `0`, fix the link in `docs/operations/ci-health.md` to the generated id.

- [ ] **Step 3: Final checks**

```bash
python3 -m unittest discover -s scripts/ci-health 2>&1 | tail -3
git status --short
git log --oneline -8
```

Expected: tests `OK`; clean tree (generated `data.json` and `site/` are ignored); one commit per task.

- [ ] **Step 4: Commit**

```bash
git add docs/operations/monitoring-observability.md
git commit -m "docs: Document CI/CD health dashboard and Slack alerts (#16)"
```

- [ ] **Step 5: Post-merge manual verification (maintainer)**

These cannot be checked before merge:

1. Add the `SLACK_WEBHOOK_URL` secret, then run `CI Failure Notify` via *Run workflow*; confirm the test message arrives.
2. Run `Build and Deploy GitHub Pages` via *Run workflow* on `main`; confirm `https://muto-io.github.io/muto/operations/ci-health/` shows current data.
3. Optionally confirm alerting end to end by temporarily failing a scheduled `E2E Tests` run or reviewing the next real failure.
