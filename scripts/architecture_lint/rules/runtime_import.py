"""Prevent new callers from bypassing the public agent-runtime seam."""

from pathlib import Path

from architecture_lint.model import Finding, Rule

from .go_imports import import_lines, is_import_at_or_below


RULE_ID = "ARCH-RUNTIME-IMPORT"
IMPORT_ROOTS = (
    "github.com/kandev/kandev/internal/agent/runtime/lifecycle",
    "github.com/kandev/kandev/internal/agent/runtime/agentctl",
)
APPROVED_PREFIX = "apps/backend/internal/agent/runtime/"


def applies_to(path: str) -> bool:
    return path.startswith("apps/backend/") and path.endswith(".go") and not path.endswith("_test.go")


def scan(path: str, source: str) -> list[Finding]:
    if path.startswith(APPROVED_PREFIX):
        return []
    findings: list[Finding] = []
    for line, import_path in import_lines(source):
        if any(is_import_at_or_below(import_path, root) for root in IMPORT_ROOTS):
            findings.append(
                Finding.create(
                    RULE_ID,
                    path,
                    line,
                    {"path": path, "import": import_path},
                    (
                        f"direct import of {import_path}; higher-level callers must use "
                        "internal/agent/runtime or an approved low-level adapter"
                    ),
                )
            )
    return findings


RULE = Rule(
    id=RULE_ID,
    slug="runtime_import",
    baseline_path=Path("config/architecture-lint/runtime_import.json"),
    applies_to=applies_to,
    scan=scan,
)
