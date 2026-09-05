#!/usr/bin/env python3
"""Contract tests for bounded backend test summaries and retained diagnostics."""

from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
BACKEND_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "backend-tests.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"


def step_block(workflow: str, name: str) -> str:
    marker = f"      - name: {name}\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"backend-tests.yml has no {name!r} step")
    return remainder.partition("\n      - name: ")[0]


class BackendTestsWorkflowContractTest(unittest.TestCase):
    def test_generated_report_is_redirected_then_published_with_a_bound(self) -> None:
        workflow = BACKEND_WORKFLOW.read_text(encoding="utf-8")
        checkout = step_block(workflow, "Checkout Go test reporter")
        generate = step_block(workflow, "Generate test report")
        publish = step_block(workflow, "Publish bounded test report summary")

        temporary_summary = (
            "${{ runner.temp }}/backend-test-summary-${{ matrix.shard }}.md"
        )
        self.assertIn(
            "uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10",
            checkout,
        )
        self.assertIn("repository: robherley/go-test-action", checkout)
        self.assertIn(
            "ref: 2f859e0c8769d755d3174eecb9af8f64660827f3",
            checkout,
        )
        self.assertIn("path: .github/vendor/go-test-action", checkout)
        self.assertIn(f'report_summary="{temporary_summary}"', generate)
        self.assertIn(': > "$report_summary"', generate)
        self.assertIn('GITHUB_STEP_SUMMARY="$report_summary"', generate)
        self.assertIn(
            'INPUT_FROMJSONFILE="$GITHUB_WORKSPACE/apps/backend/'
            'test-results-${{ matrix.shard }}.json"',
            generate,
        )
        self.assertIn(
            'INPUT_MODULEDIRECTORY="$GITHUB_WORKSPACE/apps/backend"',
            generate,
        )
        self.assertIn('INPUT_OMIT="successful"', generate)
        self.assertIn(
            'node ".github/vendor/go-test-action/dist/index.js"',
            generate,
        )
        self.assertNotIn("uses: robherley/go-test-action", generate)
        self.assertIn("if: always()", publish)
        self.assertIn("python3 .github/scripts/bounded-step-summary.py", publish)
        self.assertIn(f'--input "{temporary_summary}"', publish)
        self.assertIn('--output "$GITHUB_STEP_SUMMARY"', publish)
        self.assertIn("--diagnostics-label", publish)
        self.assertIn("backend-test-results-${{ matrix.shard }}", publish)
        self.assertIn("${{ github.run_id }}#artifacts", publish)

    def test_full_json_diagnostics_remain_in_the_named_artifact(self) -> None:
        workflow = BACKEND_WORKFLOW.read_text(encoding="utf-8")
        upload = step_block(workflow, "Upload test artifacts")

        self.assertIn("if: always()", upload)
        self.assertIn("name: backend-test-results-${{ matrix.shard }}", upload)
        self.assertIn("apps/backend/test-results-${{ matrix.shard }}.json", upload)

    def test_summary_writer_changes_run_the_backend_workflow(self) -> None:
        workflow = BACKEND_WORKFLOW.read_text(encoding="utf-8")
        detect = step_block(workflow, "Detect relevant changes")

        self.assertIn(".github/scripts/bounded-step-summary.py", detect)

    def test_bounded_summary_contract_runs_in_ci(self) -> None:
        workflow = LINT_WORKFLOW.read_text(encoding="utf-8")

        self.assertIn(
            "python3 .github/scripts/bounded-step-summary_test.py",
            workflow,
        )
        self.assertIn(
            "python3 .github/scripts/backend-tests-workflow-contract_test.py",
            workflow,
        )


if __name__ == "__main__":
    unittest.main()
