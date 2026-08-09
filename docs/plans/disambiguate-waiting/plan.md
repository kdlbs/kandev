---
spec: docs/specs/disambiguate-waiting/spec.md
created: 2026-08-09
status: todo
---

# Implementation Plan: Waiting Attribution

## Overview

Observe, project, and render whether a `WAITING_FOR_INPUT` session is parked on a live
background shell process ("parked on background work") versus genuinely awaiting operator
input. The full signal chain is:

1. **`turn_started`** — a new stream event emitted by the agentctl adapter and relayed by
   the lifecycle manager so the orchestrator can timestamp each new prompt turn.
2. **Process-tree probe** — the agentctl server walks the agent process-tree and applies a
   start-time predicate to classify background children as `live | settled | unknown`.
3. **`BackgroundProbe` port** — an injectable interface that decouples the backend
   projection from the agentctl implementation detail.
4. **Recogniser + attestation** — the orchestrator records whether the session ever received
   a detached-shell background tool launch (`observed_detached`) and what turn it is on
   (`turn_marker`), feeding the three-term projection formula.
5. **Backend projection** — the `parkedState` + `taskParkedState` structs, the three-term
   formula (`observed_detached AND last_sample==live AND state==WAITING_FOR_INPUT`), the
   settle hook, the sampling loop, the publish rules and the task-level OR.
6. **Frontend** — `BackgroundWorkTaskIcon` promotion, both resolvers updated, all six data
   producers, merge-helper discard rule, and boot-snapshot reset.

## Dependency order

```
Wave 0 (seams, no-op — do first, ship clean):
  task-01  turn_started skeleton
  task-02  probe seam (port, client, manager, agentctl stub)
  task-03  parked DTO/wire/TS fields, hardcoded false/0

Wave 1 (parallel — all depend on Wave 0):
  task-04  agentctl probe implementation (real process-tree walk)
  task-05  recogniser + attestation (observed_detached, turn_marker)
  task-06  frontend (icon, resolvers, producers, discard rule)

Wave 2 (sequential — depends on task-04 AND task-05):
  task-07  backend projection (parkedState, sampling loop, publish rules)

Wave 3 (depends on Wave 2):
  task-08  guards, E2E, contract amendment docs
```

## Task list

| # | Task | Wave | Depends on | Status |
|---|------|------|-----------|--------|
| 01 | [turn_started skeleton](task-01-turn-started-skeleton.md) | 0 | — | todo |
| 02 | [probe seam](task-02-probe-seam.md) | 0 | — | todo |
| 03 | [parked DTO/wire/TS fields](task-03-parked-dto-fields.md) | 0 | — | todo |
| 04 | [agentctl probe](task-04-agentctl-probe.md) | 1 | 01, 02 | todo |
| 05 | [recogniser + attestation](task-05-recogniser-attestation.md) | 1 | 01 | todo |
| 06 | [frontend](task-06-frontend.md) | 1 | 03 | todo |
| 07 | [backend projection](task-07-backend-projection.md) | 2 | 02, 04, 05 | todo |
| 08 | [guards, E2E, docs](task-08-guards-e2e-docs.md) | 3 | 03, 06, 07 | todo |

## Key invariants from the spec

- **`parked_on_background_work`** is a runtime-only boolean — never persisted, restarts
  cleanly to `false`, epoch change is the client's restart signal.
- **Three-term conjunction**: `observed_detached AND last_sample==live AND
  session==WAITING_FOR_INPUT`. Any term `false` → `parked=false`.
- **`turn_started` notifQueue ordering**: both the turn-marker stamp and the emit happen
  inside `syncNotifQueueThen(afterBarrier)` callback on the update worker. Never emit
  outside the barrier. Never hold `asyncTurnMu` while awaiting the barrier (deadlock).
- **BackgroundProbe port** takes Kandev session id; agentctl `Client` method takes ACP id.
  The lifecycle manager translates. Passing the wrong id makes every probe return `ok==false`
  and the feature silently never parks (AC-45).
- **Kind filter is not optional**: `stampSubagentBackgroundWork` stamps `Detached=true` with
  `Kind=subagent` for both Claude and `mock-agent`. An unfiltered predicate breaks AC-37 in
  every `dev` and `e2e` profile run.
- **Settle hook seam is `updateTaskSessionStateWithHook`**, not `setSessionWaitingForInput`
  (three workflow sites bypass it), not `handleCompleteStreamEvent` (late state write).
- **Eviction of a `parked==true` row MUST publish the un-park first** then remove the row.
  Backend-shutdown eviction is the only case that publishes nothing.
- **Frontend: `buildSidebarItem` / `toSheetItem` read the parked bit from the TASK record**
  — NOT from `TaskStatusSummary` (which carries no parked field and gains none here).
