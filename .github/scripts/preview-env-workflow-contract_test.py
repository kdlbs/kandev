#!/usr/bin/env python3
"""Contract tests for the fork preview authorization workflow."""

from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "preview-env.yml"


class PreviewEnvironmentWorkflowContractTest(unittest.TestCase):
    def test_claude_allowlist_authorizes_fork_preview_deployment(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        _, separator, deploy_job = workflow.partition("  deploy-fork:")

        self.assertTrue(separator, "Fork preview deploy job is missing")
        self.assertIn("github.event_name == 'pull_request_target'", deploy_job)
        self.assertIn(
            "github.event.pull_request.head.repo.full_name != github.repository",
            deploy_job,
        )
        self.assertIn(
            "contains(github.event.pull_request.labels.*.name, 'safe-to-test')",
            deploy_job,
        )
        self.assertIn("vars.PREVIEW_ENV_ALLOWLIST != ''", deploy_job)
        self.assertIn("vars.CLAUDE_REVIEW_ALLOWLIST != ''", deploy_job)
        self.assertIn(
            "contains(fromJSON(vars.CLAUDE_REVIEW_ALLOWLIST), github.event.pull_request.user.login)",
            deploy_job,
        )


if __name__ == "__main__":
    unittest.main()
