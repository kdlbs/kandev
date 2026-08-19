---
spec: docs/specs/tasks/task-create-escape-dismissal.md
created: 2026-08-19
status: complete
---

# Implementation Plan: Create Task Escape Dismissal

## Overview

The Create Task dialog uses the default Escape dismissal from Radix Dialog.
Its autocomplete handler receives the event after Radix requests dialog closure.
The fix blocks Radix dismissal only in create mode and lets the event reach the autocomplete handler.

## Confirmed root cause

- PR #1155 added prompt autocomplete to the task-create prompt field.
- `TaskCreateDialog` does not pass `onEscapeKeyDown` to `DialogContent`.
- Radix handles Escape on the document capture phase and requests closure first.
- `useInlineMention` prevents the same event later in the input event path.
- The current E2E assertion checks visibility before the 100 ms close animation completes.
  This timing lets a closing dialog satisfy the assertion.

The smallest reproduction opens Create Task and presses Escape in the prompt field.
The dialog enters `data-state="closed"` with or without an open autocomplete menu.

## Frontend

### Create Task dialog

Update `apps/web/components/task-create-dialog.tsx`.
Pass an `onEscapeKeyDown` handler to `DialogContent`.
Call `preventDefault()` only when `setup.isCreateMode` is true.
Do not stop propagation because the nested autocomplete handler must receive Escape.

Keep Edit Task and New Agent behavior unchanged.
Do not change the shared `@kandev/ui` dialog component.

## Tests

- **What:** Create mode prevents Radix Escape dismissal, while edit mode does not add the create-mode guard.
  **File:** `apps/web/components/task-create-dialog.test.tsx`.
  **How:** expose the mocked `DialogContent` Escape callback and call it with a `preventDefault` spy.

## E2E Tests

- **Scenario:** An open `@` autocomplete menu closes on Escape while Create Task remains open.
  **File:** `apps/web/e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts`.
  **What to verify:** the menu disappears, the query remains, and the dialog keeps `data-state="open"`.
- **Scenario:** Create Task remains open when no autocomplete menu is visible.
  **File:** `apps/web/e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts`.
  **What to verify:** Escape keeps `data-state="open"` and preserves the prompt value.
- **Scenario:** A phone user with a hardware keyboard presses Escape without an open menu.
  **File:** `apps/web/e2e/tests/task/mobile-task-create-escape.spec.ts`.
  **What to verify:** the full-height dialog keeps `data-state="open"` in the `mobile-chrome` project.

## Mobile parity

The existing mobile entry point is the Kanban task-creation action.
The nearest shipped exemplar is `mobile-create-task-webkit-rendering.spec.ts`.
The same `TaskCreateDialog` component owns desktop and mobile Escape behavior.

This fix does not change layout, touch targets, scroll ownership, safe areas, or the primary action.
The new mobile E2E scenario covers a hardware keyboard without changing the phone composition.

## Verification Results

- RED component test: 3 expected failures because `DialogContent` had no Escape handler.
- RED desktop E2E: 2 expected failures because the dialog changed to `data-state="closed"`.
- GREEN component test: 7 tests passed.
- GREEN desktop E2E: 2 Escape scenarios passed.
- GREEN mobile E2E: 1 hardware-keyboard Escape scenario passed.
- Frontend typecheck passed.
- Frontend lint passed.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-guard-dialog-escape](task-01-guard-dialog-escape.md)

This plan does not authorize subagents.

## Open Questions

None.
