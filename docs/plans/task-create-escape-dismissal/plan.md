---
spec: docs/specs/tasks/requirements/task-create-escape-dismissal.md
created: 2026-08-19
status: complete
---

# Implementation Plan: Create Task Escape Dismissal

## Overview

The Create Task dialog uses the default Escape dismissal from Radix Dialog.
Its autocomplete handler receives the event after Radix requests dialog closure.
The first fix blocked Radix dismissal only in create mode and relied on the event reaching the
autocomplete's later bubble handler. The follow-up makes autocomplete dismissal and focus retention
capture-safe across browser runtimes.

## Confirmed root cause

- PR #1155 added prompt autocomplete to the task-create prompt field.
- `TaskCreateDialog` does not pass `onEscapeKeyDown` to `DialogContent`.
- Radix handles Escape on the document capture phase and requests closure first.
- `useInlineMention` prevents the same event later in the input event path.
- The current E2E assertion checks visibility before the 100 ms close animation completes.
  This timing lets a closing dialog satisfy the assertion.
- The first repair added a create-mode Radix guard but left autocomplete handling in the textarea's
  React bubble handler. The PR preview can therefore keep the dialog open while a browser/runtime
  moves focus before that later handler closes the menu.
- Nested Escape behavior must be claimed in capture phase with both `preventDefault()` and
  `stopPropagation()`, then explicitly restore prompt focus after the event cycle.

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

### Prompt autocomplete

Update `apps/web/components/task-create-dialog-selectors.tsx` and
`apps/web/hooks/use-inline-mention.ts`.
Run the mention keyboard handler from the prompt textarea's capture phase while leaving the normal
form shortcut handler in bubble phase. When Escape closes an open mention menu, claim the event and
restore the prompt textarea's focus on the next animation frame.

## Tests

- **What:** Create mode prevents Radix Escape dismissal, while edit mode does not add the create-mode guard.
  **File:** `apps/web/components/task-create-dialog.test.tsx`.
  **How:** expose the mocked `DialogContent` Escape callback and call it with a `preventDefault` spy.
- **What:** Mention Escape runs even when a target listener prevents the later bubble phase.
  **File:** `apps/web/components/task-create-dialog-selectors.test.tsx`.
  **How:** stop propagation at the textarea target and assert the mention handler already ran in capture.
- **What:** Mention Escape claims the event and restores input focus.
  **File:** `apps/web/hooks/use-inline-mention.test.ts`.
  **How:** open a mention menu, press Escape, and flush the scheduled focus restoration.

## E2E Tests

- **Scenario:** An open `@` autocomplete menu closes on Escape while Create Task remains open.
  **File:** `apps/web/e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts`.
  **What to verify:** the menu disappears, the query remains, the dialog keeps `data-state="open"`,
  the textarea stays focused, and typing continues.
- **Scenario:** Create Task remains open when no autocomplete menu is visible.
  **File:** `apps/web/e2e/tests/task/task-create-prompt-autocomplete-qa.spec.ts`.
  **What to verify:** Escape keeps `data-state="open"` and preserves the prompt value.
- **Scenario:** A phone user with a hardware keyboard presses Escape without an open menu.
  **File:** `apps/web/e2e/tests/task/mobile-task-create-escape.spec.ts`.
  **What to verify:** the full-height dialog keeps `data-state="open"` in the `mobile-chrome` project.
- **Scenario:** A phone user with a hardware keyboard closes an open `@` menu.
  **File:** `apps/web/e2e/tests/task/mobile-task-create-escape.spec.ts`.
  **What to verify:** the menu closes and prompt focus remains in the shared mobile dialog.
- **Scenario:** Task chat closes open `@` and `#` menus without losing editor focus.
  **File:** `apps/web/e2e/tests/chat/entity-reference-composer.spec.ts`.
  **What to verify:** typing continues in the same editor after Escape.

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
- Follow-up RED unit run: 2 expected failures and 32 passing tests. The mention handler did not
  receive target-stopped Escape during capture, and Escape did not stop propagation or restore focus.
- Follow-up GREEN unit run: 34 tests passed across the inline mention hook and Task Form Inputs.
- Follow-up desktop E2E: 1 Create Task autocomplete focus and continued-typing scenario passed.
- Follow-up mobile E2E: 2 hardware-keyboard Escape scenarios passed, including autocomplete focus.
- Follow-up task-chat E2E: 2 `@` and `#` dismissal focus and continued-typing scenarios passed.
- Follow-up frontend typecheck and changed-file ESLint passed.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-guard-dialog-escape](task-01-guard-dialog-escape.md)

Wave 2 (sequential):

- [x] [task-02-capture-autocomplete-escape](task-02-capture-autocomplete-escape.md)

This plan does not authorize subagents.

## Open Questions

None.
