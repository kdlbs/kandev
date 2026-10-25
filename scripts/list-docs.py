#!/usr/bin/env python3
"""List and validate architecture decisions and specifications."""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

try:
    from spec_metadata import (
        classify_path,
        metadata_text as spec_metadata_text,
        parse_frontmatter as parse_spec_frontmatter,
        validate_metadata,
    )
except ModuleNotFoundError:
    from scripts.spec_metadata import (
        classify_path,
        metadata_text as spec_metadata_text,
        parse_frontmatter as parse_spec_frontmatter,
        validate_metadata,
    )


SCHEMA_VERSION = 1
DECISION_METADATA_KEYS = {"status", "date", "area", "scope", "tags"}
SPEC_KINDS = ("system", "glossary", "requirement", "system-design", "product", "legacy")
SPEC_KIND_ORDER = {kind: index for index, kind in enumerate(SPEC_KINDS)}
FIELD_LINE = re.compile(
    r"^\*{0,2}(?P<key>Status|Date|Area|Scope|Tags)"
    r"(?::\s*\*{0,2}\s*|\*{0,2}\s*:\s*)(?P<value>.*?)\s*$",
    re.IGNORECASE,
)
HEADING_FIELD = re.compile(r"^##\s+(?P<key>Status|Date|Area|Scope|Tags)\s*$", re.IGNORECASE)
FIRST_HEADING = re.compile(r"^#\s+(?P<title>\S.*)$")
DECISION_ID_PREFIX = re.compile(
    r"^(?:ADR-)?(?:\d{4}|(?:\d{4}-\d{2}-\d{2}(?:-[a-z0-9-]+)?))\s*(?::|[—–])\s+",
    re.IGNORECASE,
)


class CatalogError(Exception):
    """A source file cannot be represented in the catalog."""


class SkipSource(Exception):
    """A Markdown file is an authoring entry page, not a catalog source."""


@dataclass(frozen=True)
class Document:
    type: str
    path: str
    title: str
    text: str
    id: str
    status: str | None = None
    area: str | None = None
    date: str | None = None
    system: str | None = None
    kind: str | None = None


def repository_path(root: Path, path: Path) -> str:
    try:
        return path.resolve().relative_to(root.resolve()).as_posix()
    except ValueError as exc:
        raise CatalogError(f"{path}: path is outside the repository root") from exc


def read_source(root: Path, path: Path) -> tuple[str, str]:
    relative = repository_path(root, path)
    if path.is_symlink() or not path.is_file():
        raise CatalogError(f"{relative}: source must be a regular file")
    try:
        return relative, path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as exc:
        raise CatalogError(f"{relative}: cannot read source file: {exc}") from exc


def first_heading(lines: list[str], start: int, relative: str) -> str:
    for line in lines[start:]:
        match = FIRST_HEADING.match(line)
        if match:
            return match.group("title").strip()
    raise CatalogError(f"{relative}: source must contain a level-one title heading")


def decision_title(heading: str) -> str:
    return DECISION_ID_PREFIX.sub("", heading, count=1).strip()


def normalize_field_value(value: str) -> str:
    return value.strip().strip("|").strip()


def metadata_key(raw_key: str) -> str:
    key = raw_key.strip().strip("*").lower()
    return "area" if key in {"scope", "tags"} else key


def add_metadata_value(
    fields: dict[str, str],
    raw_key: str,
    raw_value: str,
    relative: str,
) -> None:
    key = metadata_key(raw_key)
    value = normalize_field_value(raw_value)
    if not value:
        return
    if key == "area" and raw_key.strip().lower() != "area" and "area" in fields:
        return
    if key in fields:
        raise CatalogError(f"{relative}: duplicate {key} metadata in the ADR header")
    fields[key] = value


def parse_metadata_line(line: str) -> tuple[str, str] | None:
    stripped = line.strip()
    stripped = re.sub(r"^[-*]\s+", "", stripped)
    if stripped.startswith("|"):
        cells = [cell.strip() for cell in stripped.strip("|").split("|")]
        if len(cells) >= 2:
            key = cells[0].strip().strip("*")
            if key.lower() in DECISION_METADATA_KEYS:
                return key, cells[1]
        return None
    match = FIELD_LINE.match(stripped)
    if match:
        return match.group("key"), match.group("value")
    return None


