---
title: "Cancel session queue on session delete"
status: pending
parallelism: sequential
parallel-safe: no
---

# Task 02: Cancel Session Queue On Session Delete

## Goal

Deleting a session removes its pending queue entries and publishes
`message.queue.status_changed` for the owning task so the sidebar badge no
longer counts orphan rows.

## Root Cause Slice

`orchestrator.Service.DeleteSession` deletes the session row and returns.
It never calls `messageQueue.CancelAll` (or equivalent) and never publishes
queue status. `CountPendingByTaskIDs` still counts those rows by `task_id`.

## RED Regression (must fail before the fix)

In `apps/backend/internal/orchestrator/` (prefer beside existing session /
queue tests):

1. Create task T with sessions S1 and S2.
2. Enqueue N pending prompts on S1 (and optionally some on S2).
3. Call `DeleteSession(S1)` successfully.
4. Assert:
   - `messageQueue` has no pending entries for S1;
   - `CountPendingByTask(T)` equals only S2's remaining pending count;
   - a `message.queue.status_changed` event was published with
     `task_id = T` (and preferably `session_id = S1`).

Name suggestion: `TestDeleteSessionCancelsQueuedPromptsAndPublishesStatus`.

Confirm RED: S1 rows remain and/or no status event / task count unchanged.

## Implementation

In `DeleteSession`, after the session row is successfully deleted (or in the
same critical section after the delete is durable — prefer after successful
DB delete so a failed delete does not empty the queue):

1. `messageQueue.CancelAll(ctx, sessionID)` (or a dedicated purge-by-session
   if CancelAll's reserved-in-flight semantics are wrong for teardown —
   match "session is gone, nothing should remain pending for it").
2. Publish queue status for the task using the helper from Task 01
   (`task_id` + `session_id`).
3. Best-effort log on cancel/publish failure; do not fail the session delete
   after the row is already gone (document that choice in the test).

If CancelAll preserves reserved-in-flight durable lifecycle rows, add an
explicit purge-by-session for deletion teardown so orphans cannot linger.

## Acceptance

- RED then GREEN on the regression test.
- Task badge / `CountPendingByTask` no longer includes deleted-session rows.
- Deleting one session does not wipe another session's queue on the same
  task.
- Task 01's purge publish helper is reused rather than a third publish path.

## Validation

```bash
cd apps/backend
go test ./internal/orchestrator/ -count=1
```

## Dependencies

Task 01 (publish helper / event shape).

## Output Contract

Report RED/GREEN evidence, whether CancelAll or a stronger purge was needed
for reserved rows, changed files, and plan/task status updates.
