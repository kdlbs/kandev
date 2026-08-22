---
spec: docs/specs/kanban-task-executor-cache-staleness/spec.md
created: 2026-08-15
status: completed
---

# Implementation Plan: Kanban task cache preserves executor fields across merges

## Overview

Add an executor-fields preserve guard to the hydration merge and a
request-start freshness guard to the workflow snapshot merge. Hydration uses
`preserveOmittedExecutorFields` when an equal-or-newer incoming task omits its
executor bundle. Snapshot reconciliation compares the current task with the
task captured at request start, then copies the complete current bundle when a
live change or in-flight task creation makes the response stale. This mirrors
the existing `preservePrimaryExecutorFields` guard used by the WebSocket
handler (`apps/web/lib/ws/handlers/tasks.ts`) while keeping HTTP authority
rules separate from raw WebSocket presence checks.

## Root cause

`primary_executor_*`/`is_remote_executor` are `omitempty` fields derived per
read from the primary session's executor binding
(`apps/backend/internal/task/service/service_events.go:566-582`,
`apps/backend/internal/task/dto/dto.go:162-164,180`); binding an executor only
updates the `task_sessions` row, never the owning task's `updated_at`
(`apps/backend/internal/task/repository/sqlite/session.go:741-743,907-921`).
`mergeKanbanTasks` compares timestamps with `incomingTime >= existingTime` and,
on that branch, replaces the cached task object wholesale
(`apps/web/lib/state/hydration/hydrator.ts:47-49`) with no gap-fill — unlike
its `backfillServerDerivedFields` helper, which only runs on the opposite
(incoming-is-older) branch and only covers five dependency-projection fields,
not the executor fields. `use-all-workflow-snapshots.ts`'s snapshot merge
(`apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts:60-83`) preserves
`primarySessionId`/`primarySessionState`/`autopilot`/`statusSummary` when a
fresh response omits them but has no equivalent for the executor fields. A
stale or equal-timestamp response landing after a `task.updated` WebSocket
event already populated the executor fields therefore blanks them back out.

`isRemoteExecutor` cannot be gap-filled with a plain per-field `undefined`
check: `toKanbanTask` maps it with `?? false`
(`apps/web/lib/kanban/map-task.ts:199`), so the mapped value is never
`undefined`, unlike its three sibling fields (`?? undefined`). Both fixed sites
still use `primaryExecutorType` as the bundle-presence signal when they need to
recognize an omitted response. The snapshot merge also compares the full
current bundle with the request-start bundle, so an explicit stale value cannot
override a live clear.

## Frontend

### Executor-fields preserve helper

- In `apps/web/lib/kanban/map-task.ts`, add and export small helpers,
  `preserveOmittedExecutorFields(merged: KanbanTask, existing: KanbanTask):
  void`, that copies `primaryExecutorId`, `primaryExecutorType`,
  `primaryExecutorName`, and `isRemoteExecutor` from `existing` onto `merged`
  only when `merged.primaryExecutorType === undefined`. This is the shared home
  both merge sites import from, matching the existing "single publisher /
  single mapper" convention already documented on `TaskLike` in that file.
- Export `copyPrimaryExecutorFields` for snapshot reconciliation. This helper
  copies all four fields, including `undefined` and `false`, when the live
  cache is newer than the HTTP response.
- Do not change `toKanbanTask`'s existing `is_remote_executor ?? false`
  mapping or the pinned `"defaults isRemoteExecutor to false when missing"`
  test in `apps/web/lib/kanban/map-task.test.ts` — out of scope per the spec's
  constraints.

### `mergeKanbanTasks` wholesale-replace branch

- In `apps/web/lib/state/hydration/hydrator.ts`, in the `incomingTime >=
  existingTime` branch of `mergeKanbanTasks`, before assigning
  `draftTasks[idx] = incoming`, call
  `preserveOmittedExecutorFields(incoming, existing)` so the mutation lands on
  the object that becomes the new cached task.
- Leave the `incomingTime < existingTime` branch and
  `backfillServerDerivedFields` unchanged; this fix is scoped to the
  wholesale-replace branch's regression only.

### Workflow snapshot merge

- In `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`, capture the
  task at request start and compare it with the current cached task.
- Copy the current executor bundle when the task changed after request start or
  when the task first appeared during the request. Let a full response win
  when no live executor change occurred.

## Tests

- **`mergeKanbanTasks` preserves known executor fields on a same/newer-timestamp
  merge that omits them:** add a test to
  `apps/web/lib/state/hydration/hydrator-kanban-tasks.test.ts` that hydrates a task with all
  four executor fields populated, then hydrates again with the same `id`, an
  equal `updatedAt`, and no executor fields on the incoming task; assert the
  merged task in `draft.kanban.tasks` still has the original executor field
  values.
- **`mergeKanbanTasks` still adopts a legitimately different executor value:**
  add a test in the same file asserting that when the second hydration's
  incoming task carries a different `primaryExecutorType` (and the other three
  fields), the merged task adopts the new values rather than keeping the old
  ones.
- **Snapshot merge preserves known executor fields when a fresh response omits
  them:** add a test to
  `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.test.ts`, modeled
  on the existing `"preserves a cached autopilot marker when a fresh snapshot
  omits it"` test, asserting the merged snapshot task keeps its cached
  `primaryExecutorId`/`primaryExecutorType`/`primaryExecutorName`/
  `isRemoteExecutor` when the fresh response's task omits all four.
- **Snapshot merge preserves a live clear over an explicit stale bundle:** add
  a deferred-response test where the request starts with executor A, a live
  event clears it, and the response returns executor A.
- **Snapshot merge preserves a task added during the request:** add a
  deferred-response test where the task is absent at request start, a live
  event adds executor B, and the response omits executor fields.
- **`preserveOmittedExecutorFields` unit coverage:** add direct test cases in
  `apps/web/lib/kanban/map-task.test.ts` (or a colocated test if the helper
  moves) covering: incoming omits all four fields (preserve fires), incoming
  carries a real `primaryExecutorType` (preserve does not fire, incoming wins).

## Task Breakdown

- **Wave 1** (sequential; task 02 imports the helper task 01 adds):
  - `task-01-preserve-helper-and-hydrator-fix.md` — add
    `preserveOmittedExecutorFields` and wire it into `mergeKanbanTasks`.
  - `task-02-snapshot-merge-fix.md` — wire the same helper into
    `use-all-workflow-snapshots.ts`.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/state/hydration/hydrator.test.ts lib/state/hydration/hydrator-kanban-tasks.test.ts lib/kanban/map-task.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts
pnpm run typecheck
```

## Risks

- Gating omission detection on `primaryExecutorType` is a deliberate
  workaround for `toKanbanTask`'s lossy `?? false` mapping.
- Snapshot reconciliation copies the complete current bundle when a live
  change is proven. This includes explicit clears and tasks added in flight.
- Hydration keeps the mapped-shape limitation because it has no request-start
  snapshot or raw payload presence data.
