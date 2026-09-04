---
id: "02-surface-stranded-signal"
title: "Surface an Office task's stranded signal"
status: done
wave: 2
depends_on: ["01-single-office-disposition"]
plan: "plan.md"
spec: "../../specs/office/requirements/stall-visibility.md"
---

# Task 02: Surface an Office task's stranded signal

Satisfies REQ-OFFICE-STALL-VISIBILITY-001.

## Acceptance

- `surfaceOfficeStalledSignal` fires on the `stuckSignalSurfaceOnly` outcome from
  both gate sites, emitting one `Warn` log with `task_id`, `session_id`,
  `step_id`, `signal_age`, and `gate`.
- `office_stall_stranded_signal_total` increments, labelled by `gate`, using the
  `expvar.NewMap` + `metricLabel` convention in
  `internal/office/scheduler/metrics_vars.go`.
- An in-memory dedupe keyed by session ID, step ID, and the signal's `SignaledAt`
  suppresses re-surfacing across the 30-second reaper ticks; the entry is dropped
  when the signal clears.
- Nothing else changes: no session state write, no turn close, no step
  transition, no decision row, no queued run.

## Verification

- `make -C apps/backend test`
- Test: an Office task with a stranded signal surfaces exactly once across three
  consecutive scans.
- Test: entered through `stuckSignalCandidate`, it surfaces (AC-001.2).
- Test: entered through `reconcileWaitingStuckSignalSessionIfDue`, it surfaces
  (AC-001.3).
- Test: after surfacing, task step, session state, decision rows and run queue
  are all unchanged (AC-003.1 through AC-003.4).
- Test: a new signal on the same session with a later `SignaledAt` surfaces
  again.

Each test must first fail on the Task 01 tree, where the surface-only outcome is
silent.

## Files likely touched

- `apps/backend/internal/orchestrator/stuck_signal_watchdog.go`
- `apps/backend/internal/orchestrator/stuck_signal_watchdog_test.go`
- `apps/backend/internal/orchestrator/service.go` (dedupe map field)
- new: `apps/backend/internal/orchestrator/office_stall_metrics.go`

## Dependencies

Task 01.

## Parallelism

Independent of Task 03 once Task 01 lands.
