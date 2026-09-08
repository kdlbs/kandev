---
status: active
system: ui
created: 2026-08-14
owners:
  - kandev
---
# Terminal close feedback Requirements

## Overview

Closing a task terminal is a UI decision. Waiting for shell teardown after the user confirms leaves the terminal tab, content, or picker row on screen and makes the task surface feel blocked by backend cleanup that should be invisible.

## Requirements

### REQ-UI-TERMINAL-CLOSE-FEEDBACK-001: Terminal close feedback

**Intent:** Closing a task terminal is a UI decision. Waiting for shell teardown after the user confirms leaves the terminal tab, content, or picker row on screen and makes the task surface feel blocked by backend cleanup that should be invisible.

#### Acceptance criteria

- **AC-UI-TERMINAL-CLOSE-FEEDBACK-001.1:** Every action that permanently destroys a task terminal requires destructive confirmation before teardown starts. Cancelling leaves the terminal unchanged.
- **AC-UI-TERMINAL-CLOSE-FEEDBACK-001.2:** Confirmation stays at the action locus instead of blocking the page: desktop and tablet use a compact popover anchored to the close control; the phone close control morphs into inline Cancel and Close terminal actions inside the existing terminal picker row.
- **AC-UI-TERMINAL-CLOSE-FEEDBACK-001.3:** Dockview tab close and context-menu Terminate share the same anchored confirmation. Terminal rows inside the add-panel menu use that same close-anchored popover. Parked-terminal rows morph in place to inline Cancel and Close actions.
- **AC-UI-TERMINAL-CLOSE-FEEDBACK-001.4:** The add-panel confirmation remains open while pointer movement returns focus to its owning menu, so the user can move from the row close control into the popover actions.
- **AC-UI-TERMINAL-CLOSE-FEEDBACK-001.5:** On fine-pointer layouts, the add-panel terminal row keeps the standard compact menu-row height; its trailing close control does not make that row taller than adjacent entries. Coarse-pointer layouts retain a 44px row and close target.
- **AC-UI-TERMINAL-CLOSE-FEEDBACK-001.6:** Task-terminal confirmation never uses a browser-native `confirm()` dialog.
- **AC-UI-TERMINAL-CLOSE-FEEDBACK-001.7:** Terminal close confirmation never adds a page-level backdrop or full modal. Unrelated task UI remains available until the user confirms or dismisses the local confirmation.
- **AC-UI-TERMINAL-CLOSE-FEEDBACK-001.8:** Confirming removes every local representation of the target immediately: the confirmation, terminal tab or picker row, mounted terminal content, and active selection.

## Migrated source detail

## Why

Closing a task terminal is a UI decision. Waiting for shell teardown after the user confirms leaves
the terminal tab, content, or picker row on screen and makes the task surface feel blocked by
backend cleanup that should be invisible.

## What

- Every action that permanently destroys a task terminal requires destructive confirmation before
  teardown starts. Cancelling leaves the terminal unchanged.
- Confirmation stays at the action locus instead of blocking the page: desktop and tablet use a
  compact popover anchored to the close control; the phone close control morphs into inline Cancel
  and Close terminal actions inside the existing terminal picker row.
- Dockview tab close and context-menu Terminate share the same anchored confirmation. Terminal rows
  inside the add-panel menu use that same close-anchored popover. Parked-terminal rows morph in
  place to inline Cancel and Close actions.
- The add-panel confirmation remains open while pointer movement returns focus to its owning menu,
  so the user can move from the row close control into the popover actions.
- On fine-pointer layouts, the add-panel terminal row keeps the standard compact menu-row height;
  its trailing close control does not make that row taller than adjacent entries. Coarse-pointer
  layouts retain a 44px row and close target.
- Task-terminal confirmation never uses a browser-native `confirm()` dialog.
- Terminal close confirmation never adds a page-level backdrop or full modal. Unrelated task UI
  remains available until the user confirms or dismisses the local confirmation.
- Confirming removes every local representation of the target immediately: the confirmation,
  terminal tab or picker row, mounted terminal content, and active selection.
