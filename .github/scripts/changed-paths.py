#!/usr/bin/env python3
"""Apply a GitHub `paths:` filter inside a job instead of on the trigger.

Why this exists
---------------
A workflow that is skipped by a trigger-level `paths:` filter reports **no
conclusion at all**. GitHub's own documentation is explicit that "checks
associated with that workflow will remain in a 'Pending' state", and a required
check that stays pending blocks a pull request from entering a merge queue and
blocks the merge group from ever merging.

The `merge_group` event makes this worse: it supports `types:` and `branches:`
filters only -- `paths:` is not available -- so a trigger-level filter cannot
even be mirrored onto the queue side.

The fix is to move the filter one level down. The workflow triggers
unconditionally, a cheap `changes` job runs this script to decide whether the
expensive jobs are relevant, and an always-running gate job reports the single
conclusion the merge queue waits on. Jobs skipped by an `if:` conditional are
fine here because they are never themselves the required check.

Usage
-----
    git diff --name-only -z "$BASE" HEAD | python3 .github/scripts/changed-paths.py

Reads the pattern list from `$PATTERNS` (override with `--patterns-env`), one
pattern per line, in the same syntax and with the same ordering semantics as a
workflow's `paths:` list. Writes `run=true` or `run=false` to stdout and, when
`$GITHUB_OUTPUT` is set, appends it there as a step output.

Pass `-z` to the diff. Git quotes any path outside ASCII under its default
`core.quotePath=true`, so plain `--name-only` reports `apps/web/café.ts` as the
literal 24-character string `"apps/web/caf\303\251.ts"`. That matches no
pattern, so a pull request touching only such a file selects nothing, every job
is skipped, and the gate reports success having tested none of it -- the exact
silent pass this whole mechanism exists to prevent. `-z` emits raw bytes with a
NUL after each path and suppresses the quoting.

Stdin is split on NUL when it contains one and on newlines otherwise, so both
forms work. The detection is not ambiguous: `-z` terminates every path with a
NUL, so any non-empty `-z` output contains at least one.

Pattern syntax
--------------
Supported: `*` (any run of characters except `/`), `**` (any run including
`/`), `?` (one character except `/`), and a leading `!` for negation. A file's
verdict is decided by the LAST pattern that matches it, which is what GitHub
does, so `!**/*.md` after `apps/web/**` excludes Markdown under `apps/web`.

Deliberately unsupported: `+`, `[...]` character classes, and `{...}` braces.
GitHub accepts the first two; this script rejects any pattern containing them
rather than silently matching them as literals, because a filter that quietly
stops matching is exactly the failure this file exists to prevent.
"""

from __future__ import annotations

import argparse
import os
import re
import sys


# Characters GitHub's filter-pattern syntax gives a meaning that this script
# does not implement. Rejected loudly rather than treated as literals.
UNSUPPORTED_METACHARACTERS = ("+", "[", "]", "{", "}")


def compile_pattern(pattern: str) -> re.Pattern[str]:
    """Translate one GitHub filter pattern into an anchored regex.

    `**` is translated before `*` so that the single-segment wildcard never
    consumes half of a double one.
    """
    parts: list[str] = []
    index = 0
    while index < len(pattern):
        char = pattern[index]
        if char == "*":
            if pattern.startswith("**", index):
                parts.append(".*")
                index += 2
            else:
                parts.append("[^/]*")
                index += 1
        elif char == "?":
            parts.append("[^/]")
            index += 1
        else:
            parts.append(re.escape(char))
            index += 1
    return re.compile("".join(parts) + r"\Z")


class Filter:
    """An ordered `paths:` list, evaluated last-match-wins per file."""

    def __init__(self, patterns: list[str]) -> None:
        self.rules: list[tuple[bool, re.Pattern[str]]] = []
        for raw in patterns:
            pattern = raw.strip()
            # Blank lines and `#` comments keep the YAML block readable.
            if not pattern or pattern.startswith("#"):
                continue

            included = True
            if pattern.startswith("!"):
                included = False
                pattern = pattern[1:]

            # A bare `!` compiles to a regex matching only the empty string,
            # which no real path is. It would append a rule that can never fire
            # and read like an exclusion that works.
            if not pattern:
                raise ValueError(
                    f"pattern {raw!r} is a bare negation with no path. Use "
                    "'!path/pattern' to exclude specific files."
                )

            found = [c for c in UNSUPPORTED_METACHARACTERS if c in pattern]
            if found:
                raise ValueError(
                    f"pattern {raw!r} uses unsupported metacharacter(s) "
                    f"{''.join(found)!r}. changed-paths.py implements *, ** and ? "
                    "only; extend it (and its tests) before using more."
                )

            self.rules.append((included, compile_pattern(pattern)))

        if not self.rules:
            raise ValueError("no patterns were supplied")

    def includes(self, path: str) -> bool:
        """Whether `path` survives the filter. Later patterns win."""
        verdict = False
        for included, regex in self.rules:
            if regex.match(path):
                verdict = included
        return verdict

    def matches_any(self, paths: list[str]) -> bool:
        return any(self.includes(path) for path in paths)


def read_changed_paths(data: bytes) -> list[str]:
    """Split a diff's path list, preferring the NUL-delimited form.

    Taken as bytes rather than text because `git diff -z` emits raw filesystem
    bytes, which need not be valid UTF-8. `surrogateescape` keeps an
    undecodable name intact and matchable instead of failing the step.
    """
    separator = b"\0" if b"\0" in data else b"\n"
    return [
        decoded
        for chunk in data.split(separator)
        if (decoded := chunk.decode("utf-8", errors="surrogateescape").strip())
    ]


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--patterns-env",
        default="PATTERNS",
        help="environment variable holding the newline-separated pattern list",
    )
    args = parser.parse_args(argv)

    raw_patterns = os.environ.get(args.patterns_env)
    if raw_patterns is None:
        print(
            f"::error::{args.patterns_env} is unset; changed-paths.py has no "
            "filter to apply.",
            file=sys.stderr,
        )
        return 2

    try:
        path_filter = Filter(raw_patterns.splitlines())
    except ValueError as error:
        print(f"::error::{error}", file=sys.stderr)
        return 2

    changed = read_changed_paths(sys.stdin.buffer.read())
    run = path_filter.matches_any(changed)

    # The matching subset is the useful half of the log: "no relevant changes"
    # is indistinguishable from "the filter is broken" without it.
    if run:
        print("Relevant changed files:")
        for path in changed:
            if path_filter.includes(path):
                print(f"  {path}")
    else:
        print(f"No relevant changes among {len(changed)} changed file(s).")

    value = "true" if run else "false"
    github_output = os.environ.get("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a", encoding="utf-8") as handle:
            handle.write(f"run={value}\n")

    print(f"run={value}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
