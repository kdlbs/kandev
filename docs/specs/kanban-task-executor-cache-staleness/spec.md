---
title: Kanban task cache preserves executor fields across merges
status: draft
created: 2026-08-15
owner: kandev
---

# Kanban task cache preserves executor fields across merges

## Problem

The kanban task cache learns a task's primary executor (`primaryExecutorId`,
`primaryExecutorType`, `primaryExecutorName`, `isRemoteExecutor`) from whichever
task payload arrives with it populated. `primary_executor_*` are `omitempty` on
the backend task DTO and are derived per read from the primary session's
executor binding (`apps/backend/internal/task/dto/dto.go:162-164,180`,
`apps/backend/internal/task/service/service_events.go:566-582`); binding an
executor updates only the `task_sessions` row, not the owning task's
`updated_at` (`apps/backend/internal/task/repository/sqlite/session.go:741-743,
907-921`).

Two of the three client code paths that merge a fresh task payload into the
cache already guard the resulting gap explicitly — an `omitempty` field can be
absent on a payload that is otherwise the newest or an equal-timestamp
duplicate, and an absent key must not erase an already-known value:

- `apps/web/lib/ws/handlers/tasks.ts:40-61` (`preservePrimaryExecutorFields`,
  keyed off raw WebSocket payload key presence).
- `apps/web/lib/state/hydration/hydrator.ts`'s `backfillServerDerivedFields`
  guards five dependency-projection fields the same way, but only on its
  "incoming is older" branch, and does not cover executor fields.

Two merge sites have no such guard at all for the executor fields:

- `apps/web/lib/state/hydration/hydrator.ts`'s `mergeKanbanTasks`, on the
  "incoming is newer-or-equal" branch, replaces the cached task object
  wholesale (`draftTasks[idx] = incoming`) with no gap-fill.
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`'s per-workflow
  snapshot merge preserves `primarySessionId`, `primarySessionState`,
  `autopilot`, and `statusSummary` when a fresh response omits them, but has no
  equivalent guard for any of the four executor fields.

Because `mergeKanbanTasks` compares `updatedAt` with `>=` (not `>`), and
because an executor binding never bumps the task row's `updated_at`, a stale or
same-timestamp response that lands after a `task.updated` WebSocket event
already populated the executor fields silently blanks them back out in the
cache. Kanban card executor badges, the task switcher, and
`getWorkspaceSourceCapabilities`-gated affordances (e.g. the workspace "Add
folder" action, which needs a known non-`undefined` executor type to render)
regress to their unknown-executor state even though the executor is already
known.

`isRemoteExecutor` needs its own handling: `toKanbanTask`
(`apps/web/lib/kanban/map-task.ts:199`) maps the wire's `is_remote_executor`
with `?? false`, so the mapped field is never `undefined` — unlike its three
sibling executor fields, which map with `?? undefined`. A plain
"preserve-when-`undefined`" rule therefore cannot detect that
`isRemoteExecutor` was actually omitted from a given payload. `is_remote_executor`
is only ever emitted alongside `primary_executor_type` (both are derived from
the same executor snapshot; see `service_events.go:566-582` and
`dto.go:775-782`), so `primaryExecutorType`'s own `undefined`-ness is the
reliable signal for whether the whole four-field executor bundle was omitted
from a given payload.

## Desired behavior

- When `mergeKanbanTasks` takes the "incoming is newer-or-equal" branch for a
  task already present in the cache, and the incoming task's
  `primaryExecutorType` is `undefined`, the merged task keeps the existing
  cached `primaryExecutorId`, `primaryExecutorType`, `primaryExecutorName`, and
  `isRemoteExecutor` values instead of losing them to the replace.
- When the incoming task's `primaryExecutorType` is defined (a real value, from
  a payload that legitimately carries executor state), the incoming value(s)
  win outright — this is not a sticky/permanent preserve, only a gap-fill for a
  payload that omits the field.
- In `use-all-workflow-snapshots.ts`, a current cached executor bundle wins
  when it changed after the snapshot request started, even when the response
  contains an older explicit executor bundle.
- When the executor bundle did not change after the request started, the full
  snapshot is authoritative. An omitted executor bundle can clear the cache.
- When a task first appears while the request is active, its current cached
  executor bundle wins over a response captured before that live event.
- Every other field's existing merge behavior (timestamp comparison, the
  dependency-projection backfill on the older-incoming branch, unrelated
  fields' wholesale replace) is unchanged.

## Regression scenarios

- **GIVEN** a cached kanban task whose executor is already known (all four
  executor fields populated), **WHEN** a `mergeKanbanTasks` merge receives an
  incoming task with the same `id`, an `updatedAt` equal to or newer than the
  cached value, and no `primary_executor_*`/`is_remote_executor` keys in its
  source payload, **THEN** the merged task keeps its previously known executor
  fields instead of losing them to the wholesale replace.
- **GIVEN** the same cached task, **WHEN** a `mergeKanbanTasks` merge receives
  an incoming task at or after the cached `updatedAt` whose payload legitimately
  carries a different executor type, **THEN** the merged task adopts the new
  executor value(s); the preserve never overrides a real value.
- **GIVEN** a workflow snapshot response whose task list omits the executor
  fields for a task the cache already knows the executor for, **WHEN**
  `useAllWorkflowSnapshots` merges that response, **THEN** the merged snapshot
  task keeps the cached executor fields.
- **GIVEN** a cached task whose executor is known at request start, **WHEN** a
  live event clears the executor and a stale response returns the old executor
  bundle, **THEN** the merged task stays clear.
- **GIVEN** a task that is absent at request start, **WHEN** a live event adds
  the task with executor data and a stale response omits that data, **THEN**
  the merged task keeps the live executor bundle.

## Constraints

- No backend, API, or persistence contract changes; this is a client-side cache
  merge correctness fix using data already delivered on the wire.
- Do not change `toKanbanTask`'s existing `is_remote_executor ?? false`
  mapping or its pinned `map-task.test.ts` contract test — the merge-layer fix
  works around that mapping's loss of "omitted" information for
  `isRemoteExecutor` by gating on `primaryExecutorType`'s own `undefined`-ness
  instead, rather than changing the shared mapper's output contract.
- The snapshot merge uses the request-start cache as its freshness boundary.
  It copies the complete current executor bundle when the task changed after
  request start or first appeared during the request. It does not use omission
  alone as proof of a stale response.
- The hydration merge still uses the mapped `KanbanTask` shape. It cannot
  distinguish an explicit clear from an omitted key in that path, so it keeps
  the existing gap-fill rule and its known limitation.

## Out of scope

- Changing `toKanbanTask` / `map-task.ts`'s field mapping.
- Adding a "session detached" disambiguation signal to the mapped `KanbanTask`
  shape or to either merge site.
- Any change to `apps/web/lib/ws/handlers/tasks.ts`'s existing
  `preservePrimaryExecutorFields`, which already handles the WebSocket path
  correctly.
- The `getWorkspaceSourceCapabilities` disabled/hidden-state UI copy itself;
  this fix addresses the cache staleness that feeds it, not its rendering.

## Implementation plans

- [Kanban task cache preserves executor fields across merges](../../plans/kanban-task-executor-cache-staleness/plan.md)
