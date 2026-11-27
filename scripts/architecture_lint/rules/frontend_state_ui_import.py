"""Keep frontend state below components and route/app modules."""

from pathlib import Path

from architecture_lint.model import Finding, Rule

from .frontend_imports import (
    is_generated,
    is_test_or_fixture,
    is_ui_or_app_module,
    module_imports,
)


RULE_ID = "ARCH-FRONTEND-STATE-UI-IMPORT"
STATE_PREFIX = "apps/web/lib/state/"


def applies_to(path: str) -> bool:
    return (
        path.startswith(STATE_PREFIX)
        and path.endswith((".ts", ".tsx", ".mts"))
        and not is_test_or_fixture(path)
    )


def scan(path: str, source: str) -> list[Finding]:
    if is_generated(source):
        return []

    findings: list[Finding] = []
    for line, specifier in module_imports(source):
        if is_ui_or_app_module(path, specifier):
            findings.append(
                Finding.create(
                    RULE_ID,
                    path,
                    line,
                    {"path": path, "import": specifier},
                    (
                        f"state imports {specifier}; components and app layers consume state, "
                        "so state must not depend on UI or route modules"
                    ),
                )
            )
    return findings


RULE = Rule(
    id=RULE_ID,
    slug="frontend_state_ui_import",
    baseline_path=Path("config/architecture-lint/frontend_state_ui_import.json"),
    applies_to=applies_to,
    scan=scan,
)
