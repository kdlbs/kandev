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

Replace `IsTaskInTerminalStep` with `GetBlockerStatus` (satisfied, failed, pending)
and apply it in admission, the scheduler cascade, and the service step-move path.
After this change, a failed or cancelled blocker never releases a dependent, in any
order and on any path.

## In scope

- `office/repository/sqlite/blockers.go`: `GetBlockerStatus` and `BlockerStatus`.
  Remove `IsTaskInTerminalStep`.
- `office/shared/assignment_eligibility.go`: use the new reader, refuse on any
  readable unsatisfied blocker, and log `blocker_status`.
- `office/scheduler/reactivity.go` `allBlockersResolvedExcept`, plus the scheduler
  repository interface and fakes.
- `office/service/event_subscribers.go` `resolveAndWakeIfUnblocked`: check the
  triggering blocker and require all others to be satisfied.
- Interface updates in onboarding and other structural lookups that embed
  `AssignmentTaskLookup`.

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
3. Step path: a move into a step named Cancelled or Done without `COMPLETED` state
   persists zero resolution runs. A `COMPLETED` move converges with the success
   bridge on one run.

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
- `apps/backend/internal/office/service/event_subscribers.go`
- New: `apps/backend/internal/office/service/blocker_policy_resolution_test.go`
- Existing fakes implementing `IsTaskInTerminalStep` (search the module)

## Risks

- Fakes that implement the old method must move to the new reader, or compilation
  fails across packages. Search the module before editing.
- Existing tests that assert a failed blocker releases its dependent encode the old
  policy. Update them and cite AC-001.1 or AC-001.2 in the test name, not in a
  production comment.

## Results

Done. `GetBlockerStatus` replaces `IsTaskInTerminalStep` in admission, the
scheduler cascade, and `resolveAndWakeIfUnblocked`, which now also checks the
triggering blocker. Existing admission and recovery tests cover failed and
cancelled blockers; `blocker_policy_resolution_test.go` covers both resolution
orders across the state-event, reactivity, and step-move producers, plus step names
without `COMPLETED`. The verification command passed with `-race`.
