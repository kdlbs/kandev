---
id: "01-owning-contract"
title: "Owning contract update"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CHILD-STALL-004
acceptance_criteria:
  - AC-TASKS-CHILD-STALL-004.1
  - AC-TASKS-CHILD-STALL-004.2
  - AC-TASKS-CHILD-STALL-004.3
  - AC-TASKS-CHILD-STALL-004.4
  - AC-TASKS-CHILD-STALL-004.5
system_design:
  - ../../specs/tasks/system-design/child-turn-stalled-signal.md
---

# Task 01: Owning Contract Update

## Summary

Record the current-primary routing and idle-notice lifecycle contract in the
owning requirement and system design before any production or permanent-test
change.

## In scope

- Add `REQ-TASKS-CHILD-STALL-004` with acceptance criteria covering current
  primary binding, re-home/withdraw/hold, stale and archived withdrawal,
  per-child coalescing, isolation from user/question/approval rows, running
  inside the existing producer scan, and observable bounded outcomes.
- Add the matching system-design section describing the reconcile steps.

## Out of scope

- Code, tests, migrations, or any dashboard/aggregate change.

## Acceptance

1. The requirement and design state the reconcile behavior, its isolation
   boundary, and its evidence-based withdrawal conditions.
2. Spec discovery and lint pass.

## Verification

From the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `docs/specs/tasks/requirements/child-turn-stalled-signal.md`
- `docs/specs/tasks/system-design/child-turn-stalled-signal.md`

## Dependencies

None.

## Risks

Leaving the requirement unchanged would let implementation drift from the
recorded contract.

## Parallelism

`sequential`

## Inputs

- `docs/specs/README.md`
- `docs/specs/tasks/requirements/child-turn-stalled-signal.md`
- `docs/specs/tasks/system-design/child-turn-stalled-signal.md`

## Results

Added `REQ-TASKS-CHILD-STALL-004` (AC-004.1..004.5) and the "Current-primary
routing and idle-notice reconciliation" design section.
