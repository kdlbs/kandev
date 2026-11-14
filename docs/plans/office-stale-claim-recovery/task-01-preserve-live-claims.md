---
id: "01-preserve-live-claims"
title: "Preserve live executions during claim recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUN-CLAIM-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-RUN-CLAIM-RECOVERY-001.1
  - AC-TASKS-RUN-CLAIM-RECOVERY-001.2
  - AC-TASKS-RUN-CLAIM-RECOVERY-001.3
  - AC-TASKS-RUN-CLAIM-RECOVERY-001.4
  - AC-TASKS-RUN-CLAIM-RECOVERY-001.6
  - AC-TASKS-RUN-CLAIM-RECOVERY-001.7
system_design:
  - ../../specs/tasks/system-design/run-claim-recovery.md
---

# Task 01: Preserve live claims

## Summary

Before stale recovery queues an old claim, check its associated session.
Preserve active or uncertain execution. Recover abandoned unlaunched claims
with a conditional update and retain normal completion for live work.

## In scope

- Add a small local runtime/session check to the existing processor dependency wiring.
- Reuse recorded session identity, the existing task/agent fallback, and taskless attempt records.
- Replace the age-only bulk reset with candidate reads and guarded per-run recovery.
- Add the focused tests named in the plan.
- Cover the completion/cancellation race and the production guard wiring.

## Out of scope

A new generic evidence model, automatic repair of parked history, additional
completion handlers, provider-classifier changes, schema changes, and live cleanup.

## Acceptance

1. A live run aged 35 minutes launches once, retains its original claim, and finishes normally after repeated scheduler ticks.
2. Uncertain execution remains untouched; an abandoned unlaunched claim recovers. Taskless active attempts remain protected.
3. Recovery preserves concurrent completion, cancellation, and successor claims on SQLite and PostgreSQL.

## TDD and verification

First add `TestRunClaimRecoveryPreservesLiveExecution` and observe the current
sweep requeue the live run. Then implement the guard and make the test pass.
Use fake age or stored timestamps instead of a 30-minute wait.
Add a wiring test so the guard must exist in production, not only in a test double.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/runs/repository/sqlite ./internal/office/service ./internal/backendapp -run 'Test(RunClaimRecovery|PostgresRunClaimRecovery)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/runs/repository/sqlite -run '^TestRunClaimRecovery' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/office/service -run '^(TestSchedulerTick_RecoversStaleClaimedRun|TestRunLifecycle_StepCompleteEventsEmitted)$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Keep existing recovery test names or update these commands to their final names.
Run the PostgreSQL matrix with `KANDEV_TEST_POSTGRES_DSN` configured.
If the service is unavailable, record an explicit skip.
Run `bash scripts/lint-go-changed` with all changed Go files and record that exact file list.

## Files likely touched

- `apps/backend/internal/runs/repository/sqlite/runs.go` or a small sibling for the new query/update.
- `apps/backend/internal/runs/repository/sqlite/run_claim_recovery_test.go` and `run_claim_recovery_postgres_test.go` (new).
- `apps/backend/internal/office/service/scheduler_integration.go` and `service.go` (guard wiring).
- `apps/backend/internal/office/service/run_claim_recovery_test.go` (new).
- `apps/backend/internal/backendapp/main.go` (wire existing runtime/session dependencies).
- `apps/backend/internal/backendapp/run_claim_recovery_test.go` (new wiring test).
- Existing test fixtures that need session-state data for the new guard.

Test ownership: the service file covers live execution, abandoned claims,
uncertain lookup, and taskless attempts. The repository files cover guarded
writes. The backend file covers production wiring.
A small backend helper file is allowed if the existing composition file exceeds lint limits.
Do not introduce a shared recovery package.

## Dependencies

None.

## Risks

A failed lookup is not an absent session. A missing run session ID needs the
existing task/agent fallback or a deferred decision. Preserve ready execution
while completion is in flight. Do not broaden `FinishRun` to queued rows.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/run-claim-recovery.md).
- [Design](../../specs/tasks/system-design/run-claim-recovery.md).
- `TestSchedulerTick_RecoversStaleClaimedRun` in `office/service/task_starter_test.go`.
- `RecoverStale` and its existing repository tests.
- Existing runtime session lookup/probe and `office/repository/sqlite/run_sessions.go`.

## Results

The new live-execution regression first failed against the old sweep: the stale
claim was requeued while its original session remained active. The guarded
recovery made the test pass. The production-wiring regression now covers the
missing `runs.session_id` fallback to a matching `WAITING_FOR_INPUT` task
session with an execution identity. Repeated stale sweeps preserve the claim,
do not relaunch, and allow delayed completion to finish the run.

Passed checks:

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/runs/repository/sqlite ./internal/office/service ./internal/backendapp -run 'Test(RunClaimRecovery|PostgresRunClaimRecovery|SchedulerTick_RecoversStaleClaimedRun|RunLifecycle_StepCompleteEventsEmitted)' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/runs/repository/sqlite -run '^TestRunClaimRecovery' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/office/service -run '^(TestSchedulerTick_RecoversStaleClaimedRun|TestRunLifecycle_StepCompleteEventsEmitted)$' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/runs/repository/sqlite -run 'Test(ReadMethodsPropagateDatabaseErrors|WriteMethodsPropagateDatabaseErrors)' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/runs/repository/sqlite -run '^TestPostgresRunClaimRecoveryCAS$' -count=1 -v)` explicitly reported `SKIP` because `KANDEV_TEST_POSTGRES_DSN` was not configured.
- `make -C apps/backend build`
- `bash scripts/lint-go-changed` with the following changed Go files:
  `apps/backend/internal/backendapp/main.go`,
  `apps/backend/internal/backendapp/office_run_claim_recovery.go`,
  `apps/backend/internal/backendapp/run_claim_recovery_test.go`,
  `apps/backend/internal/office/repository/sqlite/runs_test.go`,
  `apps/backend/internal/office/service/finish_run_error_logging_test.go`,
  `apps/backend/internal/office/service/scheduler_integration.go`,
  `apps/backend/internal/office/service/service.go`,
  `apps/backend/internal/office/service/task_starter_test.go`,
  `apps/backend/internal/office/service/run_claim_recovery_test.go`,
  `apps/backend/internal/runs/repository/sqlite/errors_test.go`,
  `apps/backend/internal/runs/repository/sqlite/runs.go`,
  `apps/backend/internal/runs/repository/sqlite/runs_queue_test.go`,
  `apps/backend/internal/runs/repository/sqlite/run_claim_recovery_test.go`,
  `apps/backend/internal/runs/repository/sqlite/run_claim_recovery_postgres_test.go`,
  `apps/backend/internal/runs/repository/sqlite/runs_claim_recovery.go`.
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

The PostgreSQL CAS test was skipped because `KANDEV_TEST_POSTGRES_DSN` was not
configured; it is not counted as passed. The SQLite CAS tests ran. The
closed-reader test now checks `ListStaleClaimedRuns`, and the closed-writer test
checks `RecoverStaleClaimedRun` with a non-nil observed claim. The PostgreSQL
CAS assertion compares against the persisted terminal timestamp. No historical
parked-row cleanup was performed. Changes remain unstaged and uncommitted.
