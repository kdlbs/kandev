"""Tests for ARCH-RUNTIME-IMPORT."""

from support import ArchitectureFixture, RUNTIME_IMPORT, RUNTIME_RULE


class RuntimeImportTest(ArchitectureFixture):
    def test_current_grandfathered_violation_passes(self) -> None:
        path = "apps/backend/internal/example/service.go"
        self.write(path, f'package example\n\nimport "{RUNTIME_IMPORT}"\n')
        self.write_baseline(runtime=[self.runtime_entry(path)])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_new_import_reports_rule_location_and_migration_guidance(self) -> None:
        path = "apps/backend/internal/example/new_service.go"
        self.write(path, f'package example\n\nimport "{RUNTIME_IMPORT}"\n')
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn(f"{path}:3", result.stdout)
        self.assertIn(RUNTIME_RULE, result.stdout)
        self.assertIn("internal/agent/runtime", result.stdout)
        self.assertIn("approved low-level adapter", result.stdout)
