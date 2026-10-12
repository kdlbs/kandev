---
status: current
system: ui
requirements:
  - REQ-UI-DELETED-TASK-CLIENT-STATE-001
---

# Deleted Task Client State System Design

## Purpose and boundaries

The `task.deleted` handler in `apps/web/lib/ws/handlers/tasks.ts` already
collects the deleted task sessions to clear local storage and queue status.
The session slice's `removeTaskSession` action
(`apps/web/lib/state/slices/session/session-slice.ts`) already removes one
session's state and calls `purgeSessionRuntimeState`. This design connects the
two. No new store shape or action is added.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-UI-DELETED-TASK-CLIENT-STATE-001.1`, `001.2` | [Removal](#removal) |
| `AC-UI-DELETED-TASK-CLIENT-STATE-001.3` | [Ordering](#ordering) |

## Removal

For each collected session ID, the handler calls
`removeTaskSession(deletedTaskId, sessionId)` in the same loop that clears the
context-files store and queue status. The collection rule is unchanged, so
sessions of other tasks are never passed.

## Ordering

The loop runs after `cleanupTaskStorage`, which still reads
`environmentIdBySessionId` from the state captured at handler entry. It runs
before the kanban, overview, and selection update, which do not read the
removed session maps. Redirect and notification logic reads task identity
only.
