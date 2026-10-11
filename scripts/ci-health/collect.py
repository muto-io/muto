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
