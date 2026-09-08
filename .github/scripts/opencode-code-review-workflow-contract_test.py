#!/usr/bin/env python3
"""Contract tests for the OpenCode fork-review workflow."""

from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "opencode-code-review.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"


class OpenCodeCodeReviewWorkflowContractTest(unittest.TestCase):
    def test_safe_to_review_approval_survives_follow_up_pushes(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        _, separator, fork_job = workflow.partition("  opencode-review-fork:")

        self.assertTrue(separator, "OpenCode fork-review job is missing")
        self.assertIn("github.event_name == 'pull_request_target'", fork_job)
        self.assertIn(
            "github.event.pull_request.head.repo.full_name != github.repository",
            fork_job,
        )
        self.assertIn(
            "((contains(github.event.pull_request.labels.*.name, 'safe-to-review')) ||",
            fork_job,
        )
        self.assertNotIn(
            "github.event.action != 'synchronize' && contains("
            "github.event.pull_request.labels.*.name, 'safe-to-review')",
            fork_job,
        )

    def test_workflow_has_no_synchronize_time_label_cleanup(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")

        self.assertNotIn("  strip-safe-to-review:", workflow)
        self.assertNotIn("safe-to-test", workflow)
        self.assertNotIn("github.rest.issues.removeLabel", workflow)

    def test_lint_workflow_runs_opencode_contract_test(self) -> None:
        workflow = LINT_WORKFLOW.read_text(encoding="utf-8")

        self.assertIn(
            "python3 .github/scripts/opencode-code-review-workflow-contract_test.py",
            workflow,
        )


if __name__ == "__main__":
    unittest.main()
