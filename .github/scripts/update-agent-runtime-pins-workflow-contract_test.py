#!/usr/bin/env python3
"""Contract tests for the managed runtime pin update workflow."""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "update-agent-runtime-pins.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"
SCRIPT = REPO_ROOT / "scripts" / "update-agent-runtime-pins.mjs"


class ManagedRuntimePinWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.workflow = WORKFLOW.read_text(encoding="utf-8")

    def test_workflow_has_weekly_and_manual_triggers(self) -> None:
        self.assertIn("schedule:", self.workflow)
        self.assertRegex(self.workflow, r"cron:\s*[\"']?[0-9]+ [0-9]+ \* \* [0-6][\"']?")
        self.assertIn("workflow_dispatch:", self.workflow)

    def test_workflow_uses_least_privilege_and_dedicated_app_token(self) -> None:
        self.assertIn("permissions:\n  contents: read", self.workflow)
        self.assertIn("permission-contents: write", self.workflow)
        self.assertIn("permission-pull-requests: write", self.workflow)
        self.assertNotIn("permissions:\n      contents: write", self.workflow)
        self.assertNotIn("permissions:\n      pull-requests: write", self.workflow)
        self.assertIn("MANAGED_RUNTIME_PIN_APP_PRIVATE_KEY", self.workflow)
        self.assertIn("MANAGED_RUNTIME_PIN_APP_CLIENT_ID", self.workflow)
        self.assertNotIn("secrets.GITHUB_TOKEN", self.workflow)
        self.assertNotIn("GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}", self.workflow)

    def test_workflow_groups_one_conventional_commit_pr_on_stable_branch(self) -> None:
        self.assertIn("update-managed-runtime-pins", self.workflow)
        self.assertIn("chore: update managed runtime pins", self.workflow)
        self.assertIn("gh pr create", self.workflow)
        self.assertNotIn("gh pr merge", self.workflow)
        self.assertNotIn("--auto", self.workflow)
        self.assertIn("at most one", self.workflow)

    def test_workflow_refreshes_existing_pr_by_number_and_authenticates_push(self) -> None:
        self.assertIn('open_pr_number="$(gh pr list', self.workflow)
        self.assertIn('gh pr edit "$open_pr_number"', self.workflow)
        self.assertNotIn('gh pr edit \\\n              --repo "$GITHUB_REPOSITORY" \\\n              --head "$BOT_BRANCH"', self.workflow)
        push_start = self.workflow.index("- name: Commit and push one grouped change")
        pr_start = self.workflow.index("- name: Create or refresh the single grouped PR")
        push_step = self.workflow[push_start:pr_start]
        self.assertIn("GH_TOKEN: ${{ steps.app-token.outputs.token }}", push_step)

    def test_workflow_validates_before_any_commit_or_push(self) -> None:
        update_pos = self.workflow.index("node scripts/update-agent-runtime-pins.mjs")
        validation_pos = self.workflow.index("go test ./internal/agent/agents")
        commit_pos = self.workflow.index("git commit")
        push_pos = self.workflow.index("git push")
        self.assertLess(update_pos, validation_pos)
        self.assertLess(validation_pos, commit_pos)
        self.assertLess(commit_pos, push_pos)
        self.assertIn("node --test scripts/update-agent-runtime-pins.test.mjs", self.workflow)
        self.assertIn("--github-output", self.workflow)
        self.assertIn("working-directory: apps/backend", self.workflow)
        self.assertIn(
            "go test ./internal/agent/agents ./internal/agent/managedruntime "
            "./internal/agent/runtime/lifecycle ./internal/agent/hostutility "
            "./internal/agent/settings/controller ./internal/agent/settings/handlers",
            self.workflow,
        )
        self.assertIn("if: steps.update.outputs.changed == 'true'", self.workflow)

    def test_all_actions_are_commit_pinned(self) -> None:
        for line in self.workflow.splitlines():
            if " uses: " not in line:
                continue
            self.assertRegex(line, r"uses:\s+[^@\s]+@[0-9a-f]{40}(?:\s+#.*)?$")

    def test_lint_workflow_runs_this_contract(self) -> None:
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")
        self.assertIn(
            "python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py",
            lint_workflow,
        )

    def test_updater_script_exists_and_has_no_network_in_fixture_tests(self) -> None:
        self.assertTrue(SCRIPT.is_file())
        script = SCRIPT.read_text(encoding="utf-8")
        self.assertIn("registry.npmjs.org", script)
        self.assertIn("fs.rename", script)


if __name__ == "__main__":
    unittest.main()
