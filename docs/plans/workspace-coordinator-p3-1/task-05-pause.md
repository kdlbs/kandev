---
id: "05-pause"
title: "Pause"
status: implemented
wave: 2
depends_on:
  - "phase 3 merged"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-PAUSE-001
  - REQ-COORDINATOR-PAUSE-002
  - REQ-COORDINATOR-PAUSE-003
acceptance_criteria:
  - AC-COORDINATOR-PAUSE-001.1
  - AC-COORDINATOR-PAUSE-001.2
  - AC-COORDINATOR-PAUSE-001.3
  - AC-COORDINATOR-PAUSE-002.1
  - AC-COORDINATOR-PAUSE-002.2
  - AC-COORDINATOR-PAUSE-002.3
  - AC-COORDINATOR-PAUSE-002.4
  - AC-COORDINATOR-PAUSE-002.5
  - AC-COORDINATOR-PAUSE-002.6
  - AC-COORDINATOR-PAUSE-002.7
  - AC-COORDINATOR-PAUSE-003.1
  - AC-COORDINATOR-PAUSE-003.2
  - AC-COORDINATOR-PAUSE-003.3
  - AC-COORDINATOR-PAUSE-003.4
system_design:
  - ../../specs/coordinator/system-design/pause.md
---

# Task 05: Pause (WP 3.1-5)

## Summary

Adds the paused state, the route, the precondition in front of wake delivery,
the dream trigger and automatic approval, the stop of a running unattended
turn, and the Pause and Resume controls.

## In scope

- `internal/coordinator/pause/`: `Gate.Active` (compiled in, enforcing stored
  state whatever the flag), `Stopper` (with a registration point for the dream
  canceller), tests beside each.
- Store: `paused_at` and `paused_by` on `coordinators`; `PUT
  /coordinators/:id/pause` behind the flag; the precondition call before
  `Admit`, at the start of `TryAutomaticApproval` and, through work order 04,
  in the dream tick.
- The unattended-turn outcome `stopped_by_pause`, and the backstop's retry of
  `Stopper.Stop` for a paused coordinator.
- The autonomy read gains `paused`, `paused_at`, `paused_by` (whatever the
  flag), and Pause and Resume publish `coordinator.updated` with
  `autonomy_changed: true`.
- Web: the fourth state of the autonomy strip and the Autonomy section
  control, phone layout, copy in six locales.
- Existing code changed: `coordinator_unattended_turns` gains nullable
  `pause_requested_at` and `pause_cancel_at` (idempotent column adds in `Store.migratePhase2`, `store_phase2_schema.go`, replay test);
  `boundTurnOutcome` in `turn_end.go` maps `pause_cancel_at` (when
  `stop_requested_at` is unset) to `stopped_by_pause` whatever the session state;
  `Store.settleOpenTurn` (`store_turns.go`) returns the wakes of a row it settles
  `stopped_by_pause` to `pending` in the settle transaction, and `settleBoundTurn`
  stays the one settle path for the Stopper and turn end; new
  `Store.settlePausedUnsentTurn` (`store_turns.go`) settles an unbound, unreserved
  row; the existing `settleUnsentTurn` and `settleNotSent` are unchanged;
  `Stopper.Stop` runs before `recoverUnboundTurn` in the backstop pass; `ceiling.go` stays reading `stop_requested_at` only, with a
  test that a turn marked by Pause alone is never settled `stopped_at_ceiling`
  and one marked by both is (the ceiling wins); tests for a stop whose session state reads `WAITING_FOR_INPUT` after the cancel
  and turn end settling first (`stopped_by_pause`, wakes `pending`), for a turn
  that finishes on its own after the pause mark with no `pause_cancel_at`
  (`completed`, wakes stay handled), for `ErrTurnNotActive` from the stop
  (`pause_cancel_at` cleared, nothing settled), for a reservation landing before
  `settlePausedUnsentTurn` (not settled), for `settleNotSent` still closing a
  reserved row as `send_failed` and `interrupted` so the next delivery is not
  blocked, for the Stopper and `recoverUnboundTurn` racing on one unsent row
  (one wins, the other no-ops, wakes `pending`), for a paused autonomy-off
  coordinator whose first Stop failed being retried by the backstop, and for a
  late accepted send on a paused coordinator (cancelled,
  `coordinator_pause_late_send_total`, ledger row an ordinary `message` turn).
- Stop by binding state (`pause_requested_at`, `settlePausedUnsentTurn` after 2
  minutes, cancel on accepted binding), the in-memory known-paused set for the
  flag-off read-error carve-out, and the read-only "Paused" badge shown with the
  flag off, per the [pause design](../../specs/coordinator/system-design/pause.md).

