"""Keep the generic runs subsystem independent from Office implementations."""

from pathlib import Path

from architecture_lint.model import Finding, Rule

from .go_imports import import_lines, is_generated, is_import_at_or_below


RULE_ID = "ARCH-RUNS-OFFICE-IMPORT"
OFFICE_IMPORT_ROOT = "github.com/kandev/kandev/internal/office"


def applies_to(path: str) -> bool:
    return (
        path.startswith("apps/backend/internal/runs/")
        and path.endswith(".go")
        and not path.endswith("_test.go")
    )


def scan(path: str, source: str) -> list[Finding]:
    if is_generated(source):
        return []

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
                        f"runs imports {import_path}; Office adapters may depend on generic runs, "
                        "but generic runs must not depend on Office implementations"
                    ),
                )
            )
    return findings


RULE = Rule(
    id=RULE_ID,
    slug="runs_office_import",
    baseline_path=Path("config/architecture-lint/runs_office_import.json"),
    applies_to=applies_to,
    scan=scan,
)
