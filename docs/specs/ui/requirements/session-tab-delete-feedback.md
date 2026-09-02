---
status: active
system: ui
created: 2026-08-05
amended: 2026-09-02
owners:
  - kandev
---
# Session tab delete feedback Requirements

## Overview

An agent-session tab's X deletes its session after confirmation. Users who want to keep the conversation but remove its panel can choose Hide from the desktop context menu. Mobile users can still delete from the Sessions picker.

## Requirements

### REQ-UI-SESSION-TAB-DELETE-FEEDBACK-001: Session tab close and delete feedback

**Intent:** Deletion and panel visibility are separate, explicit actions. The tab X is the existing confirmed session-delete action. Hide removes only the panel and keeps the conversation available for reopening.

#### Acceptance criteria

- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.1:** Clicking the X opens the session-delete confirmation. Confirming removes the session and its panel. Cancelling leaves both unchanged.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.2:** The desktop context menu offers Hide, which removes only that panel without a confirmation or lifecycle change. A hidden session remains listed under **+ > Agents** and reopening restores the same existing conversation.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.3:** The X is shown only when the task has more than one session and the target session is deletable. It is unavailable for the sole task session or for a running session that cannot be deleted.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.4:** Close Others affects only sibling visible agent-session panels in its Dockview group; it does not remove backend sessions or non-session panels.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.5:** Ordinary synchronization does not recreate a panel explicitly hidden during the mounted layout; newly created sessions and explicit reopen targets still open.
- **AC-UI-SESSION-TAB-DELETE-FEEDBACK-001.6:** Desktop context-menu Delete and mobile Sessions-picker Delete retain their existing confirmation and permanent deletion behavior.

## Migrated source detail

## Why

Users need to delete a session from the tab while also having a safe panel-only option. The X retains confirmation before permanent deletion. The explicit Hide command
provides panel-only removal, while the explicit Delete command remains available from the context menu. The existing
**Close Others** action also previously removed panels only briefly because session synchronization
recreated every missing sibling panel.

Permanent deletion remains a deliberate lifecycle action in the session context menu and mobile
Sessions picker. Promoting a session to primary is also a routine tab state change whose successful
result does not need a global toast.

Session deletion is available from several surfaces. Confirmation should stay beside the
initiating action where the layout permits, instead of opening a second blocking surface that
hides the session context the user is acting on.

## What

- Clicking a desktop agent-session tab's X opens the existing confirmation and, after confirmation, calls the session deletion API and removes its Dockview panel.
- The context-menu Hide command removes only the Dockview panel. It does not call the session deletion
  API or change session lifecycle state.
- The X is available only while the task has more than one session and the target session is deletable. A running or sole session has no X.
- A hidden session remains available under **+ > Agents** and reopening it restores the same
  session and conversation.
- **Close Others** removes the other visible agent-session panels in the current group. It does not
  close neighboring Plan, review, file, or other non-session panels, and synchronization does not
  recreate the hidden agent panels.
- A newly added backend session still opens automatically. Synchronization tracks only explicit Hide
  actions as hidden; Dockview drag, restore, and reconciliation do not imply hidden intent.
- Deletion from the session context menu continues to open the existing confirmation dialog, then
  permanently removes the session and uses the existing toast feedback.
- Deletion from the mobile Sessions picker remains available and keeps its existing confirmation
  and feedback behavior.
- Promoting a non-primary agent session to primary updates the primary marker without a progress or
  success toast. If the promotion fails, one error toast explains the failure.

## Failure modes

- An explicit Hide that races with active-session synchronization remains closed; synchronization may
  ensure the effective active session but must not auto-create intentionally hidden siblings.
- A rejected or timed-out explicit delete request does not remove local session state or its
  Dockview panel, and the existing error detail is surfaced in one toast.
- A failed primary-session promotion leaves the existing primary session unchanged and surfaces one
  error toast; a successful promotion surfaces no progress or success toast.

## Scenarios

- **GIVEN** a task with two deletable agent-session tabs, **WHEN** the user clicks one tab's X and confirms,
  **THEN** that session and its panel are removed and the remaining tab stays active.
- **GIVEN** a session panel was hidden from its context menu, **WHEN** the user opens **+ > Agents** and selects
  that session, **THEN** the same session panel opens again with its existing conversation.
- **GIVEN** only one agent-session panel remains visible, **WHEN** its tab renders, **THEN** it does
  not offer an X even if another task session is hidden.
- **GIVEN** two visible session tabs and neighboring non-session panels, **WHEN** the user chooses
  **Close Others** on one session tab, **THEN** only the other session tabs close and remain hidden
  through active-session synchronization.
- **GIVEN** a session was explicitly deleted from the desktop context menu, **WHEN** deletion
  succeeds, **THEN** its backend session and panel are removed.
- **GIVEN** a task with a non-primary agent session, **WHEN** the user chooses Set as Primary from
  an agent-session action menu, **THEN** the selected session becomes primary without a progress or
  success toast.
- **GIVEN** a primary-session promotion request fails, **WHEN** the request settles, **THEN** the
  current primary session remains unchanged and one error toast is shown.
- **GIVEN** the user cancels an explicit delete confirmation, **WHEN** the dialog closes, **THEN**
  the session and its panel remain unchanged.
- **GIVEN** a phone viewport, **WHEN** the user deletes a session from the Sessions picker, **THEN**
  the mobile flow remains reachable and removes the selected session without relying on a desktop
  tab X.

## Out of scope

- Removing or redesigning the explicit delete confirmation dialog.
- Changing backend session-deletion semantics or active-session selection.
- Changing the mobile session picker layout or controls; its shared primary-session action follows
  the same no-success-toast feedback rule.
- Replacing feedback for context-menu, mobile, stop, or resume actions.
- Persisting the set of hidden session panels across a full layout remount or browser restart.
