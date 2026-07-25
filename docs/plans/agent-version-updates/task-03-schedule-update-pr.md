---
id: "03-schedule-update-pr"
title: "Schedule the update pull request"
status: done
wave: 3
depends_on: ["02-build-version-updater"]
plan: "plan.md"
decision: "../../decisions/2026-07-25-scheduled-core-agent-version-pins.md"
---

# Task 03: Schedule the update pull request

## Acceptance

- A weekly and manually dispatchable workflow creates or refreshes one PR only
  when core versions change.
- The workflow uses a dedicated least-privilege GitHub App token, immutable
  action refs, a fixed concurrency group, targeted verification, and no
  auto-merge.
- Contract tests prove the workflow does not execute candidate packages in the
  token-bearing job and retains its branch/PR behavior.

## Verification

- `python3 .github/scripts/update-agent-versions-workflow_test.py`
- `python3 .github/scripts/lint-action-pinning_test.py`
- `python3 .github/scripts/lint-action-pinning.py`

## Files likely touched

- `.github/workflows/update-agent-versions.yml`
- `.github/scripts/update-agent-versions-workflow_test.py`
- `.github/workflows/lint-action-pinning.yml`

## Dependencies

Task 02 supplies the updater CLI and report contract.

## Inputs

- ADR Decision section
- Plan section "Scheduled pull request"
- Existing scheduled workflows and action-pinning enforcement

## Output contract

Report workflow behavior, files changed, RED/GREEN evidence, verification
results, operational prerequisites, and risk tags. Set this task to `done` only
after all verification commands pass.
