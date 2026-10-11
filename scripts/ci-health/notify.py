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
        if not metrics.is_health_run(candidate):
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
