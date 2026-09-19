---
id: "01-clear-deleted-session-error"
title: "Clear the deleted session error"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 01: Clear the Deleted Session Error

## Intent

Remove a deleted session's error from the task status summary immediately after session deletion.

## Acceptance

- A successful `Service.DeleteSession` publishes one inactive error occurrence for the removed session.
- The persisted task summary has no error from the removed session after the projector processes the occurrence.
- If another session has a recoverable error, deleting the current summary winner leaves the retained session's error projected.
- Recoverable error persistence and publication serialize with session deletion for the same session.
- A transient non-not-found session read error does not skip recovery state reconciliation, messaging, or execution cleanup.
- Concurrent deletions of sessions in one task cannot restore an error for a session that was already deleted.
- The projector restart regression stops the old projector synchronously before cold-start hydration.
- Existing queue cleanup, workspace retention, and primary-session promotion behavior remains unchanged.

## Files Likely Touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/queue_purge_status_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_session_error_fixup_test.go`

## Dependencies

None.

## Parallelism

`sequential`

## Inputs

- The `active_error` derivation and deletion scenario in the bounded task-status delivery spec.
- The existing inactive `TaskSessionErrorChanged` contract in `statussummary.Projector`.
- The queue-status publication pattern in `Service.DeleteSession`.

## Implementation

1. Add a regression test that starts the status-summary projector on the service event bus.
2. Publish an active error for a completed session and make sure that the stored summary contains it.
3. Delete the session and make sure that the stored summary removes the error.
4. Publish an inactive session-error occurrence after the repository removes the session row.
5. Use `context.WithoutCancel` for the post-commit publication and log publication errors.
6. Re-publish the newest retained session error after deletion to repair a summary hydrated before the deletion.
7. Serialize recoverable-error side effects with the session deletion guard.
8. Continue recoverable-error handling after transient session reads, while suppressing it only for
   `models.ErrTaskSessionNotFound`.
9. Serialize deletion and retained-error repair by task, with a barrier regression for concurrent deletions.
10. Stop the old status-summary projector synchronously in the restart regression.

## Verification

Run this command from the repository root:

```bash
cd apps/backend && go test -tags fts5 -run 'TestDeleteSessionClearsProjectedAgentError|TestDeleteSessionCancelsQueuedPromptsAndPublishesStatus' ./internal/orchestrator -count=1
```

## Output Contract

Report the source and test files that changed.
Report the exact command and its result.
Update this task status, this task's results, the plan checkbox, and the plan verification results.
Report blockers and remaining risks.

## Results

Completed.

- Changed `apps/backend/internal/orchestrator/task_operations.go` to publish an
  inactive `TaskSessionErrorChanged` occurrence after session deletion commits.
- Added `TestDeleteSessionClearsProjectedAgentError` to
  `apps/backend/internal/orchestrator/queue_purge_status_test.go`.
- Added restart and guard regressions in
  `apps/backend/internal/orchestrator/queue_purge_status_test.go`.
- Verification passed: the original 2-test command passed, and the 4-test
  focused fixup command passed.
- Re-published the newest retained session error after deletion and serialized
  recoverable-error publication with the session deletion guard.
- Added a transient-read fault-injection regression and restored the existing
  recovery/state/cleanup path for non-not-found read errors.
- Added task-level serialization and a barrier-based concurrent-deletion regression.
- The restart regression now closes the old projector before starting the replacement.
- Verification passed: 3 focused tests, 6 focused race tests, and 1,812 orchestrator
  package tests.
- No blockers. Event publication remains best effort, and a later authoritative
  rebuild can repair a summary if publication fails.
