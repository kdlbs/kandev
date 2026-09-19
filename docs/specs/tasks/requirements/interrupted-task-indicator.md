---
status: active
system: tasks
created: 2026-08-02
updated: 2026-08-09
owners:
  - kandev
---
# Interrupted Task Indicator Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001: Interrupted Task Indicator

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-INTERRUPTED-TASK-INDICATOR-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

When Kandev crashes, is killed, or restarts while tasks are mid-turn, those
sessions are interrupted: the agent process is gone and the turn was cut short.
Startup reconciliation already flips such sessions to `WAITING_FOR_INPUT` and
the task to `REVIEW`, so after a restart an interrupted task is visually
indistinguishable from a task that finished its turn and is waiting normally.
Operators cannot tell which tasks lost work to the crash. This spec adds a
durable "interrupted" marker set at startup for tasks whose sessions were
mid-turn, surfaced as a red icon in the task list.

## What

- At startup, when reconciliation finds a session that was `STARTING` or
  `RUNNING` (mid-turn) when the backend died, the owning task is marked
  interrupted with a durable task metadata key `interrupted_at` holding the
  reconciliation timestamp (RFC 3339 UTC).
- Sessions that were `WAITING_FOR_INPUT`, `IDLE`, `CREATED`, or terminal at
  startup are not marked: they were not interrupted mid-work.
- Archived or deleted tasks are not marked (they do not appear in the active
  task list).
- The task API exposes `interrupted: true` on the task DTO while the marker is
  present; it is derived from the metadata key, so no schema migration is
  required.
- Task list surfaces render a red alert icon beside the task while
  `interrupted` is true: sidebar task rows, the mobile task-switcher drawer
  (shared row rendering), kanban board cards, graph/swimlane nodes, the rich
  task-list row, and the open-task state icon. The icon carries an accessible
  label and tooltip ("Interrupted by restart").
- The marker clears when a session of the task next enters `STARTING` or
  `RUNNING` — work has resumed, so the task is no longer interrupted — and the
  cleared state is published over the existing `task.updated` event so open
  clients drop the icon live.
- The red icon renders instead of the idle/done affordances only. Pending
  permission, pending clarification, generating/background activity, and
  running spinners keep their existing precedence; terminal `COMPLETED`,
  `FAILED`, and `CANCELLED` states keep their existing icons.

## Data model

No schema change. The marker lives in the existing `tasks.metadata` JSON
column under a new reserved key:

```
MetaKeyInterruptedAt = "interrupted_at"   // value: RFC 3339 UTC timestamp string
```

Task metadata is already serialized into the API task DTO and already
survives restarts. The repository's existing `SetTaskMetadataKey` /
`RemoveTaskMetadataKey` helpers (concurrent-key-safe JSON patch, both SQLite
and Postgres) are the only writers; nothing else may mutate the key.

## API surface

- `v1.Task` gains `interrupted bool json:"interrupted,omitempty"`.
- The field is computed at DTO conversion time in both task serializers
  (`internal/task/dto` and `internal/task/models` ToAPI) as
  `metadata["interrupted_at"] != nil`. It is not a stored column.
- The field flows through every existing task payload automatically: boot
  payload, `kanban.update`, `task.created`, `task.updated`, `task.state_changed`,
  and `GET /api/v1/tasks/:id`.
- No new HTTP route, WebSocket action, or event type.

## State machine

The marker follows this lifecycle:

- `unmarked`: no `interrupted_at` key.
- `marked`: `interrupted_at` present; task DTO reports `interrupted: true`.
- `cleared`: key removed; DTO reports `interrupted: false` (omitted).

Transitions:

- `unmarked -> marked` at startup reconciliation when a preserved
  `executors_running` row's session was `STARTING` or `RUNNING` and the task
  exists and is not archived. Overwriting an existing key is safe: a marked
  task cannot have a mid-turn session at the next startup unless it was
  resumed and crashed again.
- `marked -> cleared` when any session of the task transitions into
  `STARTING` or `RUNNING` (launch, resume, prompt dispatch, agent-ready
  wake). The transition chokepoint in the orchestrator
  (`updateTaskSessionStateWithHook` / `setSessionStarting`; the implementer
  verifies the funnel covers every start path) removes the key and publishes
  `task.updated` only when the key was actually removed.
