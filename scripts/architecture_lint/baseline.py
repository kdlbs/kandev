"""Per-rule baseline loading, stale-entry checks, and shrink-only comparison."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from .model import Diagnostic, Finding, Rule
from .repository import ConfigurationError, GitDiscoveryError, run_git


def canonical_identity(identity: dict[str, object]) -> str:
    return json.dumps(identity, sort_keys=True, separators=(",", ":"))


def load_json_file(path: Path) -> Any:
    try:
        with path.open(encoding="utf-8") as handle:
            return json.load(handle)
    except FileNotFoundError as exc:
        raise ConfigurationError(f"required configuration file does not exist: {path}") from exc
    except (OSError, json.JSONDecodeError) as exc:
        raise ConfigurationError(f"cannot load {path}: {exc}") from exc


def validate_rule_baseline(data: Any, rule: Rule, label: str) -> list[dict[str, object]]:
    if (
        not isinstance(data, dict)
        or data.get("version") != 1
        or data.get("rule") != rule.id
        or not isinstance(data.get("entries"), list)
    ):
        raise ConfigurationError(
            f"{label} must contain version 1, rule {rule.id}, and an entries array"
        )
    entries = data["entries"]
    if not all(isinstance(entry, dict) for entry in entries):
        raise ConfigurationError(f"{label} entries must be objects")
    keys = [canonical_identity(entry) for entry in entries]
    if len(keys) != len(set(keys)):
        raise ConfigurationError(f"{label} contains duplicate entries")
    return entries


def load_baselines(root: Path, rules: tuple[Rule, ...]) -> dict[str, list[dict[str, object]]]:
    return {
        rule.id: validate_rule_baseline(load_json_file(root / rule.baseline_path), rule, str(rule.baseline_path))
        for rule in rules
    }


def baseline_diagnostics(
    findings: list[Finding],
    baselines: dict[str, list[dict[str, object]]],
    rules: tuple[Rule, ...],
) -> list[Diagnostic]:
    diagnostics: list[Diagnostic] = []
    actual_by_rule: dict[str, dict[str, Finding]] = {rule.id: {} for rule in rules}
    for finding in findings:
        actual_by_rule[finding.rule][canonical_identity(finding.identity_dict())] = finding

    for rule in rules:
        expected = {canonical_identity(entry): entry for entry in baselines[rule.id]}
        actual = actual_by_rule[rule.id]
        for key in sorted(actual.keys() - expected.keys()):
            finding = actual[key]
            diagnostics.append(Diagnostic(finding.rule, finding.path, finding.line, finding.message))
        for key in sorted(expected.keys() - actual.keys()):
            diagnostics.append(
                Diagnostic(
                    rule.id,
                    rule.baseline_path.as_posix(),
                    1,
                    (
                        "stale baseline entry no longer matches tracked source; remove it from "
                        f"{rule.baseline_path}: {key}"
                    ),
                )
            )
    return diagnostics


def load_baselines_from_ref(
    root: Path,
    ref: str,
    rules: tuple[Rule, ...],
    allow_missing: bool,
) -> dict[str, list[dict[str, object]] | None]:
    run_git(root, "cat-file", "-e", f"{ref}^{{commit}}")
    baselines: dict[str, list[dict[str, object]] | None] = {}
    for rule in rules:
        try:
            raw = run_git(root, "show", f"{ref}:{rule.baseline_path.as_posix()}")
        except GitDiscoveryError as exc:
            if allow_missing:
                baselines[rule.id] = None
                continue
            raise ConfigurationError(f"baseline {rule.baseline_path} does not exist at {ref}") from exc
        try:
            data = json.loads(raw.decode("utf-8"))
        except (UnicodeError, json.JSONDecodeError) as exc:
            raise ConfigurationError(f"baseline {rule.baseline_path} at {ref} is invalid JSON: {exc}") from exc
        baselines[rule.id] = validate_rule_baseline(data, rule, f"{ref}:{rule.baseline_path}")
    return baselines


def baseline_growth_diagnostics(
    current: dict[str, list[dict[str, object]]],
    base: dict[str, list[dict[str, object]] | None],
    rules: tuple[Rule, ...],
) -> list[Diagnostic]:
    diagnostics: list[Diagnostic] = []
    for rule in rules:
        if base[rule.id] is None:
            continue
        current_entries = {canonical_identity(entry) for entry in current[rule.id]}
        base_entries = {canonical_identity(entry) for entry in base[rule.id] or []}
        for added in sorted(current_entries - base_entries):
            diagnostics.append(
                Diagnostic(
                    rule.id,
                    rule.baseline_path.as_posix(),
                    1,
                    f"baseline may only shrink; newly allowlisted entry is not permitted: {added}",
                )
            )
    return diagnostics
