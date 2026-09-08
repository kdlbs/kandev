---
status: draft
system: platform
created: 2026-08-03
owners:
  - cfl
---
# Shared Cron Loop Safety When Office Is Disabled Requirements

## Overview

The shared cron loop (ADR-0004 Phase 5) drives every backend timer — heartbeat,
budget, Office routines, and Office recovery — on a single goroutine. Office is
an optional feature. When Office is disabled the routines collaborator does not
exist, and the cron loop must keep running the other handlers instead of
crashing. A crash here is severe because the loop's panic recovery re-arms every
tick: the backend logs a nil-pointer panic every 30 seconds for the life of the
process and never fires heartbeat or budget work.

## Requirements

### REQ-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001: Shared Cron Loop Safety When Office Is Disabled

**Intent:** The shared cron loop (ADR-0004 Phase 5) drives every backend timer — heartbeat, budget,
Office routines, and Office recovery — on a single goroutine. Office is an optional feature. When
Office is disabled the routines collaborator does not exist, and the cron loop must keep running the
other handlers instead of crashing. A crash here is severe because the loop's panic recovery re-arms
every tick: the backend logs a nil-pointer panic every 30 seconds for the life of the process and
never fires heartbeat or budget work.

#### Acceptance criteria

- **AC-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001.1:** The shared cron loop starts and keeps ticking whether or not Office is enabled.
- **AC-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001.2:** When the Office routines service is absent, the routines cron handler performs no work and returns success on every tick. It does not panic, log an error, or abort the loop.
- **AC-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001.3:** When the Office routines service is present, the routines cron handler forwards each tick to `RoutineService.TickScheduledTriggers` exactly as before. Enabling Office does not change routine dispatch behavior.
- **AC-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001.4:** The remaining cron handlers (heartbeat, budget, Office recovery) run independently of whether the routines handler is a no-op.
- **AC-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001.5:** Wiring a nil collaborator into any cron handler is safe: a genuinely absent collaborator produces a no-op handler, never a handler that dereferences a nil value during `Tick`.
- **AC-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001.6:** **GIVEN** the backend starts with Office disabled, **WHEN** the shared cron loop ticks, **THEN** the routines handler performs no work, returns success, does not panic, and the heartbeat, budget, and Office-recovery handlers keep running.
- **AC-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001.7:** **GIVEN** the backend starts with Office disabled, **WHEN** the cron loop runs for multiple ticks, **THEN** no nil-pointer panic is logged for the routines handler on any tick.
- **AC-PLATFORM-CRON-OFFICE-DISABLED-SAFETY-001.8:** **GIVEN** the backend starts with Office enabled and a real routines service, **WHEN** the cron loop ticks, **THEN** the routines handler forwards each tick to `RoutineService.TickScheduledTriggers` and normal routine dispatch is unchanged.

## Migrated source detail

## Why

The shared cron loop (ADR-0004 Phase 5) drives every backend timer — heartbeat,
budget, Office routines, and Office recovery — on a single goroutine. Office is
an optional feature. When Office is disabled the routines collaborator does not
exist, and the cron loop must keep running the other handlers instead of
crashing. A crash here is severe because the loop's panic recovery re-arms every
tick: the backend logs a nil-pointer panic every 30 seconds for the life of the
process and never fires heartbeat or budget work.

## What

- The shared cron loop starts and keeps ticking whether or not Office is
  enabled.
- When the Office routines service is absent, the routines cron handler performs
  no work and returns success on every tick. It does not panic, log an error, or
  abort the loop.
- When the Office routines service is present, the routines cron handler forwards
  each tick to `RoutineService.TickScheduledTriggers` exactly as before. Enabling
  Office does not change routine dispatch behavior.
- The remaining cron handlers (heartbeat, budget, Office recovery) run
  independently of whether the routines handler is a no-op.
- Wiring a nil collaborator into any cron handler is safe: a genuinely absent
  collaborator produces a no-op handler, never a handler that dereferences a nil
  value during `Tick`.

## Data Model

No data model change. The behavior concerns in-process wiring of the cron loop
handlers only.

## API Surface

No user-facing HTTP or WebSocket surface. The affected contract is the internal
cron handler seam:

```go
// RoutineTicker is the surface RoutinesHandler needs from the office
// routines service.
type RoutineTicker interface {
    TickScheduledTriggers(ctx context.Context, now time.Time) error
}

func NewRoutinesHandler(ticker RoutineTicker, now func() time.Time, log *logger.Logger) *RoutinesHandler
```

`RoutinesHandler.Tick` treats a nil ticker as a no-op. The safety guarantee is
that this nil check is reached whenever no routines service is configured,
regardless of how the absence is expressed by the caller.

## Failure Modes

- **Typed-nil collaborator (the regression).** A concrete nil pointer
  (`(*office/routines.RoutineService)(nil)`) assigned into the `RoutineTicker`
  interface produces a non-nil interface value. `RoutinesHandler.Tick`'s
  `h.ticker == nil` guard is then bypassed, `TickScheduledTriggers` runs on a nil
  receiver, dereferences the service's repository, and panics. The cron loop's
  per-handler panic recovery converts this into a recurring error every tick.
  Desired behavior: the handler must observe a genuinely nil interface when no
  service exists, so the no-op branch is taken.
- **Office enabled but routines service still nil.** Should never happen in
  practice, but the handler must still no-op rather than panic.
- **Routines service returns an error while Office is enabled.** Unchanged: the
  error propagates from `Tick` to the loop, which logs it without aborting other
  handlers.

## Persistence Guarantees

None. This behavior is process-local wiring and has no persisted state.

## Scenarios

- **GIVEN** the backend starts with Office disabled, **WHEN** the shared cron
  loop ticks, **THEN** the routines handler performs no work, returns success,
  does not panic, and the heartbeat, budget, and Office-recovery handlers keep
  running.
- **GIVEN** the backend starts with Office disabled, **WHEN** the cron loop runs
  for multiple ticks, **THEN** no nil-pointer panic is logged for the routines
  handler on any tick.
- **GIVEN** the backend starts with Office enabled and a real routines service,
  **WHEN** the cron loop ticks, **THEN** the routines handler forwards each tick
  to `RoutineService.TickScheduledTriggers` and normal routine dispatch is
  unchanged.
- **GIVEN** a routines cron handler is constructed with a nil routines
  collaborator by any wiring path, **WHEN** it ticks, **THEN** it returns success
  without dereferencing the collaborator.

## Out of Scope

- Changing routine dispatch, concurrency policy, fingerprinting, or cron
  expression evaluation inside the Office routines service.
- Changing heartbeat, budget, or Office-recovery handler behavior.
- Adding a user-facing surface for cron health.
- Making the cron tick interval configurable.

## Implementation Plan

See [the implementation plan](../../../plans/backend-regressions-office-disabled-cleanup/plan.md).
