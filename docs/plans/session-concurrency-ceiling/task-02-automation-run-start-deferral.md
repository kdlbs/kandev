---
id: "02-automation-run-start-deferral"
title: "Keep ceiling-queued automation runs open"
status: done
wave: 2
depends_on:
  - "01-session-concurrency-ceiling"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.1
  - AC-AGENTS-SESSION-CEILING-001.5
  - AC-AGENTS-SESSION-CEILING-001.10
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 02: Keep ceiling-queued automation runs open

## Summary

An automation run's start refused by the ceiling was treated as a failed
dispatch. The run failed, and failure cleanup deleted the task with its
deferred record, so the sweep had nothing to replay. Queue the start as other
automatic callers do, and bind the replayed launch to its run.

## In scope

- Let the automation dispatcher recognize a queued start and leave the run
  `triggered` with its task, instead of failing it.
- Skip failure cleanup for a queued start so the task and record remain.
- Record the run identity and thread disposition in the `start` payload.
- Replay the start through the run dispatcher to bind its session and turn,
  or fail the run on a non-ceiling launch failure.
- Stop a replayed session whose run could not be bound.
- Exempt run starts from workflow-step auto-start eligibility.
- Drop a start whose run was deleted or is no longer `triggered`, whether the
  sweep's check or the replay's dispatch finds it closed.
- Fail the run of any other dropped queued start before clearing its record.
- Fail the unbound run when task deletion removes its deferred start record.

## Out of scope

- A new run status, a schema change, or a change to run history presentation.
- Keeping a queued start across a backend restart. Startup reconciliation
  still fails an unbound run, and the sweep then drops its start.
- Continuation runs, whose prompt is refused at seam 3 rather than queued as a
  `start` record.
- Runs whose automation service has no run dispatcher.

## Acceptance

- `AC-AGENTS-SESSION-CEILING-001.10`: a queued start keeps its task, record,
  and open run. The replay binds the session and turn, and the bound turn's
  completion settles the run and frees its concurrency slot. A replay whose
  session launched but could not be bound fails the run and stops the session.
  A record that survives its replay is dropped by the next sweep. Deleting a
  task with a deferred start fails its unbound run and releases the automation
  concurrency slot.
- `AC-AGENTS-SESSION-CEILING-001.1` and `AC-AGENTS-SESSION-CEILING-001.5`: the
  refusal persists a replayable record, and a still-refused replay keeps it.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/automation -count=1
cd apps/backend && go test -tags fts5 ./internal/orchestrator -run "TestAutomationStartDeferredByCeiling" -count=1
cd apps/backend && go test -tags fts5 ./internal/orchestrator -run "TestAutomationStartDeferredByCeiling|Ceiling|Replay|Sweep|Automation" -count=1
cd apps/backend && golangci-lint run ./internal/automation/... ./internal/orchestrator/... --new-from-rev=e96910ff9a44d9c5e2970f8b92f50039b001d31e --timeout=10m
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- `TestAutomationStartDeferredByCeiling_*` failed before the change: the task
  was deleted and the run failed with the ceiling sentinel as its error.
- The same tests pass after the change. They cover replay binding and
  settlement, a stopped or deleted run's start not launching, a dropped start
  and a failed replay each failing the run once, a replay that could not bind
  its run stopping its session, a run closed before the replay's dispatch, and
  a surviving record of a bound run being dropped, with and without a workflow
  step.
- `TestDispatchRunDeferredLaunchLeavesTheRunOpen` covers the dispatcher.
- `TestAutomationStartDeferredByCeiling_TaskDeletionFailsRunAndReleasesCapacity`
  covers hard deletion of a queued task: the unbound run fails, its slot is
  released, a later trigger is admitted, and the deleted task is not launched.
- The automation package and the focused orchestrator selection pass.
- The deletion regression passes with the race detector. The pre-existing
  route-recovery test that failed in the earlier CI run passes five race-enabled
  repetitions.
- Public documentation and specification validation pass.
- Backend lint against the base revision reports 0 issues.

## Risks

- A queued start holds its run's concurrency slot for as long as the ceiling
  stays full.
