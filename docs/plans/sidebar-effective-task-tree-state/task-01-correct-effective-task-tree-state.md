---
id: "01-correct-effective-task-tree-state"
title: "Correct effective task-tree state"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001
acceptance_criteria:
  - AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.1
  - AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.2
  - AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.3
  - AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.4
  - AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.5
  - AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.6
  - AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.7
system_design:
  - ../../specs/ui/system-design/sidebar-effective-task-tree-state.md
---

# Task 01: Correct Effective Task-Tree State

## Summary

Replace the direct-child bucket minimum with one recursive effective-state resolver shared by
sidebar state grouping and sorting. Prove that active child work keeps the tree active while each
row retains its own state.

## In scope

- Add the regression cases before changing production logic.
- Resolve active and completed tree states across every included descendant with cycle protection.
- Reuse the resolved group identity and action bucket in state grouping and state sorting.
- Extend the existing desktop sidebar E2E regression.

## Out of scope

- Backend or persistence changes.
- Non-state grouping and sorting behavior.
- Row presentation, mobile composition, and new user-facing copy.

## Acceptance

- Completed, review, and waiting-for-input parents with a running included descendant render under
  In progress; scheduling is used only when no member is already in progress.
- Completed is selected only when every included tree member is completed, including nested
  descendants; filtered-out descendants do not participate.
- State sorting and grouping consume the same result while parent and child row states remain
  unchanged.

## Verification

```bash
cd apps/web && pnpm exec vitest run lib/sidebar/apply-view-effective-state.test.ts
cd apps/web && pnpm e2e:run tests/task/sidebar-subtask-state-sort.spec.ts
```

## Files likely touched

- `apps/web/lib/sidebar/apply-view.ts`
- `apps/web/lib/sidebar/apply-view-effective-state.test.ts`
- `apps/web/e2e/tests/task/sidebar-subtask-state-sort.spec.ts`

## Dependencies

None.

## Risks

- Existing action buckets intentionally prioritize review attention for unrelated roots; the fix
  must change mixed-state trees without globally reordering independent tasks.
- Malformed parent cycles must not recurse indefinitely.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001` and its acceptance criteria.
- `docs/specs/ui/system-design/sidebar-effective-task-tree-state.md`.
- Existing effective-state unit and Playwright tests.

## Results

Pending.
