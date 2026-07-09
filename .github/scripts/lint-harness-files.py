#!/usr/bin/env python3
"""Lint Kandev harness files.

Usage:
  python3 .github/scripts/lint-harness-files.py --all
  python3 .github/scripts/lint-harness-files.py AGENTS.md .agents/skills/fix/SKILL.md
"""

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

try:
    import tomllib
except ModuleNotFoundError:  # Python < 3.11
    try:
        import tomli as tomllib
    except ModuleNotFoundError:
        tomllib = None


DEFAULT_RULES_BY_KIND = {
    "agent": {
        "agent-line-limit": {"limit": 300},
    },
    "skill": {
        "skill-line-limit": {"limit": 500},
        "description-word-limit": {"limit": 600},
    },
    "reference": {
        "reference-line-limit": {"limit": 400},
    },
    "role-agent": {
        "role-agent-line-limit": {"limit": 300},
        "description-word-limit": {"limit": 600},
    },
    "command": {
        "command-line-limit": {"limit": 300},
        "description-word-limit": {"limit": 600},
    },
    "cursor-rule": {
        "cursor-rule-line-limit": {"limit": 300},
        "description-word-limit": {"limit": 600},
    },
    "config": {
        "config-line-limit": {"limit": 300},
    },
}

LINE_LIMIT_RULES = {
    "agent-line-limit",
    "skill-line-limit",
    "reference-line-limit",
    "role-agent-line-limit",
    "command-line-limit",
    "cursor-rule-line-limit",
    "config-line-limit",
}

OPT_IN_RULES = {
    "description-when",
    "no-at-dot-slash",
    "no-emoji",
    "no-em-dash",
}

OPT_IN_RULES_BY_KIND = {
    "description-when": {"skill", "role-agent", "command", "cursor-rule"},
    "no-at-dot-slash": set(DEFAULT_RULES_BY_KIND),
    "no-emoji": set(DEFAULT_RULES_BY_KIND),
    "no-em-dash": set(DEFAULT_RULES_BY_KIND),
}

YAML_BLOCK_SCALAR_MARKERS = {">", "|", ">-", "|-", ">+", "|+"}

EMOJI_RANGES = (
    (0x2600, 0x27BF),
    (0x1F000, 0x1FAFF),
)


class Violation:
    def __init__(self, rule: str, path: Path, line: int, msg: str) -> None:
        self.rule = rule
        self.path = path
        self.line = line
        self.msg = msg

    def __repr__(self) -> str:
        return f"Violation(rule={self.rule!r}, path={str(self.path)!r}, line={self.line!r}, msg={self.msg!r})"


def main() -> int:
    parser = argparse.ArgumentParser(description="Lint Kandev harness files.")
    parser.add_argument("targets", nargs="*", help="Files or directories to lint")
    parser.add_argument("--all", action="store_true", help="Lint every tracked harness file")
    parser.add_argument("--disable", action="append", default=[], help="Disable one rule by id")
    parser.add_argument("--enable", action="append", default=[], help="Enable one opt-in rule by id")
    args = parser.parse_args()

    if not args.all and not args.targets:
        parser.print_usage(sys.stderr)
        return 2

    cwd = Path.cwd()
    try:
        paths = discover_all(cwd) if args.all else discover_targets([Path(target) for target in args.targets], cwd)
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 2

    disabled = set(args.disable)
    enabled = set(args.enable)
    unknown_enabled = sorted(enabled - OPT_IN_RULES)
    if unknown_enabled:
        print(f"unknown opt-in rule(s): {', '.join(unknown_enabled)}", file=sys.stderr)
        return 2
    violations = []
    linted = 0

    try:
        for path in paths:
            if not path.exists():
                continue
            kind = classify_path(path)
            if kind == "unknown":
                continue
            linted += 1
            rules = dict(DEFAULT_RULES_BY_KIND.get(kind, {}))
            for rule in enabled:
                if kind not in OPT_IN_RULES_BY_KIND[rule]:
                    continue
                rules.setdefault(rule, {})
            violations.extend(lint_path(path, kind, rules=rules, disabled=disabled))
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 2

    for violation in violations:
        print(format_violation(violation, cwd))
        if os.environ.get("GITHUB_ACTIONS") == "true":
            print(format_github_annotation(violation, cwd))

    if violations:
        print(f"\n{len(violations)} harness lint violation(s) found.", file=sys.stderr)
        return 1

    print(f"All {linted} harness file(s) passed.")
    return 0