def parse_decision_metadata(
    lines: list[str], start: int, frontmatter: dict[str, object], relative: str
) -> dict[str, str]:
    fields: dict[str, str] = {}
    for key in ("status", "date", "area"):
        value = frontmatter.get(key)
        if isinstance(value, list):
            value = ", ".join(str(item) for item in value)
        if isinstance(value, str):
            add_metadata_value(fields, key, value, relative)

    pending_key: str | None = None
    pending_value: list[str] = []

    def flush_pending() -> None:
        nonlocal pending_key, pending_value
        if pending_key is not None:
            add_metadata_value(fields, pending_key, " ".join(pending_value), relative)
        pending_key = None
        pending_value = []

    index = start
    while index < len(lines):
        line = lines[index]
        heading_match = HEADING_FIELD.match(line.strip())
        if heading_match:
            flush_pending()
            value_index = index + 1
            while value_index < len(lines) and not lines[value_index].strip():
                value_index += 1
            if value_index < len(lines) and not lines[value_index].lstrip().startswith("#"):
                pending_key = heading_match.group("key")
                pending_value = [lines[value_index].strip()]
                index = value_index + 1
                continue
        elif re.match(r"^##\s+", line):
            flush_pending()
            break

        parsed = parse_metadata_line(line)
        if parsed:
            flush_pending()
            pending_key = parsed[0]
            pending_value = [parsed[1]]
        elif not line.strip():
            flush_pending()
        elif pending_key is not None:
            pending_value.append(line.strip())
        index += 1
    flush_pending()
    if not fields.get("status"):
        raise CatalogError(f"{relative}: ADR metadata must define a non-empty status")
    date = fields.get("date")
    if date and not re.match(r"^\d{4}-\d{2}-\d{2}\b", date):
        raise CatalogError(f"{relative}: ADR date must start with YYYY-MM-DD")
    return fields


def parse_decision(root: Path, path: Path) -> Document:
    relative, text = read_source(root, path)
    lines = text.splitlines()
    parsed_frontmatter = parse_spec_frontmatter(text)
    if parsed_frontmatter.error:
        raise CatalogError(
            f"{relative}:{parsed_frontmatter.line}: {parsed_frontmatter.error}"
        )
    frontmatter = parsed_frontmatter.metadata or {}
    frontmatter_end = parsed_frontmatter.end
    heading = first_heading(lines, frontmatter_end, relative)
    fields = parse_decision_metadata(lines, frontmatter_end, frontmatter, relative)
    return Document(
        type="decision",
        path=relative,
        title=decision_title(heading),
        text=text,
        id=path.stem,
        status=fields.get("status"),
        area=fields.get("area"),
        date=fields.get("date"),
    )


def decision_files(root: Path) -> Iterable[Path]:
    directory = root / "docs" / "decisions"
    if not directory.exists():
        return ()
    return (path for path in sorted(directory.rglob("*.md")) if path.name != "INDEX.md")


def spec_files(root: Path) -> Iterable[Path]:
    directory = root / "docs" / "specs"
    if not directory.exists():
        return ()
    return (
        path
        for path in sorted(directory.rglob("*.md"))
        if path.name != "INDEX.md"
        and path != directory / "README.md"
        and "guide" not in path.relative_to(directory).parts
        and "templates" not in path.relative_to(directory).parts
    )


def parse_spec_kind(
    relative: str, frontmatter: dict[str, object] | None = None
) -> tuple[str, str | None]:
    del frontmatter
    inside = Path(relative).relative_to("docs/specs").parts
    if not inside:
        raise CatalogError(f"{relative}: specification path has no system directory")
    source_kind, source_system = classify_path(Path(relative))
    if source_kind in {"guide", "template"}:
        raise CatalogError(f"{relative}: authoring support files are not catalog sources")
    if source_kind == "system-index":
        if inside[-1] == "README.md":
            return "system", source_system
        return "glossary", source_system
    if source_kind in {"requirement", "system-design", "product", "legacy"}:
        return source_kind, source_system
    raise CatalogError(f"{relative}: unsupported specification source")


