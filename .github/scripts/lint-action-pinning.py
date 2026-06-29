#!/usr/bin/env python3
"""Verify every `uses:` ref in .github/workflows/ is pinned to a 40-char commit SHA.

Exits 0 when all refs are pinned, 1 when any violation is found.
Emits GitHub Actions `::error::` annotations so failures appear inline in PRs.

Usage (locally):   python3 .github/scripts/lint-action-pinning.py
Usage (CI):        same — no arguments needed.
"""

import re
import sys
from pathlib import Path

# A valid pinned ref is exactly 40 lowercase hex characters.
SHA_RE = re.compile(r"^[0-9a-f]{40}$")

# Matches a `uses:` step line, capturing everything after `uses:` up to an
# optional trailing YAML comment. The outer group is intentionally lazy so
# trailing `# tag` comments are consumed by the optional group rather than
# included in the captured value. Expression-based refs like
# `${{ inputs.ref }}` contain spaces and are captured in full because
# the lazy match expands until the optional comment group can anchor at `$`.
USES_RE = re.compile(r"^\s*-?\s*uses:\s+(.+?)(?:\s+#.*)?$")

workflows_dir = Path(__file__).parent.parent / "workflows"

violations: list[str] = []

for path in sorted(p for ext in ("*.yml", "*.yaml") for p in workflows_dir.glob(ext)):
    for lineno, line in enumerate(path.read_text().splitlines(), start=1):
        m = USES_RE.match(line)
        if not m:
            continue

        # Split on the last `@` to separate the action name from its ref.
        # rpartition returns ("", "", value) when `@` is absent.
        uses_value = m.group(1).strip()
        action, sep, ref = uses_value.rpartition("@")
        if not sep:
            # No `@` — local action (`./...`) without a pin; skip.
            continue

        # Local reusable workflows: uses: ./.github/workflows/foo.yml@ref
        # These reference local content with no external registry.
        if action.startswith("./"):
            continue

        # Docker container actions: uses: docker://image@sha256:digest
        # The ref after `@` is a registry digest, not a commit SHA.
        if action.startswith("docker://"):
            continue

        if not SHA_RE.match(ref):
            rel = path.relative_to(Path(__file__).parent.parent.parent)
            violations.append((str(rel), lineno, line.strip()))

if violations:
    for file, lineno, text in violations:
        # GitHub Actions annotation — shown inline on the PR diff.
        print(f"::error file={file},line={lineno}::Unpinned action ref (use a 40-char commit SHA): {text}")
    print(
        f"\n{len(violations)} unpinned ref(s) found. "
        "Pin each `uses:` to a commit SHA and keep the version tag as a comment:\n"
        "  uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6",
        file=sys.stderr,
    )
    sys.exit(1)

print(f"✓ All {sum(1 for ext in ('*.yml', '*.yaml') for _ in workflows_dir.glob(ext))} workflow file(s) use SHA-pinned action refs.")
