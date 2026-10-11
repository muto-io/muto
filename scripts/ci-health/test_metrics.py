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
