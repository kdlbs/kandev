#!/usr/bin/env python3
"""Lint Kandev specification files."""

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Violation:
    rule: str
    path: Path
    line: int
    message: str


VALID_REQUIREMENT_STATUSES = {"draft", "active", "deprecated"}
VALID_DESIGN_STATUSES = {"draft", "current", "superseded"}
VALID_SYSTEM_STATUSES = {"draft", "active", "retired"}
VALID_MIGRATION_STATUSES = {"in_progress", "complete"}
REQ_ID_PATTERN = r"REQ-[A-Z0-9]+(?:-[A-Z0-9]+)*-\d{3}"
AC_ID_PATTERN = r"AC-[A-Z0-9]+(?:-[A-Z0-9]+)*-\d{3}\.\d+"
REQ_HEADING = re.compile(rf"^###\s+(?P<id>{REQ_ID_PATTERN}):\s+\S")
AC_DEFINITION = re.compile(rf"^\s*-\s+\*\*(?P<id>{AC_ID_PATTERN}):\*\*\s+\S")


def load_config(root: Path) -> dict:
    path = root / "docs/specs/spec-lint.json"
    return json.loads(path.read_text(encoding="utf-8"))


def lint_specs(root: Path, config: dict | None = None) -> list[Violation]:
    config = config or load_config(root)
    validate_config(config)
    specs_root = root / "docs/specs"
    if not specs_root.exists():
        return []

    paths = sorted(path for path in specs_root.rglob("*.md") if path.is_file())
    migrated_systems = set()
    for path in paths:
        inside = path.relative_to(specs_root).parts
        if (
            len(inside) != 2
            or inside[1] != "README.md"
            or inside[0] in {"guide", "product", "templates"}
        ):
            continue
        metadata, _, error = parse_frontmatter(path.read_text(encoding="utf-8"))
        if (
            error is None
            and metadata is not None
            and metadata.get("specification_version") == "1"
            and metadata.get("migration") == "complete"
        ):
            migrated_systems.add(inside[0])
    violations: list[Violation] = []
    requirement_definitions: dict[str, list[tuple[Path, int]]] = {}
    acceptance_definitions: dict[str, list[tuple[Path, int]]] = {}
    design_references: list[tuple[str, Path, int]] = []

    for path in paths:
        relative = path.relative_to(root)
        kind, system = classify_path(relative)
        violations.extend(check_size(path, relative, kind, config))
        inside = relative.relative_to("docs/specs").parts
        if inside and inside[0] in migrated_systems and kind == "legacy":
            violations.append(
                Violation(
                    "system-layout",
                    path,
                    1,
                    "migrated system Markdown must be README.md, glossary.md, or under requirements/ or system-design/",
                )
            )
        if kind == "system-index" and path.name == "README.md":
            text = path.read_text(encoding="utf-8")
            metadata, metadata_line, metadata_error = parse_frontmatter(text)
            if metadata_error:
                violations.append(Violation("frontmatter", path, metadata_line, metadata_error))
            else:
                assert metadata is not None
                violations.extend(check_system_index_metadata(path, metadata, system))
            continue
        if kind not in {"requirement", "system-design"}:
            continue

        text = path.read_text(encoding="utf-8")
        metadata, metadata_line, metadata_error = parse_frontmatter(text)
        if metadata_error:
            violations.append(Violation("frontmatter", path, metadata_line, metadata_error))
            continue

        assert metadata is not None
        declared_system = metadata.get("system")
        if declared_system != system:
            violations.append(
                Violation(
                    "system-owner",
                    path,
                    1,
                    f"frontmatter system must be `{system}`, found `{declared_system or ''}`",
                )
            )

        if kind == "requirement":
            violations.extend(check_requirement_metadata(path, metadata))
            reqs, criteria = collect_requirement_ids(path, text)
            if not reqs:
                violations.append(
                    Violation(
                        "missing-requirement-id",
                        path,
                        1,
                        "requirement document must define at least one REQ-* heading",
                    )
                )
            for req_id, line in reqs:
                requirement_definitions.setdefault(req_id, []).append((path, line))
            local_requirements = {req_id for req_id, _ in reqs}
            local_acceptance_parents = {acceptance_parent(ac_id) for ac_id, _ in criteria}
            for req_id, line in reqs:
                if req_id not in local_acceptance_parents:
                    violations.append(
                        Violation(
                            "missing-acceptance-criteria",
                            path,
                            line,
                            f"{req_id} must have at least one matching AC-* criterion",
                        )
                    )
            for ac_id, line in criteria:
                acceptance_definitions.setdefault(ac_id, []).append((path, line))
                parent = acceptance_parent(ac_id)
                if parent not in local_requirements:
                    violations.append(
                        Violation(
                            "orphan-acceptance-id",
                            path,
                            line,
                            f"{ac_id} has no matching {parent} in this document",
                        )
                    )
        else:
            violations.extend(check_design_metadata(path, metadata))
            references = metadata.get("requirements", [])
            if isinstance(references, str):
                references = [references] if references else []
            for reference in references:
                design_references.append((str(reference), path, 1))

    violations.extend(check_duplicate_ids("requirement", requirement_definitions))
    violations.extend(check_duplicate_ids("acceptance", acceptance_definitions))
    known_requirements = set(requirement_definitions)
    for requirement, path, line in design_references:
        if requirement not in known_requirements:
            violations.append(
                Violation(
                    "unknown-requirement-reference",
                    path,
                    line,
                    f"system design references unknown requirement {requirement}",
                )
            )

    violations.extend(check_size_exception_catalog(root, config))
    return sorted(violations, key=lambda item: (item.path.as_posix(), item.line, item.rule))


