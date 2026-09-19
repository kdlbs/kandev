---
id: "02-caret-preserving-clamp"
title: "Implement the caret-preserving clamp hook"
status: done
wave: 2
depends_on: ["01-regression-e2e"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/title-length-limit.md"
---

# Task 02: Implement the caret-preserving clamp hook

## Acceptance

- `apps/web/hooks/use-task-title-selection-restore.ts` exports
  `useTaskTitleSelectionRestore(value: string)` returning
  `{ inputRef, clampChange }`:
  - `clampChange(e)` returns `clampTaskTitleInput(e.target.value)`; it records
    the DOM `selectionStart`/`selectionEnd` in a ref when the clamp truncates
    (`next !== e.target.value`) and clears the ref on a non-truncating change,
    so a stale record (e.g. from typing at the very end at the cap, which never
    commits) cannot be replayed by a later commit.
  - A `useLayoutEffect` keyed on `value` restores the recorded selection with
    `setSelectionRange(min(start, value.length), min(end, value.length))` and
    clears the record; it skips when the input is not focused or no selection
    was recorded.
  - The ref type supports both `HTMLInputElement` and `HTMLTextAreaElement`.
- `use-task-title-selection-restore.test.tsx` passes:
  - a truncating change (61 code points) pins the caret to the recorded
    position, not the end;
  - a non-truncating change leaves the caret untouched and clears any stale
    recorded selection (a later non-truncating change must not replay it);
  - the restore is skipped when the input is not focused.
- `clampTaskTitleInput` and `apps/web/lib/task-title.ts` are unchanged.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run apps/web/hooks/use-task-title-selection-restore.test.tsx`
- `cd apps && pnpm --filter @kandev/web test -- --run apps/web/lib/task-title.test.ts`

## Files likely touched

- `apps/web/hooks/use-task-title-selection-restore.ts` (new)
- `apps/web/hooks/use-task-title-selection-restore.test.tsx` (new)

## Dependencies

- Wave 1 (failing tests) must exist so this task's unit test can go green.

## Parallelism

Sequential. No other task edits these files.

## Inputs

- The `clampChange`/ref/layout-effect design in `plan.md` ("New hook").
- Test-drive pattern: `apps/web/components/task-create-dialog-selectors.test.tsx`
  sets `setSelectionRange` then asserts `selectionStart` under `act`, which
  works with happy-dom.
- The hook must not add user-facing copy (i18n ratchet applies to the files it
  touches).

## Output contract

Report the hook implementation, the exact unit test command and result, and the
task status in the same conversation. Do not modify the dialog components in
this task.
