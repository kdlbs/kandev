---
spec: docs/specs/ui/requirements/sidebar-queued-prompt-count.md
created: 2026-08-11
status: draft
---

# Implementation Plan: Clear Stale Sidebar Queued Prompt Count

## Overview

The sidebar mail badge reads `status_summary.queued_prompt_count`. Ordinary
enqueue/dequeue/cancel paths publish `message.queue.status_changed`, so the
status-summary projector keeps the count live. Task archive/delete (and
workspace cascade variants) purge `queued_messages` in SQL but never publish
that event, so live clients keep the last positive projected count. Session
delete is worse: it removes the session row without cancelling its pending
queue entries, so orphans keep inflating the task badge. Close both gaps and
prove the badge drops to nothing without a page reload.

## Confirmed Root Cause

1. **Lifecycle purge is silent.**
   `Repository.DeleteTask` / `ArchiveTask` / `ArchiveTaskIfActive` and workspace
   cascade call `purgeTaskQueueInTx` → `messagequeue.PurgeTaskInTransaction`,
   then `notifyTaskQueuePurged`. In production the purger is only registered
   for the ephemeral in-memory queue fallback
   (`orchestrator.Service` wires `SetTaskQueuePurger` only when
   `usesDefaultEphemeralQueue`). The SQLite purge itself publishes no
   `message.queue.status_changed`. The projector therefore never recomputes
   pending = 0. Frontend `mergeTaskUpdate` also preserves an omitted
   `status_summary` on archive events, so archived rows keep the stale badge.
2. **Session delete leaves orphan queue rows.**
   `orchestrator.Service.DeleteSession` deletes the session row and never
   calls `messageQueue.CancelAll` / purge, and never publishes queue status.
   `CountPendingByTaskIDs` still counts those rows by `task_id`.

Evidence: user saw "11 queued prompts" on a task after the task with the
queued session was deleted / no longer had a live queue.

## Approach

Reuse the existing live path. Do not invent a new WS action or a special-case
frontend zeroing hack.

1. After a task-scoped queue purge commits, publish one
   `message.queue.status_changed` carrying `task_id` (session id optional /
   empty). The projector already refreshes from the authoritative queue store
   on that event (`applyQueueStatusEvent` → `CountQueuedPrompts`).
2. On session delete, cancel that session's pending queue entries, then
   publish the same event with the owning `task_id` + `session_id`.
3. Keep deleted-task frontend removal as-is (`removeTaskFromBothKanbans`
   already clears archived cache). Do not resurrect deleted tasks from late
   summary events (already a no-op).

## Task Waves

### Wave 1 — backend publish + purge wiring (sequential)

- [ ] [Task 01: Publish queue-status after task queue purge](task-01-purge-publishes-queue-status.md) — `parallel-safe: no`
- [ ] [Task 02: Cancel session queue on session delete](task-02-session-delete-cancels-queue.md) — depends on Task 01 publish helper shape; `parallel-safe: no`

### Wave 2 — archived badge freshness (sequential after wave 1)

- [ ] [Task 03: Archive retains fresh zeroed summary](task-03-archive-badge-zero.md) — `parallel-safe: no`

## Files Likely Touched

- `apps/backend/internal/orchestrator/messagequeue/` (optional helper if purge
  should publish from the service; or keep publish at orchestrator/task seam)
- `apps/backend/internal/orchestrator/service.go` / event handlers (publish
  helper usable with only `task_id`)
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
  (`publishQueueStatusEvent` currently session-keyed)
- `apps/backend/internal/task/repository/sqlite/{task,base,workspace}.go`
  (after-commit hook that can fire a task-scoped status event)
- `apps/backend/internal/orchestrator/task_operations.go` (`DeleteSession`)
- `apps/backend/internal/task/statussummary/projector_queued_test.go` (existing)
- New/extended unit tests beside the above packages
- Spec already amended: `docs/specs/ui/requirements/sidebar-queued-prompt-count.md`

## Validation Commands

```bash
# From apps/backend
go test ./internal/orchestrator/... ./internal/task/repository/sqlite/... ./internal/task/statussummary/... ./internal/task/service/... -count=1

# Targeted packages as each task lands (see task files)
```

No frontend change required if the projector emits
`task.status_summary.updated` with `queued_prompt_count` omitted/0; existing
sidebar badge tests already hide at 0/undefined.

## Risks And Out Of Scope

- **Risk:** publishing queue status after delete when the task no longer
  exists. Projector `ensureState` must not fail the bus handler; prefer
  resolving workspace before delete and tolerating missing task on count=0.
- **Risk:** double-purge with ephemeral memory queue purger. Keep the existing
  "SQLite purge is in-tx; memory purger is post-commit only for ephemeral"
  invariant.
- Out of scope: WIP step queues, badge click-to-open, per-session breakdown,
  historical backfill of stale persisted summaries beyond the next event/list.

## Design-Package Handoff Notes

Implement only after an explicit user request. Wave 1 first; do not open a PR
from this planning turn.
