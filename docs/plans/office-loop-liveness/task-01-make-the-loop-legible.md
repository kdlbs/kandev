---
id: "01-make-the-loop-legible"
title: "Make the Office loop legible end-to-end"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-LOOP-LIVENESS-001
  - REQ-OFFICE-LOOP-LIVENESS-002
  - REQ-OFFICE-LOOP-LIVENESS-003
  - REQ-OFFICE-LOOP-LIVENESS-004
  - REQ-OFFICE-LOOP-LIVENESS-005
acceptance_criteria:
  - AC-OFFICE-LOOP-LIVENESS-001.1
  - AC-OFFICE-LOOP-LIVENESS-001.2
  - AC-OFFICE-LOOP-LIVENESS-001.3
  - AC-OFFICE-LOOP-LIVENESS-001.4
  - AC-OFFICE-LOOP-LIVENESS-001.5
  - AC-OFFICE-LOOP-LIVENESS-001.6
  - AC-OFFICE-LOOP-LIVENESS-001.7
  - AC-OFFICE-LOOP-LIVENESS-001.8
  - AC-OFFICE-LOOP-LIVENESS-002.1
  - AC-OFFICE-LOOP-LIVENESS-002.2
  - AC-OFFICE-LOOP-LIVENESS-002.3
  - AC-OFFICE-LOOP-LIVENESS-002.4
  - AC-OFFICE-LOOP-LIVENESS-002.5
  - AC-OFFICE-LOOP-LIVENESS-002.6
  - AC-OFFICE-LOOP-LIVENESS-002.7
  - AC-OFFICE-LOOP-LIVENESS-002.8
  - AC-OFFICE-LOOP-LIVENESS-002.9
  - AC-OFFICE-LOOP-LIVENESS-002.10
  - AC-OFFICE-LOOP-LIVENESS-002.11
  - AC-OFFICE-LOOP-LIVENESS-003.1
  - AC-OFFICE-LOOP-LIVENESS-003.2
  - AC-OFFICE-LOOP-LIVENESS-003.3
  - AC-OFFICE-LOOP-LIVENESS-003.4
  - AC-OFFICE-LOOP-LIVENESS-003.5
  - AC-OFFICE-LOOP-LIVENESS-003.6
  - AC-OFFICE-LOOP-LIVENESS-003.7
  - AC-OFFICE-LOOP-LIVENESS-003.8
  - AC-OFFICE-LOOP-LIVENESS-003.9
  - AC-OFFICE-LOOP-LIVENESS-004.1
  - AC-OFFICE-LOOP-LIVENESS-004.2
  - AC-OFFICE-LOOP-LIVENESS-004.3
  - AC-OFFICE-LOOP-LIVENESS-004.4
  - AC-OFFICE-LOOP-LIVENESS-004.5
  - AC-OFFICE-LOOP-LIVENESS-004.6
  - AC-OFFICE-LOOP-LIVENESS-004.7
  - AC-OFFICE-LOOP-LIVENESS-004.8
  - AC-OFFICE-LOOP-LIVENESS-004.9
  - AC-OFFICE-LOOP-LIVENESS-004.10
  - AC-OFFICE-LOOP-LIVENESS-004.11
  - AC-OFFICE-LOOP-LIVENESS-004.12
  - AC-OFFICE-LOOP-LIVENESS-004.13
  - AC-OFFICE-LOOP-LIVENESS-004.14
  - AC-OFFICE-LOOP-LIVENESS-004.15
  - AC-OFFICE-LOOP-LIVENESS-004.16
  - AC-OFFICE-LOOP-LIVENESS-004.17
  - AC-OFFICE-LOOP-LIVENESS-005.1
  - AC-OFFICE-LOOP-LIVENESS-005.2
  - AC-OFFICE-LOOP-LIVENESS-005.3
  - AC-OFFICE-LOOP-LIVENESS-005.4
  - AC-OFFICE-LOOP-LIVENESS-005.5
  - AC-OFFICE-LOOP-LIVENESS-005.6
  - AC-OFFICE-LOOP-LIVENESS-005.7
  - AC-OFFICE-LOOP-LIVENESS-005.8
  - AC-OFFICE-LOOP-LIVENESS-005.9
system_design:
  - ../../specs/office/system-design/loop-liveness.md
---

# Task 01: Make the Office loop legible end-to-end

## Summary

Persist a monotonic `last_run_at` on every routine fire, mint one
`causation_id` per wake origin and carry it (plus a real `session_id`) from
routine run to wake to `runs` row, expose per-hop counters and a liveness
verdict endpoint outside dev mode, and classify every terminal run shape so
"quiet because there is no work" reads differently from "quiet because it
silently failed."

## In scope

- `TouchRoutineLastRun`, a narrow monotonic-guarded write called from
  `dispatchRoutineRun` before concurrency-policy evaluation; `UpdateRoutine`
  stops writing `last_run_at`.
- A `causation_id` minted in `dispatchRoutineRun`, carried through the wakeup
  and into the claimed `runs` row; `StartTaskWithRoute` widened to return the
  session id so the Office launch seam stops discarding it.
- expvar counters per loop hop (tick evaluated, trigger fired, wake created,
  run claimed, agent launched, terminal outcome by kind), reachable outside
  dev mode via `GET /api/v1/office/workspaces/:wsId/loop-counters`.
- `GET /api/v1/office/workspaces/:wsId/loop-health`: a verdict
  (`unknown -> dead -> degraded -> not_armed -> healthy`) with evidence lists
  and thresholds, failing loud (503 naming the failing input).
- `ClassifyTerminalRun`, a function over `(status, outcome, session_id)` plus
  the activation instant, separating `silent_success` from
  `unlaunched_skipped`, `unlaunched_failed`, `launched_completed`,
  `launched_failed`, `pre_activation`, and `unclassified`.

## Out of scope

- Repairing a detected-dead loop (detection only; `runs.outcome` is not
  widened).
- `office_routine_runs`' terminal transition / `SyncRunStatus`.
- Operator notification on a detected stall.

## Acceptance

- `routines.last_run_at` advances monotonically on every dispatch, including
  a skip, and is never written by `UpdateRoutine`.
- A `causation_id` minted once per wake origin is readable end to end from
  `office_routine_runs` through `agent_wakeup_requests` into the claimed
  `runs` row; `runs.session_id` reflects the real launched session.
- `/loop-counters` and `/loop-health` are reachable without dev mode and
  without pprof enabled.
- `/loop-health` returns a verdict, its supporting evidence, and the
  thresholds used, and responds 503 naming the failing input on a read
  failure rather than failing closed.
- Every terminal run classifies into exactly one named shape; a legacy value
  outside the current outcome enum and a pre-activation row both classify
  without error.

## Verification

```bash
cd apps/backend
go build ./... && go vet ./...
go test -tags fts5 -count=1 ./internal/office/... ./internal/runs/...
golangci-lint run ./internal/office/... ./internal/runs/...
```