- Also in scope, from the pause design: the route's check order (404, 404, 403,
  400) and its response before `Stopper.Stop` finishes; the service-owned
  goroutine that runs `Stop` after a committed pause and the `OnAccepted` cancel;
  the re-read of the paused state before each cancel write; `ErrCancelInFlight`
  retry and per-row failure isolation; the atomic Resume guard in
  `settlePausedUnsentTurn`; the paused note placement in `TryAutomaticApproval`;
  the known-paused set refresh in the backstop pass; the strip absent with
  autonomy off.
- The dream half of `AC-COORDINATOR-PAUSE-002.2` is verified end to end by work
  order 04, which creates the dream; this work order ships the `Stopper`'s
  registration point for the dream canceller and tests it with a fake.

## Out of scope

- Pausing routines, scheduled pause, workspace-wide pause.

## ASCII UI preview

Screens changed: UI-31-03 of [the plan](plan.md#ascii-ui-previews).

```text
Needs you strip   Autonomy: Paused by Ada . 09:12 . 3 pending   [Resume]
                  Autonomy: Active . Last woke 12m ago . 3 pending [Pause]
Settings > Autonomy   Paused since 09:12 by Ada   [Resume]
  A paused coordinator keeps its queue. Turning autonomy off does not.
Phone: [ Pause ] full-width button at the end of the strip's second line.
```

## Acceptance

- Pause and Resume are idempotent; a reader and a coordinator principal get
  403; commit order decides two managers.
- While paused, no unattended turn starts, no wake is superseded or delivered,
  and an automatic-class proposal stays `pending` with the note and is not
  counted.
- A running unattended turn is stopped with `stopped_by_pause` and its wakes
  return to `pending`; a state read error is treated as paused.
- A manager's message still starts a turn; Resume delivers pending wakes
  through the ordinary admission and the cooldown.
- The strip shows Paused, Held and Active correctly, with a full-width 44 px
  control on a phone.

## Validation

- `make -C apps/backend test` for the touched packages, with `synctest` for any timer and no `time.Sleep`; store conformance on SQLite and PostgreSQL for each new table or column.
- `cd apps && pnpm --filter @kandev/web` typecheck, lint and Vitest for the touched modules, `cd apps/web && pnpm run i18n:check`, and the Playwright spec of this work order (plan verification strategy) on desktop and `mobile-chrome`, with the `auth` project for reader cases.
- `python3 scripts/list-docs.py validate` if a specification changes.

## Implementation record

### Changes

- Backend: new `coordinator/pause/` (Gate, Stopper, DreamStop ports), `store_pause.go`,
  `service_pause.go`, `pause_stop.go`; changes to the store, schema (`paused_at`,
  `paused_by`, `pause_requested_at`, `pause_cancel_at`), turns, turn end, delivery,
  wake backstop, automatic approval, DTO, autonomy read and routes
  (`PUT .../coordinators/:cid/pause`). `auth.Service.UserDisplayName` resolves the
  pausing manager's name.
- Web: `PauseControl`, `usePauseControl`, the fourth strip state, the Autonomy
  section block, the flag-off badge, the autonomy write-ordering token, copy in six
  locales plus pseudo.
- Tests: Go (Gate, Stopper, delivery, automatic approval, store, route, Postgres
  conformance, env-gated), Vitest, and Playwright (`pause.spec.ts` on chromium,
  `mobile-pause.spec.ts` on `mobile-chrome`).
- Docs: `docs/public/coordinator.md` Pause section and a `coordinator-pause` entry
  in `docs/public/coverage.json`.

### Assumptions and coordination

- Flag registration: `features.coordinatorPhase31` /
  `KANDEV_FEATURES_COORDINATOR_PHASE31` was absent on the base, so this work order
  registers it (off in every profile, effective only with coordinator, phase 2 and
  phase 3). The e2e specs enable it through `backend.useEnv`. Work order 01 or any
  earlier registration must be reconciled on merge into one registry entry.
- Dream seam (coordination with work order 04): this work order builds only the
  no-op dream-stop hook (`Service.SetDreamStop`, called by `Stopper.Stop`). Work order
  04 owns dream-episode cleanup, the dream tables and links, the restart-cleanup
  query, and the dream tick's `Gate.Active` call. It registers its canceller through
  `SetDreamStop` and verifies the dream half of AC-COORDINATOR-PAUSE-002.2 end to end.
- Spec review findings 28 and 29: deferred to work order 04.
- e2e reader case (403 for a reader): covered by the Go route tests and the Vitest
  reader test; the e2e fixtures have no reader principal for a coordinator workspace.

### Commands run

- `make fmt`; `make typecheck test lint`; `make lint-format`;
  `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`
- `go run ./cmd/sqlguard ./internal`; `python3 scripts/list-docs.py validate`;
  `node scripts/validate-public-docs.mjs`
- `cd apps/web && pnpm e2e:run --host -- e2e/tests/coordinator/pause.spec.ts` (3 passed);
  `pnpm e2e:run --host --no-build -- --project=mobile-chrome e2e/tests/coordinator/mobile-pause.spec.ts` (1 passed)
