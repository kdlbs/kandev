"""Keep construction of the backend-wide runs scheduler in the composition root."""

from pathlib import Path

from architecture_lint.model import Finding, Rule

from .go_imports import import_lines, is_generated, is_import_at_or_below


RULE_ID = "ARCH-RUN-SCHEDULER-OWNER"
SCHEDULER_IMPORT_ROOT = "github.com/kandev/kandev/internal/runs/scheduler"
COMPOSITION_ROOT = "apps/backend/internal/backendapp/"


def applies_to(path: str) -> bool:
    return path.startswith("apps/backend/") and path.endswith(".go") and not path.endswith("_test.go")


def scan(path: str, source: str) -> list[Finding]:
    if path.startswith(COMPOSITION_ROOT) or is_generated(source):
        return []

    findings: list[Finding] = []
    for line, import_path in import_lines(source):
        if is_import_at_or_below(import_path, SCHEDULER_IMPORT_ROOT):
            findings.append(
                Finding.create(
                    RULE_ID,
                    path,
                    line,
                    {"path": path, "import": import_path},
                    (
                        f"direct import of {import_path}; the one backend-wide scheduler must be "
                        "constructed in internal/backendapp, while other packages depend on "
                        "runs services or interfaces"
                    ),
                )
            )
    return findings


RULE = Rule(
    id=RULE_ID,
    slug="run_scheduler_owner",
    baseline_path=Path("config/architecture-lint/run_scheduler_owner.json"),
    applies_to=applies_to,
    scan=scan,
)
