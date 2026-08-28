---
id: "04-weekly-pin-workflow"
title: "Automate weekly pin PRs"
status: complete
wave: 2
depends_on: ["01-default-pins"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 04: Automate weekly pin PRs

## Acceptance

- A fixture-tested script updates only trusted catalogue entries to valid stable
  npm `latest` versions and handles no-change, prerelease, malformed, and lookup
  failure cases without partial writes.
- A weekly/manual least-privilege workflow runs validation and opens at most one
  grouped Conventional Commit PR through a token that permits normal PR CI.
- The workflow uses pinned actions, never auto-merges, and has a contract test
  wired into the repository's action-pinning workflow.

## Verification

```bash
node --test scripts/update-agent-runtime-pins.test.mjs && python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py && python3 .github/scripts/lint-action-pinning_test.py && python3 .github/scripts/lint-action-pinning.py && zizmor .github/workflows/update-agent-runtime-pins.yml && git diff --check
```

## Files likely touched

- `scripts/update-agent-runtime-pins.mjs`
- `scripts/update-agent-runtime-pins.test.mjs`
- `.github/workflows/update-agent-runtime-pins.yml`
- `.github/scripts/update-agent-runtime-pins-workflow-contract_test.py`
- `.github/workflows/lint-action-pinning.yml`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 02 after Task 01. Ownership is restricted to scripts and
`.github/**`; do not edit runtime selection files or the pin catalogue format.

## Inputs

- ADR: reviewed weekly PR, no automatic merge or runtime mutation
- Plan: Weekly Pin Update Workflow
- Scoped rules: `.github/AGENTS.md`

## Output contract

Report the required token boundary, workflow/branch behavior, files changed,
all security and contract checks, external side effects (expected: none during
local tests), risks, and synchronized task/plan status.

## Results

Complete. The updater changes only existing trusted catalogue entries, rejects
missing, malformed, and prerelease values before its atomic write, and the
weekly/manual workflow uses a pinned GitHub App token, a stable bot branch, one
grouped PR, and no auto-merge or `GITHUB_TOKEN` fallback.

Verification: updater tests passed 7/7; workflow contract tests passed 8/8;
action-pinning tests passed 9/9 and the linter accepted all 19 workflow files;
`zizmor .github/workflows/update-agent-runtime-pins.yml` reported no findings.
Local checks caused no external workflow or PR side effects.

Follow-up review verification keeps the App token in the push step, edits an
existing updater PR by its number, and runs the focused managed-runtime Go
suite after the catalogue update and before any commit or push. The workflow
contract now asserts that validation boundary; the contract passed 8/8 and
the action-pinning checks remained green.
