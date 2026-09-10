---
id: "04-drop-producer-reads"
title: "Stop producers paying for discarded child summaries"
status: done
wave: 3
depends_on: ["03-assembly-time-enrichment"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-CHILD-SUMMARIES-002
acceptance_criteria:
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.4
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.5
  - AC-OFFICE-WAKE-CHILD-SUMMARIES-002.9
system_design:
  - ../../specs/office/system-design/children-completed-wake-summaries.md
---

# Task 04: Stop producers paying for discarded child summaries

## Summary

Remove the child-summary and pull-request reads from the two Office producers
that perform them only to fill a trigger-payload field nobody reads. The
reconciler's `buildPayload` becomes infallible, so a child-summary read can no
longer prevent a wake that readiness already judged due.

## In scope

- `Service.queueChildrenCompletedRun`: drop the `GetChildSummaries` and
  `lookupChildPRLinks` calls and dispatch `OnChildrenCompletedPayload` with no
  summaries. Leave `AreAllChildrenTerminal`, `GetChildSetKey`, and the operation
  id untouched.
- `ParentWakeReconciler.buildPayload`: same removal; drop its `error` return and
  the caller's error branch in `reconcileOne`, along with the now-dead
  `truncated` debug log.
- Tests asserting each producer performs no child-summary read and still queues.

## Out of scope

- `orchestrator.childCompletionPayload`. It builds summaries from rows already
  loaded for the readiness check, so it costs no extra read.
- `SchedulerService.cascadeChildrenCompleted`. It queues a bare
  `scheduler.RunContext` with no child-list field and reads nothing to discard.
- Deleting `engine.OnChildrenCompletedPayload.ChildSummaries` or
  `engine.ChildSummary`. Out of scope in the requirement.
- The readiness, identity, and receipt guards.

## Acceptance

- Neither producer calls `GetChildSummaries` or `lookupChildPRLinks`, and both
  still queue the run with the same operation id for the same parent and child
  set.
- `buildPayload` returns no error, and `reconcileOne` dispatches on a tick where a
  child-summary read failure previously aborted it, while `GetChildSetKey`, its
  revalidation against the candidate's key, and the transactional receipt still
  gate dispatch unchanged.
- `lookupChildPRLinks` retains exactly one caller, the Task 03 enricher, and is
  not reported by the deadcode scan.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/office/service/ -run 'TestQueueChildrenCompletedRun|TestReconcileOne|TestParentWake' -count=1
```

```bash
cd apps/backend && go test -tags fts5 ./internal/office/... ./internal/orchestrator/... -count=1
```

```bash
cd apps/backend && golangci-lint run ./internal/office/...
```

## Files likely touched

- `apps/backend/internal/office/service/event_subscribers.go`
- `apps/backend/internal/office/service/scheduler_wake_reconciler.go`
- `apps/backend/internal/office/service/event_subscribers_children_completed_test.go`
- `apps/backend/internal/office/service/scheduler_wake_reconciler_test.go`

## Dependencies

Task 03. Landing this first would leave `lookupChildPRLinks` with no caller.

## Risks

- The reconciler becomes strictly more permissive. That is required by
  AC-OFFICE-WAKE-CHILD-SUMMARIES-002.9, but it is a real behaviour change, so the
  test must pin that the guards which decide *whether* to dispatch are still the
  ones gating it.
- `queueChildrenCompletedRun` currently downgrades a `GetChildSummaries` error to
  a log and continues; the reconciler aborts. Only the reconciler's arm is being
  removed, and the two must not be conflated into a single edit that also relaxes
  a readiness check.
- `event_subscribers_children_completed_test.go` and the reconciler tests may
  assert on populated `ChildSummaries` in the dispatched payload. Those assertions
  become wrong, not merely stale, and must be replaced with "performs no
  child-summary read" rather than deleted.

## Parallelism

`sequential`

## Inputs

- System design, [Producer changes](../../specs/office/system-design/children-completed-wake-summaries.md#producer-changes).
- `apps/backend/internal/office/service/event_subscribers.go:1113-1150`.
- `apps/backend/internal/office/service/scheduler_wake_reconciler.go:93-176`.

## Results

Implemented. Both producers dispatch `engine.OnChildrenCompletedPayload{}`; `ParentWakeReconciler.buildPayload` is deleted and its caller's error arm with it. `lookupChildPRLinks` and `GetChildSummaries` each retain exactly one caller, the enricher. The reconciler test drops `task_comments` to show a tick that previously aborted now dispatches, and a sibling test pins that the readiness gate still suppresses a wake.
