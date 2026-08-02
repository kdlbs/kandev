"""Prevent new unsafe casts in frontend root-state composition."""

from pathlib import Path

from architecture_lint.model import Finding, Rule


RULE_ID = "ARCH-FRONTEND-ROOT-STATE-CAST"
STORE_PATH = "apps/web/lib/state/store.ts"


def applies_to(path: str) -> bool:
    return path == STORE_PATH


def normalize_marker(line: str) -> str:
    return " ".join(line.strip().split())


def _line_number(source: str, index: int) -> int:
    return source.count("\n", 0, index) + 1


def _skip_quoted(source: str, index: int, quote: str) -> int:
    index += 1
    while index < len(source):
        if source[index] == "\\":
            index += 2
        elif source[index] == quote:
            return index + 1
        else:
            index += 1
    return index


def _skip_comment(source: str, index: int) -> int:
    if source.startswith("//", index):
        newline = source.find("\n", index + 2)
        return len(source) if newline < 0 else newline
    end = source.find("*/", index + 2)
    return len(source) if end < 0 else end + 2


def _scan_template(source: str, index: int, tokens: list[tuple[str, int]]) -> int:
    """Skip template text while recursively tokenizing `${...}` expressions."""

    index += 1
    while index < len(source):
        char = source[index]
        if char == "\\":
            index += 2
        elif char == "`":
            return index + 1
        elif source.startswith("${", index):
            index = _scan_code(source, index + 2, tokens, stop_at_brace=True)
        else:
            index += 1
    return index


def _scan_code(
    source: str,
    index: int,
    tokens: list[tuple[str, int]],
    *,
    stop_at_brace: bool = False,
) -> int:
    brace_depth = 0
    while index < len(source):
        char = source[index]
        if char.isspace():
            index += 1
        elif source.startswith("//", index) or source.startswith("/*", index):
            index = _skip_comment(source, index)
        elif char in {'"', "'"}:
            index = _skip_quoted(source, index, char)
        elif char == "`":
            index = _scan_template(source, index, tokens)
        elif char == "{":
            brace_depth += 1
            index += 1
        elif char == "}":
            if stop_at_brace and brace_depth == 0:
                return index + 1
            brace_depth = max(0, brace_depth - 1)
            index += 1
        elif char.isalpha() or char in "_$":
            start = index
            index += 1
            while index < len(source) and (source[index].isalnum() or source[index] in "_$"):
                index += 1
            tokens.append((source[start:index], _line_number(source, start)))
        else:
            index += 1
    return index


def unsafe_casts(source: str) -> list[tuple[int, str]]:
    tokens: list[tuple[str, int]] = []
    _scan_code(source, 0, tokens)

    findings: list[tuple[int, str]] = []
    for index, (token, line) in enumerate(tokens):
        if token == "as" and index + 1 < len(tokens) and tokens[index + 1][0] == "any":
            findings.append((line, "as any"))
        elif (
            token == "as"
            and index + 2 < len(tokens)
            and tokens[index + 1][0] == "unknown"
            and tokens[index + 2][0] == "as"
        ):
            findings.append((line, "as unknown as"))
    return findings


def scan(path: str, source: str) -> list[Finding]:
    findings: list[Finding] = []
    source_lines = source.splitlines()
    occurrences: dict[tuple[str, str], int] = {}
    for line_number, escape in unsafe_casts(source):
        marker = normalize_marker(source_lines[line_number - 1])
        occurrence_key = (marker, escape)
        occurrences[occurrence_key] = occurrences.get(occurrence_key, 0) + 1
        findings.append(
            Finding.create(
                RULE_ID,
                path,
                line_number,
                {
                    "path": path,
                    "marker": marker,
                    "escape": escape,
                    "occurrence": occurrences[occurrence_key],
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
