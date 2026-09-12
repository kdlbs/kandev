---
id: "01-guard-retained-capacity"
title: "Guard session termination on retained capacity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SESSION-TERM-001
  - REQ-OFFICE-SESSION-TERM-002
  - REQ-OFFICE-SESSION-TERM-003
acceptance_criteria:
  - AC-OFFICE-SESSION-TERM-001.1
  - AC-OFFICE-SESSION-TERM-001.2
  - AC-OFFICE-SESSION-TERM-001.3
  - AC-OFFICE-SESSION-TERM-001.4
  - AC-OFFICE-SESSION-TERM-001.5
  - AC-OFFICE-SESSION-TERM-001.6
  - AC-OFFICE-SESSION-TERM-001.7
  - AC-OFFICE-SESSION-TERM-001.8
  - AC-OFFICE-SESSION-TERM-001.9
  - AC-OFFICE-SESSION-TERM-001.10
  - AC-OFFICE-SESSION-TERM-001.11
  - AC-OFFICE-SESSION-TERM-002.1
  - AC-OFFICE-SESSION-TERM-002.2
  - AC-OFFICE-SESSION-TERM-002.3
  - AC-OFFICE-SESSION-TERM-002.4
  - AC-OFFICE-SESSION-TERM-002.5
  - AC-OFFICE-SESSION-TERM-002.6
  - AC-OFFICE-SESSION-TERM-002.7
  - AC-OFFICE-SESSION-TERM-002.8
  - AC-OFFICE-SESSION-TERM-002.9
  - AC-OFFICE-SESSION-TERM-002.10
  - AC-OFFICE-SESSION-TERM-002.11
  - AC-OFFICE-SESSION-TERM-002.12
  - AC-OFFICE-SESSION-TERM-003.1
  - AC-OFFICE-SESSION-TERM-003.2
  - AC-OFFICE-SESSION-TERM-003.3
  - AC-OFFICE-SESSION-TERM-003.4
  - AC-OFFICE-SESSION-TERM-003.5
  - AC-OFFICE-SESSION-TERM-003.6
  - AC-OFFICE-SESSION-TERM-004.1
  - AC-OFFICE-SESSION-TERM-004.2
  - AC-OFFICE-SESSION-TERM-004.4
system_design:
  - ../../specs/office/system-design/task-session-termination-01.md
---

# Task 01: Guard Session Termination On Retained Capacity

## Summary

Add one retained-capacity determination to the office dashboard service and
place it in front of all three guarded `TerminateOfficeSession` call sites, so
a session ends only when the agent has lost its last capacity on the task. The
determination fails closed and emits two distinguishable records, so a
suppressed termination is never mistaken for a path that did not run.

## In scope

- One unexported determination on `*DashboardService` answering whether an
  agent retains a capacity on a task, and naming the capacity it found.
- Runner capacity read through the existing repository accessor whose assignee
  value is produced by `sqlite.RunnerProjection`.
- Seat capacity read through `Repository.ListAllTaskParticipants`.
- Guard insertion at the three call sites in `service_tasks.go`: participant
  removal, displaced-seat claim, and assignee reassignment.
- Fail-closed handling of a read error: suppress, record, return success.
- The retained-capacity record and the read-failure record, distinguishable
  from each other, at the existing call-site loggers.
- Unit coverage for every acceptance criterion listed above, including the
  three regressions the system design names as silently passing when written
  casually.

## Out of scope

- The suppression counter and the operator-facing documentation, which
  Task 02 owns.
- Any change to `internal/orchestrator/office_session_terminator.go`. It stays
  the idempotent executor with both methods and both interfaces unchanged.
- Guarding `TerminateAllForAgent` or any cascade caller.
- Session creation, reuse, lookup, identity, schema, or index.
- Participant casting, seat provenance, and claim behavior.

## Acceptance

- All three guarded call sites route through exactly one determination; no
  second copy exists in application code.
- An agent that is both the task's runner and a seated reviewer keeps its
  session row untouched when displaced from the reviewer seat, removed from
  that role, or reassigned away from the runner standing, and the row still
  ends when its last capacity goes.
- A reassignment that suppresses the session termination still hard-cancels the
  previous runner's running execution.
