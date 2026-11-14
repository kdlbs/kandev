---
created: 2026-11-13
updated: 2026-11-14
status: complete
requirements:
  - REQ-TASKS-RUN-CLAIM-RECOVERY-001
system_design:
  - ../../specs/tasks/system-design/run-claim-recovery.md
legacy_specs: []
---

# Fix plan: Office stale-claim recovery

## Overview

Add a live-session guard to the existing stale-claim recovery path.
One work order implements and verifies the repair.
The user requested a simple fix on 2026-11-14.

Issue: [#5299](https://github.com/kdlbs/kandev/issues/5299), assigned to
`carlosflorencio`. Investigated revision: `841d12d1dd06eadf2fe2b066cddd8832ad2e1a25`.

## Mental model

A run is the scheduler's work record. A session is the agent doing that work.
The run normally moves from queued to claimed, then finished.
It stays claimed for the whole execution, however long that execution takes.

The sweep mistakes an old claim for an abandoned claim. After 30 minutes,
it queues the record again while the original agent keeps working.
The second launch is rejected. Routing can park the record as a provider error.
The original completion then misses it because the record is no longer claimed.

The repair checks the associated session before reclaiming the record.
Active or uncertain execution means leave the claim alone.
Abandoned claims that never launched remain recoverable.

## Scope

### In scope

- One local session-in-use check using existing runtime/session information.
- A guarded per-run recovery write instead of an unconditional bulk reset.
- Protection for task-bound execution and unfinished taskless attempts.
- Focused regressions and production dependency-wiring coverage.

### Out of scope

- A generic recovery framework or shared evidence model.
- A new heartbeat, table, status, timer, or provider classifier rule.
- Automated historical-row repair, reattachment, or additional completion handlers.
- Changes to concurrency ceilings or live user data.

The earlier draft's historical reconciliation work order is removed.
Its requirement IDs remain reserved. This package now delivers requirement 001 only.
Previously parked rows need targeted cleanup after their executions stop.
That is separate from preventing new occurrences and is not authorized here.

## Technical approach

Follow the [small repair design](../../specs/tasks/system-design/run-claim-recovery.md).
Keep the check local to Office's existing processor and backend wiring.
Use a boolean plus an error rather than a new execution-evidence state machine.
An error means skip this row, not assume its execution stopped.

Keep `FinishRun`, provider routing, and launch accounting unchanged.
A live run that retains its claim follows its existing completion path.
Use conditional SQL to preserve cancellation, completion, and a newer claim.

## Tests and E2E evidence

- `TestRunClaimRecoveryPreservesLiveExecution`: launch once, age the claim by
  35 minutes, run two ticks, then complete. Assert one launch and a finished run.
- `TestRunClaimRecoveryRequeuesAbandonedClaim`: an unlaunched abandoned claim recovers.
- `TestRunClaimRecoveryUnknownSession`: a failed or uncertain lookup preserves the claim.
- `TestRunClaimRecoveryTasklessAttempt`: preparing/running taskless attempts remain protected.
- `TestRunClaimRecoveryCAS` and `TestPostgresRunClaimRecoveryCAS`: stale observations cannot overwrite another writer.
- `TestRunClaimRecoveryProductionWiring`: exercise the real processor with the production dependency adapter and a controlled runtime.

The first test covers the changed backend lifecycle end to end.
No new browser flow or UI exists to test.
The work order gives the exact commands and test-file ownership.

## Work orders

- [x] [Task 01: Preserve live claims](task-01-preserve-live-claims.md) (complete).

Sequential implementation in the primary conversation. No delegation is authorized.

## Investigation and validation evidence

The temporary service reproduction confirmed two failures: a live claim became
queued and launched twice; original completion left a parked row queued.
It used real queue/repository/subscriber code and a controlled starter.
The park was set through the repository API; routing classification was traced in source.
The temporary test and log were removed.

The previous baseline commands passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/office/service -run '^(TestSchedulerTick_RecoversStaleClaimedRun|TestRunLifecycle_StepCompleteEventsEmitted)$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/runs/repository/sqlite -run '^TestRunClaimRecovery' -count=1)
```

Current claim admission counts claimed rows only. The issue's reported queued-row
capacity loss was not independently reproduced. Preserve the existing count.

The revised package passed catalog validation, specification lint, all 36
specification-linter tests, planned-source documentation coverage, and its
initial whitespace checks. Implementation adds guarded per-run recovery and
session checks. Review follow-up also protects a matching ready task session
when `runs.session_id` is empty, fixes the reader/writer error-test ownership,
and makes the PostgreSQL timestamp assertion compare persisted values. The
race-enabled recovery tests, repository error tests, backend build, changed-file
Go lint, current catalog validation, specification lint, and whitespace check
passed. PostgreSQL CAS execution was explicitly skipped because
`KANDEV_TEST_POSTGRES_DSN` was not configured. The design package and
implementation remain unstaged and uncommitted.

## Risks

- A missing session ID does not establish absence. Use the task/agent lookup or defer.
- Preserve a ready execution until its normal completion settles the run.
- Do not trade this defect for automatic replay of launched work with missing lifecycle evidence.
- Previously stranded rows remain a separate cleanup concern.
- Real PostgreSQL race evidence needs `KANDEV_TEST_POSTGRES_DSN`; record an unavailable service as skipped.

## Handoff

The design package and implementation remain unstaged and uncommitted.
