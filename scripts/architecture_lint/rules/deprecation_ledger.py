"""Find canonical handwritten Go and TypeScript deprecation annotations."""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path

from architecture_lint.model import Finding, Rule

from .frontend_imports import is_generated as is_generated_typescript
from .go_imports import is_generated as is_generated_go


RULE_ID = "ARCH-DEPRECATION-LEDGER"
GENERATED_PARTS = {"gen", "generated", "node_modules", "third_party", "vendor"}
TEST_PARTS = {"e2e", "fixtures", "test", "testdata", "tests", "__fixtures__", "__tests__"}
GENERATED_SUFFIXES = (
    ".gen.go",
    ".gen.ts",
    ".gen.tsx",
    ".gen.mts",
    ".gen.cts",
    ".generated.go",
    ".generated.ts",
    ".generated.tsx",
    ".generated.mts",
    ".generated.cts",
    ".pb.go",
)
TEST_SUFFIXES = ("_test.go", "_test.ts", "_test.tsx", "_test.mts", "_test.cts")
GO_MARKER = "Deprecated:"
TYPESCRIPT_MARKER = "@deprecated"
GO_DECLARATION = re.compile(r"^\s*(?:type|func|const|var)\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\b")
GO_METHOD = re.compile(r"^\s*func\s*\(([^)]*)\)\s*([A-Za-z_]\w*)\b")
GO_SCOPE = re.compile(r"\btype\s+([A-Za-z_]\w*)\s*(?:\[[^\]\n]+\])?\s+(struct|interface)\s*\{")
GO_FIELD = re.compile(r"^\s*([A-Za-z_]\w*)\s+")
GO_INTERFACE_METHOD = re.compile(r"^\s*([A-Za-z_]\w*)\s*\(")
TS_DECLARATION = re.compile(
    r"^\s*(?:(?:export|declare|default|abstract|async)\s+)*"
    r"(type|class|interface|enum|function|const|let|var)\s+([A-Za-z_$][\w$]*)\b"
)
TS_SCOPE = re.compile(r"\b(interface|class|enum)\s+([A-Za-z_$][\w$]*)\b[^{};]*$")
TS_TYPE_SCOPE = re.compile(r"\btype\s+([A-Za-z_$][\w$]*)\b[^{};]*=\s*[^{}]*$")
TS_MEMBER = re.compile(
    r"^\s*(?:(?:public|private|protected|static|readonly|declare|abstract|override|async|"
    r"accessor|get|set)\s+)*([A-Za-z_$][\w$]*)\s*(?:[?!])?"
    r"(?P<tail>[:(<=>,;\[]|$)"
)


@dataclass(frozen=True)
class Comment:
    start: int
    end: int
    line: int
    text: str
    kind: str


def applies_to(path: str) -> bool:
    parts = path.split("/")
    filename = parts[-1]
    if any(part in GENERATED_PARTS or part in TEST_PARTS for part in parts):
        return False
    if (
        filename.endswith(TEST_SUFFIXES)
        or ".test." in filename
        or ".spec." in filename
        or ".fixture." in filename
    ):
        return False
    if filename.endswith(GENERATED_SUFFIXES):
        return False
    return filename.endswith((".go", ".ts", ".tsx", ".mts", ".cts"))


def _blank(value: str) -> str:
    return "".join("\n" if char == "\n" else " " for char in value)


