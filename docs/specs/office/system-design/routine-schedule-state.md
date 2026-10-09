---
status: current
system: office
requirements:
  - REQ-OFFICE-ROUTINE-ARMING-001
  - REQ-OFFICE-ROUTINE-ARMING-002
  - REQ-OFFICE-ROUTINE-ARMING-003
---

# Office Routine Schedule State System Design

## Purpose and boundaries

An Office routine has two independent switches: intent
(`office_routines.status`, operator-set) and schedule state (whether its
triggers can actually fire it, derived). The product reported only intent.
This design adds a second, read-only classification, surfaces it everywhere
intent is already surfaced (routine list/detail reads, the routines UI), and
adds an unattended-install detection path (a startup scan) for the same
classification. It repairs nothing — no code path here creates, edits, or
enables/disables a trigger. Dispatch keeps its existing status gates: the due
trigger query is status-independent, then the service checks the routine before
claiming a cron slot, while the manual and webhook paths reject non-firing
statuses. See [coordinator-install-idempotency.md](coordinator-install-idempotency.md)
for the one trigger-creation path this plan touches.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-ROUTINE-ARMING-001` | [Classification](#classification) |
| `REQ-OFFICE-ROUTINE-ARMING-002` | [Read-path reporting](#read-path-reporting), [Frontend rendering](#frontend-rendering) |
| `REQ-OFFICE-ROUTINE-ARMING-003` | [Startup scan](#startup-scan) |

## Components and responsibilities

- **Classifier** (`apps/backend/internal/office/routines/arming.go`) — pure,
  total function from one routine's trigger rows to a `ScheduleState` plus an
  unarmed-cron-trigger list. No I/O, no routine status input.
- **Batch classification** (`ClassifyRoutines`, same file) — the read
  boundary every consumer shares: one batched trigger read across many
  routines, with a per-routine fallback read when the batch read itself
  fails.
- **Read-path reporting** (`RoutineService.AttachScheduleState`,
  `internal/office/routines/service.go`) — the API consumer: wraps the
  existing list/get routine responses with the classification.
- **Startup scan** (`RunStartupScan`,
  `internal/office/routines/startup_scan.go`) — the unattended-install
  consumer: a read-only pass over every enumerable workspace and routine at
  boot, ordered after reconciliation, that logs and counts what it finds
  instead of returning it to a caller.
- **Frontend rendering**
  (`apps/web/app/office/routines/schedule-state.ts`,
  `schedule-state-badge.tsx`) — collapses the nine wire `ScheduleState`
  values into five label groups and renders `ScheduleStateBadge` /
  `UnarmedScheduleHint` on the routine list and detail views
  (`routine-row.tsx`).

## Data and contracts

### `ScheduleState`

One of nine values (`internal/office/routines/arming.go`):
`armed`, `trigger_invalid`, `trigger_unscheduled`, `trigger_disabled`,
`event_only`, `unscheduled_manual_only`, `unscheduled_no_trigger`, `unknown`.
`unknown` is reserved for `ClassifyRoutines`' per-routine fallback-read
failure; `ClassifyRoutine` itself never returns it.

### Classification rule table

`ClassifyRoutine(triggers []*RoutineTrigger, now time.Time)` evaluates, in
order, stopping at the first match:

1. Any enabled cron trigger with a next occurrence, or one that fired within
   the last `dispatchGrace` (60s, a compile-time constant shared verbatim
   with the startup scan) and hasn't been recomputed yet → `armed`.
2. Any enabled cron trigger for which the shared scheduler cannot compute a
   next occurrence (`shared.NextCronTime`), including an impossible but
   syntactically valid expression → `trigger_invalid`.
3. Any enabled, schedulable cron trigger with no next occurrence recorded →
   `trigger_unscheduled`.
4. One or more cron triggers exist but none matched 1-3 (all disabled) →
   `trigger_disabled`.
5. No cron trigger, but an enabled webhook trigger exists → `event_only`.
6. No cron trigger, but some other (e.g. manual) trigger exists →
   `unscheduled_manual_only`.
7. No trigger at all → `unscheduled_no_trigger`.

Rule 1 does not require schedulability: a trigger that already claimed a next
occurrence still fires on it even if edited into an unparseable expression
afterward — `unarmedCronTriggers` reports that separately, so the same
trigger can be simultaneously `armed` and listed as unarmed-with-reasons.

### `UnarmedCronTrigger`

`{trigger_id, reasons: UnarmedReason[]}`, one entry per cron trigger matching
the disabled-OR-not-schedulable-OR-stalled-past-grace predicate, independent
of the routine's overall `ScheduleState`. `UnarmedReason` is `disabled`,
`not_schedulable`, or `stalled`; more than one may apply to the same trigger.

### API contract

`RoutineListResponse` / `RoutineResponse`
(`internal/office/routines/dto.go`) embed `RoutineWithSchedule`:
`{*Routine, schedule_state, unarmed_cron_triggers}` (JSON tags), returned
from `GET /workspaces/:wsId/routines`, `GET /routines/:id`, and the
create/update routine handlers (`internal/office/routines/handler.go`).
`AttachScheduleState` classifies with one `time.Now().UTC()` capture per
call — never per-routine — so every routine in one response is judged
against the same instant.

### Frontend contract

`readScheduleState` / `readUnarmedCronTriggers`
(`apps/web/app/office/routines/schedule-state.ts`) tolerate the raw
snake_case wire shape directly, matching the codebase's established
`fetchJson`-does-not-case-convert pattern. `scheduleStateGroup` collapses the
nine wire values into five label groups (`armed`, `broken`, `event_only`,
`no_schedule`, `unknown`) so two routines in different groups never render
the same label. `hasSchedulableUnarmedEntry` distinguishes an unarmed entry
that only needs re-arming from one whose expression needs editing first.

## Control flow

**Read path:** handler calls `AttachScheduleState(ctx, routines)` →
`ClassifyRoutines` batch-reads triggers for all routine IDs via one
repository call → falls back to `ListTriggersByRoutineID` per routine only if
the batch call errors → `ClassifyRoutine` runs the pure rule table per
routine → handler serializes `RoutineWithSchedule`.

**Startup scan:** `Reconciler.ReconcileAll` returns a `ReconcileSignal`
(the only way to construct one is `SignalReconcileComplete`, called after
`ReconcileAll` returns — a zero-value signal, which an unauthorized caller or
a refusal-path test would hold, carries no completion) →
`internal/backendapp/main.go` launches `RunStartupScan` in a goroutine after
logging "Office reconciliation complete" → the scan enumerates workspaces,
then routines per workspace, then classifies each batch via the same
`ClassifyRoutines` path the read API uses → emits one structured log record
and one `expvar` observation per routine, plus a skip counter for any input
read that failed.

## Failure and recovery

- A batch trigger read failure degrades to per-routine reads; a
  per-routine read failure yields `ScheduleStateUnknown` for that routine
  only, never an error returned to the caller.
- The startup scan fails closed at every enumeration boundary (no signal, no
  workspaces, one workspace's routines unreadable) by skipping and counting
  the skip, never retrying and never aborting the rest of the scan.
- The scan is fire-and-forget (`go officeroutines.RunStartupScan(...)`): no
  outcome inside it can fail or delay Office startup.

## Persistence

No new tables or columns. The classifier and the scan are read-only over
`office_routines` and `office_routine_triggers`
(`internal/office/repository/sqlite/base.go`).

## Security

Read-only; no new authorization surface. Existing routine-read
authorization (workspace scoping) applies unchanged, since the classification
is attached to the same response the caller already had access to.

## Observability

- `office_routine_arming_scan_observations_total` — every routine the
  startup scan classified, labelled `workspace`, `intent`, `schedule_state`.
- `office_routine_arming_scan_skipped_total` — a scan step abandoned because
  an input could not be read, labelled `reason`
  (`no_signal`, `workspace_enum_failed`, `routine_enum_failed`,
  `classification_unknown`).
- Both are `expvar.Map` counters (`internal/office/routines/arming_metrics.go`),
  exposed via the stdlib `/debug/vars` handler in dev mode, following
  `internal/orchestrator/office_stall_metrics.go`'s label-string convention.
- The scan also emits one structured (`zap`) log record per routine reached,
  at `Warn` for a routine that cannot fire or has no schedule, `Info`
  otherwise.

## Related decisions

None — this design does not introduce a durable boundary or contract beyond
what the linked requirements themselves specify.
