---
id: "04-create-dialog-selectors"
title: "Add Parent and Related-to selectors with smart ranking"
status: done
wave: 2
depends_on: ["01-relation-storage-and-creation"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-001
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-002
  - REQ-TASKS-TASK-CREATE-RELATIONSHIPS-003
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.1
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.2
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.3
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.4
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-001.5
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.1
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-002.5
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.1
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.2
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.3
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.4
  - AC-TASKS-TASK-CREATE-RELATIONSHIPS-003.5
system_design:
  - ../../specs/tasks/system-design/task-create-relationships.md
---

# Task 04: Add Parent and Related-to selectors with smart ranking

## Summary

Add `rankCreateDialogCandidates`, a `TaskCreateParentSelector`, and a
`TaskCreateRelatedSelector` to the main create dialog's advanced-settings
option grid, and wire `relatedTaskIds`/`parentTaskId` (when not externally
supplied) through to the create payload as `related_task_ids`/`parent_id`.

## In scope

- `rankCreateDialogCandidates` in `task-create-dialog-dependency-search.ts`,
  wrapping `compareDependencyCandidates` as its fallback and adding
  same-swimlane/column, recently-viewed/created, and title-match-first
  ordering ahead of it.
- Re-point the existing Depends-on selector's `.sort()` at the new ranking
  function.
- `TaskCreateParentSelector`: single-select, renders only when no external
  `parentTaskId` is supplied and the dialog is in create mode; excludes
  archived tasks and would-be-cycle candidates; sets `parentTaskId`.
- `TaskCreateRelatedSelector`: multi-select; excludes the task's own
  prospective identity, selected dependencies, and the selected parent; sets
  `relatedTaskIds`.
- Payload wiring: `relatedTaskIds` → `related_task_ids` in
  `task-create-dialog-helpers.ts`, alongside the existing `blocked_by` wiring.
- Localized copy for both selectors' labels, help text, and trigger states.

## Out of scope

- The New Subtask dialog's Related-to row (task 06).
- The dependency chip's third section (task 05).
- Backend changes (done in task 01).

## Acceptance

- With no caller-supplied parent, the advanced settings show Depends on,
  Parent, and Related to, each defaulting to empty with no confirmation step
  on submit.
- Selecting a parent and two related tasks and submitting produces a create
  payload with `parent_id` and `related_task_ids` populated; the Related-to
  candidate list excludes the selected dependency and parent tasks.
- Opening any of the three selectors from a known board column ranks
  same-column tasks first, then recently viewed/created tasks, before falling
  back to the existing recency/title order.

## TDD sequence

1. Failing `rankCreateDialogCandidates` unit tests for each ranking signal and
   the fallback, including a regression test that the Depends-on selector's
   order is unchanged when no new signal applies.
2. Failing `TaskCreateParentSelector` tests: visibility gate, cycle exclusion,
   archived exclusion, `parentTaskId` wiring.
3. Failing `TaskCreateRelatedSelector` tests: multi-select, exclusion of
   dependency/parent selections, `relatedTaskIds` wiring.
4. Failing `task-create-dialog-helpers` test for `related_task_ids` payload
   mapping.
5. Implement, then refactor with tests green.

## Verification

```bash
cd apps/web && pnpm vitest run components/task-create-dialog-dependency-search components/task-create-dialog-dependencies components/task-create-dialog-advanced-settings components/task-create-dialog-submit components/task-create-dialog-helpers
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task-create-dialog-dependency-search.ts`
- `apps/web/components/task-create-dialog-dependencies.tsx`
- `apps/web/components/task-create-dialog-advanced-settings.tsx`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/src/locales/*/task.json` (seven catalogs plus pseudo)

## Dependencies

Task 01 (`related_task_ids` accepted by the backend).

## Risks

- `useDependencyPickerData`'s candidate union/dedup is shared infrastructure;
  extending it for three selectors instead of one must not change its output
  for existing Depends-on callers outside the create dialog.
- Parent-cycle exclusion is a client-side convenience check only; the
  authoritative guard remains server-side validation, so a race (another
  session reparenting a candidate mid-pick) must fail safely at submit, not
  silently create a cycle.

## Parallelism

`parallel-safe` with task 02 and task 05 (different files).

## Results

Added shared candidate ranking, the Parent and Related-to selectors, related-task filtering, creation payload wiring, state reset behavior, and localized selector copy across all locale catalogs. The existing Depends-on selector keeps its prior search behavior while using the new ranking signals.

Validation results:

- Focused Vitest suite: 8 files and 138 tests passed.
- `node_modules/.bin/tsc --noEmit`: passed.
- `pnpm run lint`: passed.
- `pnpm run i18n:check`: passed; all seven locales and pseudo are complete.
- `pnpm run i18n:ratchet`: passed.
- `git diff --check`: passed.