- Terminal task/session transitions do not clear the marker; terminal state
  icons take precedence over the red icon, and archiving/deleting the task
  removes the row with its metadata.

## Permissions

None. The marker is display metadata on a task the viewer can already see. It
is user-writable only through the generic task metadata update surface and has
no effect beyond the icon.

## Failure modes

- A metadata write fails during reconciliation: the marker is skipped with a
  warning and startup continues; the task simply shows no icon until the next
  restart.
- A metadata removal fails during a session start: the marker survives and the
  next successful start transition clears it; the icon may briefly persist
  behind the running spinner, which takes precedence.
- Remote executors: the backend connection is severed even if the remote agent
  is still alive. The task is marked. If the remote status poll later reports
  the session running again and the session transitions, the marker clears; if
  not, the marker stays until the next resume. This is acceptable — from
  Kandev's perspective the turn was cut off.
- An already-open client during startup reconnects to the boot payload, which
  carries the marker; no startup-time event is required.
- A `task.updated` event that omits `interrupted` must not wipe a set marker in
  client state (frontend merge preserves the field when the payload omits it,
  mirroring `foreground_activity`).

## Persistence guarantees

- The marker is durable in `tasks.metadata` and survives any number of
  restarts until a session start clears it.
- Clearing is a normal key removal; a crash between removal and the
  `task.updated` publish leaves the client icon stale until the next payload,
  never a corrupt row.

## Scenarios

- **GIVEN** a task whose primary session was `RUNNING` when the backend died,
  **WHEN** the backend starts and reconciliation runs, **THEN** the task's
  metadata contains `interrupted_at`, the task DTO reports `interrupted: true`,
  and the sidebar row, board card, graph node, rich list row, and mobile
  switcher row show the red alert icon.
- **GIVEN** that task also has a stored `generating` task summary, **WHEN** reconciliation changes
  the primary session to `WAITING_FOR_INPUT`, **THEN** the summary clears `generating` and the red
  alert icon is not hidden by a stale running spinner.
- **GIVEN** a task whose session was `WAITING_FOR_INPUT` when the backend
  died, **WHEN** reconciliation runs, **THEN** the task is not marked and shows
  no red icon.
- **GIVEN** an archived task with a mid-turn session row, **WHEN**
  reconciliation runs, **THEN** the task is not marked.
- **GIVEN** a marked task, **WHEN** the user resumes the session and it enters
  `STARTING`/`RUNNING`, **THEN** the marker is removed, `task.updated` is
  published, and every surface drops the red icon (running spinners take over).
- **GIVEN** a marked task that is never resumed, **WHEN** the backend restarts
  again, **THEN** the marker survives and the icon still renders.
- **GIVEN** a marked task whose state is `COMPLETED`, **WHEN** any surface
  renders it, **THEN** the green done icon wins and no red icon is shown.
- **GIVEN** a `task.updated` event without an `interrupted` field for a marked
  task, **WHEN** a client merges it, **THEN** the client keeps the marker.

## Out of scope

- Changing the task or session state machine (no new task state; the task
  remains `REVIEW` with a `WAITING_FOR_INPUT` session after reconciliation).
- Marking sessions or tasks `FAILED` on interruption; resumability behavior is
  unchanged.
- Persisting the marker as a typed column; metadata is the single source.
- Office agent cards and the Office dashboard surface, which use their own
  status presentation.
- Reconstructing detached remote-work liveness after restart.
- A user-facing action to clear or dismiss the icon manually.

## Related

- [Task Runtime Cleanup](../system-design/runtime-cleanup.md) — startup reconciliation semantics
  the marker hooks into.
- [Sidebar Task Completion Icons](../../ui/requirements/sidebar-task-completion-icons.md) —
  the shared task-row icon precedence this feature extends.
- [Backend Runtime-State Ownership](../../../plans/backend-runtime-state-ownership/plan.md) —
  startup publication that clears stale generating activity.