---
spec: docs/specs/platform/requirements/bounded-task-status-delivery.md
created: 2026-08-15
status: done
---

# Implementation Plan: Clear Deleted Session Errors

## Overview

`DeleteSession` removes the session row but does not publish an inactive session-error occurrence.
The stored task summary therefore retains an error that belongs to a removed session.
The repair publishes that occurrence after a successful deletion and proves the full summary path.
When a restarted projector has hydrated the deleted session as the current winner,
the repair also republishes the newest retained session error so the single-value
summary converges to the remaining authoritative error.
The repair treats only an authoritative not-found read as proof that a session was
deleted, and serializes deletion, retained-error selection, and repair publication
per task so concurrent deletions cannot restore a removed error.

## Backend

### Session deletion

Update `Service.DeleteSession` in `apps/backend/internal/orchestrator/task_operations.go`.
After the repository removes the session row, publish `events.TaskSessionErrorChanged` with these fields:

- `task_id`: the owning task ID.
- `session_id`: the removed session ID.
- `active`: `false`.

Use a context without request cancellation because the database deletion is complete.
Treat publication as best effort and log an error with the task and session IDs.
Keep the existing runtime quiescence, queue cleanup, workspace retention, and primary-session promotion behavior.

The deletion and retained-error repair run under a reference-counted task-level
guard. A transient session read failure in recoverable-error handling is logged
and continues through the existing persistence, state-reconciliation, recovery
message, and execution-cleanup path; only `models.ErrTaskSessionNotFound`
suppresses that work.

## Frontend

No frontend change is required.
Desktop and mobile task switchers already consume the complete `status_summary` replacement.

## Tests

- **What:** A successful session deletion removes that session's active error from the persisted task summary.
- **File:** `apps/backend/internal/orchestrator/queue_purge_status_test.go`
- **How:** Start the real status-summary projector on the service event bus.
  Publish an active error for a completed session, delete the session, and load the stored summary.
  The test must fail before the source change and pass after it.
- **What:** Session deletion still removes only the deleted session's queued prompts.
- **File:** `apps/backend/internal/orchestrator/queue_purge_status_test.go`
- **How:** Run the existing queue cleanup regression with the new error-summary regression.
- **What:** Deleting the newer error after a projector restart preserves an older retained error.
- **File:** `apps/backend/internal/orchestrator/queue_purge_status_test.go`
- **How:** Hydrate a summary with two session errors, restart the projector, delete the newer
  session, and assert that the retained session is the summary's active error.
- **What:** Recoverable error handling serializes with session deletion.
- **File:** `apps/backend/internal/orchestrator/queue_purge_status_test.go`
- **How:** Hold the per-session deletion guard and assert that recovery waits before persisting
  or publishing an active error.
- **What:** A transient session read failure does not suppress recovery or cleanup.
- **File:** `apps/backend/internal/orchestrator/task_session_error_fixup_test.go`
- **How:** Inject one non-not-found `GetTaskSession` error and assert that error metadata,
  waiting-for-input state, recovery messaging, and execution cleanup still occur.
- **What:** Concurrent deletions cannot re-publish an error for a session deleted by a sibling.
- **File:** `apps/backend/internal/orchestrator/task_session_error_fixup_test.go`
- **How:** Block the retained-error publication from one deletion, start a second deletion,
  and assert that the second deletion waits and the final summary has no active error.
- **What:** Projector restart tests stop the old projector before starting its replacement.
- **File:** `apps/backend/internal/orchestrator/queue_purge_status_test.go`
- **How:** Close the first projector synchronously in the returned stop function before cold-start
  hydration, so deletion events cannot be consumed by the stale in-memory projector.

No E2E change is required.
The backend integration test covers the same complete summary payload that both task switchers consume.

## Verification Results

Passed:

```text
cd apps/backend && go test -tags fts5 -run 'TestDeleteSessionClearsProjectedAgentError|TestDeleteSessionCancelsQueuedPromptsAndPublishesStatus' ./internal/orchestrator -count=1
```

Result: 2 tests passed initially; the retained-session restart regression and guard regression
also passed during PR fixup, for 4 focused tests total.

Blocker fixup verification:

```text
go test -tags fts5 -run 'TestHandleRecoverableFailureContinuesAfterTransientSessionReadError|TestConcurrentSessionDeletionDoesNotRepublishDeletedSessionError|TestDeleteSessionRetainsOtherProjectedAgentErrorAfterRestart' ./internal/orchestrator -count=1
Go test: 3 passed in 1 packages

go test -race -tags fts5 -run 'TestHandleRecoverableFailureContinuesAfterTransientSessionReadError|TestConcurrentSessionDeletionDoesNotRepublishDeletedSessionError|TestDeleteSessionRetainsOtherProjectedAgentErrorAfterRestart|TestRecoverableFailureWaitsForSessionDeletionGuard|TestDeleteSessionClearsProjectedAgentError|TestDeleteSessionCancelsQueuedPromptsAndPublishesStatus' ./internal/orchestrator -count=1
Go test: 6 passed in 1 packages

go test -tags fts5 ./internal/orchestrator -count=1
Go test: 1812 passed in 1 packages
```

## Implementation Waves And Parallel Candidates

- [x] [Task 01: Clear the deleted session error](task-01-clear-deleted-session-error.md)

Execution is sequential in the primary conversation.

## Risks And Out Of Scope

- The repair does not change session deletion permissions or running-session validation.
- The repair does not delete task workspaces, task environments, transcripts, or replacement sessions.
- The repair does not change error classification, error text, or error dismissal behavior.
- A failed event publication retains the last valid summary until a later authoritative rebuild.
- A transient read failure during recoverable-error handling is logged and follows the existing
  recovery path; only an authoritative missing-session error suppresses it.
- Task-level serialization keeps concurrent deletion repair events ordered with the authoritative
  session-row removals.