def _mask_source(source: str) -> tuple[str, list[Comment]]:
    """Hide literals and comments while retaining source offsets and lines."""

    masked = list(source)
    comments: list[Comment] = []
    index = 0
    while index < len(source):
        if source.startswith("//", index):
            end = source.find("\n", index + 2)
            if end < 0:
                end = len(source)
            comments.append(
                Comment(index, end, source.count("\n", 0, index) + 1, source[index + 2 : end], "line")
            )
            masked[index:end] = _blank(source[index:end])
            index = end
        elif source.startswith("/*", index):
            close = source.find("*/", index + 2)
            end = len(source) if close < 0 else close + 2
            comment_text = source[index + 2 : close] if close >= 0 else source[index + 2 : end]
            comments.append(
                Comment(
                    index,
                    end,
                    source.count("\n", 0, index) + 1,
                    comment_text,
                    "jsdoc" if source.startswith("/**", index) else "block",
                )
            )
            masked[index:end] = _blank(source[index:end])
            index = end
        elif source[index] in {'"', "'", "`"}:
            quote = source[index]
            start = index
            index += 1
            while index < len(source):
                if source[index] == "\\":
                    index += 2
                elif source[index] == quote:
                    index += 1
                    break
                else:
                    index += 1
            masked[start:index] = _blank(source[start:index])
        else:
            index += 1
    return "".join(masked), comments


def _comment_has_go_marker(comment: Comment) -> int | None:
    if comment.kind != "line":
        return None
    for offset, line in enumerate(comment.text.splitlines() or [comment.text]):
        cleaned = line.strip()
        if cleaned.startswith("*"):
            cleaned = cleaned[1:].strip()
        if re.match(r"^Deprecated:\s*\S", cleaned):
            return comment.line + offset
    return None


def _comment_has_typescript_marker(comment: Comment) -> int | None:
    if comment.kind != "jsdoc":
        return None
    for offset, line in enumerate(comment.text.splitlines() or [comment.text]):
        cleaned = line.strip()
        if cleaned.startswith("*"):
            cleaned = cleaned[1:].strip()
        if re.match(r"^@deprecated(?:\s|$)", cleaned):
            return comment.line + offset
    return None


def _next_code_index(masked: str, start: int) -> int | None:
    while start < len(masked) and masked[start].isspace():
        start += 1
    return start if start < len(masked) else None


def _is_attached(source: str, comment_end: int, target: int) -> bool:
    gap = source[comment_end:target]
    return re.search(r"(?:\r\n|\r|\n)[ \t]*(?:\r\n|\r|\n)", gap) is None


def _go_contexts(masked: str) -> list[tuple[int, tuple[str, str] | None]]:
    depth = 0
    scopes: list[tuple[str, str, int]] = []
    contexts: list[tuple[int, tuple[str, str] | None]] = []
    for line in masked.splitlines():
        scope = (scopes[-1][1], scopes[-1][0]) if scopes and scopes[-1][2] == depth else None
        contexts.append((depth, scope))
        open_scope = GO_SCOPE.search(line)
        depth += line.count("{") - line.count("}")
        while scopes and scopes[-1][2] > depth:
            scopes.pop()
        if open_scope and depth > 0:
            scopes.append((open_scope.group(1), open_scope.group(2), depth))
    return contexts


def _go_declaration(code: str, context: tuple[int, tuple[str, str] | None]) -> str | None:
    depth, scope = context
    if scope is not None:
        scope_kind, scope_name = scope
        if scope_kind == "interface":
            method = GO_INTERFACE_METHOD.match(code)
            if method:
                return f"method:{scope_name}.{method.group(1)}"
        field = GO_FIELD.match(code)
        return f"field:{scope_name}.{field.group(1)}" if field else None
    if depth != 0:
        return None
    method = GO_METHOD.match(code)
    if method:
        receiver = method.group(1).strip().lstrip("*").split()[-1]
        return f"method:{receiver}.{method.group(2)}"
    declaration = GO_DECLARATION.match(code)
    if declaration:
        kind = re.search(r"\b(type|func|const|var)\b", code)
        if kind:
            return f"{kind.group(1)}:{declaration.group(1)}"
    return None


