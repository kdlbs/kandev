---
id: "01-clear-deleted-session-error"
title: "Clear the deleted session error"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/bounded-task-status-delivery.md"
---

# Task 01: Clear the Deleted Session Error

## Intent

Remove a deleted session's error from the task status summary immediately after session deletion.

## Acceptance

- A successful `Service.DeleteSession` publishes one inactive error occurrence for the removed session.
- The persisted task summary has no error from the removed session after the projector processes the occurrence.
- Existing queue cleanup, workspace retention, and primary-session promotion behavior remains unchanged.

## Files Likely Touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/queue_purge_status_test.go`

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
- Verification passed: 2 tests passed with the command in this task's
  Verification section.
- No blockers. Event publication remains best effort, and a later authoritative
  rebuild can repair a summary if publication fails.
