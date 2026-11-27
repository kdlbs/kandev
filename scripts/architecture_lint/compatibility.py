"""Compatibility-ledger schema, expiry, and source-locator validation."""

from __future__ import annotations

import datetime as dt
import re
from dataclasses import dataclass
from pathlib import Path

from .baseline import load_json_file
from .model import Diagnostic, Finding
from .repository import read_text


RULE_ID = "COMPAT-LEDGER"
_SEMVER_NUMBER = r"(?:0|[1-9][0-9]*)"
_SEMVER_PRERELEASE_IDENTIFIER = (
    r"(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)"
)
SEMVER = re.compile(
    rf"^{_SEMVER_NUMBER}\.{_SEMVER_NUMBER}\.{_SEMVER_NUMBER}"
    rf"(?:-{_SEMVER_PRERELEASE_IDENTIFIER}"
    rf"(?:\.{_SEMVER_PRERELEASE_IDENTIFIER})*)?"
    rf"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$"
)
STABLE_ID = re.compile(r"^[a-z0-9]+(?:[a-z0-9._-]*[a-z0-9])?$")


@dataclass(frozen=True)
class LedgerValidation:
    diagnostics: list[Diagnostic]
    registered_findings: frozenset[Finding]


def diagnostic(path: str, message: str) -> Diagnostic:
    return Diagnostic(RULE_ID, path, 1, message)


def nonempty_string(entry: dict[str, object], field: str) -> bool:
    return isinstance(entry.get(field), str) and bool(str(entry[field]).strip())


def parse_date(value: object) -> dt.date | None:
    if not isinstance(value, str):
        return None
    try:
        return dt.date.fromisoformat(value)
    except ValueError:
        return None


def validate_ledger(
    root: Path,
    ledger_path: Path,
    tracked: set[str],
    today: dt.date,
    findings: list[Finding],
) -> LedgerValidation:
    data = load_json_file(ledger_path)
    try:
        label = ledger_path.relative_to(root).as_posix()
    except ValueError:
        label = str(ledger_path)
    if not isinstance(data, dict) or data.get("version") != 1 or not isinstance(data.get("entries"), list):
        return LedgerValidation(
            [diagnostic(label, "ledger must contain version 1 and an entries array")],
            frozenset(),
        )

    diagnostics: list[Diagnostic] = []
    seen: set[str] = set()
    raw_entries = data["entries"]
    id_counts: dict[str, int] = {}
    for raw_entry in raw_entries:
        if isinstance(raw_entry, dict) and isinstance(raw_entry.get("id"), str):
            entry_id = str(raw_entry["id"])
            id_counts[entry_id] = id_counts.get(entry_id, 0) + 1

    registrations: list[tuple[str, tuple[str, str, str]]] = []
    for index, raw_entry in enumerate(raw_entries):
        prefix = f"entry {index + 1}"
        if not isinstance(raw_entry, dict):
            diagnostics.append(diagnostic(label, f"{prefix} must be an object"))
            continue
        entry_diagnostics: list[Diagnostic] = []
        entry: dict[str, object] = raw_entry
        entry_id = entry.get("id")
        if not isinstance(entry_id, str) or not STABLE_ID.fullmatch(entry_id):
            entry_diagnostics.append(diagnostic(label, f"{prefix} has an invalid stable id"))
            entry_id = prefix
        elif entry_id in seen:
            entry_diagnostics.append(diagnostic(label, f"duplicate compatibility id: {entry_id}"))
        else:
            seen.add(entry_id)
        if isinstance(entry.get("id"), str) and id_counts.get(str(entry["id"]), 0) > 1:
            duplicate = diagnostic(label, f"duplicate compatibility id: {entry_id}")
            if duplicate not in entry_diagnostics:
                entry_diagnostics.append(duplicate)

        for field in ("reason", "owner", "removal_condition"):
            if not nonempty_string(entry, field):
                entry_diagnostics.append(
                    diagnostic(label, f"{entry_id}: required field {field} is empty or missing")
                )

        entry_diagnostics.extend(validate_introduction(label, str(entry_id), entry, today))
        entry_diagnostics.extend(validate_removal_target(label, str(entry_id), entry, today))
        entry_diagnostics.extend(validate_locator(root, label, str(entry_id), entry, tracked))
        diagnostics.extend(entry_diagnostics)
        registration = declaration_registration(entry, str(entry_id))
        if registration and not entry_diagnostics:
            registrations.append(registration)

    registration_counts: dict[tuple[str, str, str], int] = {}
    for _, identity in registrations:
        registration_counts[identity] = registration_counts.get(identity, 0) + 1

    findings_by_declaration: dict[tuple[str, str, str], Finding] = {}
    ambiguous_declarations: set[tuple[str, str, str]] = set()
    for finding in findings:
        identity = finding.identity_dict()
        declaration = identity.get("declaration")
        marker = identity.get("marker")
        if identity.get("ambiguous") is True:
            if isinstance(declaration, str) and isinstance(marker, str):
                ambiguous_identity = (finding.path, declaration.rsplit("#", 1)[0], marker)
                if ambiguous_identity not in ambiguous_declarations:
                    ambiguous_declarations.add(ambiguous_identity)
                    diagnostics.append(
                        diagnostic(
                            label,
                            "ambiguous repeated deprecation identity cannot be registered: "
                            f"{finding.path} {ambiguous_identity[1]} ({marker}); "
                            "distinguish or remove the duplicate declaration",
                        )
                    )
            continue
        if isinstance(declaration, str) and isinstance(marker, str):
            findings_by_declaration[(finding.path, declaration, marker)] = finding

    registered_findings: set[Finding] = set()
    for entry_id, identity in registrations:
        path, declaration, marker = identity
        if registration_counts[identity] > 1:
            diagnostics.append(
                diagnostic(
                    label,
                    f"{entry_id}: duplicate declaration registration for {path} {declaration} ({marker})",
                )
            )
            continue
        finding = findings_by_declaration.get(identity)
        if finding is None:
            diagnostics.append(
                diagnostic(
                    label,
                    f"{entry_id}: locator declaration does not match a current declaration finding: "
                    f"{path} {declaration} ({marker})",
                )
            )
            continue
        registered_findings.add(finding)

    return LedgerValidation(diagnostics, frozenset(registered_findings))


