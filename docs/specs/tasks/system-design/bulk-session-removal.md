---
status: current
system: tasks
requirements:
  - REQ-TASKS-BULK-SESSION-REMOVAL-001
  - REQ-TASKS-BULK-SESSION-REMOVAL-002
created: 2026-09-27
owners:
  - kandev
---

# Bulk Session Removal System Design

## Boundaries

The task system owns the persisted session universe and deletion lifecycle.
The web client owns target presentation, confirmation, ordered client
orchestration, local projection cleanup, and responsive access. The existing
`session.delete` request remains the only deletion wire action; no persistence
or backend contract changes.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-BULK-SESSION-REMOVAL-001` | [Target snapshots](#target-snapshots), [Deletion orchestration](#deletion-orchestration), [Desktop presentation](#desktop-presentation) |
| `REQ-TASKS-BULK-SESSION-REMOVAL-002` | [Phone presentation](#phone-presentation), [Localization and accessibility](#localization-and-accessibility) |

## Target snapshots

The owner reads `useTaskSessions(taskId)` and its loaded/loading state. It
builds the target set from the persisted task-session list, not Dockview
panels. Remove Others excludes the invoking `sessionId`; Remove All does not.
The action is unavailable for an unloaded or loading list, an empty target set,
or a target in `RUNNING` or `STARTING`. `WAITING_FOR_INPUT` remains eligible
because the single-session deletion contract permits it.

Opening confirmation captures target IDs, their state eligibility, action
scope, selected session ID, and count. Immediately before submission, the
owner rebuilds the set and compares it with that snapshot. A changed set or
state closes the old confirmation and requires the user to confirm the new
count. The task ID and selected session ID are fixed inputs, so session IDs
from another task and non-session panels cannot enter the request set.

## Deletion orchestration

A shared web helper supplies pure target selection and ordered deletion. It
uses the existing `useSessionActions.remove` cleanup path for each session ID,
so successful requests remove task projections, Quick Chat projections where
applicable, and stale session UI state only after the backend accepts deletion.
It performs sequential requests rather than `Promise.all`.

For Remove Others, non-primary targets are requested before a primary target
when ordering information is available, and the selected session is excluded.
For Remove All, non-primary targets precede the primary target where possible.
Each successful request is final. On the first false or rejected result, the
loop stops and reports the exact completed and remaining target counts; it does
not remove the failed target locally or attempt rollback. A pending guard
prevents re-entry. The backend remains authoritative for authorization,
runtime quiescence, refusal of active sessions, primary promotion, and task
workspace retention.

Remove All suppresses automatic session provisioning so the confirmed empty
state survives navigation and reload. A failed bulk deletion clears that
suppression. Creating a new session through the task's New Session action
clears it after the server returns a session ID, restoring ordinary
single-session deletion behavior for that task.

## Desktop presentation

`SessionContextMenuItems` retains its Dockview-only Close Others callback and
adds a separated destructive Remove Others / Remove All group after it. A
stable coordinator above the removable tab owns bulk confirmation and request
execution, so Remove All can delete the invoking session without unmounting
the confirmation owner. It uses the existing session-delete confirmation
visual language, localized plural count, retained-workspace warning, progress,
and partial-failure feedback.

## Phone presentation

`MobileSessionsPicker` obtains the same target snapshot from its task-session
rows. Each selected session's visible touch action menu exposes the same two
destructive actions. `MobilePickerSheet` remains the confirmation host, so the
confirmation replaces sheet content and returns focus to its source on cancel
or Back. The confirmation surface uses touch-sized controls, an internal
scroll owner where content needs it, and safe-area bottom clearance. After
Remove All, the existing zero-session picker state remains selectable and its
new-session action is usable.

## Localization and accessibility

New copy lives in the `task` namespace. Count-bearing strings use i18next
`_one` / `_other` keys. All six shipped locale catalogs receive matching keys;
Traditional Chinese uses the repository's generated synchronization flow.
Destructive actions use accessible names that include the scope, and disabled
actions retain a discoverable eligibility reason.

## Verification

Pure helper and session-action tests cover task isolation, hidden sessions,
selected-session preservation, zero/loading/active refusal, snapshot changes,
ordering, duplicate submission, and partial failure. Desktop and phone
Playwright flows use the API session list before and after reload to prove that
the persisted conversations are gone. The existing single-session delete and
task-environment ownership tests remain the regression contract for workspace
retention.

## Related contracts

- [Session Delete Preserves Task Workspaces](session-delete-resource-cleanup.md)
- [Session Delete Preserves Task Workspaces Requirements](../requirements/session-delete-resource-cleanup.md)