def validate_spec_metadata(
    relative: str,
    kind: str,
    system: str | None,
    metadata: dict[str, object],
    has_frontmatter: bool,
) -> None:
    issues = validate_metadata(kind, system, metadata, has_frontmatter)
    if issues:
        raise CatalogError(f"{relative}: {issues[0].message}")


def parse_spec(root: Path, path: Path) -> Document:
    relative, text = read_source(root, path)
    if relative == "docs/specs/product/README.md":
        raise SkipSource
    parsed_frontmatter = parse_spec_frontmatter(text)
    if parsed_frontmatter.error:
        raise CatalogError(
            f"{relative}:{parsed_frontmatter.line}: {parsed_frontmatter.error}"
        )
    metadata = parsed_frontmatter.metadata or {}
    frontmatter_end = parsed_frontmatter.end
    kind, system = parse_spec_kind(relative, metadata)
    has_frontmatter = bool(text.splitlines() and text.splitlines()[0] == "---")
    validate_spec_metadata(relative, kind, system, metadata, has_frontmatter)
    title = spec_metadata_text(metadata, "title")
    if not title:
        try:
            title = first_heading(text.splitlines(), frontmatter_end, relative)
        except CatalogError:
            title = " ".join(
                word.capitalize()
                for word in re.split(r"[-_]+", Path(relative).stem)
                if word
            )
    identifier = spec_metadata_text(metadata, "id") or relative.removesuffix(".md")
    return Document(
        type="specification",
        path=relative,
        title=title,
        text=text,
        id=identifier,
        status=spec_metadata_text(metadata, "status"),
        system=system,
        kind=kind,
    )


def load_documents(
    root: Path, files: Iterable[Path], parser
) -> tuple[list[Document], list[str]]:
    documents: list[Document] = []
    errors: list[str] = []
    identities: dict[str, str] = {}
    for path in files:
        try:
            document = parser(root, path)
        except SkipSource:
            continue
        except CatalogError as exc:
            errors.append(str(exc))
            continue
        identity = document.id if document.type == "decision" else f"{document.type}:{document.id}"
        if identity in identities:
            errors.append(
                f"{document.path}: duplicate catalog identity {document.id} "
                f"(already used by {identities[identity]})"
            )
            continue
        identities[identity] = document.path
        documents.append(document)
    return documents, sorted(errors)


def decision_sort_key(document: Document) -> tuple[int, int | str, str]:
    if re.match(r"^\d{4}-\d{2}-\d{2}-", document.id):
        return (1, document.id, document.path)
    numeric = re.match(r"^(\d+)(?:-|$)", document.id)
    if numeric:
        return (0, int(numeric.group(1)), document.path)
    return (2, document.id, document.path)


def spec_sort_key(document: Document) -> tuple[str, int, str]:
    assert document.kind is not None
    return (document.system or "", SPEC_KIND_ORDER[document.kind], document.path)


def filter_decisions(
    documents: Iterable[Document],
    status: str | None,
    area: str | None,
    text: str | None,
) -> list[Document]:
    result = []
    for document in documents:
        if status and leading_status_class(document.status) != status.lower():
            continue
        if area and not area_matches(document.area, area):
            continue
        if text and text.lower() not in document.text.lower():
            continue
        result.append(document)
    return sorted(result, key=decision_sort_key)


def leading_status_class(value: str | None) -> str:
    if not value:
        return ""
    match = re.match(r"^\s*([a-z][a-z0-9_-]*)", value, re.IGNORECASE)
    return match.group(1).lower() if match else ""


def area_matches(value: str | None, requested: str) -> bool:
    if not value:
        return False
    wanted = requested.strip().lower()
    if value.lower() == wanted:
        return True
    tokens = [token.strip().lower() for token in value.split(",") if token.strip()]
    if wanted in tokens:
        return True
    return wanted in {
        token.lower() for token in re.findall(r"[a-z0-9][a-z0-9_-]*", value)
    }


def filter_specs(
    documents: Iterable[Document],
    system: str | None,
    kind: str | None,
    status: str | None,
    text: str | None,
) -> list[Document]:
    result = []
    for document in documents:
        if system and (not document.system or document.system.lower() != system.lower()):
            continue
        if kind and document.kind != kind:
            continue
        if status and (not document.status or document.status.lower() != status.lower()):
            continue
        if text and text.lower() not in document.text.lower():
            continue
        result.append(document)
    return sorted(result, key=spec_sort_key)


