---
status: shipped
created: 2026-07-02
owner: José Almeida
---

# Quick Chat Persistence & Expiration

## Why

A user reported her quick chats "disappearing much faster." Quick chats are
ephemeral tasks whose open-tab list lives only in the in-memory frontend store,
so a page reload drops every tab even though the underlying task, session, and
workspace directory still exist on the backend. Those now-unreachable rows and
directories never get cleaned up, so they leak indefinitely. Users want their
recent quick chats to survive a reload, and the system needs a bound so
abandoned quick chats don't accumulate forever.

## What

- Open quick-chat tabs SHALL be restored after a full page reload, reconstructed
  from backend state rather than client-only memory.
- Hydration SHALL resolve the workspace represented by the current route. On `/t/:id`
  and `/office/tasks/:id` task-detail routes, the task's workspace is authoritative over
  a stale active-workspace cookie or user setting.
- A quick chat that has been idle longer than the retention window SHALL be
  deleted automatically (task row, its sessions, and its workspace directory),
  the same as an explicit close.
- The default retention window is **7 days** of inactivity, applied per quick
  chat based on its last activity. The window is a hardcoded constant; it is not
  user-configurable in this iteration.
- Expiration SHALL reuse the existing quick-chat delete path so that workspace
  directory cleanup and task-lifecycle events fire exactly as they do for a
  user-initiated close.
- Only quick chats expire. Config-mode chats, automation-run ephemeral tasks,
  and non-ephemeral kanban/office tasks are untouched.
- A quick chat that is actively in use (recent message or running session)
  SHALL NOT expire, regardless of how old it is.

## Data model

No new tables. This feature operates on existing `tasks` and `task_sessions`
rows. A **quick chat** is precisely the set of tasks matching:

```
tasks
  is_ephemeral = 1
  workflow_id  = ''                      -- ephemeral tasks carry no workflow
  origin      != 'automation_run'        -- excludes run-mode ephemeral tasks
  metadata.config_mode IS NOT 1          -- excludes config-mode settings chats
  archived_at  IS NULL
```

This mirrors the existing list filters: `onlyEphemeral = true` plus the
`excludeConfigModePredicate` (`json_extract(metadata,'$.config_mode') IS NOT 1`,
`internal/task/repository/sqlite/task.go:62`) plus an `origin != 'automation_run'`
guard. The `origin` guard distinguishes user-started quick chats (`origin =
'manual'`) from automation run-mode ephemeral tasks.

**Last-activity timestamp** for a quick chat is defined as:

```
last_activity = MAX(task.updated_at, MAX(session.updated_at) over its task_sessions)
```

Message and turn writes update their own rows only. A dispatched prompt changes
the quick-chat session's runtime state, which updates `task_sessions.updated_at`;
the associated task runtime-state transition can also update `tasks.updated_at`.
Using the greater of the task row and its newest session row therefore captures
the persisted activity written by the shipped prompt lifecycle. A quick chat
expires when `now - last_activity > 7 days`.

## API surface

- **Hydration (read):** reuse the existing workspace task listing rather than a
  new endpoint. The backend boot payload fetches quick chats via
  `ListTasksByWorkspace(workspaceId, workflowID="", …, onlyEphemeral=true,
  excludeConfig=true)` and filters out `origin == "automation_run"`, then
  rebuilds the `quickChat.sessions` store slice (one tab per returned task, in
  creation order). Each restored tab needs
  `{ sessionId, workspaceId, agentProfileId }`, all resolvable from the task and
  its primary session. The persisted display name is already available via
  `getStoredQuickChatName` / `setStoredQuickChatName`.
- **Deletion (expiration):** no new external contract. Expiration runs as a
  backend background sweep that calls the existing `Service.DeleteTask` path
  (which invokes `cleanupQuickChatDirs`, `internal/task/service/service_tasks.go:1402`,
  and publishes `task.deleted`). Deleted quick chats propagate to any connected
  client through the existing `task.deleted` WS handler.

## State machine

A quick chat task is created `active`, and reaches a terminal `deleted` state via
one of these triggers (this feature adds only the last one):

| Trigger                                     | Actor                | Existing?                                            |
| ------------------------------------------- | -------------------- | ---------------------------------------------------- |
| User closes a tab (confirm)                 | user                 | yes (`use-quick-chat-modal.ts` `handleConfirmClose`) |
| Rapid re-pick supersedes an in-flight start | frontend             | yes (`useAgentSelection` orphan cleanup)             |
| Agent profile deleted                       | user                 | yes (`DeleteEphemeralTasksByAgentProfile`)           |
| **Idle longer than retention window**       | **background sweep** | **new**                                              |

