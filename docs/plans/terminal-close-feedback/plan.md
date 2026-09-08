---
spec: docs/specs/ui/requirements/terminal-close-feedback.md
created: 2026-08-14
status: shipped
---

# Implementation Plan: Terminal close feedback

## Overview

Treat confirmation as the terminal's final visible state. Remove the target terminal locally in the
same action, continue `user_shell.destroy` in the background, and notify only on failure. Keep the
backend terminal list authoritative after reload or reconnect.

## Backend

No backend changes. Existing destroy ownership, PTY teardown, compatibility fallback, and terminal
list reconciliation remain authoritative.

## Frontend

### Local confirmation and immediate dismissal

`CloseTerminalConfirmPopover` anchors a compact, non-modal confirmation to the desktop or tablet
close control. It closes its controlled popover before calling `onConfirm`; cancellation and
outside dismissal make no changes and return focus to the close control.

### Shared optimistic destroy lifecycle

`useTerminalDestroy` owns a ref-only set of in-flight terminal IDs. A new request:

- rejects a duplicate ID synchronously;
- invokes `onDestroyed` before awaiting transport so local terminal state and active selection move
  immediately;
- awaits `destroyUserShell` only as background cleanup;
- emits no pending or success feedback; and
- emits one localized error toast on rejection without restoring local UI.

`useTerminals` removes the target from both the environment shell store and its local terminal list
through the existing live active-tab fallback helper.

### Desktop, tablet, and phone surfaces

- Dockview opens a compact confirmation beside the tab close control, then closes the target panel
  immediately after confirmation and marks the close as a terminal termination so layout
  persistence cannot reopen it.
- Tablet right-panel tabs use the same anchored popover and consume the optimistically filtered
  terminal list. They have no teardown spinner or pending transport state.
- Phone terminal rows retain their visible 44px close action and existing picker composition. When
  confirmation is required, that action morphs into touch-sized inline Cancel and Close terminal
  actions. No nested modal or drawer is introduced.
- After phone confirmation, the target row and mounted xterm disappear through shared state before
  teardown settles.
- Final-phone-terminal auto-create guard release still follows a successful teardown result; it is
  background bookkeeping and does not keep terminal UI visible.

### Consistent destructive entry points

- Dockview tab close and context-menu Terminate enter the same close-anchored confirmation flow.
- Tablet and phone close controls always confirm, instead of gating confirmation on client-only
  terminal activity tracking.
- The add-panel terminal row opens the shared popover beside its close control. The parked-terminal
  row replaces its destructive affordance in place with Cancel and Close actions. Neither calls
  browser-native `confirm()`.
- Every confirmation path reaches the same immediate-removal/background-teardown lifecycle.

### Add-panel overlay and row geometry

- The add-panel popover treats focus movement inside its owning dropdown as internal interaction.
  Pointer-driven Radix menu focus therefore cannot dismiss the confirmation while the user moves
  toward its actions; focus elsewhere still dismisses normally.
- The terminal row's trailing close control is positioned outside normal flex layout. Fine-pointer
  rows keep the shared compact menu height, while coarse-pointer rows and close controls retain a
  44px active dimension.

### Localization

Use `task:failedToCloseTerminal` for the sole failure toast. Task-terminal close surfaces no longer
render progress copy.

## Mobile design contract

- **Desktop outcome:** a compact popover stays anchored to the initiating close control. After
  confirmation, popover, Dockview tab, and terminal panel disappear in one action; fallback content
  remains usable.
- **Phone entry point:** the existing terminal pill and `MobilePickerSheet` remain unchanged.
- **Nearest shipped exemplar:** `mobile-terminals-section.tsx` and `mobile-picker-sheet.tsx` retain
  the terminal hierarchy, inset drawer, safe-area handling, and internal scroll ownership.
- **Hierarchy and action:** terminal selection remains primary; close remains a visible 44px
  secondary action. Required confirmation morphs that action in place into 44px Cancel and Close
  terminal controls. No transport state occupies the row.
- **Shared state:** optimistic removal, duplicate suppression, transport, failure notification, and
  fallback selection are shared. Viewport components only render the resulting terminal list.
- **Parity proof:** desktop and `mobile-chrome` scenarios pause the destroy request at the main
  WebSocket boundary and prove the target is already absent while a sibling remains interactive.

## Tests

- A controlled popover test proves confirmation is non-modal and dismisses before an unresolved
  callback settles.
- Destroy-hook tests prove immediate local removal, synchronous duplicate suppression, success
  silence, and error-only failure behavior.
- Dockview tests prove direct and confirmed closes call panel close immediately, never render a
  teardown spinner, suppress repeated transport, and toast after background failure.
- Tablet and phone focused tests prove no pending transport state leaks into controls; the phone
  close target stays at least 44px and busy close morphs into an inline confirmation.
- Entry-point tests prove tab close, context-menu Terminate, add-panel menu destroy, parked-terminal
  destroy, tablet close, and phone close all require the intended localized confirmation before
  teardown.
- Add-panel E2E moves the pointer across the owning menu and still cancels through the popover, then
  compares the terminal row height with an adjacent standard menu item.

## E2E tests

- `terminal-dockview-ui.spec.ts` pauses `user_shell.destroy`, confirms a busy terminal, proves its
  tab is absent, and executes input in the sibling before releasing teardown.
- The Dockview spec also proves tab close, context-menu Terminate, and the add-panel row open the
  anchored popover without emitting a browser dialog.
- `mobile-terminal-close.spec.ts` performs the equivalent touch flow through the phone picker,
  checks the target row disappears while transport is paused, selects the sibling, and verifies no
  document horizontal overflow.
- `terminal-close-pause.ts` is the shared WebSocket transport boundary. It forwards every unrelated
  frame and releases the exact held destroy request after the UI assertion.

## Verification results

- Focused Vitest: 4 files, 12 tests passed.
- Frontend typecheck: passed.
- Targeted ESLint: passed.
- i18n checks and new-code ratchet: passed.
- E2E sleep ratchet: passed.
- Desktop Chromium entry-point and delayed-destroy E2E: 3 tests passed.
- Mobile Chrome delayed-destroy E2E: 1 test passed.
- Add-panel hover and compact-row regression E2E: 1 test passed.

## Implementation waves

- [x] [task-01-nonblocking-close-feedback](task-01-nonblocking-close-feedback.md)
- [x] [task-02-responsive-close-feedback](task-02-responsive-close-feedback.md)
- [x] [task-03-terminal-close-e2e](task-03-terminal-close-e2e.md)
- [x] [task-04-consistent-confirmation-entrypoints](task-04-consistent-confirmation-entrypoints.md)
- [x] [task-05-stable-add-panel-confirmation](task-05-stable-add-panel-confirmation.md)

## Risks

- A failed backend teardown can leave the server terminal present. The current UI remains dismissed,
  while a later reload or reconciliation may surface authoritative server state.
- React state is not a synchronous duplicate guard, so the in-flight set remains ref-backed even
  though it is no longer rendered.
- Holding a destroy request in E2E must happen before the first page navigation so the main socket is
  routed; the shared helper is installed before opening the task.
- Radix dropdown hover changes DOM focus. The add-panel popover must distinguish that owning-menu
  focus transition from a true outside dismissal without becoming globally modal.

## Public documentation

No public documentation change. This changes transient task-terminal behavior without changing a
command, configuration key, workflow, executor contract, or public API.