def markdown_escape(value: object) -> str:
    return (
        str(value if value is not None else "")
        .replace("\\", "\\\\")
        .replace("|", "\\|")
        .replace("\n", " ")
    )


def markdown_table(documents: list[Document], document_type: str) -> str:
    if document_type == "decision":
        headers = ("ID", "Title", "Status", "Area", "Date", "Path")
        rows = [
            (
                document.id,
                document.title,
                document.status,
                document.area,
                document.date,
                document.path,
            )
            for document in documents
        ]
    else:
        headers = ("System", "Kind", "Status", "Title", "Path")
        rows = [
            (
                document.system,
                document.kind,
                document.status,
                document.title,
                document.path,
            )
            for document in documents
        ]
    lines = [
        "| " + " | ".join(headers) + " |",
        "| " + " | ".join("---" for _ in headers) + " |",
    ]
    lines.extend(
        "| " + " | ".join(markdown_escape(value) for value in row) + " |"
        for row in rows
    )
    return "\n".join(lines) + "\n"


def json_output(documents: list[Document]) -> str:
    items = []
    for document in documents:
        item = {
            "id": document.id,
            "path": document.path,
            "title": document.title,
            "type": document.type,
        }
        if document.type == "decision":
            item.update(
                {
                    "area": document.area,
                    "date": document.date,
                    "status": document.status,
                }
            )
        else:
            item.update(
                {
                    "kind": document.kind,
                    "status": document.status,
                    "system": document.system,
                }
            )
        items.append(item)
    return (
        json.dumps(
            {"schema_version": SCHEMA_VERSION, "documents": items},
            ensure_ascii=False,
            indent=2,
        )
        + "\n"
    )


def format_documents(
    documents: list[Document], output_format: str, document_type: str
) -> str:
    if output_format == "paths":
        return "".join(f"{document.path}\n" for document in documents)
    if output_format == "json":
        return json_output(documents)
    return markdown_table(documents, document_type)


def add_common_arguments(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--format",
        choices=("markdown", "paths", "json"),
        default="markdown",
        help="output format",
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="List and validate Kandev documentation sources."
    )
    parser.add_argument(
        "--root",
        type=Path,
        default=Path.cwd(),
        help="repository root (defaults to the current directory)",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    decisions = subparsers.add_parser("decisions", help="list architecture decisions")
    decisions.add_argument("--status")
    decisions.add_argument("--area")
    decisions.add_argument("--text")
    add_common_arguments(decisions)

    specs = subparsers.add_parser("specs", help="list specifications")
    specs.add_argument("--system")
    specs.add_argument("--kind", choices=SPEC_KINDS)
    specs.add_argument("--status")
    specs.add_argument("--text")
    add_common_arguments(specs)

    subparsers.add_parser("validate", help="validate both documentation catalogs")
    return parser


def run(args: argparse.Namespace) -> int:
    root = args.root.resolve()
    if not root.is_dir():
        print(f"error: repository root does not exist: {root}", file=sys.stderr)
        return 2

    if args.command == "decisions":
        documents, errors = load_documents(root, decision_files(root), parse_decision)
        if errors:
            for error in errors:
                print(f"error: {error}", file=sys.stderr)
            return 1
        selected = filter_decisions(documents, args.status, args.area, args.text)
        print(format_documents(selected, args.format, "decision"), end="")
        return 0

    if args.command == "specs":
        documents, errors = load_documents(root, spec_files(root), parse_spec)
        if errors:
            for error in errors:
                print(f"error: {error}", file=sys.stderr)
            return 1
        selected = filter_specs(documents, args.system, args.kind, args.status, args.text)
        print(format_documents(selected, args.format, "specification"), end="")
        return 0

    decisions, decision_errors = load_documents(root, decision_files(root), parse_decision)
    specs, spec_errors = load_documents(root, spec_files(root), parse_spec)
    errors = sorted(decision_errors + spec_errors)
    if errors:
        for error in errors:
            print(f"error: {error}", file=sys.stderr)
        return 1
    print(f"Validated {len(decisions)} decisions and {len(specs)} specifications.")
    return 0


def main() -> int:
    parser = build_parser()
    try:
        args = parser.parse_args()
        return run(args)
    except CatalogError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