def validate_config(config: dict) -> None:
    if config.get("version") != 1:
        raise ValueError("specification lint configuration must use version 1")
    required_limits = {
        "guide",
        "legacy",
        "product",
        "requirement",
        "system-design",
        "system-index",
        "template",
    }
    limits = config.get("limits", {})
    missing = sorted(required_limits - set(limits))
    if missing:
        raise ValueError(f"specification lint configuration is missing limits: {', '.join(missing)}")
    if any(not isinstance(limits[key], int) or limits[key] <= 0 for key in required_limits):
        raise ValueError("specification size limits must be positive integers")


def classify_path(relative: Path) -> tuple[str, str | None]:
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


def check_size(path: Path, relative: Path, kind: str, config: dict) -> list[Violation]:
    if kind == "unknown":
        return []
    size = path.stat().st_size
    default_limit = config["limits"][kind]
    exception = config.get("legacy_size_exceptions", {}).get(relative.as_posix())
    limit = exception if kind == "legacy" and exception is not None else default_limit
    if size <= limit:
        return []
    ceiling = f"frozen ceiling {limit}" if exception is not None else f"limit {limit}"
    return [
        Violation(
            "file-size",
            path,
            1,
            f"{kind} file is {size} bytes and exceeds its {ceiling} bytes",
        )
    ]


def check_size_exception_catalog(root: Path, config: dict) -> list[Violation]:
    violations = []
    default_limit = config["limits"]["legacy"]
    for relative_text, ceiling in config.get("legacy_size_exceptions", {}).items():
        relative = Path(relative_text)
        path = root / relative
        if not path.exists():
            violations.append(
                Violation(
                    "missing-size-exception-file",
                    path,
                    1,
                    "legacy size exception references a missing file",
                )
            )
            continue
        kind, _ = classify_path(relative)
        if kind != "legacy":
            violations.append(
                Violation(
                    "invalid-size-exception",
                    path,
                    1,
                    "size exceptions apply only to legacy specification files",
                )
            )
            continue
        size = path.stat().st_size
        if size < ceiling or ceiling <= default_limit:
            violations.append(
                Violation(
                    "stale-size-exception",
                    path,
                    1,
                    f"file is {size} bytes. Lower the frozen ceiling to {size} or remove the exception",
                )
            )
    return violations


def parse_frontmatter(text: str) -> tuple[dict | None, int, str | None]:
    lines = text.splitlines()
    if not lines or lines[0] != "---":
        return None, 1, "new specification document must start with YAML frontmatter"
    try:
        end = lines.index("---", 1)
    except ValueError:
        return None, 1, "YAML frontmatter has no closing delimiter"

    metadata: dict[str, str | list[str]] = {}
    current_list: str | None = None
    for line_number, line in enumerate(lines[1:end], start=2):
        if re.match(r"^\s+-\s+", line) and current_list:
            value = re.sub(r"^\s+-\s+", "", line).strip().strip('"\'`')
            current_value = metadata.setdefault(current_list, [])
            if not isinstance(current_value, list):
                return None, line_number, f"frontmatter field `{current_list}` mixes scalar and list values"
            current_value.append(value)
            continue
        match = re.match(r"^(?P<key>[a-z][a-z0-9_-]*):(?:\s*(?P<value>.*))?$", line)
        if not match:
            if not line.strip():
                continue
            return None, line_number, "frontmatter contains unsupported YAML"
        key = match.group("key")
        value = (match.group("value") or "").strip()
        if value == "[]":
            metadata[key] = []
            current_list = None
        elif value:
            metadata[key] = value.strip('"\'')
            current_list = None
        else:
            metadata[key] = []
            current_list = key
    return metadata, 1, None


