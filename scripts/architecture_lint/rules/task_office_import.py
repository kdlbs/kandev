"""Prevent shared task code from depending on Office implementations."""

from pathlib import Path

from architecture_lint.model import Finding, Rule

from .go_imports import import_lines, is_import_at_or_below


RULE_ID = "ARCH-TASK-OFFICE-IMPORT"
OFFICE_IMPORT_ROOT = "github.com/kandev/kandev/internal/office"


def applies_to(path: str) -> bool:
    return (
        path.startswith("apps/backend/internal/task/")
        and path.endswith(".go")
        and not path.endswith("_test.go")
    )


def scan(path: str, source: str) -> list[Finding]:
    findings: list[Finding] = []
    for line, import_path in import_lines(source):
        if is_import_at_or_below(import_path, OFFICE_IMPORT_ROOT):
            findings.append(
                Finding.create(
                    RULE_ID,
                    path,
                    line,
                    {"path": path, "import": import_path},
                    (
                        f"task imports {import_path}; the shared task model owns task concepts "
                        "and Office consumes or adapts that model, never the reverse"
                    ),
                )
            )
    return findings


RULE = Rule(
    id=RULE_ID,
    slug="task_office_import",
    baseline_path=Path("config/architecture-lint/task_office_import.json"),
    applies_to=applies_to,
    scan=scan,
)