def discover_all(cwd: Path) -> list[Path]:
    result = subprocess.run(
        ["git", "ls-files", "-z"],
        cwd=cwd,
        text=False,
        capture_output=True,
        check=False,
    )
    if result.returncode != 0:
        stderr = result.stderr.decode("utf-8", errors="replace").strip()
        raise RuntimeError(stderr or "git ls-files failed")
    names = [name.decode("utf-8", errors="replace") for name in result.stdout.split(b"\0") if name]
    return [cwd / name for name in names]


def discover_targets(targets: list[Path], cwd: Path) -> list[Path]:
    paths = []
    for target in targets:
        path = target if target.is_absolute() else cwd / target
        if not path.exists():
            continue
        if path.is_dir() and not path.is_symlink():
            paths.extend(walk_without_symlink_dirs(path))
            continue
        paths.append(path)
    return sorted(paths)


def walk_without_symlink_dirs(root: Path) -> list[Path]:
    paths = []
    for dirpath, dirnames, filenames in os.walk(root, followlinks=False):
        current = Path(dirpath)
        dirnames[:] = [name for name in dirnames if not (current / name).is_symlink()]
        for filename in filenames:
            paths.append(current / filename)
    return paths


def classify_path(path: Path) -> str:
    name = path.name
    if name in {"AGENTS.md", "CLAUDE.md", ".cursorrules"}:
        return "agent"

    parts = normalized_parts(path)
    if (
        has_subpath(parts, [".agents", "skills"])
        or has_subpath(parts, [".augment", "skills"])
        or has_subpath(parts, [".claude", "skills"])
        or has_subpath(parts, [".cursor", "skills"])
        or has_subpath(parts, [".opencode", "skills"])
    ):
        if name == "SKILL.md":
            return "skill"
        if name.endswith(".md"):
            return "reference"

    if has_subpath(parts, ["apps", "backend", "internal", "office", "configloader", "skills"]):
        if name == "SKILL.md":
            return "skill"
        if name.endswith(".md"):
            return "reference"

    if has_subpath(parts, [".agents", "agents"]) and name.endswith(".md"):
        return "role-agent"
    if has_subpath(parts, [".opencode", "agents"]) and name.endswith(".md"):
        return "role-agent"
    if has_subpath(parts, [".claude", "agents"]) and name.endswith(".md"):
        return "role-agent"
    if has_subpath(parts, [".codex", "agents"]) and name.endswith(".toml"):
        return "role-agent"
    if has_subpath(parts, [".codex"]) and name == "config.toml":
        return "config"
    if has_subpath(parts, [".claude"]) and name == "settings.json":
        return "config"
    if has_subpath(parts, [".claude", "commands"]) and name.endswith(".md"):
        return "command"
    if has_subpath(parts, [".claude", "rules"]) and name.endswith(".md"):
        return "cursor-rule"
    if has_subpath(parts, [".cursor", "rules"]) and name.endswith(".mdc"):
        return "cursor-rule"

    return "unknown"


def normalized_parts(path: Path) -> list[str]:
    return [part for part in path.as_posix().split("/") if part not in {"", "."}]


def has_subpath(parts: list[str], needle: list[str]) -> bool:
    if len(parts) < len(needle):
        return False
    for index in range(0, len(parts) - len(needle) + 1):
        if parts[index : index + len(needle)] == needle:
            return True
    return False


