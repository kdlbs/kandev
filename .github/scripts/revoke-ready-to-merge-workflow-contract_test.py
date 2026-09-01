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
        self.assertEqual(
            trigger,
            "  pull_request_target:\n    types: [synchronize]\n",
        )

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

        attempt_cleanup_start = self.workflow.index("async function attemptCleanup")
        attempt_cleanup_end = self.workflow.index("async function finish")
        attempt_cleanup = self.workflow[attempt_cleanup_start:attempt_cleanup_end]
        self.assertIn("catch (error)", attempt_cleanup)
        self.assertIn("recordFailure(`${operation} cleanup`, error)", attempt_cleanup)
        self.assertNotIn("throw ", attempt_cleanup)

        auto_cleanup = self.workflow.index(
            "await attemptCleanup(\n                'auto-merge',"
        )
        queue_cleanup = self.workflow.index(
            "await attemptCleanup(\n                'merge queue',"
        )
        self.assertLess(auto_cleanup, queue_cleanup)

        auto_merge_mutation = self.workflow.index("disablePullRequestAutoMerge")
        auto_merge_block = self.workflow[auto_merge_mutation - 200 : auto_merge_mutation + 300]
        self.assertIn("{ id: currentPullRequest.id }", auto_merge_block)

        queue_mutation = self.workflow.index("dequeuePullRequest")
        queue_block = self.workflow[queue_mutation - 200 : queue_mutation + 300]
        self.assertIn("{ id: currentPullRequest.id }", queue_block)

    # @covers AC-CI-MERGE-APPROVAL-001.3
    def test_event_label_snapshot_gates_all_cleanup(self) -> None:
        gate = self.workflow.index("if (!hasApprovalLabel)")
        write_gate = self.workflow.index(
            "if (permission === 'write' || permission === 'admin')"
        )

        for cleanup in (
            "github.rest.issues.removeLabel",
            "disablePullRequestAutoMerge",
            "dequeuePullRequest",
        ):
            cleanup_index = self.workflow.index(cleanup)
            self.assertLess(write_gate, cleanup_index)
            self.assertLess(gate, cleanup_index)

        remove_label = self.workflow.index("github.rest.issues.removeLabel")
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
        permission_error = self.workflow.index("if (permissionError)")
        remove_label = self.workflow.index("github.rest.issues.removeLabel")
        permission_failure_branch = self.workflow[permission_error:trusted_gate]

        self.assertLess(permission_error, trusted_gate)
        self.assertNotIn("return;", permission_failure_branch)
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

        expected_branch = self.workflow.index(
            "if (isExpectedAlreadyCleanError(operation, error))"
        )
        expected_outcome = self.workflow.index(
            "recordResult(operation, 'already clean; concurrent cleanup won')",
            expected_branch,
        )
        expected_return = self.workflow.index("return;", expected_outcome)
        unexpected_failure = self.workflow.index(
            "recordFailure(`${operation} cleanup`, error)",
            expected_branch,
        )

        self.assertLess(expected_branch, expected_outcome)
        self.assertLess(expected_outcome, expected_return)
        self.assertLess(expected_return, unexpected_failure)

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

        self.assertRegex(
            lint_workflow,
            r"(?m)^      - name: Test stale merge approval revocation workflow contract$\n"
            r"^        run: python3 .github/scripts/revoke-ready-to-merge-workflow-contract_test.py$",
        )


if __name__ == "__main__":
    unittest.main()
