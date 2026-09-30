---
id: "05-pause"
title: "Pause"
status: draft
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
- Web: the fourth state of the autonomy strip and the Autonomy section
  control, phone layout, copy in six locales.
- Stop by binding state (`stop_requested_at`, `settleUnsentTurn` after 2
  minutes, cancel on accepted binding), the in-memory known-paused set for the
  flag-off read-error carve-out, and the read-only "Paused" badge shown with the
  flag off, per the [pause design](../../specs/coordinator/system-design/pause.md).

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