def lint_path(
    path: Path,
    kind: str | None = None,
    rules: dict[str, dict[str, int]] | None = None,
    disabled: set[str] | None = None,
) -> list[Violation]:
    kind = kind or classify_path(path)
    if kind == "unknown":
        return []
    rules = rules if rules is not None else DEFAULT_RULES_BY_KIND.get(kind, {})
    disabled = disabled or set()
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()

    violations = []
    for rule, options in rules.items():
        if rule in disabled:
            continue
        if rule in OPT_IN_RULES_BY_KIND and kind not in OPT_IN_RULES_BY_KIND[rule]:
            continue
        if rule in LINE_LIMIT_RULES:
            violations.extend(check_line_limit(path, rule, lines, int(options.get("limit", 0))))
        elif rule == "description-word-limit":
            violations.extend(check_description_word_limit(path, text, int(options.get("limit", 0))))
        elif rule == "no-emoji":
            violations.extend(check_no_emoji(path, lines))
        elif rule == "no-em-dash":
            violations.extend(check_no_em_dash(path, lines))
        elif rule == "no-at-dot-slash":
            violations.extend(check_no_at_dot_slash(path, lines))
        elif rule == "description-when":
            violations.extend(check_description_when(path, text))
    return violations


def check_line_limit(path: Path, rule: str, lines: list[str], limit: int) -> list[Violation]:
    if limit <= 0:
        return []
    count = len(lines)
    if count <= limit:
        return []
    return [Violation(rule, path, 0, line_limit_message(rule, count, limit))]


def line_limit_message(rule: str, count: int, limit: int) -> str:
    prefix = f"file is {count} lines (limit {limit}, over by {count - limit})."
    target = f"target: {limit} lines or fewer."
    if rule == "agent-line-limit":
        return (
            f"{prefix} why: this file is reloaded into the agent's context on every turn; every line here is paid for "
            "in every conversation. fix: trim down to the essential rules and routing signals. move detailed examples, "
            f"command catalogues, or reference material into a sibling file (e.g. REFERENCE.md, EXAMPLES.md) and link "
            f"to it from this file. {target}"
        )
    if rule == "skill-line-limit":
        return (
            f"{prefix} why: when this skill triggers, the body loads into context and competes with the running "
            "conversation. fix: keep SKILL.md focused on (1) when to trigger and (2) the core steps. move long examples, "
            f"command catalogues, troubleshooting, or reference material into sibling files (REFERENCE.md, EXAMPLES.md, "
            f"examples/*.md) and link them from SKILL.md. {target}"
        )
    if rule == "reference-line-limit":
        return (
            f"{prefix} why: oversize reference files signal a monolithic skill that is hard to navigate and slow to load "
            "on demand. fix: split this file into multiple focused files in the same skill directory (one per topic) "
            f"and update the SKILL.md links. {target}"
        )
    if rule == "role-agent-line-limit":
        return (
            f"{prefix} why: agent definitions are loaded into model/router context when selecting or spawning specialist "
            "agents. fix: keep the file focused on role, tool constraints, and workflow; move long examples or command "
            f"catalogues into referenced files. {target}"
        )
    if rule == "command-line-limit":
        return (
            f"{prefix} why: slash-command prompts are loaded when invoked and compete with the active task context. "
            "fix: keep the command compact and move long examples, command catalogues, or rationale into referenced "
            f"files. {target}"
        )
    if rule == "cursor-rule-line-limit":
        return (
            f"{prefix} why: Cursor rules are routing and style context that should stay concise. fix: move detailed "
            f"examples or long reference material into sibling files and link to them. {target}"
        )
    if rule == "config-line-limit":
        return (
            f"{prefix} why: agent config files are read when spawning or routing agent runs, and bloated config obscures "
            "the active model, permission, and tool choices. fix: keep config files to the settings the runtime needs; "
            f"move rationale, examples, or setup notes into a referenced markdown file. {target}"
        )
    return f"{prefix} {target}"


def check_description_word_limit(path: Path, text: str, limit: int) -> list[Violation]:
    if limit <= 0:
        return []
    description = extract_description(path, text)
    if not description:
        return []
    count = len(description.split())
    if count <= limit:
        return []
    message = (
        f"description is {count} words (limit {limit}, over by {count - limit}). why: harness descriptions are loaded "
        "into routing context before the body is selected, and long descriptions compete with discovery of sibling "
        "artifacts. fix: keep the description to a single routing sentence ('Use when X. Do Y.') and move details, "
        f"examples, command lists, or rationale into the body or a referenced file. target: {limit} words or fewer."
    )
    return [Violation("description-word-limit", path, 1, message)]


