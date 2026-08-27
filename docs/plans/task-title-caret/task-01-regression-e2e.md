---
id: "01-regression-e2e"
title: "Write failing regression tests for the caret jump"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/title-length-limit.md"
---

# Task 01: Write failing regression tests for the caret jump

## Acceptance

- `apps/web/e2e/tests/kanban/task-title-caret.spec.ts` exists with two desktop
  tests (edit dialog, rename dialog) that seed a 60-character task title, place
  the caret at position 6 of the title input, type `XY`, and assert the value
  length stays 60 and `selectionStart` is 8.
- `apps/web/e2e/tests/task/mobile-task-title-caret.spec.ts` exists with the
  phone drawer flow (right-click task row in the `Tasks` dialog) covering the
  same rename + edit assertions.
- `apps/web/hooks/use-task-title-selection-restore.test.tsx` exists with a
  focused test that renders a controlled input through the (not yet existing)
  hook and asserts the caret is pinned after a truncating change.
- All three files FAIL on the current code for the expected reason: the caret
  lands at the end (60) instead of staying after the inserted text, and the
  hook module does not exist.

## Verification

- `cd apps/web && pnpm e2e:raw tests/kanban/task-title-caret.spec.ts` — both
  tests fail with `selectionStart` 60, not 8.
- `cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/task/mobile-task-title-caret.spec.ts`
  — both tests fail the same way.
- `cd apps && pnpm --filter @kandev/web test -- --run apps/web/hooks/use-task-title-selection-restore.test.tsx`
  — fails (module not found).

## Files likely touched

- `apps/web/e2e/tests/kanban/task-title-caret.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-task-title-caret.spec.ts` (new)
- `apps/web/hooks/use-task-title-selection-restore.test.tsx` (new)

## Dependencies

None.

## Parallelism

Sequential. This is the RED wave for the whole fix.

## Inputs

- The reproduction technique from `plan.md` ("Reproduction evidence"): seed a
  task with `"T".repeat(60)`, open the dialog, `click()` the input,
  `setSelectionRange(6, 6)`, `testPage.keyboard.type("XY")`, then read
  `selectionStart` and the value.
- Existing flows to model: `tests/kanban/dialog-enter-confirms.spec.ts`
  (kanban actions menu), `tests/task/mobile-sidebar-task-actions.spec.ts`
  (phone drawer rename/edit), `pages/kanban-page.ts`, `pages/session-page.ts`.
- The amended spec scenario: caret stays immediately after inserted text at the
  60-character cap.

## Output contract

Report the changed files, the exact failing assertions (observed caret values),
and the task status in the same conversation. Do not implement the fix in this
task. Do not delegate to a subagent without explicit user authorization.
