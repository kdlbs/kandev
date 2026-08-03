"""CLI orchestration for registered architecture rules."""

from __future__ import annotations

import argparse
import datetime as dt
import sys
from pathlib import Path

from .baseline import (
    baseline_diagnostics,
    baseline_growth_diagnostics,
    load_baselines,
    load_baselines_from_ref,
)
from .compatibility import validate_ledger
from .model import Finding
from .output import print_diagnostics
from .repository import (
    ConfigurationError,
    GitDiscoveryError,
    discover_root,
    read_text,
    tracked_files,
)
from .rules import RULES


LEDGER_PATH = Path("config/architecture-lint/compatibility-ledger.json")


def scan_architecture(root: Path, paths: list[str]) -> list[Finding]:
    findings: list[Finding] = []
    for path in paths:
        matching = tuple(rule for rule in RULES if rule.applies_to(path))
        if not matching:
            continue
        source = read_text(root, path)
        for rule in matching:
            findings.extend(rule.scan(path, source))
    return sorted(findings, key=lambda item: (item.path, item.line, item.rule, item.identity))


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--all", action="store_true", help="scan all tracked source files")
    parser.add_argument("--ledger", type=Path, default=LEDGER_PATH)
    parser.add_argument(
        "--baseline-base-ref",
        help="git commit whose per-rule baselines are the growth ceiling",
    )
    parser.add_argument(
        "--allow-missing-base-baseline",
        action="store_true",
        help="allow initial rollout of a rule whose baseline is absent from the base commit",
    )
    args = parser.parse_args(argv)
    if not args.all:
        parser.error("--all is required")
    return args


def resolve_config_path(root: Path, path: Path) -> Path:
    return path if path.is_absolute() else root / path


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv or sys.argv[1:])
    try:
        root = discover_root(Path.cwd())
        paths = tracked_files(root)
        baselines = load_baselines(root, RULES)
        findings = scan_architecture(root, paths)
        diagnostics = baseline_diagnostics(findings, baselines, RULES)
        ledger_path = resolve_config_path(root, args.ledger)
        diagnostics.extend(validate_ledger(root, ledger_path, set(paths), dt.date.today()))

        if args.baseline_base_ref:
            base = load_baselines_from_ref(
                root,
                args.baseline_base_ref,
                RULES,
                args.allow_missing_base_baseline,
            )
            diagnostics.extend(baseline_growth_diagnostics(baselines, base, RULES))
    except GitDiscoveryError as exc:
        print(f"architecture-lint: git discovery failed: {exc}", file=sys.stderr)
        return 3
    except ConfigurationError as exc:
        print(f"architecture-lint: configuration error: {exc}", file=sys.stderr)
        return 2

    print_diagnostics(diagnostics)
    return 1 if diagnostics else 0