def extract_description(path: Path, text: str) -> str | None:
    if path.suffix == ".toml":
        return extract_toml_description(text)
    status, description = extract_yaml_description(text.splitlines())
    if status != "ok":
        return None
    return description


def extract_yaml_description(lines: list[str]) -> tuple[str, str | None]:
    if not lines or lines[0].strip() != "---":
        return "missing", None
    close_index = None
    for index in range(1, len(lines)):
        if lines[index] == "---":
            close_index = index
            break
    if close_index is None:
        return "unclosed", None

    frontmatter = lines[1:close_index]
    for index, line in enumerate(frontmatter):
        if line.startswith((" ", "\t")):
            continue
        if not line.startswith("description:"):
            continue
        value = line[len("description:") :].strip()
        chunks = [strip_yaml_scalar(value)] if value and value not in YAML_BLOCK_SCALAR_MARKERS else []
        for continuation in frontmatter[index + 1 :]:
            if not continuation.startswith((" ", "\t")):
                break
            stripped = continuation.strip()
            if stripped:
                chunks.append(strip_yaml_scalar(stripped))
        return ("empty" if not chunks else "ok"), " ".join(chunks).strip()
    return "ok", None


def strip_yaml_scalar(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
        return value[1:-1]
    return value


def extract_toml_description(text: str) -> str | None:
    if tomllib is None:
        raise RuntimeError(
            "TOML harness files require Python 3.11+ or the 'tomli' package. "
            "Install tomli or run this linter with Python 3.11+."
        )
    try:
        data = tomllib.loads(text)
    except tomllib.TOMLDecodeError as exc:
        raise RuntimeError(f"invalid TOML in harness file: {exc}") from exc
    description = data.get("description")
    if not isinstance(description, str):
        return None
    return description.strip()


def check_no_emoji(path: Path, lines: list[str]) -> list[Violation]:
    violations = []
    for line_number, line in enumerate(lines, start=1):
        for column, char in enumerate(line, start=1):
            if is_emoji(char):
                message = (
                    f'emoji "{char}" at column {column}. why: emojis spend context tokens with no semantic value for '
                    "the model and are banned by repo style in harness guidance files. fix: remove the emoji entirely, "
                    "or replace it with a short text label (e.g. [OK], [WARN], [FAIL])."
                )
                violations.append(Violation("no-emoji", path, line_number, message))
                break
    return violations


def is_emoji(char: str) -> bool:
    codepoint = ord(char)
    return any(start <= codepoint <= end for start, end in EMOJI_RANGES)


def check_no_em_dash(path: Path, lines: list[str]) -> list[Violation]:
    violations = []
    for line_number, line in enumerate(lines, start=1):
        index = line.find("\u2014")
        if index == -1:
            continue
        column = index + 1
        message = (
            f"em-dash (U+2014) at column {column}. why: em-dashes are a stylistic tell of AI-generated prose; repo "
            "style requires plain ASCII dashes in harness files. fix: replace U+2014 with ' - ' (space-hyphen-space) "
            "or rephrase the sentence to remove the dash. search for the literal em-dash character (U+2014) in this "
            "file to locate every occurrence."
        )
        violations.append(Violation("no-em-dash", path, line_number, message))
    return violations


def check_no_at_dot_slash(path: Path, lines: list[str]) -> list[Violation]:
    violations = []
    for line_number, line in enumerate(lines, start=1):
        index = line.find("@./")
        if index == -1:
            continue
        column = index + 1
        message = (
            f"`@./` reference at column {column}. why: non-standard import/path pattern that confuses "
            "harness loaders and is not documented behaviour. fix: drop the `@./` prefix. example: `@./SKILL.md` "
            "becomes `./SKILL.md` or `SKILL.md`; `@./examples/foo.sh` becomes `examples/foo.sh`."
        )
        violations.append(Violation("no-at-dot-slash", path, line_number, message))
    return violations


def check_description_when(path: Path, text: str) -> list[Violation]:
    if path.suffix == ".toml":
        return check_toml_description_when(path, text)
    return check_yaml_description_when(path, text)


def check_yaml_description_when(path: Path, text: str) -> list[Violation]:
    lines = text.splitlines()
    status, description = extract_yaml_description(lines)
    if status == "missing":
        return [
            Violation(
                "description-when",
                path,
                1,
                "file is missing YAML frontmatter (`--- ... ---` block). why: harness artifacts are discovered via "
                "their frontmatter `name` and `description` fields; without a frontmatter block the router cannot "
                "trigger the artifact. fix: add a YAML block at the very top of the file: `---` on its own line, then "
                "`name: <kebab-case-skill-name>`, then `description: Use when <concrete trigger>. <one-line outcome>.`, "
                "then a closing `---` line.",
            )
        ]
    if status == "unclosed":
        return [
            Violation(
                "description-when",
                path,
                1,
                "file frontmatter opens with `---` but never closes. why: a malformed frontmatter block is "
                "unparseable and the artifact becomes invisible to the router. fix: add a `---` line on its own "
                "(no leading whitespace) after the last frontmatter field to close the block.",
            )
        ]
    if status == "empty":
        return [
            Violation(
                "description-when",
                path,
                1,
                "file frontmatter has an empty `description:` field. why: the description is the primary signal the "
                "router uses to decide whether this artifact applies to the user's request. fix: fill in "
                "`description: Use when <concrete trigger>. <one-line outcome>.` between the opening and closing "
                "--- fences.",
            )
        ]
    if not description:
        return [
            Violation(
                "description-when",
                path,
                1,
                "file frontmatter has no `description:` field. why: the description is the primary signal the "
                "router uses to decide whether this artifact applies to the user's request. fix: add `description: "
                "Use when <concrete trigger>. <one-line outcome>.` between the opening and closing `---` fences. "
                "example: `description: Use when the user says 'create a branch' or references a CLIP-XXX ticket.`",
            )
        ]
    if not has_routing_phrase(description):
        return [description_routing_violation(path)]
    return []


def check_toml_description_when(path: Path, text: str) -> list[Violation]:
    description = extract_toml_description(text)
    if not description:
        return [
            Violation(
                "description-when",
                path,
                1,
                'agent TOML has no `description = "..."` field. why: the description is the primary router signal '
                "for selecting this specialist agent. fix: add a concise `description = \"Use when ...\"` entry with "
                "concrete task types or user phrases.",
            )
        ]
    if not has_routing_phrase(description):
        return [description_routing_violation(path)]
    return []


def has_routing_phrase(description: str) -> bool:
    # Before enabling description-when by default, update artifacts that currently
    # use looser phrasing like "Useful for ..." or incidental "... when ...".
    return re.search(r"\b(use when|use for|trigger on|trigger when)\b", description, re.IGNORECASE) is not None


def description_routing_violation(path: Path) -> Violation:
    return Violation(
        "description-when",
        path,
        1,
        'description lacks a routing phrase. why: the model needs an explicit trigger signal ("when" / '
        '"use for") to pick this skill over siblings; vague descriptions cause missed triggers or wrong routing. '
        'fix: start the description with one of "Use when ...", "Use for ...", or "Trigger on ..." followed by '
        'concrete user phrases, file paths, or task types. example: "Use when the user says \'fix the bug\' or '
        '\'X is broken\'."',
    )


def format_violation(violation: Violation, cwd: Path) -> str:
    path = display_path(violation.path, cwd)
    if violation.line > 0:
        return f"{path}:{violation.line}: [{violation.rule}] {violation.msg}"
    return f"{path}: [{violation.rule}] {violation.msg}"


def format_github_annotation(violation: Violation, cwd: Path) -> str:
    path = display_path(violation.path, cwd)
    location = f"file={path}"
    if violation.line > 0:
        location += f",line={violation.line}"
    return f"::error {location}::{escape_github_message(f'[{violation.rule}] {violation.msg}')}"


def display_path(path: Path, cwd: Path) -> str:
    try:
        return path.resolve().relative_to(cwd.resolve()).as_posix()
    except ValueError:
        return path.as_posix()


def escape_github_message(message: str) -> str:
    return message.replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")


if __name__ == "__main__":
    raise SystemExit(main())
