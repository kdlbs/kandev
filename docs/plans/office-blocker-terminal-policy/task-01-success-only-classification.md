---
id: "01-success-only-classification"
title: "Classify Office blockers as satisfied only on COMPLETED"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-BLOCKER-POLICY-001
acceptance_criteria:
  - AC-OFFICE-BLOCKER-POLICY-001.1
  - AC-OFFICE-BLOCKER-POLICY-001.2
  - AC-OFFICE-BLOCKER-POLICY-001.3
  - AC-OFFICE-BLOCKER-POLICY-001.4
  - AC-OFFICE-BLOCKER-POLICY-001.5
  - AC-OFFICE-BLOCKER-POLICY-001.6
system_design:
  - ../../specs/office/system-design/blocker-terminal-policy.md
---

# Task 01: Success-only blocker classification

## Summary

Replace blocker uses of `IsTaskInTerminalStep` with `GetBlockerStatus` (satisfied,
failed, pending) in assignment admission and the scheduler success cascade. Keep
`IsTaskInTerminalState` for general lifecycle checks. Committed task-state events
own completion effects; a step name alone never releases a dependent.

## In scope

- `office/repository/sqlite/blockers.go`: `GetBlockerStatus` and `BlockerStatus`.
  Keep `IsTaskInTerminalState` for non-blocker lifecycle checks.
- `office/shared/assignment_eligibility.go`: use the new reader, refuse on any
  readable unsatisfied blocker, and log `blocker_status`.
- `office/scheduler/reactivity.go` `allBlockersResolvedExcept`, plus the scheduler
  repository interface and fakes.
- Interface updates in onboarding and other structural lookups that embed
  `AssignmentTaskLookup`.
- The task-completion consolidation's `task.state_changed` handler validates the
  committed `COMPLETED` state before it invokes the blocker cascade.

## Out of scope

- Escalation, publication, and registry changes (Task 02).
- Kanban dependency code, children helpers, and `GetTaskTerminalStatus`.

## Acceptance

1. Admission: a `FAILED` or `CANCELLED` blocker admits zero `task_assigned` runs
   across the TaskCreated/TaskUpdated, recovery, and assignee-mutation producers.
   The existing pending, completed, missing, and unreadable outcomes are unchanged.
2. Resolution: both orders of "one blocker fails, one completes" persist zero
   `task_blockers_resolved` runs. Retrying the failed blocker to `COMPLETED` then
   persists exactly one, with the existing digest key.
3. State path: a `task.moved` event without a committed `COMPLETED` state persists
   zero resolution runs. A committed `COMPLETED` event wakes only when every other
   blocker is also satisfied.

## Verification

From `apps/backend`:

```bash
go test -trimpath -race ./internal/office/repository/sqlite/ ./internal/office/shared/ \
  ./internal/office/scheduler/ ./internal/office/service/ ./internal/office/onboarding/ -count=1
```

From the repo root:

```bash
bash scripts/lint-go-changed <changed Go files>
```

## Likely files

- `apps/backend/internal/office/repository/sqlite/blockers.go`
- `apps/backend/internal/office/shared/assignment_eligibility.go` and its test
- `apps/backend/internal/office/scheduler/reactivity.go`
- `apps/backend/internal/office/scheduler/task_completion.go`
- `apps/backend/internal/office/scheduler/reactivity_blockers_resolved_test.go`
- Existing fakes implementing `IsTaskInTerminalStep` (search the module)

## Risks

- Fakes that implement the old method must move to the new reader, or compilation
  fails across packages. Search the module before editing.
- Existing tests that assert a failed blocker releases its dependent encode the old
  policy. Update them and cite AC-001.1 or AC-001.2 in the test name, not in a
  production comment.

## Results

Done. `GetBlockerStatus` replaces blocker uses of `IsTaskInTerminalStep` in
assignment admission and the scheduler cascade. The task-completion consolidation
removed `resolveAndWakeIfUnblocked`; the committed-state handler verifies
`COMPLETED` before it skips the triggering blocker. Existing admission, scheduler,
and failure-escalation tests cover failed and cancelled blockers. The merge
regression test confirms that `FAILED` and `CANCELLED` state events do not queue a
`task_blockers_resolved` run. The latest-base integration also confirms that a
`task.moved` event without a committed state event does not wake dependents. The
focused Office scheduler and service race suites passed. Changed-Go lint and both
specification checks passed. The broader race run timed out in the unchanged task
repository SQLite serialization test; the integration record lists the result.
