#!/usr/bin/env python3
"""Run orchestration specs in a reproducible order on one shared worker fixture."""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", choices=("chromium", "mobile-chrome"), required=True)
    parser.add_argument("--order", choices=("forward", "reverse"), required=True)
    parser.add_argument("--cycles", type=int, choices=(1, 2, 3), default=2)
    parser.add_argument("--no-build", action="store_true")
    args = parser.parse_args()
    web = Path(__file__).resolve().parents[2]
    suite = web / "e2e/tests/orchestration"
    if list(suite.glob("evidence-*.spec.ts")):
        raise SystemExit("An evidence run owns temporary specs; finish that run before continuing.")
    sources = sorted(suite.glob("*.spec.ts"))
    mobile = args.project == "mobile-chrome"
    sources = [p for p in sources if p.name.startswith("mobile-") == mobile]
    if args.order == "reverse":
        sources.reverse()
    prefix = "evidence-" + uuid.uuid4().hex + "-"
    created = []
    manifest = []
    try:
        for cycle in range(args.cycles):
            for source in sources:
                content = source.read_bytes()
                copy = suite / f"{prefix}{len(created):04d}-{source.name}"
                # Keep relative imports unchanged. Alphabetical discovery now
                # expresses the requested order; repeat-each would replace the
                # worker fixture between cycles and miss this cleanup boundary.
                with copy.open("xb") as output:
                    output.write(content)
                created.append(copy)
                manifest.append({"cycle": cycle + 1, "source": source.name,
                                 "sha256": hashlib.sha256(content).hexdigest()})
        print(json.dumps({"project": args.project, "order": args.order,
                          "cycles": args.cycles, "sources": manifest}), flush=True)
        command = ["pnpm", "e2e:run", "--host", "--shards", "1", "--project", args.project]
        if args.no_build:
            command.append("--no-build")
        command.extend(["tests/orchestration/" + prefix, "--", "--retries=0"])
        return subprocess.call(command, cwd=web)
    finally:
        for copy in created:
            copy.unlink(missing_ok=True)


if __name__ == "__main__":
    raise SystemExit(main())
