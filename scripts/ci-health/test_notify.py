import json
import os
import tempfile
import unittest
from unittest import mock

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
    def __init__(self, jobs=None, previous_runs=None, jobs_error=None, attempt_runs=None):
        self.jobs = jobs or []
        self.previous_runs = previous_runs or []
        self.jobs_error = jobs_error
        self.attempt_runs = attempt_runs or {}
        self.get_paths = []

    def paginate(self, path, params=None, key=None, max_pages=20):
        if self.jobs_error:
            raise self.jobs_error
        return iter(self.jobs)

    def get(self, path, params=None):
        self.get_paths.append(path)
        if path in self.attempt_runs:
            result = self.attempt_runs[path]
            if isinstance(result, Exception):
                raise result
            return result
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
            {"id": 99, "conclusion": "failure", "created_at": "2026-10-11T09:00:00Z",
             "event": "push", "head_branch": "main"},
        ]
        event = {"workflow_run": run(conclusion="success")}
        text = self.decide("workflow_run", event, FakeClient(previous_runs=previous))
        self.assertIn("recovered", text)

    def test_recovery_ignores_failed_fork_pr_run_on_branch_named_main(self):
        previous = [
            {"id": 99, "conclusion": "failure", "created_at": "2026-10-11T09:00:00Z",
             "event": "pull_request", "head_branch": "main"},
        ]
        event = {"workflow_run": run(conclusion="success")}
        self.assertIsNone(self.decide("workflow_run", event, FakeClient(previous_runs=previous)))

    def test_success_after_success_is_silent(self):
        previous = [{"id": 99, "conclusion": "success", "created_at": "2026-10-11T09:00:00Z",
                     "event": "push", "head_branch": "main"}]
        event = {"workflow_run": run(conclusion="success")}
        self.assertIsNone(self.decide("workflow_run", event, FakeClient(previous_runs=previous)))

    def test_first_ever_run_success_is_silent(self):
        event = {"workflow_run": run(conclusion="success")}
        self.assertIsNone(self.decide("workflow_run", event, FakeClient(previous_runs=[])))

    def test_recovery_skips_cancelled_runs_and_the_run_itself(self):
        previous = [
            {"id": 100, "conclusion": "success", "created_at": "2026-10-11T10:00:00Z",
             "event": "push", "head_branch": "main"},
            {"id": 99, "conclusion": "cancelled", "created_at": "2026-10-11T09:30:00Z",
             "event": "push", "head_branch": "main"},
            {"id": 98, "conclusion": "failure", "created_at": "2026-10-11T09:00:00Z",
             "event": "push", "head_branch": "main"},
        ]
        event = {"workflow_run": run(conclusion="success")}
        text = self.decide("workflow_run", event, FakeClient(previous_runs=previous))
        self.assertIn("recovered", text)

    def test_rerun_passing_after_failed_attempt_sends_recovery(self):
        path = f"/repos/{REPO}/actions/runs/100/attempts/1"
        client = FakeClient(attempt_runs={path: {"conclusion": "failure"}})
        event = {"workflow_run": run(conclusion="success", run_attempt=2)}
        text = self.decide("workflow_run", event, client)
        self.assertIn("recovered", text)

    def test_rerun_after_successful_attempt_is_silent(self):
        path = f"/repos/{REPO}/actions/runs/100/attempts/1"
        client = FakeClient(attempt_runs={path: {"conclusion": "success"}})
        event = {"workflow_run": run(conclusion="success", run_attempt=2)}
        self.assertIsNone(self.decide("workflow_run", event, client))

    def test_first_attempt_never_queries_attempts(self):
        client = FakeClient()
        event = {"workflow_run": run(conclusion="success", run_attempt=1)}
        self.assertIsNone(self.decide("workflow_run", event, client))
        self.assertFalse([p for p in client.get_paths if "/attempts/" in p])

    def test_attempt_lookup_failure_is_silent(self):
        path = f"/repos/{REPO}/actions/runs/100/attempts/1"
        client = FakeClient(attempt_runs={path: ghapi.GitHubError("HTTP 502")})
        event = {"workflow_run": run(conclusion="success", run_attempt=2)}
        self.assertIsNone(self.decide("workflow_run", event, client))

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
