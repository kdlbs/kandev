"""CLI, ordering, annotation, and modular-layout tests."""

import os
import subprocess
import sys
import tempfile
from pathlib import Path

from support import ArchitectureFixture, LINTER, REPO_ROOT, RULES, RUNTIME_IMPORT


class CliTest(ArchitectureFixture):
    def test_findings_are_deterministic_and_untracked_files_are_ignored(self) -> None:
        z_path = "apps/backend/internal/zeta/service.go"
        a_path = "apps/backend/internal/alpha/service.go"
        self.write(z_path, f'package zeta\nimport "{RUNTIME_IMPORT}"\n')
        self.write(a_path, f'package alpha\nimport "{RUNTIME_IMPORT}"\n')
        self.track_all()
        self.write(
            "apps/backend/internal/aaa_untracked/service.go",
            f'package untracked\nimport "{RUNTIME_IMPORT}"\n',
        )

        first = self.run_cli("--all")
        second = self.run_cli("--all")

        self.assertEqual(first.returncode, 1)
        self.assertEqual(first.stdout, second.stdout)
        self.assertLess(first.stdout.index(a_path), first.stdout.index(z_path))
        self.assertNotIn("aaa_untracked", first.stdout)

    def test_cli_misuse_and_git_discovery_have_distinct_exit_codes(self) -> None:
        self.track_all()
        misuse = self.run_cli("--not-an-option")
        with tempfile.TemporaryDirectory() as tmp:
            git_failure = self.run_cli("--all", cwd=Path(tmp))

        self.assertEqual(misuse.returncode, 2)
        self.assertEqual(git_failure.returncode, 3)
        self.assertIn("git repository", git_failure.stderr)

    def test_github_actions_output_uses_error_annotations(self) -> None:
        path = "apps/backend/internal/example/new_service.go"
        self.write(path, f'package example\nimport "{RUNTIME_IMPORT}"\n')
        self.track_all()
        result = subprocess.run(
            [sys.executable, str(LINTER), "--all"],
            cwd=self.repo,
            env={**os.environ, "GITHUB_ACTIONS": "true"},
            text=True,
            capture_output=True,
            check=False,
        )

        self.assertEqual(result.returncode, 1)
        self.assertIn(f"::error file={path},line=2::", result.stdout)

    def test_each_rule_has_dedicated_implementation_test_and_baseline_files(self) -> None:
        for rule in RULES:
            with self.subTest(rule=rule.id):
                self.assertTrue(
                    (REPO_ROOT / "scripts/architecture_lint/rules" / f"{rule.slug}.py").is_file()
                )
                self.assertTrue(
                    (REPO_ROOT / "scripts/architecture_lint_tests" / f"test_{rule.slug}.py").is_file()
                )
                self.assertEqual(rule.baseline_path.parent, Path("config/architecture-lint"))
                self.assertTrue((REPO_ROOT / rule.baseline_path).is_file())

        self.assertTrue((REPO_ROOT / "scripts/lint-architecture.py").is_file())
        self.assertTrue((REPO_ROOT / ".github/workflows/architecture-lint.yml").is_file())
        self.assertTrue((REPO_ROOT / "docs/architecture-lint.md").is_file())

    def test_workflow_fetches_full_history_for_push_baseline_comparisons(self) -> None:
        workflow = (REPO_ROOT / ".github/workflows/architecture-lint.yml").read_text(encoding="utf-8")
        self.assertIn("fetch-depth: 0", workflow)

    def test_push_runs_are_not_cancelled_before_baseline_comparison(self) -> None:
        workflow = (REPO_ROOT / ".github/workflows/architecture-lint.yml").read_text(encoding="utf-8")
        self.assertIn("github.event_name == 'pull_request'", workflow)
        self.assertIn("cancel-in-progress: ${{ github.event_name == 'pull_request' }}", workflow)
        self.assertIn("github.sha", workflow)