All triggers use the existing quick-chat deletion cleanup and event path.
Expiration first rechecks the expiry predicate at delete time, then performs
the same directory cleanup and event publishing.

## Failure modes

- **Last-activity query fails / returns error:** the sweep fails closed — it
  deletes nothing that pass and logs a warning. A transient DB error must never
  cause an over-broad delete.
- **A single quick chat delete fails** (dir busy, session stop error): the sweep
  logs the failure, leaves that task in place, and continues with the rest. One
  bad row does not abort the batch. The next sweep retries it.
- **Hydration list query fails on boot:** quick chat tabs render empty (current
  behavior); the app is otherwise unaffected. No error toast is required — this
  degrades to today's behavior.
- **A hydrated tab's backend session is already stopped/gone** (e.g. backend was
  restarted between reloads): the tab still restores and shows its history;
  sending a new message follows the normal resume/relaunch path. A tab whose
  task no longer exists is simply not returned by the list and thus not shown.
- **Clock skew / non-monotonic time:** expiration compares stored timestamps to
  `now`; it does not assume monotonic wall clock beyond the 7-day granularity.

## Persistence guarantees

- **Survives reload:** open quick-chat tabs (restored from backend), quick-chat
  task rows, sessions, and workspace directories — until they expire or are
  closed.
- **Survives backend restart:** quick-chat task rows and directories (subject to
  the same 7-day idle expiration). Tab restoration on the client depends only on
  the backend list, so it works across a backend restart as long as the task row
  still exists.
- **Does NOT survive:** a quick chat idle > 7 days (deleted on the next sweep).
  The in-memory `quickChat.sessions` slice remains client-only and is rebuilt
  from the backend on each load rather than persisted to `localStorage`.
- **Retention window:** 7 days of inactivity, hardcoded. Sweep cadence is an
  implementation detail (piggy-backing on an existing periodic job is
  acceptable) but SHALL run at least daily so expiry is observed within roughly
  a day of the window elapsing.

## Scenarios

- **GIVEN** an open quick chat with one or more tabs, **WHEN** the user reloads
  the page, **THEN** the same tabs reappear in creation order,
  each showing its prior chat history and persisted name.
- **GIVEN** Quick Chats created from a task whose workspace differs from the
  persisted active-workspace setting, **WHEN** the user reloads the task route,
  **THEN** the task workspace's tabs restore without tabs from another workspace.
- **GIVEN** a quick-chat task whose `last_activity` is 8 days ago, **WHEN** the
  expiration sweep runs, **THEN** the task, its sessions, and its workspace
  directory are deleted, a `task.deleted` event is published, and the tab
  disappears from any connected client.
- **GIVEN** a quick-chat task with a message sent 10 minutes ago (older than 7
  days since creation but recently active), **WHEN** the sweep runs, **THEN** the
  task is retained.
- **GIVEN** a config-mode chat (`metadata.config_mode = 1`) idle for 30 days,
  **WHEN** the sweep runs, **THEN** it is NOT deleted.
- **GIVEN** an automation-run ephemeral task (`origin = 'automation_run'`) idle
  for 30 days, **WHEN** the sweep runs, **THEN** it is NOT deleted.
- **GIVEN** a non-ephemeral kanban task idle for a year, **WHEN** the sweep runs,
  **THEN** it is NOT deleted.
- **GIVEN** the last-activity query errors, **WHEN** the sweep runs, **THEN** no
  quick chats are deleted and a warning is logged.
- **GIVEN** two idle quick chats where deleting the first fails, **WHEN** the
  sweep runs, **THEN** the second is still evaluated and deleted, and the failure
  on the first is logged.
- **GIVEN** the workspace has 12 quick chats within the retention window, **WHEN**
  the user reloads, **THEN** all 12 tabs restore (no count cap in this
  iteration).

## Out of scope

- A user-configurable retention window (per-workspace or global setting). Ships
  as a hardcoded 7-day constant; revisit only if requested.
- A hard count cap on number of quick chats (idle TTL only). If clutter becomes
  a problem, a cap is a follow-up.
- Persisting quick-chat tabs to `localStorage`. Backend is the source of truth;
  no client-side durable store.
- Archiving/soft-deleting expired quick chats — they are hard-deleted, matching
  the existing close behavior.
- Changing config-mode chat, automation-run, or kanban/office task lifecycles.

## Verified implementation

- Raw message and turn persistence does not update the parent task or session.
- Prompt dispatch and completion transition the quick-chat session state and
  update `task_sessions.updated_at`.
- A quick-chat task runtime-state transition can also update `tasks.updated_at`.
- The expiration query reads both parent timestamps and excludes `RUNNING` and
  `IDLE` sessions, so active or recently completed prompt lifecycles are retained.