def _go_findings(masked: str, comments: list[Comment], source: str) -> list[tuple[int, str, str]]:
    contexts = _go_contexts(masked)
    lines = masked.splitlines()
    findings: list[tuple[int, str, str]] = []
    for comment in comments:
        marker_line = _comment_has_go_marker(comment)
        if marker_line is None:
            continue
        comment_line_index = comment.line - 1
        line_start = masked.rfind("\n", 0, comment.start) + 1
        prefix = masked[line_start : comment.start]
        if prefix.strip():
            declaration = _go_declaration(prefix, contexts[comment_line_index])
            if declaration:
                findings.append((marker_line, declaration, GO_MARKER))
            continue
        target_index = _next_code_index(masked, comment.end)
        if target_index is None or not _is_attached(source, comment.end, target_index):
            continue
        target_line = masked.count("\n", 0, target_index) + 1
        if target_line <= len(lines):
            declaration = _go_declaration(lines[target_line - 1], contexts[target_line - 1])
            if declaration:
                findings.append((marker_line, declaration, GO_MARKER))
    return findings


def _typescript_scope(masked: str, position: int) -> str | None:
    scopes: list[str | None] = []
    index = 0
    while index < position:
        if masked[index] == "{":
            boundary = max(
                masked.rfind(";", 0, index),
                masked.rfind("{", 0, index),
                masked.rfind("}", 0, index),
            ) + 1
            header = masked[boundary:index]
            scope_match = TS_SCOPE.search(header)
            if scope_match:
                scopes.append(scope_match.group(2))
            else:
                type_match = TS_TYPE_SCOPE.search(header)
                scopes.append(type_match.group(1) if type_match else None)
        elif masked[index] == "}" and scopes:
            scopes.pop()
        index += 1
    return scopes[-1] if scopes else None


def _typescript_declaration(code: str, scope: str | None) -> str | None:
    if scope:
        member = TS_MEMBER.match(code)
        if not member:
            return None
        kind = "method" if member.group("tail").startswith(("(", "<")) else "property"
        return f"{kind}:{scope}.{member.group(1)}"
    declaration = TS_DECLARATION.match(code)
    if declaration:
        return f"{declaration.group(1)}:{declaration.group(2)}"
    return None


def _typescript_findings(
    masked: str, comments: list[Comment], source: str
) -> list[tuple[int, str, str]]:
    findings: list[tuple[int, str, str]] = []
    for comment in comments:
        marker_line = _comment_has_typescript_marker(comment)
        if marker_line is None:
            continue
        target_index = _next_code_index(masked, comment.end)
        if target_index is None or not _is_attached(source, comment.end, target_index):
            continue
        line_end = masked.find("\n", target_index)
        if line_end < 0:
            line_end = len(masked)
        code = masked[target_index:line_end]
        scope = _typescript_scope(masked, target_index)
        declaration = _typescript_declaration(code, scope)
        if declaration:
            findings.append((marker_line, declaration, TYPESCRIPT_MARKER))
    return findings


def find_declarations(path: str, source: str) -> list[tuple[int, str, str]]:
    """Return supported annotations as line, normalized declaration, and marker."""

    if not applies_to(path):
        return []
    if path.endswith(".go"):
        if is_generated_go(source):
            return []
        masked, comments = _mask_source(source)
        return _go_findings(masked, comments, source)
    if is_generated_typescript(source):
        return []
    masked, comments = _mask_source(source)
    return _typescript_findings(masked, comments, source)


def scan(path: str, source: str) -> list[Finding]:
    findings: list[Finding] = []
    occurrences: dict[str, int] = {}
    for line, declaration, marker in find_declarations(path, source):
        occurrences[declaration] = occurrences.get(declaration, 0) + 1
        identity_declaration = f"{declaration}#{occurrences[declaration]}"
        findings.append(
            Finding.create(
                RULE_ID,
                path,
                line,
                {"path": path, "declaration": identity_declaration, "marker": marker},
                (
                    f"deprecated declaration {identity_declaration} ({marker}) is unregistered; "
                    "add a matching compatibility-ledger entry with locator.path, "
                    "locator.declaration, and locator.marker"
                ),
            )
        )
    return findings


RULE = Rule(
    id=RULE_ID,
    slug="deprecation_ledger",
    baseline_path=Path("config/architecture-lint/deprecation_ledger.json"),
    applies_to=applies_to,
    scan=scan,
)
