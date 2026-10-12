---
created: 2026-09-07
status: done
requirements:
  - REQ-OFFICE-LOOP-LIVENESS-001
  - REQ-OFFICE-LOOP-LIVENESS-002
  - REQ-OFFICE-LOOP-LIVENESS-003
  - REQ-OFFICE-LOOP-LIVENESS-004
  - REQ-OFFICE-LOOP-LIVENESS-005
system_design:
  - ../../specs/office/system-design/loop-liveness.md
legacy_specs: []
---

# Implementation Plan: Office Loop Liveness

## Overview

Office's unattended loop (cron tick -> trigger claim -> routine run -> wake ->
`runs` row -> claim -> launch -> terminal) has no surface that says whether it
ran. On the instance this card was written against, the loop had been dead
since 2026-08-05 and nothing surfaced it; discovering that took a code read
plus hand-written SQL. This plan makes the loop legible without repairing it:
persist `last_run_at`, carry one causation id end to end, add counters and a
liveness verdict endpoint outside dev mode, and classify every terminal run
shape so a silent failure reads differently from quiet success.

## Root cause

No counter exists for routine ticks, runs launched, or taskless failures.
`/debug/vars` is dev-mode only. `routines.last_run_at` is never written, so
"never ran" and "ran correctly 326 times" are indistinguishable from any
available surface. `runs.session_id` can be silently dropped on launch, and
five distinct terminal shapes all persist `status = 'finished'`, so a
dashboard bucketing them collapses real failures into apparent success.

## Scope

### In scope

- `REQ-OFFICE-LOOP-LIVENESS-001`: monotonic `last_run_at` write via a narrow
  `TouchRoutineLastRun`, `UpdateRoutine` no longer writes the column.
- `REQ-OFFICE-LOOP-LIVENESS-002`: a `causation_id` minted once per wake
  origin, carried `office_routine_runs` -> `agent_wakeup_requests` -> `runs`,
  and a real `runs.session_id` (the Office launch seam stops discarding the
  session id the orchestrator returns).
- `REQ-OFFICE-LOOP-LIVENESS-003`: per-hop counters readable without dev mode
  via `GET /api/v1/office/workspaces/:wsId/loop-counters`.
- `REQ-OFFICE-LOOP-LIVENESS-004`: `GET .../loop-health` returning a verdict
  (`unknown -> dead -> degraded -> not_armed -> healthy`) plus evidence and
  thresholds.
- `REQ-OFFICE-LOOP-LIVENESS-005`: a total terminal-shape classification over
  `(status, outcome, session_id)`, separating `silent_success` from the other
  terminal shapes.

### Out of scope

- Repairing the loop once it is detected as dead (detection only; `runs.outcome`
  is not widened).
- `office_routine_runs`' terminal transition / `SyncRunStatus` (owned by a
  sibling card).
- Operator notification on a detected stall (blocked on this card, tracked
  separately).

## Work orders

- [Task 01: Make the Office loop legible end-to-end](task-01-make-the-loop-legible.md)

## Verification

```bash
cd apps/backend
go test -tags fts5 -count=1 ./internal/office/... ./internal/runs/...
golangci-lint run ./internal/office/... ./internal/runs/...
```
