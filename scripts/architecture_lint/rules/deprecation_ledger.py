"""Find canonical handwritten Go and TypeScript deprecation annotations."""

from __future__ import annotations

import ast
import json
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
GO_TYPE_SPEC_SCOPE = re.compile(
    r"^\s*([A-Za-z_]\w*)\s*(?:\[[^\]\n]+\])?\s+(struct|interface)\s*\{"
)
GO_FIELD = re.compile(r"^\s*([A-Za-z_]\w*)\s+")
GO_EMBEDDED_FIELD = re.compile(
    r"^\s*\*?(?:[A-Za-z_]\w*\s*\.\s*)?([A-Za-z_]\w*)"
    r"(?:\s*\[[^\]\n]+\])?(?:\s|$|`)"
)
GO_INTERFACE_METHOD = re.compile(r"^\s*([A-Za-z_]\w*)\s*\(")
GO_GROUP = re.compile(r"^\s*(type|const|var)\s*\(")
GO_GROUPED_NAMES = re.compile(r"^\s*([A-Za-z_]\w*(?:\s*,\s*[A-Za-z_]\w*)*)\b")
TS_DECLARATION = re.compile(
    r"^\s*(?:(?:export|declare|default|abstract|async)\s+)*"
    r"(type|class|interface|enum|function|const|let|var)\s+([A-Za-z_$][\w$]*)\b"
)
TS_SCOPE = re.compile(r"\b(interface|class|enum)\s+([A-Za-z_$][\w$]*)\b[^{};]*$")
TS_TYPE_SCOPE = re.compile(r"\btype\s+([A-Za-z_$][\w$]*)\b[^{};]*$")
TS_MEMBER = re.compile(
    r"^\s*(?:(?:public|private|protected|static|readonly|declare|abstract|override|async|"
    r"accessor|get|set)\s+)*(?P<key>"
    r"[A-Za-z_$][\w$]*|\d+(?:\.\d+)?|\"(?:\\.|[^\"\\])*\"|"
    r"'(?:\\.|[^'\\])*'|\[[^\]\n]+\])\s*(?:[?!])?"
    r"(?P<tail>[:(<=>,;]|$)"
)
TS_DECORATOR = re.compile(r"@[A-Za-z_$][\w$]*(?:\s*\.\s*[A-Za-z_$][\w$]*)*")


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
                elif source[index] == "\n" and quote != "`":
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


def _next_typescript_code_index(masked: str, start: int, source: str) -> int | None:
    target = _next_code_index(masked, start)
    raw_target = _next_code_index(source, start)
    if raw_target is not None and source[raw_target] in {'"', "'"}:
        target = raw_target
    while target is not None and masked[target] == "@":
        decorator = TS_DECORATOR.match(masked, target)
        if decorator is None:
            return target
        cursor = decorator.end()
        while (
            cursor < len(masked)
            and masked[cursor].isspace()
            and masked[cursor] != "\n"
        ):
            cursor += 1
        while cursor < len(masked) and masked[cursor] == "(":
            depth = 0
            while cursor < len(masked):
                if masked[cursor] == "(":
                    depth += 1
                elif masked[cursor] == ")":
                    depth -= 1
                    if depth == 0:
                        cursor += 1
                        break
                cursor += 1
        while cursor < len(masked) and masked[cursor].isspace():
            cursor += 1
        target = cursor if cursor < len(masked) else None
    return target


def _is_attached(source: str, comment_end: int, target: int) -> bool:
    gap = source[comment_end:target]
    return re.search(r"(?:\r\n|\r|\n)[ \t]*(?:\r\n|\r|\n)", gap) is None


def _go_contexts(masked: str) -> list[tuple[int, tuple[str, str] | None, str | None]]:
    depth = 0
    paren_depth = 0
    scopes: list[tuple[str, str, int]] = []
    groups: list[tuple[str, int]] = []
    contexts: list[tuple[int, tuple[str, str] | None, str | None]] = []
    for line in masked.splitlines():
        scope = (scopes[-1][1], scopes[-1][0]) if scopes and scopes[-1][2] == depth else None
        group_kind = groups[-1][0] if groups else None
        contexts.append((depth, scope, group_kind))
        open_scope = GO_SCOPE.search(line)
        if open_scope is None and group_kind == "type":
            open_scope = GO_TYPE_SPEC_SCOPE.match(line)
        depth += line.count("{") - line.count("}")
        while scopes and scopes[-1][2] > depth:
            scopes.pop()
        if open_scope and depth > 0:
            scopes.append((open_scope.group(1), open_scope.group(2), depth))
        group = GO_GROUP.match(line)
        paren_before = paren_depth
        paren_depth += line.count("(") - line.count(")")
        if group:
            groups.append((group.group(1), paren_before))
        while groups and paren_depth <= groups[-1][1]:
            groups.pop()
    return contexts


