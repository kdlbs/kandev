#!/usr/bin/env python3
"""Run one of three complete native cohorts of the task repository package."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys

PACKAGE = "github.com/kandev/kandev/internal/task/repository/sqlite"
PACKAGE_PATH = "./internal/task/repository/sqlite"
SCALE_SKIP = "^TestDeliveryRetentionStorage(ProtectedGap)?$/SQLite$"


def checked_events(events):
    done = False
    for event in events:
        action = event.get("Action")
        if not event.get("Package") and action in ("build-start", "build-output"):
            continue
        if event.get("Package") != PACKAGE or done:
            raise ValueError("unexpected package or event after package completion")
        if action not in ("start", "run", "pause", "cont", "output", "pass", "skip", "fail"):
            raise ValueError("unknown Go test event")
        if action == "fail":
            raise ValueError("Go reported a test or package failure")
        if not event.get("Test") and action in ("pass", "skip"):
            if action != "pass":
                raise ValueError("repository package did not execute successfully")
            done = True
        yield event
    if not done:
        raise ValueError("missing successful package completion")


def inventory(events):
    names = set()
    for event in checked_events(events):
        if event.get("Test"):
            raise ValueError("test execution appeared during enumeration")
        if event["Action"] != "output":
            continue
        for line in event.get("Output", "").splitlines():
            if not line or line == "PASS" or line.startswith(("ok \t", "ok  \t", "ok\t", "ok ")):
                continue
            if (not line.startswith(("Test", "Fuzz", "Example"))
                    or not line.isidentifier() or line == "TestMain" or line in names):
                raise ValueError("invalid or duplicate native test name: " + line)
            names.add(line)
    if len(names) < 3:
        raise ValueError("repository inventory cannot form three nonempty cohorts")
    return sorted(names)


def partition(names, index):
    if index not in (1, 2, 3) or len(names) < 3 or len(set(names)) != len(names):
        raise ValueError("invalid repository cohort or inventory")
    return sorted(names)[index - 1::3]


def validate_result(events, selected):
    expected, completed = set(selected), set()
    for event in checked_events(events):
        name = event.get("Test")
        if not name:
            continue
        if name.split("/", 1)[0] not in expected:
            raise ValueError("unselected test executed: " + name)
        if "/" not in name and event["Action"] in ("pass", "skip"):
            if name in completed:
                raise ValueError("duplicate test completion: " + name)
            completed.add(name)
    if completed != expected:
        raise ValueError("missing test completions: " + ", ".join(sorted(expected - completed)))


def read_events(path):
    with path.open(encoding="utf-8") as stream:
        for line in stream:
            event = json.loads(line)
            if not isinstance(event, dict):
                raise ValueError("Go output is not an event object")
            yield event


def command(mode, output, selector=None):
    args = ["go", "test", "-trimpath", "-race", "-json", "-count=1"]
    args += ["-timeout=90m" if mode == "postgres" else "-timeout=120m"]
    if mode == "normal":
        args += ["-covermode=atomic"]
        if selector is not None:
            args += ["-coverprofile=" + str(output / "coverage.out")]
        else:
            args += ["-cover"]
    if selector is None:
        args += ["-list", "^(Test|Fuzz|Example)"]
    else:
        args += ["-run", selector]
        if mode == "postgres":
            args += ["-skip", SCALE_SKIP]
    return [*args, PACKAGE_PATH]


def execute(args, output, label):
    stdout_path, stderr_path = output / (label + ".jsonl"), output / (label + ".stderr")
    print("Running " + label, flush=True)
    with stdout_path.open("x", encoding="utf-8") as stdout, stderr_path.open("x", encoding="utf-8") as stderr:
        result = subprocess.run(args, stdout=stdout, stderr=stderr, check=False)
    if result.returncode:
        print(stderr_path.read_text(encoding="utf-8", errors="replace")[-12000:], file=sys.stderr)
        print(stdout_path.read_text(encoding="utf-8", errors="replace")[-12000:], file=sys.stderr)
        raise RuntimeError(f"{label} exited {result.returncode}; full diagnostics in {output}")
    return stdout_path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("normal", "postgres"), required=True)
    parser.add_argument("--shard", type=int, choices=(1, 2, 3), required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    if args.mode == "postgres" and not os.environ.get("KANDEV_TEST_POSTGRES_DSN"):
        raise ValueError("PostgreSQL cohort requires the service DSN")
    names = inventory(read_events(execute(command(args.mode, output), output, "inventory")))
    selected = partition(names, args.shard)
    selector = "^(" + "|".join(re.escape(name) for name in selected) + ")$"
    report = {"package": PACKAGE, "mode": args.mode, "shard": args.shard, "total": 3,
              "source": os.environ.get("GITHUB_SHA"), "inventory": names, "selected": selected,
              "complete": False}
    report_path = output / "selection.json"
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(f"Selected {len(selected)} of {len(names)} native top-level tests", flush=True)
    events = execute(command(args.mode, output, selector), output, "results")
    validate_result(read_events(events), selected)
    if args.mode == "normal" and not (output / "coverage.out").is_file():
        raise ValueError("missing repository coverage profile")
    report["complete"] = True
    report_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(f"Complete repository cohort {args.shard}/3", flush=True)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, OSError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
