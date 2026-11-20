#!/usr/bin/env python3
"""Contract tests for trusted PR screenshot-evidence evaluation."""

from __future__ import annotations

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "pr-ui-evidence.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"


class UIScreenshotEvidenceWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.assertTrue(WORKFLOW.is_file(), "UI screenshot evidence workflow is missing")
        self.workflow = WORKFLOW.read_text(encoding="utf-8")

    # @covers AC-CI-UI-SCREENSHOTS-003.1, AC-CI-UI-SCREENSHOTS-003.2
    def test_refreshes_screenshot_evidence_for_pr_and_manual_retry_events(self) -> None:
        trigger = self.workflow.partition("on:\n")[2].partition("\nconcurrency:")[0]
        self.assertIn("pull_request_target:", trigger)
        for event in ("opened", "reopened", "synchronize", "edited", "labeled", "unlabeled"):
            self.assertIn(event, trigger)
        self.assertIn("workflow_dispatch:", trigger)
        self.assertIn("pr_number:", trigger)
        self.assertNotIn("paths:", trigger)

    # @covers AC-CI-UI-SCREENSHOTS-002.1, AC-CI-UI-SCREENSHOTS-002.2
    def test_only_the_exact_exception_label_triggers_label_reevaluation(self) -> None:
        job = self.workflow.partition("jobs:\n")[2]
        self.assertIn("github.event.action != 'labeled'", job)
        self.assertIn("github.event.action != 'unlabeled'", job)
        self.assertIn("github.event.label.name == 'no-screenshots-allow'", job)
        self.assertLess(job.index("github.event.label.name"), job.index("runs-on:"))

    # @covers AC-CI-UI-SCREENSHOTS-001.3, AC-CI-UI-SCREENSHOTS-003.1
    def test_body_and_base_edits_are_checked_before_runner_admission(self) -> None:
        job = self.workflow.partition("jobs:\n")[2]
        self.assertIn("github.event.action != 'edited'", job)
        self.assertIn("github.event.changes.body != null", job)
        self.assertIn("github.event.changes.base != null", job)
        self.assertLess(job.index("github.event.changes.body"), job.index("runs-on:"))

    # @covers AC-CI-UI-SCREENSHOTS-003.3
    def test_uses_trusted_checkout_least_privilege_and_never_executes_the_pr(self) -> None:
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
        self.assertIn("TRUSTED_CHECKOUT_SHA: ${{ github.workflow_sha }}", self.workflow)
        self.assertIn("ref: ${{ env.TRUSTED_CHECKOUT_SHA }}", self.workflow)
        self.assertIn("github.ref == format('refs/heads/{0}', github.event.repository.default_branch)", self.workflow)
        self.assertNotIn("github.event.pull_request.head.sha", self.workflow)
        self.assertNotIn("pull_request:\n", self.workflow)
        self.assertNotRegex(self.workflow, r"(?m)^\s*(?:npm|pnpm|yarn) install\b")
        self.assertNotRegex(self.workflow, r"(?m)^\s*(?:git checkout|gh api)\b")
        self.assertIn("node .github/scripts/pr-ui-evidence.cjs", self.workflow)

    # @covers AC-CI-UI-SCREENSHOTS-003.1
    def test_serializes_evaluations_per_pr_without_cancelling_active_runs(self) -> None:
        self.assertRegex(self.workflow, r"group:.*pull_request.number.*inputs.pr_number")
        self.assertIn("cancel-in-progress: false", self.workflow)

    # @covers AC-CI-UI-SCREENSHOTS-003.1, AC-CI-UI-SCREENSHOTS-003.2
    def test_pending_relevant_event_survives_a_later_filtered_event(self) -> None:
        concurrency = self.workflow.partition("concurrency:\n")[2].partition("\npermissions:")[0]
        trigger = self.workflow.partition("on:\n")[2].partition("\nconcurrency:")[0]
        job = self.workflow.partition("jobs:\n")[2]

        # A body edit can queue behind an active evaluation, then be followed
        # by a title edit or unrelated label event for the same PR group.
        self.assertIn("edited", trigger)
        self.assertIn("labeled", trigger)
        self.assertIn("queue: max", concurrency)
        self.assertIn("cancel-in-progress: false", concurrency)
        self.assertLess(concurrency.index("queue: max"), concurrency.index("cancel-in-progress: false"))
        self.assertIn("github.event.changes.body != null", job)
        self.assertIn("github.event.label.name == 'no-screenshots-allow'", job)
        self.assertLess(self.workflow.index("concurrency:"), self.workflow.index("jobs:"))

    def test_contract_and_node_tests_are_registered_in_action_pinning_gate(self) -> None:
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")
        self.assertRegex(
            lint_workflow,
            r"(?m)^      - name: Test UI screenshot evidence workflow contract$\n"
            r"^        run: python3 .github/scripts/pr-ui-evidence-workflow-contract_test.py$",
        )
        self.assertRegex(
            lint_workflow,
            r"(?m)^      - name: Test UI screenshot evidence validator$\n"
            r"^        run: node --test .github/scripts/pr-ui-evidence.test.cjs$",
        )


if __name__ == "__main__":
    unittest.main()
