---
id: "01-office-disabled-routines-noop"
title: "Office-disabled routines cron no-op"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/cron-office-disabled-safety.md"
parallel-safe: true
---

# Task 01: Office-Disabled Routines Cron No-Op

## Root Cause

`backendapp/main.go:1280-1283` leaves `officeRoutines` a nil
`*office/routines.RoutineService` pointer when Office is disabled.
`startCronScheduler` (`backendapp/cron.go:39`) passes it into
`schedulercron.NewRoutinesHandler`, storing a typed-nil pointer in the
`RoutineTicker` interface field. `RoutinesHandler.Tick`'s `h.ticker == nil`
guard (`scheduler/cron/routines.go:57-61`) is bypassed because a typed-nil
pointer in an interface is a non-nil interface, so `TickScheduledTriggers` runs
on a nil receiver and panics every 30 seconds.

## Acceptance

- With Office disabled, the shared cron loop ticks without a routines
  nil-pointer panic and other handlers keep running.
- A `RoutinesHandler` constructed from an absent routines service treats each
  tick as a no-op returning success.
- With Office enabled, each tick still forwards to
  `RoutineService.TickScheduledTriggers` (behavior unchanged).

## Regression Test (RED first)

- In `scheduler/cron/routines_test.go`: build a handler whose ticker is a
  typed-nil concrete `RoutineTicker` (i.e. a `(*fakeTicker)(nil)` assigned into
  the interface, mirroring the production typed-nil) and assert `Tick` returns
  nil without panic. Add/confirm a positive case that a real ticker is forwarded.
- Add a `backendapp` test that `startCronScheduler` (or the routines-handler
  wiring helper) with a nil routines service produces a handler that ticks safely.

## Verification

```bash
cd apps/backend && go test -tags fts5 -run 'TestRoutinesHandler' ./internal/scheduler/cron
cd apps/backend && go test -tags fts5 -run 'TestStartCronScheduler|TestCron' ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/backendapp/cron.go`
- `apps/backend/internal/backendapp/main.go` (only if wiring moves here)
- `apps/backend/internal/scheduler/cron/routines.go` (optional hardening)
- `apps/backend/internal/scheduler/cron/routines_test.go`
- `apps/backend/internal/backendapp/*_test.go` (wiring regression)

## Dependencies

None.

## Parallelism

`sequential` within itself; `parallel-safe` against Tasks 02 and 03 (disjoint
packages, no shared schema/contract/config).

## Inputs

- Repair spec: `docs/specs/platform/requirements/cron-office-disabled-safety.md`.
- Confirmed 22 recurring panics in `~/.kandev/logs/backend-logs.log`.

## Output contract

Report the RED panic reproduction, the wiring change, files changed, exact
verification result, residual risks, and update this task plus `plan.md` status.
