#!/usr/bin/env python3
"""Contract tests for the stale merge-approval revocation workflow."""

from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "revoke-ready-to-merge.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"


class RevokeReadyToMergeWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.assertTrue(WORKFLOW.is_file(), "Revocation workflow is missing")
        self.workflow = WORKFLOW.read_text(encoding="utf-8")

    # @covers AC-CI-MERGE-APPROVAL-001.1
    def test_workflow_only_handles_pull_request_synchronize_events(self) -> None:
        self.assertIn(
            "on:\n  pull_request_target:\n    types: [synchronize]\n",
            self.workflow,
        )
        trigger = self.workflow.partition("on:\n")[2].partition("\nconcurrency:")[0]
        self.assertNotIn("\n  pull_request:\n", trigger)

    # @covers AC-CI-MERGE-APPROVAL-001.1
    def test_removes_only_the_exact_merge_approval_label(self) -> None:
        self.assertIn("const approvalLabel = 'ready-to-merge';", self.workflow)
        self.assertIn("eventPullRequest?.labels", self.workflow)
        self.assertIn(
            "label => label?.name === approvalLabel",
            self.workflow,
        )
        self.assertIn("github.rest.issues.removeLabel", self.workflow)
        self.assertIn("name: approvalLabel", self.workflow)
        self.assertNotIn("safe-to-review", self.workflow)

    # @covers AC-CI-MERGE-APPROVAL-001.2
    def test_reads_and_independently_cleans_active_merge_states(self) -> None:
        self.assertIn("autoMergeRequest { id }", self.workflow)
        self.assertIn("mergeQueueEntry { id }", self.workflow)
        self.assertIn("disablePullRequestAutoMerge", self.workflow)
        self.assertIn("dequeuePullRequest", self.workflow)
        self.assertIn("pullRequestId: $id", self.workflow)
        self.assertIn("input: { id: $id }", self.workflow)
        self.assertIn("currentPullRequest.autoMergeRequest?.id", self.workflow)
        self.assertIn("currentPullRequest.mergeQueueEntry?.id", self.workflow)
        self.assertIn("attemptCleanup(\n                'auto-merge',", self.workflow)
        self.assertIn("attemptCleanup(\n                'merge queue',", self.workflow)

    # @covers AC-CI-MERGE-APPROVAL-001.3
    def test_event_label_snapshot_gates_all_cleanup(self) -> None:
        gate = self.workflow.index("if (!hasApprovalLabel)")
        remove_label = self.workflow.index("github.rest.issues.removeLabel")

        self.assertLess(gate, remove_label)
        self.assertIn("return;", self.workflow[gate:remove_label])
        self.assertIn("hasApprovalLabel", self.workflow)

    # @covers AC-CI-MERGE-APPROVAL-001.4
    def test_write_and_admin_pushers_return_before_cleanup(self) -> None:
        self.assertIn("context.payload.sender?.login", self.workflow)
        self.assertIn(
            "github.rest.repos.getCollaboratorPermissionLevel",
            self.workflow,
        )
        self.assertIn(
            "permission === 'write' || permission === 'admin'",
            self.workflow,
        )
        trusted_gate = self.workflow.index(
            "permission === 'write' || permission === 'admin'"
        )
        remove_label = self.workflow.index("github.rest.issues.removeLabel")
        self.assertIn("return;", self.workflow[trusted_gate:remove_label])

    # @covers AC-CI-MERGE-APPROVAL-001.5
    def test_permission_lookup_fails_closed_and_reports_failures(self) -> None:
        self.assertIn("permissionError", self.workflow)
        self.assertIn("permission = response.data.permission", self.workflow)
        self.assertIn("permissionError = new Error", self.workflow)
        self.assertIn("core.warning", self.workflow)
        self.assertIn("core.setFailed", self.workflow)
        self.assertIn("!senderLogin", self.workflow)

    # @covers AC-CI-MERGE-APPROVAL-001.6
    def test_cleanup_is_idempotent_but_unexpected_errors_are_failures(self) -> None:
        self.assertIn("function isExpectedAlreadyCleanError", self.workflow)
        self.assertIn("status === 404", self.workflow)
        self.assertIn("Label does not exist", self.workflow)
        self.assertIn("Pull request is not set to auto-merge", self.workflow)
        self.assertIn("Pull request is not in the merge queue", self.workflow)
        self.assertIn("if (isExpectedAlreadyCleanError", self.workflow)
        self.assertIn("failures.push", self.workflow)

    # @covers AC-CI-MERGE-APPROVAL-001.6
    def test_concurrency_serializes_each_pull_request_without_cancellation(self) -> None:
        self.assertRegex(
            self.workflow,
            r"group: revoke-ready-to-merge-\$\{\{ github\.event\.pull_request\.number \}\}",
        )
        self.assertIn("cancel-in-progress: false", self.workflow)

    # @covers AC-CI-MERGE-APPROVAL-001.7
    def test_workflow_has_trusted_least_privilege_and_pinned_execution(self) -> None:
        permissions = self.workflow.partition("permissions:\n")[2].partition("\njobs:")[0]
        self.assertEqual("pull-requests: write", permissions.strip())
        self.assertNotIn("actions/checkout", self.workflow)
        self.assertNotIn("\n      - run:", self.workflow)
        self.assertNotIn("child_process", self.workflow)
        self.assertNotIn("eval(", self.workflow)
        self.assertRegex(
            self.workflow,
            r"uses: actions/github-script@[0-9a-f]{40} # v9\.[0-9]+\.[0-9]+",
        )

    def test_contract_is_registered_in_the_required_lint_workflow(self) -> None:
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")

        self.assertIn(
            "python3 .github/scripts/revoke-ready-to-merge-workflow-contract_test.py",
            lint_workflow,
        )


if __name__ == "__main__":
    unittest.main()
