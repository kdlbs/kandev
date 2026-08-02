"""Prevent new unsafe casts in frontend root-state composition."""

import re
from pathlib import Path

from architecture_lint.model import Finding, Rule


RULE_ID = "ARCH-FRONTEND-ROOT-STATE-CAST"
STORE_PATH = "apps/web/lib/state/store.ts"
UNSAFE_CAST = re.compile(r"\bas\s+unknown\s+as\b|\bas\s+any\b")


def applies_to(path: str) -> bool:
    return path == STORE_PATH


def normalize_marker(line: str) -> str:
    return " ".join(line.strip().split())


def scan(path: str, source: str) -> list[Finding]:
    findings: list[Finding] = []
    for line_number, line in enumerate(source.splitlines(), start=1):
        marker = normalize_marker(line)
        occurrences: dict[str, int] = {}
        for match in UNSAFE_CAST.finditer(line):
            escape = normalize_marker(match.group(0))
            occurrences[escape] = occurrences.get(escape, 0) + 1
            findings.append(
                Finding.create(
                    RULE_ID,
                    path,
                    line_number,
                    {
                        "path": path,
                        "marker": marker,
                        "escape": escape,
                        "occurrence": occurrences[escape],
                    },
                    (
                        f"unsafe root-state composition escape `{escape}`; derive typed domain "
                        "state, actions, and defaults instead"
                    ),
                )
            )
    return findings


RULE = Rule(
    id=RULE_ID,
    slug="frontend_root_state_cast",
    baseline_path=Path("config/architecture-lint/frontend_root_state_cast.json"),
    applies_to=applies_to,
    scan=scan,
)
