---
title: "Publish queue-status after task queue purge"
status: pending
parallelism: sequential
parallel-safe: no
---

# Task 01: Publish Queue-Status After Task Queue Purge

## Goal

After archive/delete (and workspace cascade) purges `queued_messages` for a
task, publish `message.queue.status_changed` with that `task_id` so the status
summary projector zeros `queued_prompt_count` and live sidebars drop the badge.

## Root Cause Slice

`purgeTaskQueueInTx` + `notifyTaskQueuePurged` empty the queue without a
queue-status event. Production SQLite does not use the ephemeral memory
purger as a publish path either.

## RED Regression (must fail before the fix)

Add a backend unit/integration test that:

1. Seeds a task with a projected status summary `queued_prompt_count > 0`
   and matching pending `queued_messages` rows.
2. Archives (or deletes) the task through the repository/service path that
   calls `purgeTaskQueueInTx`.
3. Asserts that:
   - a `message.queue.status_changed` event is published with that `task_id`,
     **or**
   - the status-summary store ends with `QueuedPromptCount == 0` and a
     `task.status_summary.updated` was published (projector path), when the
     test wires the projector.

Preferred location options (pick one coherent seam):

- `apps/backend/internal/task/repository/sqlite/` proving the post-commit hook
  fires a registered callback with the task id after purge, **plus** an
  orchestrator/statussummary test that the callback publishes and the
  projector zeros the count; or
- a higher-level orchestrator/service test that archives a task with a real
  messagequeue + projector and observes `QueuedPromptCount == 0`.

Name suggestion:
`TestArchiveTaskPublishesQueueStatusAndZerosQueuedPromptCount`.

Confirm RED: count stays positive / no event, matching current production.

## Implementation

Minimal fix:

1. Extend the after-purge notification so production can react for **every**
   task queue purge (not only ephemeral memory queues). Options:
   - generalize `SetTaskQueuePurger` into a post-purge observer that always
     runs after successful commit; SQLite rows are already purged in-tx, so
     the observer must **not** call `PurgeTask` again on the SQLite-backed
     queue (only publish); keep the existing memory-queue purger behavior
     for the ephemeral fallback; or
   - add a separate `SetTaskQueuePurgeNotifier(func(ctx, taskID))` used only
     to publish status, registered at the composition root beside the
     current purger.
2. Implement publish-with-task-id:
   - Either teach `publishQueueStatusEvent` a task-scoped variant that does
     not require a live session (`task_id` required; `session_id` optional;
     `count`/`entries` may be empty/0 when no session is available), or
   - publish `events.MessageQueueStatusChanged` directly with
     `{"task_id": id}` so `Projector.applyQueueStatusEvent` recounts.
3. Wire the notifier at composition root from orchestrator/backendapp so
   archive, delete, cascade archive, and workspace cascade all fire it via
   existing `notifyTaskQueuePurged` call sites.
4. Ensure the projector still no-ops when the recomputed count already
   matches (idempotent double-fire safe).

Do **not** change frontend badge rendering in this task.

## Acceptance

- RED then GREEN on the regression test above.
- Archive/delete of a task with pending prompts yields projected
  `queued_prompt_count = 0` without a list reload.
- No second SQL purge of an already-empty SQLite queue that races a later
  generation (respect existing archive/unarchive generation comments).
- Existing projector queued tests still pass.

## Validation

```bash
cd apps/backend
go test ./internal/task/repository/sqlite/ ./internal/task/statussummary/ ./internal/orchestrator/ ./internal/backendapp/ -count=1
```

Narrower package list OK if the test lives in one package; run that package
and any package that gained a call site.

## Dependencies

None. First wave.

## Output Contract

Report RED/GREEN evidence, the publish seam chosen, changed files, and mark
this task + `plan.md` status when implementing.
