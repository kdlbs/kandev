---
id: "06-pin-event-path-guarantees"
title: "Pin the taskless event-path guarantees no test asserts"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria:
  - AC-OFFICE-TASKLESS-001.2
  - AC-OFFICE-TASKLESS-001.4
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 06: Pin the taskless event-path guarantees no test asserts

## Summary

Two guarantees on the taskless event path hold today and are asserted by
nothing. Both are the kind that stay true until an unrelated refactor quietly
ends them. A third case does not hold today at all: a continuation-summary write
that fails is swallowed, and `AC-OFFICE-TASKLESS-001.2` now states an outcome
for it. That case carries this work order's only production change, one run
event.

**A failed attempt must not replace the last successful continuation summary**
(`AC-OFFICE-TASKLESS-001.2`, second clause). The summary is refreshed from the
completion handler after the run's terminal compare-and-set succeeds, so a
failed attempt never reaches it. `continuation_summary_reader_test.go` covers
scope selection and round-tripping, and
`event_subscribers_run_output_test.go` covers what a *completion* writes.
Neither drives a failure and then reads the summary back.

**Task consumers must not act on run-owned events** (system design, events and
observation). What enforces this is structural: a run-owned `AgentExecution`
carries an empty `SessionID`, so a task consumer resolves no task session and
falls out. Nothing asserts it, so the guarantee currently rests on an empty
string continuing to behave that way.

## In scope

- Drive a taskless attempt to a failed terminal outcome with a prior successful
  summary in place for the same scope, and assert the stored summary is
  byte-identical to the successful one afterwards. Cover both scopes
  (`routine:<id>` and `agent:<id>`), since scope is chosen at run creation and
  a regression could hit one and not the other.
- Assert the same for an interrupted attempt recovered at startup, which is the
  other way an attempt ends without success.
- Cover the third case `AC-OFFICE-TASKLESS-001.2` now names explicitly: a
  summary **write** that itself fails. Today both the load error and the upsert
  error are swallowed as a log warning after the run's completion has already
  committed, so a run completes carrying a stale summary with no durable trace.
  The criterion's outcome is preserve-and-complete with visibility: the run stays
  complete, the stored summary stays byte-identical, and the failure is recorded
  on the run's own event stream naming the scope that was being written. Assert
  all three, for a failing load and a failing upsert separately. Emitting that
  run event is the one production change this work order carries; it adds no
  rollback and changes no terminal state.
- Assert that a run-owned lifecycle event reaching the task-side consumers
  produces no task-session read or write and no workflow transition. Prefer an
  assertion on the observable effect (no task session mutated, no workflow
  event published) over asserting the internal branch taken.

## Out of scope

- Any rollback or retry of the failed summary write, or any change to the run's
  terminal state because of it. The criterion says the completed run stays
  complete.
- Adding an explicit owner-kind guard to the task consumers. The rule is being
  pinned, not reimplemented; if the test shows the structural guard is
  insufficient, that is a finding against `REQ-OFFICE-TASKLESS-001`.
- Continuation scope *selection*, already covered by
  `continuation_summary_reader_test.go`.
- Usage deduplication, already covered by `TestTasklessUsageDuplicate`.

## Validation

```text
cd apps/backend
go test ./internal/office/service/ -run 'Continuation|Taskless' -v -count=1
make lint
```

## Reference

- `apps/backend/internal/office/service/event_subscribers.go` —
  `handleTasklessAgentCompleted` and the summary refresh it gates
- `apps/backend/internal/office/service/continuation_summary_reader_test.go`
- `apps/backend/internal/office/service/taskless_lifecycle_test.go`

## Dependencies

None.

## Risks

- **Asserting a summary that was never written.** Seed the successful summary
  through the real writer, not by hand: a test that hand-writes the row proves
  the reader works, not that the failure path left it alone. The existing
  `office/wakeup/routine_e2e_test.go` is the cautionary example.
