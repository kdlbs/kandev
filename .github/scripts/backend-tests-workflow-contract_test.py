#!/usr/bin/env python3
"""Contract tests for the fixed persistence gates in backend-tests.yml."""

from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "backend-tests.yml"
BASE_IMAGE_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "ci-base-image.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"

POSTGRES_18_DIGEST = "sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280"


class BackendTestsWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.workflow = WORKFLOW.read_text(encoding="utf-8")
        self.base_image_workflow = BASE_IMAGE_WORKFLOW.read_text(encoding="utf-8")

    def test_postgres_16_uses_fixed_catalog_commands(self) -> None:
        self.assertNotIn(
            "PostgresDSNFromEnv",
            self.workflow,
            "PostgreSQL CI must not discover packages by searching for a test helper.",
        )
        self.assertNotIn(
            "mapfile -t postgres_packages",
            self.workflow,
            "PostgreSQL CI must use fixed package and test commands.",
        )
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

    def test_base_image_mirrors_postgres_18_at_the_same_digest(self) -> None:
        self.assertIn("POSTGRES_18_DIGEST: " + POSTGRES_18_DIGEST, self.base_image_workflow)
        self.assertIn("POSTGRES_18_SOURCE: docker.io/library/postgres:18@" + POSTGRES_18_DIGEST, self.base_image_workflow)
        self.assertIn("POSTGRES_18_TARGET: ghcr.io/kdlbs/kandev-ci:postgres-18", self.base_image_workflow)
        self.assertIn("docker buildx imagetools create", self.base_image_workflow)
        self.assertIn('--tag "${POSTGRES_18_TARGET}"', self.base_image_workflow)

    def test_contract_is_registered_in_the_unfiltered_lint_workflow(self) -> None:
        self.assertIn(
            "python3 .github/scripts/backend-tests-workflow-contract_test.py",
            LINT_WORKFLOW.read_text(encoding="utf-8"),
        )


if __name__ == "__main__":
    unittest.main()
