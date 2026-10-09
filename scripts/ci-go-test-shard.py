#!/usr/bin/env python3
"""Run a disjoint shard of Go tests, examples, and fuzz seed cases."""

import argparse
import json
import re
import subprocess
import sys


def discover_names(output: str) -> list[str]:
    return sorted(set(
        line for line in output.splitlines()
        if re.fullmatch(r"(?:Test|Example|Fuzz)\w*", line)
    ))


def select_names(names: list[str], shard: int, shards: int) -> list[str]:
    if not 1 <= shard <= shards:
        raise ValueError("The shard must be between 1 and the shard count.")
    selected = names[shard - 1::shards]
    if not selected:
        raise ValueError("The shard contains no tests.")
    return selected


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--shard", type=int, required=True)
    parser.add_argument("--shards", type=int, required=True)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("packages", nargs="+")
    args = parser.parse_args(argv)
    if not 1 <= args.shard <= args.shards:
        parser.error("The shard must be between 1 and the shard count.")

    discovery = subprocess.run(
        ["go", "test", "-race", "-list", ".", "-timeout", "25m", *args.packages],
        capture_output=True, text=True, encoding="utf-8", check=False,
    )
    if discovery.returncode:
        sys.stderr.write(discovery.stdout + discovery.stderr)
        return discovery.returncode
    names = discover_names(discovery.stdout)
    try:
        selected = select_names(names, args.shard, args.shards)
    except ValueError as error:
        parser.error(str(error))
    pattern = "^(" + "|".join(re.escape(name) for name in selected) + ")$"
    # Leave room for the other arguments within Windows' command-line limit.
    if len(pattern) > 28_000:
        parser.error("The test filter is too long. Use more shards.")
    command = [
        "go", "test", "-race", "-v", "-json", "-timeout", "25m",
        "-run", pattern, *args.packages,
    ]
    if args.dry_run:
        print(json.dumps({
            "shard": args.shard, "shards": args.shards,
            "total_tests": len(names), "selected_tests": selected, "command": command,
        }))
        return 0
    print(f"Running {len(selected)} of {len(names)} names in shard {args.shard}/{args.shards}", flush=True)
    return subprocess.run(command, check=False).returncode


if __name__ == "__main__":
    raise SystemExit(main())
