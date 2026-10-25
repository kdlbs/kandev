---
id: "01-single-office-disposition"
title: "Express the watchdog's Office boundary once"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/stall-visibility.md"
---

# Task 01: Express the watchdog's Office boundary once

Pure refactor. No behavior change; the existing suite must stay green.

## Acceptance

- A single `stuckSignalDisposition` helper is the only place `task.IsFromOffice`
  is evaluated in `stuck_signal_watchdog.go`, returning `stuckSignalNotCandidate`,
  `stuckSignalReclaimable`, or `stuckSignalSurfaceOnly`.
- `reconcileWaitingStuckSignalSessionIfDue` (currently :138) and
  `stuckSignalCandidate` (currently :388) both consult it; neither retains an
  inline `IsFromOffice` term.
- An Office task returns `stuckSignalSurfaceOnly` from both sites and reaches
  neither reclaim path.
- A non-Office task's reclaim behavior is byte-for-byte unchanged.
- Passthrough sessions still return `stuckSignalNotCandidate` from both sites.

## Verification

- `make -C apps/backend test` — the whole existing
  `internal/orchestrator` suite, including
  `stuck_signal_watchdog_test.go` and
  `stuck_signal_watchdog_cancellation_test.go`, passes unchanged.
- New table-driven test asserting the disposition for each of: Office task,
  non-Office task, passthrough session, step mismatch, signal too young.
- New test asserting an Office task reaches neither reclaim path, entered
  independently through each of the two sites. This is the guard named in the
  plan's risk section and must not be weakened.

Write the disposition tests first against the current inline predicates so they
pass before and after the refactor; that is what proves it is behavior-preserving.

## Files likely touched

- `apps/backend/internal/orchestrator/stuck_signal_watchdog.go`
- `apps/backend/internal/orchestrator/stuck_signal_watchdog_test.go`

## Dependencies

None.

## Parallelism

Blocks Tasks 02 and 03. Both build on the disposition helper.
