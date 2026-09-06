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
    def assert_planner(self, workflow_name: str, workflow_key: str, outputs: tuple[str, ...]) -> str:
        workflow = (WORKFLOW_ROOT / workflow_name).read_text(encoding="utf-8")
        planner = job_block(workflow, "runner_plan", "changes" if "changes:" in workflow else "lint")
        self.assertIn("runs-on: ubuntu-latest", planner)
        self.assertIn("python3 .github/scripts/runner-plan.py", planner)
        self.assertIn(f"--workflow {workflow_key}", planner)
        self.assertIn("KANDEV_CI_EXTERNAL_ENABLED", planner)
        self.assertIn("KANDEV_CI_EXTERNAL_PERCENT", planner)
        for output in outputs:
            self.assertIn(f"{output}: ${{{{ steps.plan.outputs.{output} }}}}", planner)
        return workflow

    def test_e2e_uses_planner_for_eligible_jobs_and_keeps_protected_jobs_hosted(self) -> None:
        workflow = self.assert_planner(
            "e2e-tests.yml",
            "e2e",
            (
                "changes_runner",
                "build_runner",
                "e2e_matrix",
                "e2e_report_runner",
                "e2e_gate_runner",
            ),
        )
        self.assertIn("runs-on: ${{ needs.runner_plan.outputs.changes_runner == 'external'", job_block(workflow, "changes", "build"))
        self.assertIn("runs-on: ${{ needs.runner_plan.outputs.build_runner == 'external'", job_block(workflow, "build", "e2e"))
        e2e = job_block(workflow, "e2e", "playwright_image")
        self.assertIn("matrix: ${{ fromJSON(needs.runner_plan.outputs.e2e_matrix) }}", e2e)
        self.assertIn("runs-on: ${{ matrix.runner == 'external'", e2e)
        self.assertIn("runs-on: ${{ needs.runner_plan.outputs.e2e_report_runner == 'external'", job_block(workflow, "e2e-report", "e2e-gate"))
        self.assertIn("runs-on: ${{ needs.runner_plan.outputs.e2e_gate_runner == 'external'", job_block(workflow, "e2e-gate", None))
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
            (
                "changes_runner",
                "static_checks_runner",
                "backend_test_matrix",
                "test_ambient_env_runner",
                "test_runner",
            ),
        )
        for job, next_job, output in (
            ("changes", "static_checks", "changes_runner"),
            ("static_checks", "test_shards", "static_checks_runner"),
            ("test_ambient_env", "test", "test_ambient_env_runner"),
            ("test", "postgres-boot", "test_runner"),
        ):
            self.assertIn(
                f"runs-on: ${{{{ needs.runner_plan.outputs.{output} == 'external'",
                job_block(workflow, job, next_job),
            )
        shards = job_block(workflow, "test_shards", "test_ambient_env")
        self.assertIn("matrix: ${{ fromJSON(needs.runner_plan.outputs.backend_test_matrix) }}", shards)
        self.assertIn("runs-on: ${{ matrix.runner == 'external'", shards)
        for job, next_job in (("postgres-boot", "test-windows"), ("test-windows", None)):
            protected = job_block(workflow, job, next_job)
            self.assertIn("runs-on: ubuntu-latest" if job == "postgres-boot" else "runs-on: windows-latest", protected)
            self.assertNotIn("runner_plan", protected)

    def test_frontend_uses_planner_for_test_and_gate(self) -> None:
        workflow = self.assert_planner(
            "frontend-tests.yml",
            "frontend",
            ("changes_runner", "frontend_runner", "frontend_gate_runner"),
        )
        self.assertIn("runs-on: ${{ needs.runner_plan.outputs.changes_runner == 'external'", job_block(workflow, "changes", "frontend"))
        self.assertIn("runs-on: ${{ needs.runner_plan.outputs.frontend_runner == 'external'", job_block(workflow, "frontend", "frontend-gate"))
        self.assertIn("runs-on: ${{ needs.runner_plan.outputs.frontend_gate_runner == 'external'", job_block(workflow, "frontend-gate", None))

    def test_short_lint_workflows_use_light_planner_output(self) -> None:
        for workflow_name, workflow_key in (
            ("architecture-lint.yml", "architecture"),
            ("lint-action-pinning.yml", "action-pinning"),
            ("lint-harness-files.yml", "harness-lint"),
        ):
            workflow = self.assert_planner(workflow_name, workflow_key, ("lint_runner",))
            lint = job_block(workflow, "lint", None)
            self.assertIn("if: always()", lint)
            self.assertIn("name: Require runner plan", lint)
            self.assertIn("runs-on: ${{ needs.runner_plan.outputs.lint_runner == 'external'", lint)

    def test_plan_script_is_checked_by_required_action_pinning_workflow(self) -> None:
        workflow = (WORKFLOW_ROOT / "lint-action-pinning.yml").read_text(encoding="utf-8")
        self.assertTrue(PLAN_SCRIPT.is_file())
        self.assertIn("python3 .github/scripts/runner-plan_test.py", workflow)
        self.assertIn("python3 .github/scripts/external-runner-workflow-contract_test.py", workflow)


if __name__ == "__main__":
    unittest.main()
