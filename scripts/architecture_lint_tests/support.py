"""Temporary-repository support for architecture-lint tests."""

import json
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
LINTER = REPO_ROOT / "scripts" / "lint-architecture.py"
sys.path.insert(0, str(LINTER.parent))

from architecture_lint.rules import RULES  # noqa: E402

RUNTIME_RULE = "ARCH-RUNTIME-IMPORT"
TASK_OFFICE_RULE = "ARCH-TASK-OFFICE-IMPORT"
ROOT_STATE_RULE = "ARCH-FRONTEND-ROOT-STATE-CAST"
FRONTEND_STATE_UI_RULE = "ARCH-FRONTEND-STATE-UI-IMPORT"
RUN_SCHEDULER_OWNER_RULE = "ARCH-RUN-SCHEDULER-OWNER"
RUNS_OFFICE_RULE = "ARCH-RUNS-OFFICE-IMPORT"
RUNTIME_IMPORT = "github.com/kandev/kandev/internal/agent/runtime/lifecycle"
OFFICE_IMPORT = "github.com/kandev/kandev/internal/office/models"
RULE_FILES = {rule.id: rule.baseline_path.name for rule in RULES}


class ArchitectureFixture(unittest.TestCase):
    def setUp(self) -> None:
        self.tempdir = tempfile.TemporaryDirectory()
        self.repo = Path(self.tempdir.name)
        self.git("init", "-q")
        self.git("config", "user.email", "test@example.com")
        self.git("config", "user.name", "Architecture Test")
        self.write_baseline()
        self.write_ledger([])

    def tearDown(self) -> None:
        self.tempdir.cleanup()

    def git(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", *args],
            cwd=self.repo,
            text=True,
            capture_output=True,
            check=True,
        )

    def write(self, relative: str, content: str) -> None:
        path = self.repo / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(textwrap.dedent(content).lstrip(), encoding="utf-8")

    def write_json(self, relative: str, content: object) -> None:
        self.write(relative, json.dumps(content, indent=2) + "\n")

    def write_baseline(
        self,
        *,
        runtime: list[dict[str, object]] | None = None,
        task_office: list[dict[str, object]] | None = None,
        root_state: list[dict[str, object]] | None = None,
        frontend_state_ui: list[dict[str, object]] | None = None,
        run_scheduler_owner: list[dict[str, object]] | None = None,
        runs_office: list[dict[str, object]] | None = None,
    ) -> None:
        entries = {rule.id: [] for rule in RULES}
        entries.update(
            {
                RUNTIME_RULE: runtime or [],
                TASK_OFFICE_RULE: task_office or [],
                ROOT_STATE_RULE: root_state or [],
                FRONTEND_STATE_UI_RULE: frontend_state_ui or [],
                RUN_SCHEDULER_OWNER_RULE: run_scheduler_owner or [],
                RUNS_OFFICE_RULE: runs_office or [],
            }
        )
        for rule in RULES:
            self.write_json(
                rule.baseline_path.as_posix(),
                {"version": 1, "rule": rule.id, "entries": entries[rule.id]},
            )

    def write_ledger(self, entries: list[dict[str, object]]) -> None:
        self.write_json(
            "config/architecture-lint/compatibility-ledger.json",
            {"version": 1, "entries": entries},
        )

    def track_all(self) -> None:
        self.git("add", ".")

    def run_cli(self, *args: str, cwd: Path | None = None) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(LINTER), *args],
            cwd=cwd or self.repo,
            text=True,
            capture_output=True,
            check=False,
        )

    def assert_diagnostic_location(self, result: subprocess.CompletedProcess[str], path: str, line: int) -> None:
        self.assertIn(path, result.stdout)
        self.assertTrue(
            f"{path}:{line}" in result.stdout or f"line={line}" in result.stdout,
            result.stdout,
        )

    @staticmethod
    def runtime_entry(path: str) -> dict[str, str]:
        return {"path": path, "import": RUNTIME_IMPORT}

    def valid_ledger_entry(self, *, marker: str = "// compat: old-name") -> dict[str, object]:
        return {
            "id": "old-name-alias",
            "locator": {
                "path": "apps/backend/internal/example/compat.go",
                "marker": marker,
            },
            "reason": "Older persisted callers still use the old name.",
            "owner": "backend maintainers",
            "introduced_on": "2026-01-15",
            "removal_condition": "Remove after all persisted callers have migrated.",
            "target_removal_date": "2099-12-31",
        }
