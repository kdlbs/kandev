#!/usr/bin/env python3
"""Conservatively exempt independent Go test changes from PR application E2E.

This is an additional exclusion after the existing path filter. Any uncertainty
keeps application E2E selected. Go tests and all backend gates still run.
"""
import argparse
import fnmatch
import os
from pathlib import PurePosixPath
import re
import shlex
import subprocess


def valid_path(path):
    parts = path.split("/")
    return bool(path) and all(p and p not in (".", "..") for p in parts) and "\\" not in path and not any(ord(c) < 32 for c in path)


def embedded(path, patterns):
    # An embedded directory includes descendants. Treat glob directory matches
    # conservatively too; over-selection costs time but cannot hide a change.
    parents = [str(parent) for parent in PurePosixPath(path).parents]
    return any(fnmatch.fnmatchcase(value, pattern) for pattern in patterns for value in [path, *parents])


def classify(data, event, embeds=()):
    if event != "pull_request":
        return False, "event-requires-e2e"
    if not data or not data.endswith(b"\0"):
        return False, "unknown-diff"
    try:
        tokens = data.decode("utf-8").split("\0")[:-1]
        index, tests = 0, 0
        while index < len(tokens):
            header = re.fullmatch(r":(\d{6}) (\d{6}) ([0-9a-f]{40}|[0-9a-f]{64}) ([0-9a-f]{40}|[0-9a-f]{64}) (A|M|D|R\d{1,3})", tokens[index])
            if not header:
                return False, "unknown-diff"
            old, new, old_oid, new_oid, status = header.groups()
            if (len(old_oid) != len(new_oid)
                    or (old == "000000") != (set(old_oid) == {"0"})
                    or (new == "000000") != (set(new_oid) == {"0"})
                    or (status.startswith("R") and int(status[1:]) > 100)):
                return False, "unknown-diff"
            count = 2 if status.startswith("R") else 1
            paths = tokens[index+1:index+1+count]
            if len(paths) != count or not all(valid_path(path) for path in paths):
                return False, "unknown-diff"
            modes = (old, new)
            if status == "A":
                modes = (new,) if old == "000000" else ()
            elif status == "D":
                modes = (old,) if new == "000000" else ()
            if not modes or any(mode not in ("100644", "100755") for mode in modes):
                return False, "nonregular-change"
            for path in paths:
                if embedded(path, embeds):
                    return False, "embedded-application-input"
                if path.endswith(".md"):
                    continue
                parts = path.split("/")
                if (not path.startswith("apps/backend/") or not path.endswith("_test.go")
                        or any(p.startswith(".") or p in ("testdata", "fixtures", "assets", "generated", "files") for p in parts)):
                    return False, "application-or-unknown-change"
                tests += 1
            index += count + 1
        return (True, "independent-go-tests") if tests else (False, "no-go-tests")
    except UnicodeError:
        return False, "unknown-diff"


def git(repo, *args):
    return subprocess.run(["git", "-C", repo, *args], capture_output=True, check=True, timeout=30).stdout


def embed_patterns(repo, revision):
    # Read committed source, not the checkout's file types or untracked files.
    result = subprocess.run(["git", "-C", repo, "grep", "-l", "-z", "-e", "^[[:space:]]*//go:embed[[:space:]]",
                             revision, "--", "apps/backend"], capture_output=True, timeout=30)
    if result.returncode not in (0, 1):
        raise ValueError("unable to inspect embedded inputs")
    patterns = []
    for entry in result.stdout.decode("utf-8").split("\0"):
        if not entry:
            continue
        path = entry.removeprefix(revision + ":")
        if not valid_path(path):
            raise ValueError("unknown embedded-input path")
        if not path.endswith(".go") or path.endswith("_test.go"):
            continue
        source = git(repo, "show", f"{revision}:{path}").decode("utf-8")
        for line in source.splitlines():
            if directive := re.match(r"^\s*//go:embed\s+(.+)$", line):
                # Go character classes and quoted escapes differ from Python's
                # glob/shell rules. Unverified syntax must retain application E2E.
                if any(char in directive.group(1) for char in "[]\\`'"):
                    raise ValueError("unsupported embed directive")
                for pattern in shlex.split(directive.group(1)):
                    pattern = pattern.removeprefix("all:")
                    if not valid_path(pattern) or "`" in pattern:
                        raise ValueError("unsupported embed directive")
                    patterns.append(str(PurePosixPath(path).parent / pattern))
    return patterns


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", required=True)
    parser.add_argument("--head", default="HEAD")
    parser.add_argument("--event", default=os.environ.get("EVENT_NAME", ""))
    args = parser.parse_args()
    exempt, reason = False, "unknown-diff"
    try:
        if args.event == "pull_request":
            base = git(".", "rev-parse", "--verify", "--end-of-options", args.base + "^{commit}").decode().strip()
            head = git(".", "rev-parse", "--verify", "--end-of-options", args.head + "^{commit}").decode().strip()
            data = git(".", "diff", "--raw", "-z", "--no-abbrev", "--no-renames", "--no-ext-diff", "--no-textconv", base, head, "--")
            exempt, reason = classify(data, args.event)
            if exempt:
                # Both revisions matter for deleted and moved embedded inputs.
                patterns = embed_patterns(".", base) + embed_patterns(".", head)
                exempt, reason = classify(data, args.event, patterns)
        else:
            reason = "event-requires-e2e"
    except (ValueError, UnicodeError, subprocess.SubprocessError):
        exempt, reason = False, "unverified-inputs"
    output = f"run={'false' if exempt else 'true'}\nreason={reason}\n"
    print(output, end="")
    if target := os.environ.get("GITHUB_OUTPUT"):
        with open(target, "a", encoding="utf-8") as handle:
            handle.write(output)


if __name__ == "__main__":
    main()