- A failing capacity read suppresses the termination, records the failure
  distinguishably, and returns success to the caller.

## Verification

```bash
# From the repository root:
cd apps/backend && go test ./internal/office/dashboard/... -race -count=1
cd apps/backend && go test ./internal/orchestrator/... -race -count=1
cd apps/backend && go test ./internal/office/repository/... -race -count=1
make -C apps/backend lint
git diff --cached --name-only | grep '\.go$' | xargs -r gofmt -l
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/dashboard/service_tasks.go`
- `apps/backend/internal/office/dashboard/session_termination_test.go`
- `apps/backend/internal/office/dashboard/session_capacity_test.go`
- `apps/backend/internal/orchestrator/office_session_terminator_test.go`

## Dependencies

None.

## Risks

- The step-scoping bound (`AC-OFFICE-SESSION-TERM-002.3`) passes without being
  exercised unless its test moves the task off the step that carries the
  agent's seat. Write the move explicitly; a same-step fixture proves nothing.
- The decoupling criterion (`AC-OFFICE-SESSION-TERM-003.2`) passes for an
  implementation that wrongly suppresses the hard cancel as well, if only the
  session row is asserted. Assert both halves.
- Existing tests assert that participant removal terminates. One whose fixture
  incidentally leaves the removed agent as the task's runner will begin
  suppressing; correct the fixture rather than relaxing the assertion.
- Placing the guard inside the orchestrator's terminator instead of the
  dashboard service would invert the office/orchestrator dependency the
  `SessionTerminator` interfaces exist to prevent, and would put
  `TerminateAllForAgent` behind a guard `AC-OFFICE-SESSION-TERM-003.1`
  requires it to stay clear of.
- Resolving the runner from participant seats rather than the shared projection
  reads correctly on a task that has never moved and disagrees with every other
  reader on one that has.

## Parallelism

`sequential`

## Inputs

- `docs/specs/office/requirements/task-session-termination.md`,
  REQ-OFFICE-SESSION-TERM-001 through -003.
- `docs/specs/office/system-design/task-session-termination-01.md`, sections
  "Where the precondition lives", "The capacity question", "Control flow".
- `apps/backend/internal/office/repository/sqlite/base.go:34`
  (`RunnerProjection`) and `participants.go:517`
  (`ListAllTaskParticipants`).
- `apps/backend/internal/office/dashboard/session_termination_test.go` for the
  existing recording-terminator test pattern.

## Results

- Added `retainsTaskCapacity` and `terminateSessionUnlessRetained` to
  `internal/office/dashboard/service_tasks.go`. The determination reads the
  runner through `GetTaskExecutionFields` (whose assignee value is
  `RunnerProjection`) and the slate through `ListAllTaskParticipants`. Both were
  already on the dashboard `Repository` interface, so no interface changed.
- Routed all three guarded call sites through the one helper: participant
  removal, `terminateDisplacedSession`, and `runReactivityForAssigneeChange`.
  The reassignment path's hard cancel was left above the guard, untouched.
- `internal/orchestrator/office_session_terminator.go` is unmodified.
- Added `internal/office/dashboard/session_capacity_test.go` (10 tests) and
  four tests plus a two-state flag subtest in `session_termination_test.go`.
- Verified the tests are not vacuous: with `retainsTaskCapacity` short-circuited
  to `capacityNone`, 10 of the new tests fail, including both flag states, the
  claim path, and the reassignment decoupling test - which fails on its session
  assertion while its cancellation assertion still passes, proving the two
  halves are independent. The three "must keep working" tests
  (`TerminatesWhenNoCapacityRemains`, `StaleSeatAtLeftStepDoesNotSuppress`,
  `TerminatesWhenPrevRunnerKeepsNothing`) pass in both states.
- `AC-OFFICE-SESSION-TERM-003.1` needed no new test: the existing
  `TestTerminateAllForAgent_CascadesAcrossTasks` already seeds the agent as
  runner on both tasks and asserts the cascade still ends every session. Its
  docstring now records that.
- Full `internal/office/dashboard` and `internal/orchestrator` suites pass with
  `-race`; no pre-existing test needed changing. Full-repo `golangci-lint run
  ./...` reports 0 issues.
