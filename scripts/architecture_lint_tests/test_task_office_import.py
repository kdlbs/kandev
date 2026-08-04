"""Tests for ARCH-TASK-OFFICE-IMPORT."""

from support import ArchitectureFixture, OFFICE_IMPORT, TASK_OFFICE_RULE


class TaskOfficeImportTest(ArchitectureFixture):
    def test_new_dependency_reports_ownership_direction(self) -> None:
        path = "apps/backend/internal/task/service/new_service.go"
        self.write(path, f'package service\n\nimport models "{OFFICE_IMPORT}"\n')
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 3)
        self.assertIn(TASK_OFFICE_RULE, result.stdout)
        self.assertIn("shared task model owns task concepts", result.stdout)
        self.assertIn("Office consumes or adapts", result.stdout)
