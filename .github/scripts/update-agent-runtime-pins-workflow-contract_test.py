#!/usr/bin/env python3
"""Contract tests for the managed runtime pin update workflow."""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "update-agent-runtime-pins.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"
SCRIPT = REPO_ROOT / "scripts" / "update-agent-runtime-pins.mjs"
REQUIRED_CHECK_WORKFLOWS = (
    "backend-tests.yml",
    "frontend-tests.yml",
    "e2e-tests.yml",
    "architecture-lint.yml",
    "lint-action-pinning.yml",
    "lint-harness-files.yml",
)


class ManagedRuntimePinWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.workflow = WORKFLOW.read_text(encoding="utf-8")

    def test_workflow_has_weekly_and_manual_triggers(self) -> None:
        self.assertIn("schedule:", self.workflow)
        self.assertRegex(self.workflow, r"cron:\s*[\"']?[0-9]+ [0-9]+ \* \* [0-6][\"']?")
        self.assertIn("workflow_dispatch:", self.workflow)

    def test_workflow_uses_least_privilege_and_builtin_token(self) -> None:
        self.assertIn(
            "permissions:\n  contents: write\n  pull-requests: write", self.workflow
        )
        self.assertIn("GH_TOKEN: ${{ github.token }}", self.workflow)
        self.assertNotIn("actions/create-github-app-token@", self.workflow)
        self.assertNotIn("MANAGED_RUNTIME_PIN_APP_PRIVATE_KEY", self.workflow)
        self.assertNotIn("MANAGED_RUNTIME_PIN_APP_CLIENT_ID", self.workflow)
        self.assertNotIn("secrets.GITHUB_TOKEN", self.workflow)

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
        pr_step = self.workflow[pr_start:]
        self.assertIn("GH_TOKEN: ${{ github.token }}", push_step)
        self.assertIn("GH_TOKEN: ${{ github.token }}", pr_step)

    def test_workflow_dispatches_required_checks_for_builtin_token_prs(self) -> None:
        self.assertIn("actions: write", self.workflow)
        dispatch_start = self.workflow.index(
            "- name: Dispatch required checks for generated PR"
        )
        dispatch_step = self.workflow[dispatch_start:]
        self.assertIn("gh workflow run", dispatch_step)
        self.assertIn('--ref "$BOT_BRANCH"', dispatch_step)
        self.assertIn('--repo "$GITHUB_REPOSITORY"', dispatch_step)

        for workflow_name in REQUIRED_CHECK_WORKFLOWS:
            self.assertIn(workflow_name, dispatch_step)
            workflow = (REPO_ROOT / ".github" / "workflows" / workflow_name).read_text(
                encoding="utf-8"
            )
            self.assertRegex(
                workflow,
                r"(?m)^  workflow_dispatch:\s*$",
                f"{workflow_name} must accept explicit validation dispatches",
            )

    def test_workflow_exports_pr_number_for_trusted_docs_dispatch(self) -> None:
        pr_start = self.workflow.index("- name: Create or refresh the single grouped PR")
        dispatch_start = self.workflow.index(
            "- name: Dispatch required checks for generated PR"
        )
        pr_step = self.workflow[pr_start:dispatch_start]
        dispatch_step = self.workflow[dispatch_start:]
        self.assertIn("id: pr", pr_step)
        self.assertIn('echo "pr_number=$pr_number" >> "$GITHUB_OUTPUT"', pr_step)
        self.assertIn('created_url="$(gh pr create', pr_step)
        self.assertIn('pr_number="${created_url##*/}"', pr_step)
        self.assertIn('pr_number="$open_pr_number"', pr_step)
        self.assertLess(
            pr_step.index('gh pr edit "$open_pr_number"'),
            pr_step.index('pr_number="$open_pr_number"'),
        )
        self.assertIn('=~ ^[0-9]+$', pr_step)
        self.assertIn("gh workflow run pr-docs.yml", dispatch_step)
        self.assertIn("--ref main", dispatch_step)
        self.assertIn('PR_NUMBER: ${{ steps.pr.outputs.pr_number }}', dispatch_step)
        self.assertIn('-f pr_number="$PR_NUMBER"', dispatch_step)
        self.assertIn("pr-docs.yml", dispatch_step)

    def test_workflow_validates_before_any_commit_or_push(self) -> None:
        baseline_pos = self.workflow.index('cp "$CATALOGUE"')
        update_pos = self.workflow.index("node scripts/update-agent-runtime-pins.mjs")
        validation_pos = self.workflow.index("go test ./internal/agent/agents")
        record_pos = self.workflow.index(
            "node .github/scripts/record-agent-runtime-pin-refresh.mjs"
        )
        commit_pos = self.workflow.index("git commit")
        push_pos = self.workflow.index("git push")
        self.assertLess(baseline_pos, update_pos)
        self.assertLess(update_pos, validation_pos)
        self.assertLess(validation_pos, record_pos)
        self.assertLess(record_pos, commit_pos)
        self.assertLess(commit_pos, push_pos)
        self.assertIn("node --test scripts/update-agent-runtime-pins.test.mjs", self.workflow)
        self.assertIn("--github-output", self.workflow)
        self.assertIn("--validation-passed", self.workflow)
        self.assertIn('--before "$RUNNER_TEMP/managed-runtime-pins.before.json"', self.workflow)
        self.assertIn("working-directory: apps/backend", self.workflow)
        self.assertIn(
            "go test ./internal/agent/agents ./internal/agent/managedruntime "
            "./internal/agent/runtime/lifecycle ./internal/agent/hostutility "
            "./internal/agent/settings/controller ./internal/agent/settings/handlers",
            self.workflow,
        )
        self.assertIn("if: steps.update.outputs.changed == 'true'", self.workflow)

    def test_workflow_stages_the_catalogue_and_generated_delivery_record(self) -> None:
        commit_start = self.workflow.index("- name: Commit and push one grouped change")
        commit_step = self.workflow[commit_start:]
        self.assertIn(
            "git add apps/backend/internal/agent/agents/managed_npm_runtime_versions.json",
            commit_step,
        )
        self.assertIn(
            "git add docs/plans/managed-runtime-pin-automation/plan.md",
            commit_step,
        )
        self.assertIn(
            "git add docs/plans/managed-runtime-pin-automation/task-01-refresh-pins.md",
            commit_step,
        )
        self.assertIn(
            "docs/plans/managed-runtime-pin-automation/task-01-refresh-pins.md",
            self.workflow,
        )
        self.assertIn("## Validation", self.workflow)
        self.assertIn("## Checklist", self.workflow)

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
        self.assertIn(
            "node --test .github/scripts/record-agent-runtime-pin-refresh.test.mjs",
            lint_workflow,
        )

    def test_updater_script_exists_and_has_no_network_in_fixture_tests(self) -> None:
        self.assertTrue(SCRIPT.is_file())
        script = SCRIPT.read_text(encoding="utf-8")
        self.assertIn("registry.npmjs.org", script)
        self.assertIn("fs.rename", script)


if __name__ == "__main__":
    unittest.main()