- The existing sibling-terminal fallback happens as part of that immediate local removal.
- Shell teardown continues in the background and never renders a spinner, disabled close control,
  blocking surface, progress toast, or success toast.
- A synchronous in-flight guard prevents duplicate teardown requests without exposing transport
  progress in UI state.
- A failed teardown shows one localized error notification. It does not restore the dismissed
  terminal in the current surface.
- Desktop Dockview, tablet right-panel tabs, the phone terminal picker, and terminal menu rows share
  this behavior.
- Reload and reconnect continue to reconcile from the server's terminal list. If teardown failed
  and the terminal still exists there, a later reconciliation may surface that server truth.

## Failure modes

- A rejected or timed-out teardown request leaves the local terminal dismissed and shows one error
  notification. There is no optimistic rollback or lingering progress state.
- Repeated close activation for a terminal already being torn down does not dispatch another
  teardown request.
- Losing the task or terminal surface while teardown is pending does not affect background cleanup
  or block navigation. The server remains authoritative for the final terminal state.

## Persistence guarantees

The local dismissal is browser state. Terminal existence, shell teardown, and post-reload
reconciliation retain their existing backend ownership and persistence behavior.

## Scenarios

- **GIVEN** a task terminal on desktop, **WHEN** the user confirms its close and teardown remains
  pending, **THEN** the confirmation, tab, and terminal content are gone and the sibling terminal is
  interactive.
- **GIVEN** a task terminal on desktop or tablet, **WHEN** the user activates Close, **THEN** a
  compact confirmation popover opens beside that close control and teardown has not started.
- **GIVEN** a Dockview terminal tab, **WHEN** the user chooses context-menu Terminate, **THEN** the
  same close-anchored popover opens and teardown has not started.
- **GIVEN** a terminal row in the add-panel menu, **WHEN** the user activates its destructive action,
  **THEN** the compact confirmation popover opens beside that row's close control without opening a
  browser dialog.
- **GIVEN** that add-panel confirmation is open, **WHEN** the pointer crosses its owning menu on the
  way to the popover, **THEN** menu focus movement does not dismiss the confirmation and its Cancel
  and Close terminal actions remain reachable.
- **GIVEN** an add-panel menu on a fine-pointer layout, **WHEN** terminal rows are rendered beside
  ordinary menu entries, **THEN** both use the same compact row height.
- **GIVEN** a terminal row in the parked-terminal menu, **WHEN** the user activates its destructive
  action, **THEN** that row shows inline Cancel and Close actions.
- **GIVEN** any task-terminal close request is pending, **WHEN** the user activates its close control
  again, **THEN** no second teardown request is sent.
- **GIVEN** terminal teardown succeeds, **WHEN** the request settles, **THEN** no additional user
  feedback or layout change occurs.
- **GIVEN** terminal teardown fails, **WHEN** the request settles, **THEN** the terminal stays
  dismissed and one error notification is shown.
- **GIVEN** the close confirmation is open, **WHEN** the user cancels it, **THEN** no teardown starts
  and the terminal remains unchanged.
- **GIVEN** a phone viewport with two task terminals, **WHEN** the user confirms a terminal close from
  the terminal picker, **THEN** the inline confirmation, row, and mounted terminal disappear
  immediately and the sibling terminal remains reachable.
- **GIVEN** a phone terminal, **WHEN** its close control is activated, **THEN**
  that row replaces the close control with touch-sized Cancel and Close terminal actions without
  opening another modal or drawer.
- **GIVEN** a tablet viewport, **WHEN** terminal teardown is pending from the right-panel strip,
  **THEN** the target tab is already absent and the task layout remains usable.

## Out of scope

- Removing the terminal activity tracker, which may retain uses outside close confirmation.
- Changing `user_shell.destroy`, PTY termination, terminal persistence, or active-tab fallback
  semantics.
- Changing Quick Terminal tabs in Quick Chat or the terminal on Settings > Agents.
- Generalizing localized confirmation into a repository-wide primitive before another concrete
  consumer establishes the shared API.
- Adding durable teardown jobs, cancellation controls, progress indicators, or success toasts.

## Implementation plan

[Terminal close feedback implementation](../../../plans/terminal-close-feedback/plan.md)
