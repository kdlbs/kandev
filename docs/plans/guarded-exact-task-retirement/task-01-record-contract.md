---
id: "01-record-contract"
title: "Record guarded exact-retirement contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-EXACT-RETIREMENT-001
  - REQ-TASKS-EXACT-RETIREMENT-002
  - REQ-TASKS-EXACT-RETIREMENT-003
system_design:
  - ../../specs/tasks/system-design/guarded-exact-task-retirement.md
---

# Task 01: Record guarded exact-retirement contract

## Summary

Define the fail-closed exact-pair behavior, its technical boundary, and the
decision that excludes ordinary cleanup paths.

## Acceptance

1. Requirements define authorized preview, preservation/handoff, and fenced
   cleanup outcomes with stable IDs.
2. The design makes missing evidence `UNKNOWN` and keeps preview side-effect
   free.
3. The ADR records why ordinary archive/delete is excluded.

## Verification

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Results

The requirements, paired design, and ADR were added. Task retirement remains a
new task-system contract; worktree, queue, environment, runtime, and PR owners
retain their inspection boundaries.
