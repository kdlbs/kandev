"""Shared specification path, frontmatter, and status metadata rules."""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path


VALID_REQUIREMENT_STATUSES = {"draft", "active", "deprecated"}
VALID_DESIGN_STATUSES = {"draft", "current", "superseded"}
VALID_SYSTEM_STATUSES = {"draft", "active", "retired"}
VALID_MIGRATION_STATUSES = {"in_progress", "complete"}


@dataclass(frozen=True)
class FrontmatterResult:
    """The parsed frontmatter and the source location of any parse error."""

    metadata: dict[str, object] | None
    end: int
    line: int
    error: str | None


@dataclass(frozen=True)
class MetadataIssue:
    """A specification metadata contract violation."""

    code: str
    message: str


def metadata_text(metadata: dict[str, object], key: str) -> str | None:
    """Return a scalar or list frontmatter value in catalog form."""
    value = metadata.get(key)
    if isinstance(value, str):
        return value.strip()
    if isinstance(value, list) and value:
        return ", ".join(str(item) for item in value)
    return None


def validate_metadata(
    kind: str,
    system: str | None,
    metadata: dict[str, object],
    has_frontmatter: bool = True,
) -> list[MetadataIssue]:
    """Validate the metadata contract shared by the catalog and spec linter."""
    contract_kind = "system-index" if kind == "system" else kind
    if contract_kind not in {"requirement", "system-design", "system-index"}:
        return []
    if not has_frontmatter:
        return [
            MetadataIssue(
                "frontmatter", "new specification document must start with YAML frontmatter"
            )
        ]

    issues: list[MetadataIssue] = []
    if system is not None and metadata_text(metadata, "system") != system:
        issues.append(
            MetadataIssue(
                "system-owner",
                f"frontmatter system must be `{system}`, found `{metadata_text(metadata, 'system') or ''}`",
            )
        )

    status = metadata_text(metadata, "status")
    if contract_kind == "requirement" and status not in VALID_REQUIREMENT_STATUSES:
        issues.append(
            MetadataIssue(
                "requirement-status",
                f"requirement status must be one of {sorted(VALID_REQUIREMENT_STATUSES)}, found `{status or ''}`",
            )
        )
    elif contract_kind == "system-design" and status not in VALID_DESIGN_STATUSES:
        issues.append(
            MetadataIssue(
                "system-design-status",
                f"system-design status must be one of {sorted(VALID_DESIGN_STATUSES)}, found `{status or ''}`",
            )
        )
    elif contract_kind == "system-index" and status not in VALID_SYSTEM_STATUSES:
        issues.append(
            MetadataIssue(
                "system-index-status",
                f"system status must be one of {sorted(VALID_SYSTEM_STATUSES)}",
            )
        )

    if contract_kind == "system-index":
        if metadata_text(metadata, "specification_version") != "1":
            issues.append(
                MetadataIssue(
                    "system-index-version", "system index must set `specification_version: 1`"
                )
            )
        migration = metadata_text(metadata, "migration")
        if migration not in VALID_MIGRATION_STATUSES:
            issues.append(
                MetadataIssue(
                    "system-index-migration",
                    f"migration must be one of {sorted(VALID_MIGRATION_STATUSES)}",
                )
            )
    elif contract_kind == "system-design" and not isinstance(metadata.get("requirements"), list):
        issues.append(
            MetadataIssue(
                "system-design-requirements",
                "system design must declare `requirements` as a YAML list, including an explicit empty list",
            )
        )
    return issues


def parse_frontmatter(text: str, *, require: bool = False) -> FrontmatterResult:
    """Parse the small YAML subset used by specification files."""
    lines = text.splitlines()
    if not lines or lines[0] != "---":
        if require:
            return FrontmatterResult(
                None, 0, 1, "new specification document must start with YAML frontmatter"
            )
        return FrontmatterResult({}, 0, 1, None)

    try:
        end = lines.index("---", 1)
    except ValueError:
        return FrontmatterResult(None, 0, 1, "YAML frontmatter has no closing delimiter")

    metadata: dict[str, object] = {}
    current_list: str | None = None
    for line_number, line in enumerate(lines[1:end], start=2):
        if re.match(r"^\s+-\s+", line) and current_list:
            value = re.sub(r"^\s+-\s+", "", line).strip().strip('"\'`')
            current_value = metadata.setdefault(current_list, [])
            if not isinstance(current_value, list):
                return FrontmatterResult(
                    None,
                    end + 1,
                    line_number,
                    f"frontmatter field `{current_list}` mixes scalar and list values",
                )
            current_value.append(value)
            continue

        match = re.match(r"^(?P<key>[a-z][a-z0-9_-]*):(?:\s*(?P<value>.*))?$", line)
        if not match:
            if not line.strip():
                continue
            return FrontmatterResult(
                None, end + 1, line_number, "frontmatter contains unsupported YAML"
            )

        key = match.group("key")
        value = (match.group("value") or "").strip()
        if value == "[]":
            metadata[key] = []
            current_list = None
        elif value:
            metadata[key] = value.strip('"\'`')
            current_list = None
        else:
            metadata[key] = []
            current_list = key

    return FrontmatterResult(metadata, end + 1, 1, None)


def classify_path(relative: Path) -> tuple[str, str | None]:
    """Classify a specification path using the authoritative linter layout."""
    parts = relative.parts
    try:
        spec_index = parts.index("specs")
    except ValueError:
        return "unknown", None
    inside = parts[spec_index + 1 :]
    if not inside:
        return "unknown", None
    if inside[0] == "guide":
        return "guide", None
    if inside[0] == "templates":
        return "template", None
    if inside[0] == "product":
        return "product", None
    if len(inside) >= 3 and inside[1] == "requirements":
        return "requirement", inside[0]
    if len(inside) >= 3 and inside[1] == "system-design":
        return "system-design", inside[0]
    if len(inside) == 2 and inside[1] in {"README.md", "glossary.md"}:
        return "system-index", inside[0]
    return "legacy", None
