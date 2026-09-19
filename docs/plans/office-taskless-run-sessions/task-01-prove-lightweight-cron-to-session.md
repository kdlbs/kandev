---
id: "01-prove-lightweight-cron-to-session"
title: "Prove the lightweight cron fire reaches a taskless session"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria:
  - AC-OFFICE-TASKLESS-001.1
  - AC-OFFICE-TASKLESS-001.2
  - AC-OFFICE-TASKLESS-001.3
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 01: Prove the lightweight cron fire reaches a taskless session

## Summary

`AC-OFFICE-TASKLESS-001.1` requires that an eligible lightweight wake with no
task starts a real agent session and reaches a visible terminal outcome
**without creating any task or task session**. The run-owned session mechanism
that satisfies this has shipped, but the end-to-end proof from an armed cron
trigger is still skipped, and the assertion it was written to make targets a
`task_sessions` row the design explicitly forbids.

Replace that skip with a real assertion against `office_run_sessions`.

## In scope

- `apps/backend/internal/backendapp/office_routine_cron_to_session_test.go`:
  - Remove the `t.Skip("blocked on card 49894d63: ...")` from
    `TestRoutine_CronFire_LightweightReachesSession` and the now-unreachable
    trailing `t.Fatal("unreachable: update this assertion when the lightweight
    path reaches a session")`.
  - Rewrite the doc comment above the test. It currently states that the
    lightweight path "has no session to reach at all today" and that the
    missing assertion is a `task_sessions` row. Both are wrong: the path
    launches through `RunSessionLauncher`, and the design forbids a
    `task_sessions` row for a taskless run.
  - Assert the positive half: after `TickScheduledTriggers` fires the armed
    trigger, an `office_run_sessions` row exists for the dispatched run, and
    `runs.session_id` is bound to that row's id.
  - Assert the negative half: the fire created no `tasks` row and no
    `task_sessions` row. This is the part of the criterion that distinguishes
    the lightweight path from the heavy one, so it must be asserted, not
    implied.
  - Wait on an observable condition (poll until the run reaches a launched
    state, bounded by a deadline), mirroring the polling the passing
    heavy-routine test in the same file already uses. Do not add a fixed sleep.
  - Assert the first clause of `AC-OFFICE-TASKLESS-001.2` across fires: arm and
    fire the trigger twice, and assert the two fires produce two distinct
    `office_run_sessions` ids with no ACP session reused between them. The
    existing continuation-summary tests assert scope selection and round-trip,
    not session identity, so nothing covers this today. The retry half of the
    same clause (a second attempt on one run) belongs to Task 05.

- `AC-OFFICE-TASKLESS-001.3`'s second sentence — "periodic idle skipping shall
  not consume a manual or webhook wake" — has no assertion anywhere. The two
  idle-skip tests both queue `shared.RunReasonRoutineDispatchCron`, so they cover
  only the periodic half. Close it beside the existing tests rather than in the
  cron-to-session file:
  - `apps/backend/internal/office/shared/runreasons_test.go`: assert
    `IsPeriodicTasklessWake(shared.RunReasonRoutineDispatchEvent)` is false. That
    value reaches the `default` branch of the switch today with nothing pinning
    it, so deleting the branch or adding the constant to the `true` case is a
    silent behavior change.
  - `apps/backend/internal/office/service/wo46_idle_skip_routine_dispatch_test.go`:
    a run queued with `RunReasonRoutineDispatchEvent`, an agent with
    `SkipIdleRuns` **on** and zero actionable tasks, is **not** skipped. This is
    the assertion that names the criterion: the mapping test in `shared` proves a
    manual or webhook fire carries that reason, and this one proves that reason
    survives the idle gate.

## Out of scope

- **Any production change.** This work order adds no production behavior; it
  only proves behavior that already exists. Concretely: no non-test file under
  `apps/backend/**` may change, and in particular nothing under
  `apps/backend/internal/office/**` or
  `apps/backend/internal/agent/runtime/**` outside the three `_test.go` files
  named in In scope.
