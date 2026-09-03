---
spec: docs/specs/office/requirements/stall-visibility.md
design: docs/specs/office/system-design/stall-visibility.md
created: 2026-09-01
status: done
---

# Implementation Plan: Office Stall Visibility

## Overview

Make two invisible Office stall states visible from the backend, without giving
any sweep permission to repair them. Work lands in three waves: restructure the
watchdog so its Office boundary is expressed once (Wave 1), add stranded-signal
surfacing for Office tasks (Wave 2), and add the decision-waiting detector
(Wave 3).

## Confirmed root cause

Verified against the working tree on 2026-09-01.

`internal/orchestrator/stuck_signal_watchdog.go` excludes Office at two sites:

- **:138** — `reconcileWaitingStuckSignalSessionIfDue`
- **:388** — `stuckSignalCandidate`

Both are **recovery** gates, not detection gates. Passing either leads to a
reclaim that cancels the agent execution, force-closes the turn, and calls
`reconcileStepCompletionSignalLocked` to apply the requested step transition.
The originating plan unit describes these exclusions as the defect and asks for
them to be lifted, while separately forbidding any automatic transition for
Office. Both cannot hold: lifting the term at either site is what causes the
automatic transition. This plan resolves the conflict in favour of the stated
non-goal — detection is lifted for Office, recovery is not.

The second gap is real and unqualified. No detector covers a task parked at a
review or approval step with no run in flight and no decision recorded. The
Office inbox's `inboxTaskReviewRequests` surfaces "a decision is outstanding",
but has no age threshold and no in-flight-run condition, so a healthy task being
actively worked is indistinguishable from a stalled one.

## Schema findings that shape the work

Confirmed before implementation, as the plan unit asked:

- The `runs` table has **no `task_id` column**. Task linkage is
  `payload -> '$.task_id'`. Use `dialect.JSONExtract(driver, "payload",
  "task_id")`, not a literal `json_extract` — Postgres is a supported driver.
  Precedent: `internal/office/repository/sqlite/failure.go:351` with
  `failure_postgres_test.go`.
- Run statuses are `queued`, `claimed`, `finished`, `failed`, `cancelled`. There
  is no `running`; **`claimed` is the executing state**. "No queued or running
  run" maps to `status IN ('queued','claimed')`. Precedent for the whole
  predicate: `ReapStaleCheckouts` in `internal/office/repository/sqlite/tasks.go`.
- `orchestrator.Service` already holds step-scoped `engineParticipants`
  (`ListStepParticipants`) and `engineDecisions` (`ListStepDecisions`). No new
  wiring is needed for seats or decisions.
- `engineRunQueue` is **write-only** (`QueueRun`). A new narrow reader interface
  is required for the in-flight-run term.
- The Office repository's `ListActiveTaskDecisions` is deliberately task-scoped
  across all steps. Use the engine's step-scoped `ListStepDecisions` instead.

---

## Backend

### Wave 1: express the Office boundary once

Replace the two inline `task.IsFromOffice` terms with a single
`stuckSignalDisposition` helper returning `notCandidate` / `reclaimable` /
`surfaceOnly`. Pure refactor: Office still returns a non-recovery outcome at both
sites, so behavior is unchanged and the existing suite must stay green.

### Wave 2: surface a stranded signal on an Office task

Add `surfaceOfficeStalledSignal`, called on the `surfaceOnly` outcome from both
sites, with an in-memory dedupe keyed by session, step, and `SignaledAt`. Emits
a `Warn` log carrying the observing gate, and increments
`office_stall_stranded_signal_total`.

### Wave 3: decision-waiting detector

Add `office_decision_stall_watchdog.go` with a second pass on the existing reaper
tick, plus `officeRunInFlightReader` and its Office repository implementation,
wired from `internal/backendapp`. Predicate order puts the run check last, since it is the
most expensive term. Every unreadable input fails closed to "do not surface".

## Waves

Wave 1:

- [x] [Task 01: Express the watchdog's Office boundary once](task-01-single-office-disposition.md)

Wave 2:

- [x] [Task 02: Surface an Office task's stranded signal](task-02-surface-stranded-signal.md)

Wave 3:

- [x] [Task 03: Detect an Office task waiting on a decision](task-03-decision-waiting-detector.md)

## Required workflow verification

After all targeted task checks pass:

1. Run `make -C apps/backend fmt`.
2. Run `make -C apps/backend test lint`.
3. Commit the explicit changed paths with a Conventional Commit message.

## Risks and non-goals

- **The reclaim path must stay closed to Office.** Wave 1 is the load-bearing
  change: if a later edit routes `surfaceOnly` into the reclaim branch, an Office
  step advances without its quorum. The Wave 1 test asserting that no Office task
  reaches the reclaim path from either site is the guard against that, and must
  not be weakened.
- **A wrong alert is the failure mode that matters here.** Nothing is repaired,
  so the only way this capability can do harm is by training operators to ignore
  it. Every unreadable input fails closed, and
  `office_stall_detector_skipped_total` exists so a silently degraded detector is
  distinguishable from a quiet system.
- **The decision-waiting threshold is provisional and deliberately separate.**
  Wave 3 uses its own `officeDecisionWaitingThreshold`, set to 60 minutes, and
  must not reuse `stuckSignalWatchdogThreshold`. Ten minutes of session silence
  is anomalous; ten minutes of a human not yet deciding is a reviewer getting
  coffee. Sharing the constant would also couple two unrelated phenomena, so
  tuning session silence would silently retune this. Sixty minutes is expected to
  move once there is field data.
- **No user-visible surface in this plan.** Detection lands as logs and expvar
  counters only. Promoting it to an Office inbox row is deliberately deferred: a
  new inbox type carries a frontend change plus copy in five languages, and it
  should be designed against real detector output rather than in advance.
- No schema migration, no new table, no new HTTP endpoint, no change to
  non-Office reclaim behavior, and no retirement of the workspace-scoped
  `Stall Session Watchdog` automation.
