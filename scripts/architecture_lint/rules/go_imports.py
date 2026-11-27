"""Minimal Go import parsing shared by import-direction rules."""

import re


IMPORT_LINE = re.compile(r'^\s*(?:(?:[A-Za-z_][\w]*|\.|_)\s+)?["`]([^"`]+)["`]')
SINGLE_IMPORT = re.compile(
    r'^\s*import\s+(?:(?:[A-Za-z_][\w]*|\.|_)\s+)?["`]([^"`]+)["`]'
)
GENERATED_HEADER = re.compile(r'^\s*//\s*Code generated .* DO NOT EDIT\.\s*$')


def import_lines(source: str) -> list[tuple[int, str]]:
    imports: list[tuple[int, str]] = []
    in_block = False
    for line_number, line in enumerate(source.splitlines(), start=1):
        stripped = line.strip()
        if not in_block:
            if re.match(r"^import\s*\(", stripped):
                in_block = True
                continue
            match = SINGLE_IMPORT.match(line)
            if match:
                imports.append((line_number, match.group(1)))
            continue
        if stripped.startswith(")"):
            in_block = False
            continue
        match = IMPORT_LINE.match(line)
        if match:
            imports.append((line_number, match.group(1)))
    return imports


def is_import_at_or_below(import_path: str, root: str) -> bool:
    return import_path == root or import_path.startswith(root + "/")


def is_generated(source: str) -> bool:
    """Recognize the standard Go generated-code header near the file start."""

    return any(GENERATED_HEADER.match(line) for line in source.splitlines()[:10])