def _go_declaration(
    code: str, context: tuple[int, tuple[str, str] | None, str | None]
) -> list[str]:
    depth, scope, group_kind = context
    if scope is not None:
        scope_kind, scope_name = scope
        if scope_kind == "interface":
            method = GO_INTERFACE_METHOD.match(code)
            if method:
                return [f"method:{scope_name}.{method.group(1)}"]
        field = GO_FIELD.match(code)
        if field:
            return [f"field:{scope_name}.{field.group(1)}"]
        embedded = GO_EMBEDDED_FIELD.match(code)
        return [f"field:{scope_name}.{embedded.group(1)}"] if embedded else []
    if depth != 0:
        return []
    if group_kind:
        if group_kind in {"const", "var"}:
            code = code.split("=", 1)[0]
        grouped = GO_GROUPED_NAMES.match(code)
        if grouped:
            names = re.findall(r"[A-Za-z_]\w*", grouped.group(1))
            return [f"{group_kind}:{name}" for name in names]
        return []
    method = GO_METHOD.match(code)
    if method:
        receiver = method.group(1).strip().lstrip("*").split()[-1]
        return [f"method:{receiver}.{method.group(2)}"]
    declaration = GO_DECLARATION.match(code)
    if declaration:
        kind = re.search(r"\b(type|func|const|var)\b", code)
        if kind:
            return [f"{kind.group(1)}:{declaration.group(1)}"]
    return []


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
            findings.extend((marker_line, item, GO_MARKER) for item in declaration)
            continue
        target_index = _next_code_index(masked, comment.end)
        if target_index is None or not _is_attached(source, comment.end, target_index):
            continue
        target_line = masked.count("\n", 0, target_index) + 1
        if target_line <= len(lines):
            declaration = _go_declaration(lines[target_line - 1], contexts[target_line - 1])
            findings.extend((marker_line, item, GO_MARKER) for item in declaration)
    return findings


def _typescript_scope(
    masked: str, position: int, source: str
) -> tuple[str | None, bool]:
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
                if type_match:
                    scopes.append(type_match.group(1))
                else:
                    parent_scope = scopes[-1] if scopes else None
                    member = TS_MEMBER.match(header)
                    if member is None:
                        member = TS_MEMBER.match(source[boundary:index])
                    if (
                        parent_scope
                        and member
                        and header.rstrip().endswith((":", "extends", "<"))
                    ):
                        key = _canonical_typescript_key(member.group("key"))
                        key_path = key if key.startswith("[") else f".{key}"
                        scopes.append(f"{parent_scope}{key_path}")
                    else:
                        scopes.append(None)
        elif masked[index] == "}" and scopes:
            scopes.pop()
        index += 1
    return (scopes[-1], False) if scopes else (None, True)


def _compact_computed_key(expression: str) -> str:
    compact: list[str] = []
    quote: str | None = None
    escaped = False
    for char in expression:
        if quote:
            compact.append(char)
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == quote:
                quote = None
        elif char in {'"', "'", "`"}:
            quote = char
            compact.append(char)
        elif not char.isspace():
            compact.append(char)
    return "".join(compact)


def _canonical_typescript_key(key: str) -> str:
    if key.startswith(("'", '"')):
        try:
            value = ast.literal_eval(key)
        except (SyntaxError, ValueError):
            value = key[1:-1]
        return f"[{json.dumps(value, ensure_ascii=False)}]"
    if key.startswith("["):
        return f"[{_compact_computed_key(key[1:-1])}]"
    if key[0].isdigit():
        return f"[{key}]"
    return key


def _typescript_declaration(
    code: str, scope: str | None, raw_code: str | None = None, top_level: bool = True
) -> str | None:
    if scope:
        member = TS_MEMBER.match(raw_code if raw_code is not None else code)
        if not member:
            return None
        kind = "method" if member.group("tail").startswith(("(", "<")) else "property"
        key = _canonical_typescript_key(member.group("key"))
        qualified_key = key if key.startswith("[") else f".{key}"
        return f"{kind}:{scope}{qualified_key}"
    if not top_level:
        return None
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
        target_index = _next_typescript_code_index(masked, comment.end, source)
        if target_index is None or not _is_attached(source, comment.end, target_index):
            continue
        line_end = masked.find("\n", target_index)
        if line_end < 0:
            line_end = len(masked)
        code = masked[target_index:line_end]
        raw_code = source[target_index:line_end]
        scope, top_level = _typescript_scope(masked, target_index, source)
        declaration = _typescript_declaration(code, scope, raw_code, top_level)
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
