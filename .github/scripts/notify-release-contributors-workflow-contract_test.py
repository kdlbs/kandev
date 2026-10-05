#!/usr/bin/env python3
"""Contract tests for manual and reusable contributor notification workflows."""

import re
from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_PATH = REPO_ROOT / ".github" / "workflows" / "notify-release-contributors.yml"
LINT_WORKFLOW_PATH = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"
WORKFLOW = WORKFLOW_PATH.read_text() if WORKFLOW_PATH.exists() else ""
LINT_WORKFLOW = LINT_WORKFLOW_PATH.read_text()


def event_block(name: str) -> str:
    marker = f"  {name}:"
    start = WORKFLOW.find(marker)
    if start == -1:
        return ""
    following = re.search(r"\n  (?:workflow_dispatch|workflow_call|permissions|jobs):", WORKFLOW[start + 1 :])
    end = len(WORKFLOW) if following is None else start + 1 + following.start()
    return WORKFLOW[start:end]


def job_block(name: str) -> str:
    marker = f"  {name}:"
    start = WORKFLOW.find(marker, WORKFLOW.find("jobs:"))
    if start == -1:
        return ""
    following = re.search(r"\n  [a-zA-Z0-9_-]+:\n", WORKFLOW[start + 1 :])
    end = len(WORKFLOW) if following is None else start + 1 + following.start()
    return WORKFLOW[start:end]


class NotifyReleaseContributorsWorkflowContractTest(unittest.TestCase):
    def test_manual_and_reusable_entry_points_have_matching_optional_inputs(self) -> None:
        dispatch = event_block("workflow_dispatch")
        reusable = event_block("workflow_call")
        self.assertTrue(dispatch, "workflow_dispatch trigger is missing")
        self.assertTrue(reusable, "workflow_call trigger is missing")

        for entry, block in (("manual", dispatch), ("reusable", reusable)):
            with self.subTest(entry=entry):
                self.assertRegex(block, r"(?m)^\s+release_tag:\n")
                self.assertRegex(block, r"(?m)^\s+dry_run:\n")
                self.assertRegex(block, r"(?m)^\s+type: string$")
                self.assertRegex(block, r"(?m)^\s+type: boolean$")
                self.assertRegex(block, r'(?m)^\s+default: ""$')
                self.assertRegex(block, r"(?m)^\s+default: false$")
        self.assertIn("latest published Stable release", dispatch)

    def test_posting_job_has_minimum_permissions_and_shared_non_cancelling_lock(self) -> None:
        job = job_block("notify")
        self.assertTrue(job, "notification job is missing")
        self.assertIn("contents: read", job)
        self.assertIn("pull-requests: write", job)
        self.assertNotIn("issues: write", job)
        self.assertIn("group: release-contributor-notifications-${{ github.repository }}", job)
        self.assertIn("cancel-in-progress: false", job)
        self.assertIn("queue: max", job)

    def test_helper_checkout_uses_the_workflow_revision_without_saved_credentials(self) -> None:
        job = job_block("notify")
        self.assertIn("repository: ${{ github.repository }}", job)
        self.assertIn("ref: ${{ github.workflow_sha }}", job)
        self.assertIn("fetch-depth: 0", job)
        self.assertIn("persist-credentials: false", job)
        self.assertIn("GITHUB_WORKFLOW_REF: ${{ github.workflow_ref }}", job)
        self.assertIn("GITHUB_WORKFLOW_SHA: ${{ github.workflow_sha }}", job)
        self.assertIn("HELPER_CHECKOUT_SHA: ${{ env.HELPER_CHECKOUT_SHA }}", job)

    def test_job_passes_inputs_to_the_helper_and_ci_runs_all_contract_tests(self) -> None:
        job = job_block("notify")
        self.assertIn("GH_TOKEN: ${{ github.token }}", job)
        self.assertIn("RELEASE_TAG: ${{ inputs.release_tag }}", job)
        self.assertIn("DRY_RUN: ${{ inputs.dry_run }}", job)
        self.assertIn("python3 .github/scripts/notify-release-contributors.py", job)

        self.assertIn("python3 .github/scripts/notify-release-contributors_test.py", LINT_WORKFLOW)
        self.assertIn(
            "python3 .github/scripts/notify-release-contributors-workflow-contract_test.py",
            LINT_WORKFLOW,
        )
        self.assertIn("python3 .github/scripts/release-workflow-contract_test.py", LINT_WORKFLOW)


if __name__ == "__main__":
    unittest.main()
