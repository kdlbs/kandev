---
id: "03-rename-edit-dialogs"
title: "Apply the hook to the rename and edit dialogs"
status: done
wave: 3
depends_on: ["02-caret-preserving-clamp"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/title-length-limit.md"
---

# Task 03: Apply the hook to the rename and edit dialogs

## Acceptance

- `apps/web/components/task/task-rename-dialog.tsx` (`TaskRenameForm`) wires
  the title `Input` through `useTaskTitleSelectionRestore`: `ref={inputRef}`,
  `onChange={(e) => setValue(clampChange(e))}`. `onFocus` select, `autoFocus`,
  submit, and cancel behavior are unchanged.
- `apps/web/components/task-create-dialog-selectors.tsx` (`InlineTaskName`)
  wires its controlled `<input>` through the hook: `ref={inputRef}`,
  `onChange={(e) => onChange(clampChange(e))}`. The `memo` boundary and the
  autoFocus/select effect are unchanged.
- The desktop and mobile E2E regression tests from Wave 1 now PASS: caret stays
  at 8 (not 60) after typing `XY` mid-title in both dialogs at the 60-character
  cap.
- Existing short-title behavior is unchanged (the Wave 1 specs' assertion also
  covers this via the value-length check).

## Verification

- `cd apps/web && pnpm e2e:raw tests/kanban/task-title-caret.spec.ts`
- `cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/task/mobile-task-title-caret.spec.ts`
- `cd apps && pnpm --filter @kandev/web test -- --run apps/web/hooks/use-task-title-selection-restore.test.tsx`

## Files likely touched

- `apps/web/components/task/task-rename-dialog.tsx`
- `apps/web/components/task-create-dialog-selectors.tsx`

## Dependencies

- Wave 2 (the hook) must exist.

## Parallelism

Sequential. Two dialog files, one shared hook contract; no other task edits
these files.

## Inputs

- The hook API from Task 02.
- The `task-title-input` testid and `create-task-dialog` testid used by the E2E
  specs (unchanged).
- The amended spec scenario this task makes true.

## Output contract

Report the changed files, the exact E2E and unit commands with results (green
after this task), and the task status in the same conversation.
