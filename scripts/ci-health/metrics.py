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
