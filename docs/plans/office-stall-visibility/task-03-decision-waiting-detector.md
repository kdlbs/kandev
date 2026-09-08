---
id: "03-decision-waiting-detector"
title: "Detect an Office task waiting on a decision"
status: done
wave: 3
depends_on: ["01-single-office-disposition"]
plan: "plan.md"
spec: "../../specs/office/requirements/stall-visibility.md"
---

# Task 03: Detect an Office task waiting on a decision

Satisfies REQ-OFFICE-STALL-VISIBILITY-002.

## Acceptance

- A second detection pass runs on the existing idle-reaper tick, bounded by the
  same `stuckSignalScanBudget` and sharing its defer-to-next-tick behavior.
- Predicate, evaluated in this order: task is Office → current step has a
  `decision_required` reviewer or approver seat
  (`engineParticipants.ListStepParticipants`) → no active decision for
  (task, current step) (`engineDecisions.ListStepDecisions`) → age over threshold
  → no in-flight run. The run check is last because it is the most expensive.
- `officeRunInFlightReader.HasInFlightRunForTask` is implemented in the Office
  SQLite repository using `dialect.JSONExtract(driver, "payload", "task_id")` and
  `status IN ('queued','claimed')`, and is wired from `internal/backendapp`, where the Office repository is constructed.
- Surfacing is a `Warn` log plus `office_stall_decision_waiting_total`, with the
  same in-memory dedupe (keyed by task ID and step ID).
- Every unreadable input fails closed to "do not surface" and increments
  `office_stall_detector_skipped_total` labelled by `reason`.
- Threshold is its own named constant, `officeDecisionWaitingThreshold` = 60
  minutes, commented as provisional. Reusing `stuckSignalWatchdogThreshold` is
  explicitly rejected: it couples two unrelated phenomena.

## Verification

- `make -C apps/backend test`
- Test: a decision-waiting Office task past the threshold is surfaced
  (AC-002.1).
- Test: **the same task with a `claimed` run is not surfaced** (AC-002.2). This
  is the false-positive guard named in the definition of done.
- Test: the same task with a `queued` run is not surfaced.
- Test: an active decision row for the current step suppresses surfacing
  (AC-002.3).
- Test: a step with only non-decision-required or watcher/collaborator seats is
  not surfaced (AC-002.4).
- Test: run reader unwired, and run reader returning an error, each skip and
  count (AC-002.5).
- Test: a non-Office task is never evaluated (AC-002.6).
- Test: surfacing writes no decision row, queues no run, and does not change the
  task's step (AC-003.1 through AC-003.3).
- Postgres-dialect test for the new repository query, mirroring
  `internal/office/repository/sqlite/failure_postgres_test.go`.

## Files likely touched

- new: `apps/backend/internal/orchestrator/office_decision_stall_watchdog.go`
- new: `apps/backend/internal/orchestrator/office_decision_stall_watchdog_test.go`
- `apps/backend/internal/orchestrator/service.go` (reader setter)
- `apps/backend/internal/orchestrator/idle_session_reaper.go` (tick call site)
- `apps/backend/internal/office/repository/sqlite/runs.go`
- `apps/backend/internal/office/repository/sqlite/runs_test.go`
- `apps/backend/internal/backendapp/main.go` (wiring)

## Dependencies

Task 01. Independent of Task 02.

## Parallelism

Can run alongside Task 02.
