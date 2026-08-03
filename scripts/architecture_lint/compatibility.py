"""Compatibility-ledger schema, expiry, and source-locator validation."""

from __future__ import annotations

import datetime as dt
import re
from pathlib import Path

from .baseline import load_json_file
from .model import Diagnostic
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


def validate_ledger(root: Path, ledger_path: Path, tracked: set[str], today: dt.date) -> list[Diagnostic]:
    data = load_json_file(ledger_path)
    try:
        label = ledger_path.relative_to(root).as_posix()
    except ValueError:
        label = str(ledger_path)
    if not isinstance(data, dict) or data.get("version") != 1 or not isinstance(data.get("entries"), list):
        return [diagnostic(label, "ledger must contain version 1 and an entries array")]

    diagnostics: list[Diagnostic] = []
    seen: set[str] = set()
    for index, raw_entry in enumerate(data["entries"]):
        prefix = f"entry {index + 1}"
        if not isinstance(raw_entry, dict):
            diagnostics.append(diagnostic(label, f"{prefix} must be an object"))
            continue
        entry: dict[str, object] = raw_entry
        entry_id = entry.get("id")
        if not isinstance(entry_id, str) or not STABLE_ID.fullmatch(entry_id):
            diagnostics.append(diagnostic(label, f"{prefix} has an invalid stable id"))
            entry_id = prefix
        elif entry_id in seen:
            diagnostics.append(diagnostic(label, f"duplicate compatibility id: {entry_id}"))
        else:
            seen.add(entry_id)

        for field in ("reason", "owner", "removal_condition"):
            if not nonempty_string(entry, field):
                diagnostics.append(diagnostic(label, f"{entry_id}: required field {field} is empty or missing"))

        diagnostics.extend(validate_introduction(label, str(entry_id), entry, today))
        diagnostics.extend(validate_removal_target(label, str(entry_id), entry, today))
        diagnostics.extend(validate_locator(root, label, str(entry_id), entry, tracked))
    return diagnostics


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
    if locator_path not in tracked:
        return [diagnostic(label, f"{entry_id}: referenced path is not tracked: {locator_path}")]
    if marker not in read_text(root, locator_path):
        return [diagnostic(label, f"{entry_id}: locator marker no longer exists in {locator_path}")]
    return []