def check_requirement_metadata(path: Path, metadata: dict) -> list[Violation]:
    status = metadata.get("status")
    if status in VALID_REQUIREMENT_STATUSES:
        return []
    return [
        Violation(
            "requirement-status",
            path,
            1,
            f"requirement status must be one of {sorted(VALID_REQUIREMENT_STATUSES)}, found `{status or ''}`",
        )
    ]


def check_system_index_metadata(path: Path, metadata: dict, system: str | None) -> list[Violation]:
    violations = []
    if metadata.get("status") not in VALID_SYSTEM_STATUSES:
        violations.append(
            Violation(
                "system-index-status",
                path,
                1,
                f"system status must be one of {sorted(VALID_SYSTEM_STATUSES)}",
            )
        )
    if metadata.get("system") != system:
        violations.append(
            Violation(
                "system-owner",
                path,
                1,
                f"frontmatter system must be `{system}`, found `{metadata.get('system', '')}`",
            )
        )
    if metadata.get("specification_version") != "1":
        violations.append(
            Violation(
                "system-index-version",
                path,
                1,
                "system index must set `specification_version: 1`",
            )
        )
    if metadata.get("migration") not in VALID_MIGRATION_STATUSES:
        violations.append(
            Violation(
                "system-index-migration",
                path,
                1,
                f"migration must be one of {sorted(VALID_MIGRATION_STATUSES)}",
            )
        )
    return violations


def check_design_metadata(path: Path, metadata: dict) -> list[Violation]:
    violations = []
    status = metadata.get("status")
    if status not in VALID_DESIGN_STATUSES:
        violations.append(
            Violation(
                "system-design-status",
                path,
                1,
                f"system-design status must be one of {sorted(VALID_DESIGN_STATUSES)}, found `{status or ''}`",
            )
        )
    if "requirements" not in metadata:
        violations.append(
            Violation(
                "system-design-requirements",
                path,
                1,
                "system design must declare a `requirements` list, including an explicit empty list",
            )
        )
    return violations


def collect_requirement_ids(path: Path, text: str) -> tuple[list[tuple[str, int]], list[tuple[str, int]]]:
    del path
    requirements = []
    criteria = []
    for line_number, line in enumerate(text.splitlines(), start=1):
        requirement_match = REQ_HEADING.match(line)
        if requirement_match:
            requirements.append((requirement_match.group("id"), line_number))
        acceptance_match = AC_DEFINITION.match(line)
        if acceptance_match:
            criteria.append((acceptance_match.group("id"), line_number))
    return requirements, criteria


def acceptance_parent(acceptance_id: str) -> str:
    return f"REQ-{acceptance_id.removeprefix('AC-').rsplit('.', 1)[0]}"


def check_duplicate_ids(kind: str, definitions: dict[str, list[tuple[Path, int]]]) -> list[Violation]:
    rule = f"duplicate-{kind}-id"
    violations = []
    for identifier, locations in definitions.items():
        if len(locations) < 2:
            continue
        first_path, first_line = locations[0]
        for path, line in locations[1:]:
            violations.append(
                Violation(
                    rule,
                    path,
                    line,
                    f"{identifier} duplicates {first_path}:{first_line}",
                )
            )
    return violations


def main() -> int:
    parser = argparse.ArgumentParser(description="Lint Kandev specification files.")
    parser.add_argument("--all", action="store_true", help="Lint all specification files")
    args = parser.parse_args()
    if not args.all:
        parser.print_usage(sys.stderr)
        return 2

    root = Path.cwd()
    violations = lint_specs(root)
    for violation in violations:
        relative = violation.path.relative_to(root)
        print(f"{relative}:{violation.line}: [{violation.rule}] {violation.message}")
    if violations:
        print(f"\n{len(violations)} specification lint violation(s) found.", file=sys.stderr)
        return 1
    print("All specification files passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
