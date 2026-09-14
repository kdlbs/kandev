---
id: "06-prepare-capacity-pilot"
title: "Prepare the runner capacity pilot"
status: done
wave: 6
depends_on: ["05-profile-remaining-costs"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
  - AC-PLATFORM-CI-PERFORMANCE-004.3
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 06: Prepare the runner capacity pilot

## Summary

Prepare a concrete 20% capacity pilot using the existing runner controls.
Document cost assumptions, protected jobs, measurement, and rollback without changing live variables.

## In scope

- Current eligible-family inventory, including any accepted frontend test matrix.
- Read-only variable snapshot and provider configuration prerequisites.
- Operator procedure for activation, measurement, rollback, and an explicit budget input.

## Out of scope

- Installing provider apps, purchasing capacity, changing variables, dispatching workflows, or moving review/Cargo/protected jobs.

## Acceptance

- The procedure preserves the current external-runner trust boundary and names jobs that remain hosted.
- It specifies a 20% pilot, at least three representative runs, separate queue/execution metrics, and no expansion without measured evidence.
- It includes explicit activation/rollback commands and cost calculations. Unknown provider rates or an absent operator budget block activation, not documentation completion.

## Verification

Run commands from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
gh variable list --repo kdlbs/kandev --json name,value --jq '.[] | select(.name | startswith("KANDEV_CI_"))'
python3 .github/scripts/runner-plan_test.py
python3 .github/scripts/external-runner-workflow-contract_test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `docs/ci-merge-queue.md`
- `docs/plans/ci-performance/evidence.md`

## Dependencies

Task 05.

## Risks

Percentage allocation controls job instances, not a fixed share of billed minutes. Changing variables affects only newly dispatched jobs.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), corresponding implementation boundary.
- [Plan](plan.md), baseline and companion-package status.
- Existing workflow contract tests under `.github/scripts/`.

## Results

Completed 2026-09-12. Updated the merge-queue runbook with the current
variable snapshot, eligible and protected job families, a 20% activation and
rollback procedure, three-run measurement fields, queue and execution
separation, and provider-rate and budget prerequisites. The procedure includes
the external runner-minute cost formula and states that unknown rates or budget
block activation. No live variable, provider setting, workflow dispatch, or
paid capacity changed.