def declaration_registration(
    entry: dict[str, object], entry_id: str
) -> tuple[str, tuple[str, str, str]] | None:
    locator = entry.get("locator")
    if not isinstance(locator, dict) or "declaration" not in locator:
        return None
    path = locator.get("path")
    declaration = locator.get("declaration")
    marker = locator.get("marker")
    if all(isinstance(value, str) and value for value in (path, declaration, marker)):
        return entry_id, (str(path), str(declaration), str(marker))
    return None


def validate_introduction(
    label: str, entry_id: str, entry: dict[str, object], today: dt.date
) -> list[Diagnostic]:
    date_present = "introduced_on" in entry
    version_present = "introduced_version" in entry
    if date_present == version_present:
        return [diagnostic(label, f"{entry_id}: set exactly one of introduced_on or introduced_version")]
    if date_present:
        introduced = parse_date(entry.get("introduced_on"))
        if introduced is None:
            return [diagnostic(label, f"{entry_id}: invalid introduced_on date")]
        if introduced > today:
            return [diagnostic(label, f"{entry_id}: introduced_on is in the future")]
    elif not isinstance(entry.get("introduced_version"), str) or not SEMVER.fullmatch(
        str(entry["introduced_version"])
    ):
        return [diagnostic(label, f"{entry_id}: invalid introduced_version")]
    return []


def validate_removal_target(
    label: str, entry_id: str, entry: dict[str, object], today: dt.date
) -> list[Diagnostic]:
    date_present = "target_removal_date" in entry
    version_present = "target_removal_version" in entry
    if date_present == version_present:
        return [
            diagnostic(
                label,
                f"{entry_id}: set exactly one of target_removal_date or target_removal_version",
            )
        ]
    if date_present:
        target = parse_date(entry.get("target_removal_date"))
        if target is None:
            return [diagnostic(label, f"{entry_id}: invalid target_removal_date")]
        if target < today:
            return [diagnostic(label, f"{entry_id}: compatibility exception expired on {target.isoformat()}")]
    elif not isinstance(entry.get("target_removal_version"), str) or not SEMVER.fullmatch(
        str(entry["target_removal_version"])
    ):
        return [diagnostic(label, f"{entry_id}: invalid target_removal_version")]
    return []


def validate_locator(
    root: Path,
    label: str,
    entry_id: str,
    entry: dict[str, object],
    tracked: set[str],
) -> list[Diagnostic]:
    locator = entry.get("locator")
    if not isinstance(locator, dict):
        return [diagnostic(label, f"{entry_id}: locator must be an object")]
    locator_path = locator.get("path")
    marker = locator.get("marker")
    if not isinstance(locator_path, str) or not locator_path:
        return [diagnostic(label, f"{entry_id}: locator.path is required")]
    if not isinstance(marker, str) or not marker:
        return [diagnostic(label, f"{entry_id}: locator.marker is required")]
    declaration = locator.get("declaration")
    if "declaration" in locator and (not isinstance(declaration, str) or not declaration.strip()):
        return [diagnostic(label, f"{entry_id}: locator.declaration must be a non-empty string")]
    if locator_path not in tracked:
        return [diagnostic(label, f"{entry_id}: referenced path is not tracked: {locator_path}")]
    if marker not in read_text(root, locator_path):
        return [diagnostic(label, f"{entry_id}: locator marker no longer exists in {locator_path}")]
    return []
