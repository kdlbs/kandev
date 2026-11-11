---
id: "05-dependency-chip-related-section"
title: "Add the dependency chip's Related section"
status: done
wave: 2
depends_on: ["01-relation-storage-and-creation"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-005
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-005.1
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-005.2
system_design:
  - ../../specs/tasks/system-design/task-create-relationships.md
---

# Task 05: Add the dependency chip's Related section

## Summary

Add a `related` field to `DependencyChipData` and a third "Related" section
to `DependencyLists`, and extend `ChipTriggerContent`'s summary so a
related-only task's chip is visible and legible.

## In scope

- `DependencyChipData.related: TaskSummary[]` in
  `hooks/domains/task/use-task-dependencies.ts`, populated from the task's
  `related` projection field.
- A third "Related" section in `DependencyLists`
  (`components/task/task-dependency-chip.tsx`), rendered only when non-empty,
  with no blocked/resolved iconography.
- `ChipTriggerContent`'s summary updated to mention a related-to count when
  "Waiting on" and "Blocks" are both empty but "Related" is not.
- Localized copy for the new section label and trigger summary.

## Out of scope

- The create-dialog selectors (task 04) and the task graph (tasks 02/03).

## Acceptance

- A task with one related-to link and no dependencies shows a chip whose
  trigger reads a localized related-to summary, and whose popover/drawer
  shows a "Related" section listing that task.
- A task with dependencies and a related-to link shows all three sections in
  the existing "Waiting on" → "Blocks" → "Related" order.

## TDD sequence

1. Failing `use-task-dependencies` test: `related` populated from the task
   projection.
2. Failing `DependencyLists` test: third section renders only when non-empty,
   with no status iconography.
3. Failing `ChipTriggerContent` test: related-only summary.
4. Implement, then refactor with tests green.

## Verification

```bash
cd apps/web && pnpm vitest run hooks/domains/task/use-task-dependencies components/task/task-dependency-chip
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/hooks/domains/task/use-task-dependencies.ts`
- `apps/web/components/task/task-dependency-chip.tsx`
- `apps/web/src/locales/*/task.json` (seven catalogs plus pseudo)

## Dependencies

Task 01 (task projection exposes `related`).

## Risks

- `ChipTriggerContent`'s summary logic already branches on several states
  (blocked, waiting, blocks-only); adding a fourth branch must not change the
  existing summaries for a task with no related-to link.

## Parallelism

`parallel-safe` with task 02 and task 04 (different files).

## Results

Added related task refs to the dependency projection and frontend task model, and added the dependency chip's third Related section and related-only summary.

Validation results:

- `vitest run components/task/task-dependency-chip hooks/domains/task/use-task-dependencies`: 2 files and 12 tests passed.
- `node_modules/.bin/tsc --noEmit`: passed.
- `pnpm run lint`: passed.
- `pnpm run i18n:check`: passed; all seven locales and pseudo are complete.
- `pnpm run i18n:ratchet`: passed.
