---
id: "02-build-version-updater"
title: "Build the allowlisted updater"
status: done
wave: 2
depends_on: ["01-pin-core-agent-runtimes"]
plan: "plan.md"
decision: "../../decisions/2026-07-25-scheduled-core-agent-version-pins.md"
---

# Task 02: Build the allowlisted updater

## Acceptance

- The updater resolves stable npm `latest` metadata for exactly the five core
  packages without executing them.
- Updates are limited to explicit paths, preflight all expected occurrence
  counts before writing, and produce both JSON and Markdown reports.
- Tests prove successful updates, no-op behavior, malformed/prerelease
  rejection, and fail-closed source-shape drift.

## Verification

- `python3 scripts/update_agent_versions_test.py`
- `python3 scripts/update_agent_versions.py --check`

## Files likely touched

- `scripts/update_agent_versions.py`
- `scripts/update_agent_versions_test.py`

## Dependencies

Task 01 supplies the exact constants and synchronized reference locations.

## Inputs

- ADR Decision section
- Plan section "Allowlisted updater"
- The five core agent source files and current-version documents from Task 01

## Output contract

Report behavior, files changed, RED/GREEN evidence, verification results, risk
tags, and any divergence from the allowlist design. Set this task to `done`
only after both commands pass.