- `TestRoutine_CronFire_HeavyRoutineReachesSession`, which passes and whose
  `task_sessions` assertion is correct for the heavy path.
- Any change to `REQ-OFFICE-TASKLESS-001` or its system design. If the test
  cannot be made to pass without a production change, stop and report a
  behavior finding rather than editing the contract or weakening the assertion.

## Acceptance conditions

1. `TestRoutine_CronFire_LightweightReachesSession` runs (is not skipped) and
   passes, asserting both an `office_run_sessions` row for the dispatched run
   and `runs.session_id` bound to it.
2. The same test asserts that neither a `tasks` row nor a `task_sessions` row
   was created by the lightweight fire.
3. `AC-OFFICE-TASKLESS-001.3`'s manual/webhook clause is asserted in both
   places In scope names: `IsPeriodicTasklessWake(RunReasonRoutineDispatchEvent)`
   is pinned false, and a run queued with that reason under `SkipIdleRuns` with
   zero actionable tasks is shown not to be skipped.
4. Exactly three source files change, all of them tests:
   `apps/backend/internal/backendapp/office_routine_cron_to_session_test.go`,
   `apps/backend/internal/office/shared/runreasons_test.go`, and
   `apps/backend/internal/office/service/wo46_idle_skip_routine_dispatch_test.go`
   — plus this work order and `plan.md`. No other file changes.

## Verification commands

```bash
cd apps/backend

# The target test, and its heavy sibling, must both pass and neither may skip.
go test ./internal/backendapp/ -run 'TestRoutine_CronFire' -v -count=1

# Repeat under the race detector: this drives an asynchronous dispatch chain.
go test -race ./internal/backendapp/ -run 'TestRoutine_CronFire' -count=1

# Flake check on the new polling assertion.
go test ./internal/backendapp/ -run 'TestRoutine_CronFire_LightweightReachesSession' -count=5

# Unchanged neighbours must stay green.
go test -count=1 ./internal/office/... ./internal/backendapp/...

gofmt -l internal/backendapp/office_routine_cron_to_session_test.go \
  internal/office/shared/runreasons_test.go \
  internal/office/service/wo46_idle_skip_routine_dispatch_test.go
make lint
```

From the repository root:

```bash
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
```

## Likely files

- `apps/backend/internal/backendapp/office_routine_cron_to_session_test.go`
- `apps/backend/internal/office/shared/runreasons_test.go`
- `apps/backend/internal/office/service/wo46_idle_skip_routine_dispatch_test.go`

Those three test files are the only source files this work order changes.

Read-only references:

- `apps/backend/internal/office/repository/sqlite/base.go` —
  `office_run_sessions` schema, and the `ListRunSessions` shape the assertion
  should use
- `apps/backend/internal/office/service/scheduler_integration.go` —
  `launchAgent`'s `taskID == ""` branch and `persistLaunchedSession`
- `apps/backend/internal/office/routines/service.go` —
  `dispatchRoutineRun`'s lightweight branch
- `apps/backend/internal/office/service/taskless_lifecycle_test.go` — existing
  taskless lifecycle assertions to match in style

## Dependencies

None. The production behavior under test is already on `main`
(`d355a672b1db624048f8ff87bc456a1729f78cb0`), and the sibling authority work
(`REQ-OFFICE-COORDINATOR-AUTHORITY-*`) is `implemented`.

## Risks

- **Flake.** The chain from cron tick to launched session is asynchronous across
  several components. Poll an observable condition with a deadline; a fixed
  sleep will be flaky under parallel shards. The `-count=5` run above is the
  check for this.
- **A real behavior gap.** If the lightweight chain does not actually reach
  `launchAgent` under this harness, that is a genuine finding against
  `AC-OFFICE-TASKLESS-001.1`. Report it; do not re-skip the test, do not
  soften the assertion to whatever the code happens to do, and do not assert on
  an intermediate row (a wakeup request or a run claim) as a substitute for the
  session the criterion actually requires.
