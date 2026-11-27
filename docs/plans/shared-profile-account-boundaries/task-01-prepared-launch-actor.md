---
id: "01-prepared-launch-actor"
title: "Require a launch actor for prepared-workspace starts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PREPARED-LAUNCH-ACTOR-001
acceptance_criteria:
  - AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.1
  - AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.2
  - AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.3
  - AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.4
  - AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.5
  - AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.6
system_design:
  - ../../specs/executors/system-design/prepared-launch-actor.md
---

# Task 01: Require a launch actor for prepared-workspace starts

## Summary

Refuse `startAgentOnPreparedWorkspace` when executor admission establishes no
identity, before any runtime call.

## In scope

- Add `LaunchOptions.RequireExecutorActor` and `requirePreparedLaunchActor`,
  and run the check after admission under the session launch lock.
- Set the option only from `startAgentOnPreparedWorkspace`.
- Add an orchestrator fixture that uses the real task-service admission and a
  real secrets store holding synthetic values.

## Out of scope

- Other launch entry points.
- The cold `ResumeSession` branch, which is in PR #5176.
- Any change to admission identity resolution.

## Files

- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/prepared_actor.go`
- `apps/backend/internal/orchestrator/executor/prepared_actor_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/prepared_session_admission_test.go`
- `apps/backend/internal/orchestrator/prepared_session_admission_cases_test.go`

## Verification

```bash
cd apps/backend
go test -trimpath -race -count=1 -timeout 600s \
  -run 'TestPreparedSessionAdmission|TestRequirePreparedLaunchActor' \
  ./internal/orchestrator ./internal/orchestrator/executor
```

## Results

| Test case | Base `52309e61` | Fix |
| --- | --- | --- |
| `UnattributedSessionFailsClosed` | fail | pass |
| `UnavailableDurableActorFailsClosed` | fail | pass |
| `ConcurrentUnattributedStartsStayClosed` | fail | pass |
| `CollaboratorSessionStartsAsCollaborator` | pass | pass |
| `OwnerSessionStartsAsOwner` | pass | pass |
| `AuthorizedCallerTakesPrecedence` | pass | pass |
| `UnauthorizedCallerDoesNotFallBack` | pass | pass |
| `AuthDisabledStartsWithSyntheticIdentity` | pass | pass |
| `PrivateProfileIsNotBorrowed` | pass | pass |
| `CancelledStartLaunchesNothing` | pass | pass |
| `ConcurrentStartsLaunchOnce` | pass | pass |

On base, the three failing cases reached the runtime with an identity-free
context that could reveal both synthetic secrets. The eight cases that pass on
base show which boundaries current main already enforces.

The full `orchestrator` package passes under `-race`. `orchestrator/executor`
has the same six host Git-maintenance refusals on base and fix.
