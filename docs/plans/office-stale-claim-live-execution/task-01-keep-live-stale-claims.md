---
id: "01-keep-live-stale-claims"
title: "Keep stale claims of live executions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-STALE-CLAIM-001
acceptance_criteria:
  - AC-OFFICE-STALE-CLAIM-001.1
  - AC-OFFICE-STALE-CLAIM-001.2
  - AC-OFFICE-STALE-CLAIM-001.3
  - AC-OFFICE-STALE-CLAIM-001.4
system_design:
  - ../../specs/office/system-design/stale-claim-live-execution.md
---

# Task 01: Keep stale claims of live executions

## In scope

- `ListStaleClaimedTaskRuns` and `RenewStaleClaim` in the runs repository.
- `TaskRunExecutionProbe` and the renewal step before `RecoverStale` in the
  Office runs processor.
- `orchestrator.Service.OfficeTaskRunExecutionLive` and its task starter
  adapter method.

## Out of scope

Runs parked before this change, launch-error classification, and finished
turns left at `WAITING_FOR_INPUT`.

## Acceptance

1. A stale claim whose session is live stays `claimed` with a renewed
   `claimed_at`; ended, probe-failed, taskless, and probe-less claims are
   requeued.
2. The probe uses the recorded session or the (task, agent) session, and
   rejects a session of another task or agent.
3. Renewal does not touch a run that is no longer a stale claim.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -count=1 ./internal/runs/repository/sqlite -run 'StaleClaim|RecoverStale')
(cd apps/backend && go test -tags fts5 -count=1 ./internal/office/service -run 'RecoverStaleClaimedRuns')
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestrator -run 'OfficeTaskRunExecutionLive')
```
