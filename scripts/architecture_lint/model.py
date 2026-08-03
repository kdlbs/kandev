"""Shared immutable types for architecture rules and diagnostics."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Callable


@dataclass(frozen=True)
class Finding:
    rule: str
    path: str
    line: int
    identity: tuple[tuple[str, object], ...]
    message: str

    @classmethod
    def create(
        cls,
        rule: str,
        path: str,
        line: int,
        identity: dict[str, object],
        message: str,
    ) -> "Finding":
        return cls(rule, path, line, tuple(sorted(identity.items())), message)

    def identity_dict(self) -> dict[str, object]:
        return dict(self.identity)


@dataclass(frozen=True)
class Diagnostic:
    rule: str
    path: str
    line: int
    message: str


@dataclass(frozen=True)
class Rule:
    id: str
    slug: str
    baseline_path: Path
    applies_to: Callable[[str], bool]
    scan: Callable[[str, str], list[Finding]]
