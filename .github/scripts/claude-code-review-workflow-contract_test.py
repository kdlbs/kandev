#!/usr/bin/env python3
"""Contract tests for the Claude Code fork-review workflow."""

from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "claude-code-review.yml"
ALLOWED_USERS_INPUT = "allowed_non_write_users: ${{ github.event.pull_request.user.login }}"


class ClaudeCodeReviewWorkflowContractTest(unittest.TestCase):
    def test_fork_review_forwards_allowlist_to_claude_action(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        _, separator, fork_job = workflow.partition("  claude-review-fork:")

        self.assertTrue(separator, "Claude fork-review job is missing")
        self.assertIn(
            ALLOWED_USERS_INPUT,
            fork_job,
            "fork review must forward its job-authorized pull request author "
            "to Claude's allowed_non_write_users input",
        )


if __name__ == "__main__":
    unittest.main()
