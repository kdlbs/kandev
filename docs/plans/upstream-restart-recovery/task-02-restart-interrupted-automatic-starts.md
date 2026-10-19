---
id: "02-restart-interrupted-automatic-starts"
title: "Restart interrupted automatic starts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RESTART-ORPHAN-SESSIONS-003
acceptance_criteria:
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-003.1
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-003.2
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-003.3
  - AC-TASKS-RESTART-ORPHAN-SESSIONS-003.4
system_design:
  - ../../specs/tasks/system-design/restart-orphaned-session-terminalization.md
---

# Task 02: Restart interrupted automatic starts

## Summary

On startup, a `SCHEDULING` task that no other recovery path owns is started
again through the existing automatic start path, re-entered on its idle
session, or has only its stale state repaired.

## In scope

- Write failing repository and orchestrator tests first.
- Add `ListSchedulingTasks` to the SQLite repository: non-archived,
  non-ephemeral, non-automation tasks in `SCHEDULING`, oldest update first.
- Add `reconcileSchedulingOrphansOnStartup` and its helpers in a new
  `scheduling_orphan_recovery.go`, reaching the repository through a narrow
  interface assertion.
- Call it from `startLifecycleSweepAsync` before
  `reconcileTaskLifecycleTokens`.

## Out of scope

- A periodic sweep or setting.
- Readiness recovery of a `STARTING` session (#3240).
- Office tasks, which their scheduler recovers.

## Acceptance

- A task in `SCHEDULING` on an automatic-start step with no session, record,
  queue destination, token, or launch error launches exactly once after the
  startup sweep.
- Each excluded case in 003.2 launches nothing and keeps its state.
- An idle session without prompt history receives the step entry without a
  second session. A session with a turn accepted for the current entry only moves the task to
  `IN_PROGRESS`. A `CREATED`, `STARTING`, or `RUNNING` session is not touched.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'ListSchedulingTasks' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'SchedulingOrphan' -count=1)
```

## Likely files

- `apps/backend/internal/orchestrator/scheduling_orphan_recovery.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- Source-adjacent `*_test.go` files in both packages

## Results

- Tests: `TestListSchedulingTasksReturnsOnlyActiveSchedulingTasks` (repository)
  and six `TestSchedulingOrphan*` tests that drive `startLifecycleSweepAsync`.
- RED: with the current main `service.go` in place, the four positive tests
  fail on their assertions (no restart, no ceiling deferral, state not
  repaired, idle session not resumed); the guard tests pass. The repository
  test cannot compile before `ListSchedulingTasks` exists.
- GREEN: all seven tests pass, and the `LifecycleSweep|Startup|ReconcileDependency|LifecycleToken`
  group passes.
- Change from the original design: the step entry for an idle session runs
  detached. Running it inline held the whole startup sweep while it waited for
  agent readiness; the test now checks that the sweep returns first.

### Independent remediation

Office ownership read errors now retain the task. Conversational evidence is
matched to the accepted transition; lifecycle-only turns and previous visits to
the same step cannot spend the current entry. Both production turn writers stamp
entry identity under the task row lock. Prepared-route recovery retains the
accepted transition and source step. The startup worker join remains bounded by
the existing 10-second timeout and does not guarantee quiescence after expiry.
Permanent SQL-backed regressions and hosted checks are required for acceptance;
the independently reproduced RED was a contained synthetic decision proof.

The state-only repair also requires the observed workflow entry and task revision
inside the persistence transaction. Deterministic startup barriers commit a
successor after the turn-evidence read, for both different-step and same-step
moves, and assert unchanged Scheduling state and no stale publication. Repository
cases pin the same timestamp across entries and run on SQLite and configured
PostgreSQL. The current-entry control verifies a committed state change and one
normal task-service publication.
