---
status: active
system: ui
created: 2026-08-05
owners:
  - kandev
---
# Session tab delete feedback Requirements

## Overview

Deleting an agent session from the task tab currently shows both a progress toast and a success toast. The repeated global notifications are noisy for an action whose initiating control can show its progress directly, and the pending tab control must use the same circular visual language as the neighboring terminal-tab close action. Promoting a session to primary is also a routine tab state change whose successful result does not need a global toast.

## Requirements

### REQ-UI-SESSION-TAB-DELETE-FEEDBACK-001: Session tab delete feedback

**Intent:** Deleting an agent session from the task tab currently shows both a progress toast and a success toast. The repeated global notifications are noisy for an action whose initiating control can show its progress directly, and the pending tab control must use the same circular visual language as the neighboring terminal-tab close action. Promoting a session to primary is also a routine tab state change whose successful result does not need a global toast.

#### Acceptance criteria

- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.1:** Clicking the X on a deletable agent-session tab continues to open the existing delete confirmation dialog.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.2:** Choosing Delete from a desktop session context menu keeps that menu mounted and opens a compact, non-modal confirmation popover anchored to the Delete item. Cancelling or dismissing the popover leaves the session unchanged.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.3:** On phone, choosing Delete from a row in the Sessions picker morphs that row into touch-sized Cancel and Delete actions. It does not open another dialog or drawer. Cancelling, selecting another session, or closing the picker clears the pending confirmation without deleting.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.4:** Desktop and phone confirmation surfaces share the same conversation-deletion, workspace-retention, primary-session, and only-session warnings.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.5:** After the user confirms, the X is replaced in place by a compact circular indeterminate spinner matching the terminal-tab close action, not the grid-shaped activity spinner, until the delete request settles. The close action is non-interactive and exposed as busy while the request is pending.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.6:** X-initiated deletion does not show progress or success toasts.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.7:** Successful deletion keeps the existing behavior: the deleted session and its tab disappear, and another session becomes active when needed.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.8:** If deletion fails, the session and tab remain, the spinner returns to the X, and one error toast explains the failure so the user can retry.

## Migrated source detail

## Why

Deleting an agent session from the task tab currently shows both a progress toast and a success
toast. The repeated global notifications are noisy for an action whose initiating control can show
its progress directly, and the pending tab control must use the same circular visual language as
the neighboring terminal-tab close action. Promoting a session to primary is also a routine tab
state change whose successful result does not need a global toast.

Session deletion is available from several surfaces. Confirmation should stay beside the
initiating action where the layout permits, instead of opening a second blocking surface that
hides the session context the user is acting on.

## What

- Clicking the X on a deletable agent-session tab continues to open the existing delete
  confirmation dialog.
- Choosing Delete from a desktop session context menu keeps that menu mounted and opens a compact,
  non-modal confirmation popover anchored to the Delete item. Cancelling or dismissing the popover
  leaves the session unchanged.
- On phone, choosing Delete from a row in the Sessions picker morphs that row into touch-sized
  Cancel and Delete actions. It does not open another dialog or drawer. Cancelling, selecting
  another session, or closing the picker clears the pending confirmation without deleting.
- Desktop and phone confirmation surfaces share the same conversation-deletion,
  workspace-retention, primary-session, and only-session warnings.
- After the user confirms, the X is replaced in place by a compact circular indeterminate spinner
  matching the terminal-tab close action, not the grid-shaped activity spinner, until the delete
  request settles. The close action is non-interactive and exposed as busy while the request is
  pending.
- X-initiated deletion does not show progress or success toasts.
- Successful deletion keeps the existing behavior: the deleted session and its tab disappear, and
  another session becomes active when needed.
- If deletion fails, the session and tab remain, the spinner returns to the X, and one error toast
  explains the failure so the user can retry.
- Promoting a non-primary agent session to primary updates the primary marker without a progress or
  success toast. If the promotion fails, one error toast explains the failure.
- After local confirmation, context-menu and mobile deletion retain the default request progress,
  success, and error feedback.

## Failure modes

- A rejected or timed-out delete request does not remove local session state or its Dockview panel.
  The close action becomes available again and the existing error detail is surfaced in one toast.
- A failed primary-session promotion leaves the existing primary session unchanged and surfaces one
  error toast; a successful promotion surfaces no progress or success toast.
- Repeated activation while deletion is pending does not dispatch another delete request.

## Scenarios

- **GIVEN** a task with two deletable agent sessions, **WHEN** the user clicks one tab's X and
  confirms deletion, **THEN** that X shows a spinner without progress or success toasts until the
  session and tab are removed.
- **GIVEN** an X-initiated delete request is pending, **WHEN** the session tab renders its pending
  close state, **THEN** the close affordance shows the compact circular spinner used for terminal-tab
  closing and does not render the grid-spinner treatment.
- **GIVEN** an X-initiated delete request is pending, **WHEN** the user attempts to activate the
  close control again, **THEN** no duplicate delete request is dispatched.
- **GIVEN** an X-initiated delete request fails, **WHEN** the request settles, **THEN** the session
  tab remains, the X becomes available again, and one error toast is shown.
- **GIVEN** a task with a non-primary agent session, **WHEN** the user chooses Set as Primary from
  an agent-session action menu, **THEN** the selected session becomes primary without a progress or
  success toast.
- **GIVEN** a primary-session promotion request fails, **WHEN** the request settles, **THEN** the
  current primary session remains unchanged and one error toast is shown.
- **GIVEN** the user cancels the delete confirmation, **WHEN** the dialog closes, **THEN** the tab
  remains unchanged and no spinner or deletion toast appears.
- **GIVEN** a desktop session context menu, **WHEN** the user chooses Delete, **THEN** a compact
  confirmation popover stays anchored to that menu item without opening an alert dialog or starting
  deletion.
- **GIVEN** a phone viewport, **WHEN** the user chooses Delete from a Sessions picker row, **THEN**
  that row shows touch-sized inline Cancel and Delete actions without opening an alert dialog.
- **GIVEN** a phone row has pending delete confirmation, **WHEN** the user closes the Sessions
  picker externally, **THEN** the pending confirmation is cleared and reopening the picker shows
  the normal row actions without dispatching deletion.
- **GIVEN** a phone viewport, **WHEN** the user confirms deletion from the inline row actions,
  **THEN** the selected session is removed and the remaining session stays reachable without
  relying on a desktop tab X.

## Out of scope

- Removing or redesigning the tab-X delete confirmation dialog.
- Changing backend session-deletion semantics, active-session selection, or Dockview reconciliation.
- Changing the Sessions picker hierarchy, drawer, or non-delete row actions.
- Replacing feedback for context-menu, mobile, stop, or resume actions.
