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
| `AC-UI-DELETED-TASK-CLIENT-STATE-001.4`, `001.5` | [Read lifetime](#read-lifetime) |

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

## Read lifetime

Session reads must be retired when their task session is deleted, including
scheduled retries and pending responses. Retirement is specific to session
identity, not the environment, which may be shared by another live session.
Deletion must prevent stale mounted consumers from recreating the retired read.
When a retired read owned a shared writer group, another active reader in
that group may claim it so its pending result remains usable. Retired read
objects are released after the last subscriber leaves, while the session's
deletion fence remains for the lifetime of the connection scope to prevent a
stale consumer from recreating the read. Scope reset on workspace or connection
changes releases those fences.

Git read handlers classify the repository's typed missing-session error as a
permanent WebSocket `NOT_FOUND` response rather than a generic internal error.
Other database and runtime errors retain their existing error semantics. A client
that missed the deletion event stops retrying this permanent response, while
transient internal and unavailable errors retain recovery behavior.
