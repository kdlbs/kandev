#!/usr/bin/env python3
"""Contract tests for the pull request documentation coverage workflow."""

from __future__ import annotations

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "pr-docs.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"


class PullRequestDocumentationWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.assertTrue(WORKFLOW.is_file(), "Documentation coverage workflow is missing")
        self.workflow = WORKFLOW.read_text(encoding="utf-8")

    # @covers AC-CI-PR-DOCS-001.1, AC-CI-PR-DOCS-002.2
    def test_runs_for_all_pr_and_manual_retry_events(self) -> None:
        trigger = self.workflow.partition("on:\n")[2].partition("\nconcurrency:")[0]
        self.assertIn("pull_request_target:", trigger)
        for event in (
            "opened",
            "reopened",
            "synchronize",
            "edited",
            "labeled",
            "unlabeled",
            "ready_for_review",
        ):
            self.assertIn(event, trigger)
        self.assertIn("merge_group:", trigger)
        self.assertIn("checks_requested", trigger)
        self.assertIn("workflow_dispatch:", trigger)
        self.assertIn("pr_number:", trigger)
        self.assertNotIn("paths:", trigger)

    # @covers AC-CI-PR-DOCS-003.1, AC-CI-PR-DOCS-003.4
    def test_serializes_prs_without_cancelling_merge_group_runs(self) -> None:
        self.assertIn("group: >-", self.workflow)
        self.assertIn("github.event.merge_group.base_ref", self.workflow)
        self.assertIn("github.event.pull_request.base.ref", self.workflow)
        self.assertIn("queue: max", self.workflow)
        self.assertIn("cancel-in-progress: false", self.workflow)

    # @covers AC-CI-PR-DOCS-003.3
    def test_uses_trusted_least_privilege_checkout(self) -> None:
        permissions = self.workflow.partition("permissions:\n")[2].partition("\njobs:")[0]
        self.assertEqual(
            permissions.strip(),
            "contents: read\n  pull-requests: read\n  statuses: write",
        )
        self.assertRegex(
            self.workflow,
            r"uses: actions/checkout@[0-9a-f]{40} # v[0-9]+",
        )
        self.assertIn("persist-credentials: false", self.workflow)
        self.assertIn("github.event.merge_group.base_sha", self.workflow)
        self.assertNotIn("github.event.pull_request.head.sha", self.workflow)
        self.assertIn("ref: ${{ env.CHECKOUT_REF }}", self.workflow)
        self.assertNotIn("pull_request:\n", self.workflow)
        self.assertNotIn("pnpm install", self.workflow)
        self.assertNotIn("npm install", self.workflow)
        self.assertIn("node .github/scripts/pr-docs.cjs", self.workflow)
        self.assertIn("name: Publish PR documentation coverage status", self.workflow)
        self.assertNotIn("name: PR documentation coverage\n", self.workflow)

    # @covers AC-CI-PR-DOCS-001.1, AC-CI-PR-DOCS-002.1, AC-CI-PR-DOCS-003.2
    def test_dispatch_is_restricted_to_the_default_branch_and_script_owns_statuses(self) -> None:
        self.assertRegex(
            self.workflow,
            re.compile(
                r"github\.event_name != 'workflow_dispatch'.*github\.ref.*default_branch",
                re.DOTALL,
            ),
        )
        self.assertIn("PR documentation coverage", (REPO_ROOT / ".github" / "scripts" / "pr-docs.cjs").read_text(encoding="utf-8"))
        self.assertIn("statuses: write", self.workflow)

    def test_contract_is_registered_in_the_required_lint_workflow(self) -> None:
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")
        self.assertRegex(
            lint_workflow,
            r"(?m)^      - name: Test pull request documentation workflow contract$\n"
            r"^        run: python3 .github/scripts/pr-docs-workflow-contract_test.py$",
        )
        self.assertRegex(
            lint_workflow,
            r"(?m)^      - name: Test pull request documentation validator$\n"
            r"^        run: node --test .github/scripts/pr-docs.test.cjs$",
        )
        makefile = (REPO_ROOT / "Makefile").read_text(encoding="utf-8")
        self.assertIn("@node --test .github/scripts/pr-docs.test.cjs", makefile)


if __name__ == "__main__":
    unittest.main()
