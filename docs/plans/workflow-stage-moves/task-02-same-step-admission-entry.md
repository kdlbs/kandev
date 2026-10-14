---
id: "02-same-step-admission-entry"
title: "Run the admitted entry once for same-step WIP queue admissions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STAGE-MOVES-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STAGE-MOVES-002.1
  - AC-TASKS-WORKFLOW-STAGE-MOVES-002.2
  - AC-TASKS-WORKFLOW-STAGE-MOVES-002.3
system_design:
  - ../../specs/tasks/system-design/workflow-stage-moves.md
---

# Work Order 02: Run the admitted entry once for same-step WIP queue admissions

## Outcome

A task admitted from a full step's WIP queue to the same step runs that step's
entry, including `auto_start_agent`, for the entry recorded when it was
queued. A delivery handled after the task re-entered the step does not run the
newer entry's actions.

## In scope

- `apps/backend/internal/task/models/models.go`: add
  `QueuePromotionEntryTransitionIDKey`, the pinned-entry field of the
  `MetaKeyQueuePromotionPending` marker.
- `apps/backend/internal/task/repository/sqlite/task.go`:
  `PromoteQueuedTaskIfWorkflowStepHasCapacity` pins the latest ledger id on
  the marker for a same-step admission, inside the promotion transaction after
  the task row lock.
- `apps/backend/internal/orchestrator/event_handlers_workflow_queue.go`:
  `queuePromotionEntryID` resolves the delivered id, then the pinned id, then
  the latest ledger id.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`: the
  queue-promotion handler passes the resolved id to `finalizeStepEnter`.

## Out of scope

- Cross-step promotions, which already carry their ledger id.
- The ledger dispatcher in `step_entry_dispatch.go` and `on_enter` ownership.

## Acceptance conditions

1. A same-step admission whose delivery carries `step_transition_id` zero runs
   the destination entry and its `auto_start_agent` action.
2. A same-step admission handled after the task left and re-entered the step
   does not run the entry again for the stale delivery.
3. A marker without the pinned value falls back to the latest ledger id.

## Verification

Add focused tests in a sibling of
`event_handlers_workflow_profile_session_policy_resolution_test.go` that
drive the repository promotion write and assert the pinned marker. Run them before the
fix (expected to fail) and after it:

```bash
cd apps/backend
go test ./internal/orchestrator/ -run 'QueuePromot' -count=1
go test ./internal/task/repository/sqlite/ -run 'PromoteQueued' -count=1
```

## Likely files

See In scope. Dependencies: none.

## Results

- Tests: `TestHandleTaskQueuePromotedSameStepEntryReplacesPreviousStepRoute`
  and `TestHandleTaskQueuePromotedSameStepIgnoresReentryAfterPromotion` in
  `event_handlers_workflow_queue_promotion_entry_test.go`. Before the fix the
  first kept the previous step's route (the entry never ran) and the second
  found no pinned entry on the marker. Both pass after the fix, together with
  the existing queue-promotion tests.
- Review verification found that untyped JSON decoding rounded the valid ledger
  ID `9007199254740993` to `9007199254740992`. The marker now stores a decimal
  string, and the reader preserves numeric-marker compatibility. A bounded
  exact-source write/read proof failed before the correction and passed after it;
  the ledger read is an explicit fake, so this is not SQL evidence.
- Genuine SQLite/PostgreSQL promotion regressions verify the large pinned ID
  after persisted metadata decoding and reject a duplicate admission. Their
  hosted execution remains required before merge. The reader regression also
  covers the maximum signed 64-bit ID and invalid strings.
