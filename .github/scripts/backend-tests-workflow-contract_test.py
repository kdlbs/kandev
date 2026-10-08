#!/usr/bin/env python3
"""Contract tests for persistence gates and bounded backend test summaries."""

import re
from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "backend-tests.yml"
BACKEND_WORKFLOW = WORKFLOW
BASE_IMAGE_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "ci-base-image.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"

POSTGRES_18_DIGEST = "sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280"


def step_block(workflow: str, name: str) -> str:
    marker = f"      - name: {name}\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"backend-tests.yml has no {name!r} step")
    return remainder.partition("\n      - name: ")[0]


class BackendTestsWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.workflow = WORKFLOW.read_text(encoding="utf-8")
        self.base_image_workflow = BASE_IMAGE_WORKFLOW.read_text(encoding="utf-8")

    def test_postgres_16_uses_fixed_catalog_commands(self) -> None:
        self.assertNotIn("PostgresDSNFromEnv", self.workflow)
        self.assertNotIn("mapfile -t postgres_packages", self.workflow)
        persistence_gate = step_block(self.workflow, "Run fixed PostgreSQL 16 persistence gates")
        self.assertIn("./internal/coordinator", persistence_gate)
        self.assertIn("./internal/persistence/storeconformance ./internal/backendapp", self.workflow)
        for test_name in (
            "TestStoreCatalogCompleteness",
            "TestStoreConformance",
            "TestPreviousStableUpgrade",
            "TestPostgresBootInitializesRepositories",
        ):
            self.assertIn(test_name, self.workflow)

    def test_sql_guard_and_local_conformance_are_explicit(self) -> None:
        self.assertIn("go run ./cmd/sqlguard ./internal", self.workflow)
        self.assertIn("Run catalog and SQLite conformance gates", self.workflow)
        self.assertIn("TestUpgradeFixtureManifest", self.workflow)

    def test_postgres_18_is_a_required_pinned_gate(self) -> None:
        self.assertIn("postgres-18:", self.workflow)
        self.assertIn("postgres-18@" + POSTGRES_18_DIGEST, self.workflow)
        self.assertIn("POSTGRES_18_RESULT", self.workflow)
        self.assertIn('"postgres-18:${POSTGRES_18_RESULT}"', self.workflow)
        self.assertIn("TestPreviousStableUpgrade", self.workflow)

    def test_windows_job_has_headroom_for_hosted_runner_variance(self) -> None:
        _, marker, windows_job = self.workflow.partition("  test-windows:\n")
        self.assertTrue(marker)
        timeout = re.search(r"(?m)^\s*timeout-minutes:\s*(\d+)\s*$", windows_job)
        self.assertIsNotNone(timeout)
        self.assertGreaterEqual(int(timeout.group(1)), 90)
        self.assertIn("go test -race -timeout 25m ./cmd/windows-process-tests", windows_job)

    def test_windows_suites_run_independently_without_fail_fast(self) -> None:
        windows_job = self.workflow.partition("  test-windows:\n")[2]
        self.assertIn("suite: [process, native]", windows_job)
        self.assertIn("fail-fast: false", windows_job)
        self.assertIn("name: Backend (windows, ${{ matrix.suite }})", windows_job)
        self.assertNotIn("continue-on-error", windows_job)
        process = step_block(windows_job, "Test Windows process package")
        self.assertIn("if: matrix.suite == 'process'", process)
        self.assertIn("go run ./cmd/windows-process-tests", process)
        helper = step_block(windows_job, "Test Windows process cohort runner")
        self.assertIn("if: matrix.suite == 'process'", helper)
        self.assertIn("go test -race -timeout 25m ./cmd/windows-process-tests", helper)
        native = step_block(windows_job, "Test windows-sensitive packages")
        self.assertNotIn("./internal/agentctl/server/process/", native)
        for package in (
            "./internal/agent/runtime/agentctl/launcher/...",
            "./internal/common/ptyexec/...",
            "./internal/backendapp/ownershiplock/...",
            "./internal/system/storage/workspaces/...",
        ):
            self.assertIn(package, native)

    def test_windows_native_checks_remain_required_in_native_suite(self) -> None:
        windows_job = self.workflow.partition("  test-windows:\n")[2]
        for name in (
            "Test Windows isolated instance safety",
            "Test Windows copy-file glob tokenization",
            "Build", "Vet", "Test windows-sensitive packages",
            "Test Windows helper console behavior",
            "Test Windows managed runtime cache repair",
            "Test Windows port collision handling",
            "Test Windows path semantics",
            "Test Windows SQLite database paths",
            "Test Cursor project slug path normalization",
            "Test Windows agent-survival flag availability",
        ):
            with self.subTest(step=name):
                self.assertIn("if: matrix.suite == 'native'", step_block(windows_job, name))
        gate = self.workflow.partition("  test:\n")[2].partition("  postgres-boot:\n")[0]
        self.assertIn("- test-windows", gate)
        self.assertIn("TEST_WINDOWS_RESULT: ${{ needs['test-windows'].result }}", gate)
        self.assertIn('"test-windows:${TEST_WINDOWS_RESULT}"', gate)

    def test_windows_runtime_observation_shims_run_in_native_suite(self) -> None:
        step = step_block(self.workflow, "Test Windows runtime observation command shims")
        self.assertIn("if: matrix.suite == 'native'", step)
        self.assertIn("TestRuntimeObservationRunsWindowsCommandShimWithSpaces", step)

    def test_linux_sharded_tests_use_go_default_package_timeout(self) -> None:
        test_step = step_block(self.workflow, "Run tests")
        self.assertIn("go test \\", test_step)
        self.assertNotIn("-timeout", test_step)

    def test_base_image_mirrors_postgres_18_at_the_same_digest(self) -> None:
        self.assertIn("POSTGRES_18_DIGEST: " + POSTGRES_18_DIGEST, self.base_image_workflow)
        self.assertIn("POSTGRES_18_SOURCE: docker.io/library/postgres:18@" + POSTGRES_18_DIGEST, self.base_image_workflow)
        self.assertIn("POSTGRES_18_TARGET: ghcr.io/kdlbs/kandev-ci:postgres-18", self.base_image_workflow)
        self.assertIn("docker buildx imagetools create", self.base_image_workflow)
        self.assertIn('--tag "${POSTGRES_18_TARGET}"', self.base_image_workflow)

    def test_contract_is_registered_in_the_unfiltered_lint_workflow(self) -> None:
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")
        self.assertIn("python3 .github/scripts/backend-tests-workflow-contract_test.py", lint_workflow)
        self.assertIn("python3 .github/scripts/bounded-step-summary_test.py", lint_workflow)

    def test_generated_report_is_redirected_then_published_with_a_bound(self) -> None:
        checkout = step_block(self.workflow, "Checkout Go test reporter")
        generate = step_block(self.workflow, "Generate test report")
        publish = step_block(self.workflow, "Publish bounded test report summary")

        temporary_summary = "$RUNNER_TEMP/backend-test-summary-${BACKEND_TEST_SHARD}.md"
        self.assertIn("uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10", checkout)
        self.assertIn("repository: robherley/go-test-action", checkout)
        self.assertIn("ref: 2f859e0c8769d755d3174eecb9af8f64660827f3", checkout)
        self.assertIn("path: .github/vendor/go-test-action", checkout)
        self.assertIn(f'report_summary="{temporary_summary}"', generate)
        self.assertIn("BACKEND_TEST_SHARD: ${{ matrix.shard }}", generate)
        self.assertNotIn("${{ runner.temp }}", generate)
        self.assertIn(': > "$report_summary"', generate)
        self.assertIn('GITHUB_STEP_SUMMARY="$report_summary"', generate)
        self.assertIn('INPUT_FROMJSONFILE="$GITHUB_WORKSPACE/apps/backend/test-results-${BACKEND_TEST_SHARD}.json"', generate)
        self.assertIn('INPUT_MODULEDIRECTORY="$GITHUB_WORKSPACE/apps/backend"', generate)
        self.assertIn('INPUT_OMIT="successful"', generate)
        self.assertIn('node ".github/vendor/go-test-action/dist/index.js"', generate)
        self.assertNotIn("uses: robherley/go-test-action", generate)
        self.assertIn("if: always()", publish)
        self.assertIn("python3 .github/scripts/bounded-step-summary.py", publish)
        self.assertIn(f'--input "{temporary_summary}"', publish)
        self.assertIn('--output "$GITHUB_STEP_SUMMARY"', publish)
        self.assertIn("--diagnostics-label", publish)
        self.assertIn("backend-test-results-${BACKEND_TEST_SHARD}", publish)
        self.assertIn("$GITHUB_RUN_ID#artifacts", publish)

    def test_full_json_diagnostics_remain_in_the_named_artifact(self) -> None:
        upload = step_block(self.workflow, "Upload test artifacts")
        self.assertIn("if: always()", upload)
        self.assertIn("name: backend-test-results-${{ matrix.shard }}", upload)
        self.assertIn("apps/backend/test-results-${{ matrix.shard }}.json", upload)

    def test_summary_writer_changes_run_the_backend_workflow(self) -> None:
        detect = step_block(self.workflow, "Detect relevant changes")
        self.assertIn(".github/scripts/bounded-step-summary.py", detect)


if __name__ == "__main__":
    unittest.main()
