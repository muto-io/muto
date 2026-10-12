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


    def test_pr_missing_created_at_nulls_only_dependabot_and_review(self):
        bad_prs = [{"number": 9, "user": {"login": "alice"}, "merged_at": None}] + PRS
        client = FakeClient(data(**{f"/repos/{REPO}/pulls": bad_prs}))
        result = collect.collect(client, REPO, NOW)
        self.assertIsNone(result["dependabot"])
        self.assertIsNone(result["review"])
        self.assertIn("dependabot", result["errors"])
        self.assertIn("review", result["errors"])
        self.assertIsNotNone(result["workflows"])
        self.assertIsNotNone(result["test_jobs"])

    def test_run_missing_name_nulls_workflows_but_not_other_sections(self):
        bad_runs = [{"id": 9, "conclusion": "failure", "head_branch": "main", "event": "push"}] + RUNS
        client = FakeClient(data(**{f"/repos/{REPO}/actions/runs": bad_runs}))
        result = collect.collect(client, REPO, NOW)
        self.assertIsNone(result["workflows"])
        self.assertIn("workflows", result["errors"])
        self.assertIsNotNone(result["test_jobs"])
        self.assertEqual(result["dependabot"]["opened"], 1)
        self.assertIsNotNone(result["review"])


if __name__ == "__main__":
    unittest.main()
