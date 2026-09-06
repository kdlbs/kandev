#!/usr/bin/env python3
"""Contract tests for external runner planner wiring and protected jobs."""

from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_ROOT = REPO_ROOT / ".github" / "workflows"
PLAN_SCRIPT = REPO_ROOT / ".github" / "scripts" / "runner-plan.py"


def job_block(workflow: str, job: str, next_job: str | None) -> str:
    marker = f"  {job}:\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"Workflow has no {job} job")
    if next_job is None:
        return remainder
    return remainder.partition(f"\n  {next_job}:\n")[0]


class ExternalRunnerWorkflowContractTest(unittest.TestCase):
    def assert_planner(
        self, workflow_name: str, workflow_key: str, families: tuple[str, ...]
    ) -> str:
        workflow = (WORKFLOW_ROOT / workflow_name).read_text(encoding="utf-8")
        planner = job_block(workflow, "runner_plan", "changes")
        self.assertIn("runs-on: ubuntu-latest", planner)
        self.assertIn("plan: ${{ steps.plan.outputs.plan }}", planner)
        self.assertIn("uses: ./.github/actions/plan-external-runners", planner)
        self.assertIn(f"workflow: {workflow_key}", planner)
        self.assertIn("families: >-", planner)
        self.assertIn("run-id: ${{ github.run_id }}", planner)
        self.assertIn("burst: ${{ vars.KANDEV_CI_EXTERNAL_ENABLED }}", planner)
        self.assertIn("percent: ${{ vars.KANDEV_CI_EXTERNAL_PERCENT }}", planner)
        self.assertIn("light-label: ${{ vars.KANDEV_CI_RUNNER_LIGHT }}", planner)
        self.assertIn("standard-label: ${{ vars.KANDEV_CI_RUNNER_STANDARD }}", planner)
        for family in families:
            self.assertIn(f'"name":"{family}"', planner)
        return workflow

    def test_e2e_uses_planner_for_eligible_jobs_and_keeps_protected_jobs_hosted(self) -> None:
        workflow = self.assert_planner(
            "e2e-tests.yml",
            "e2e",
            ("changes", "e2e", "e2e_gate"),
        )
        self.assertIn(
            "runs-on: ${{ fromJSON(needs.runner_plan.outputs.plan).changes_runner }}",
            job_block(workflow, "changes", "build"),
        )
        build = job_block(workflow, "build", "e2e")
        self.assertIn("runs-on: ubuntu-latest", build)
        self.assertNotIn("runner_plan", build)
        e2e = job_block(workflow, "e2e", "playwright_image")
        self.assertIn(
            "matrix: ${{ fromJSON(needs.runner_plan.outputs.plan).e2e_matrix }}",
            e2e,
        )
        self.assertIn("runs-on: ${{ matrix.runner }}", e2e)
        report = job_block(workflow, "e2e-report", "e2e-gate")
        self.assertIn("runs-on: ubuntu-latest", report)
        self.assertNotIn("runner_plan", report)
        self.assertIn(
            "runs-on: ${{ fromJSON(needs.runner_plan.outputs.plan).e2e_gate_runner }}",
            job_block(workflow, "e2e-gate", None),
        )
        for job, next_job in (
            ("playwright_image", "e2e-containers"),
            ("e2e-containers", "e2e-kubernetes-compatibility"),
            ("e2e-kubernetes-compatibility", "desktop-e2e"),
            ("desktop-e2e", "e2e-report"),
        ):
            protected = job_block(workflow, job, next_job)
            self.assertIn("runs-on: ubuntu-latest", protected)
            self.assertNotIn("runner_plan", protected)
            self.assertNotIn("KANDEV_CI_EXTERNAL", protected)

    def test_backend_uses_planner_and_keeps_services_and_windows_hosted(self) -> None:
        workflow = self.assert_planner(
            "backend-tests.yml",
            "backend",
            ("test",),
        )
        for job, next_job in (
            ("changes", "static_checks"),
            ("static_checks", "test_shards"),
            ("test_ambient_env", "test"),
        ):
            protected = job_block(workflow, job, next_job)
            self.assertIn("runs-on: ubuntu-latest", protected)
            self.assertNotIn("runner_plan", protected)
        shards = job_block(workflow, "test_shards", "test_ambient_env")
        self.assertIn("- shard: 1", shards)
        self.assertIn("- shard: 2", shards)
        self.assertIn("runs-on: ubuntu-latest", shards)
        self.assertNotIn("runner_plan", shards)
        self.assertNotIn("matrix.runner", shards)
        test_gate = job_block(workflow, "test", "postgres-boot")
        self.assertIn(
            "runs-on: ${{ fromJSON(needs.runner_plan.outputs.plan).test_runner }}",
            test_gate,
        )
        for job, next_job in (("postgres-boot", "test-windows"), ("test-windows", None)):
            protected = job_block(workflow, job, next_job)
            self.assertIn("runs-on: ubuntu-latest" if job == "postgres-boot" else "runs-on: windows-latest", protected)
            self.assertNotIn("runner_plan", protected)

    def test_frontend_uses_planner_for_test_and_gate(self) -> None:
        workflow = self.assert_planner(
            "frontend-tests.yml",
            "frontend",
            ("changes", "frontend", "frontend_gate"),
        )
        self.assertIn(
            "runs-on: ${{ fromJSON(needs.runner_plan.outputs.plan).changes_runner }}",
            job_block(workflow, "changes", "frontend"),
        )
        self.assertIn(
            "runs-on: ${{ fromJSON(needs.runner_plan.outputs.plan).frontend_runner }}",
            job_block(workflow, "frontend", "frontend-gate"),
        )
        self.assertIn(
            "runs-on: ${{ fromJSON(needs.runner_plan.outputs.plan).frontend_gate_runner }}",
            job_block(workflow, "frontend-gate", None),
        )

    def test_protected_lint_workflows_do_not_run_the_planner(self) -> None:
        for workflow_name in (
            "architecture-lint.yml",
            "lint-action-pinning.yml",
            "lint-harness-files.yml",
        ):
            workflow = (WORKFLOW_ROOT / workflow_name).read_text(encoding="utf-8")
            lint = job_block(workflow, "lint", None)
            self.assertNotIn("runner_plan:", workflow)
            self.assertNotIn("name: Require runner plan", lint)
            self.assertIn("runs-on: ubuntu-latest", lint)

    def test_plan_script_is_checked_by_required_action_pinning_workflow(self) -> None:
        workflow = (WORKFLOW_ROOT / "lint-action-pinning.yml").read_text(encoding="utf-8")
        self.assertTrue(PLAN_SCRIPT.is_file())
        action = (WORKFLOW_ROOT / ".." / "actions" / "plan-external-runners" / "action.yml").resolve()
        self.assertTrue(action.is_file())
        action_text = action.read_text(encoding="utf-8")
        self.assertIn("using: composite", action_text)
        self.assertIn("families:", action_text)
        self.assertIn("plan:", action_text)
        self.assertIn("runner-plan.py", action_text)
        self.assertNotIn("changes_runner:", action_text)
        self.assertIn("python3 .github/scripts/runner-plan_test.py", workflow)
        self.assertIn("python3 .github/scripts/external-runner-workflow-contract_test.py", workflow)


if __name__ == "__main__":
    unittest.main()
