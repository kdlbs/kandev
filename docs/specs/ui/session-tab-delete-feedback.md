---
status: shipped
created: 2026-08-05
owner: kandev
---

# Session tab delete feedback

## Why

Deleting an agent session from the task tab currently shows both a progress toast and a success
toast. The repeated global notifications are noisy for an action whose initiating control can show
its progress directly.

## What

- Clicking the X on a deletable agent-session tab continues to open the existing delete
  confirmation dialog.
- After the user confirms, the X is replaced in place by a spinner until the delete request
  settles. The close action is non-interactive and exposed as busy while the request is pending.
- X-initiated deletion does not show progress or success toasts.
- Successful deletion keeps the existing behavior: the deleted session and its tab disappear, and
  another session becomes active when needed.
- If deletion fails, the session and tab remain, the spinner returns to the X, and one error toast
  explains the failure so the user can retry.
- Deletion started from the session context menu or a mobile session action keeps its existing
  feedback behavior.

## Failure modes

- A rejected or timed-out delete request does not remove local session state or its Dockview panel.
  The close action becomes available again and the existing error detail is surfaced in one toast.
- Repeated activation while deletion is pending does not dispatch another delete request.

## Scenarios

- **GIVEN** a task with two deletable agent sessions, **WHEN** the user clicks one tab's X and
  confirms deletion, **THEN** that X shows a spinner without progress or success toasts until the
  session and tab are removed.
- **GIVEN** an X-initiated delete request is pending, **WHEN** the user attempts to activate the
  close control again, **THEN** no duplicate delete request is dispatched.
- **GIVEN** an X-initiated delete request fails, **WHEN** the request settles, **THEN** the session
  tab remains, the X becomes available again, and one error toast is shown.
- **GIVEN** the user cancels the delete confirmation, **WHEN** the dialog closes, **THEN** the tab
  remains unchanged and no spinner or deletion toast appears.
- **GIVEN** a phone viewport, **WHEN** the user deletes a session from the Sessions picker, **THEN**
  the mobile flow remains reachable and removes the selected session without relying on a desktop
  tab X.

## Out of scope

- Removing or redesigning the delete confirmation dialog.
- Changing backend session-deletion semantics, active-session selection, or Dockview reconciliation.
- Replacing feedback for context-menu, mobile, stop, resume, or set-primary actions.
