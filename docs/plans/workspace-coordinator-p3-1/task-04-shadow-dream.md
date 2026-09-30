---
id: "04-shadow-dream"
title: "Shadow dream, report, ratings and Learning section"
status: draft
wave: 3
depends_on:
  - "01-turn-ledger"
  - "02-outcomes-overrides"
  - "03-replay-harness"
  - "05-pause"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-SHADOW-DREAM-001
  - REQ-COORDINATOR-SHADOW-DREAM-002
  - REQ-COORDINATOR-SHADOW-DREAM-003
  - REQ-COORDINATOR-SHADOW-DREAM-004
  - REQ-COORDINATOR-SHADOW-DREAM-005
  - REQ-COORDINATOR-SHADOW-DREAM-006
  - REQ-COORDINATOR-REPLAY-005
  - REQ-COORDINATOR-OUTCOMES-003
acceptance_criteria:
  - AC-COORDINATOR-SHADOW-DREAM-001.1
  - AC-COORDINATOR-SHADOW-DREAM-001.2
  - AC-COORDINATOR-SHADOW-DREAM-001.3
  - AC-COORDINATOR-SHADOW-DREAM-001.4
  - AC-COORDINATOR-SHADOW-DREAM-001.5
  - AC-COORDINATOR-SHADOW-DREAM-001.6
  - AC-COORDINATOR-SHADOW-DREAM-002.1
  - AC-COORDINATOR-SHADOW-DREAM-002.2
  - AC-COORDINATOR-SHADOW-DREAM-002.3
  - AC-COORDINATOR-SHADOW-DREAM-002.4
  - AC-COORDINATOR-SHADOW-DREAM-002.5
  - AC-COORDINATOR-SHADOW-DREAM-002.6
  - AC-COORDINATOR-SHADOW-DREAM-002.7
  - AC-COORDINATOR-SHADOW-DREAM-003.1
  - AC-COORDINATOR-SHADOW-DREAM-003.2
  - AC-COORDINATOR-SHADOW-DREAM-003.3
  - AC-COORDINATOR-SHADOW-DREAM-003.4
  - AC-COORDINATOR-SHADOW-DREAM-003.5
  - AC-COORDINATOR-SHADOW-DREAM-004.1
  - AC-COORDINATOR-SHADOW-DREAM-004.2
  - AC-COORDINATOR-SHADOW-DREAM-004.3
  - AC-COORDINATOR-SHADOW-DREAM-004.4
  - AC-COORDINATOR-SHADOW-DREAM-004.5
  - AC-COORDINATOR-SHADOW-DREAM-005.1
  - AC-COORDINATOR-SHADOW-DREAM-005.2
  - AC-COORDINATOR-SHADOW-DREAM-005.3
  - AC-COORDINATOR-SHADOW-DREAM-005.4
  - AC-COORDINATOR-SHADOW-DREAM-005.5
  - AC-COORDINATOR-SHADOW-DREAM-005.6
  - AC-COORDINATOR-SHADOW-DREAM-006.1
  - AC-COORDINATOR-SHADOW-DREAM-006.2
  - AC-COORDINATOR-SHADOW-DREAM-006.3
  - AC-COORDINATOR-SHADOW-DREAM-006.4
  - AC-COORDINATOR-REPLAY-005.4
  - AC-COORDINATOR-OUTCOMES-003.4
system_design:
  - ../../specs/coordinator/system-design/shadow-dream.md
  - ../../specs/coordinator/system-design/replay.md
  - ../../specs/coordinator/system-design/outcomes.md
---

# Task 04: Shadow dream, report, ratings and Learning section (WP 3.1-4)

## Summary

Runs the dream end to end in Shadow: scheduler tick and lease, the one-tool
episode, the report, the gate, replay of up to five items, ratings, health,
the routes and the Learning section with the measures and report views.

## In scope

- `internal/coordinator/dream/`: `Scheduler.Tick` (called from the wake
  backstop pass), the lease, `Evidence.Build`, the episode runner with the
  constant `dreamProfile`, `parse.Answer`, `gate.Check`, replay hookup,
  `Health`, and the `AgreementSource` implementation, with tests beside each.
- Store: `coordinator_dreams`, `coordinator_dream_items`,
  `coordinator_dream_ratings`, `shadow_dream_enabled` on `coordinators`;
  routes for learning, dreams, detail and rating, all behind the flag; the
  dream task (spend counts it, conversation binding and cleanup skip it) and
  the dream canceller registered with Pause's `Stopper`.
- The guard refuses every action but `list_coordinator_turns_kandev` from the
  episode session.
- Web: the Learning section (switch, health line, the five measures, report
  list), the report detail with rating control, states, phone layout, copy in
  six locales.

## Out of scope

- Any apply, approve or copy of an item (phase 3.5).
- A manual Dream now.

## ASCII UI preview

Screens changed: UI-31-01, UI-31-02 of [the plan](plan.md#ascii-ui-previews).

```text
+-------------------------------------------------------------------+
| Coordinator settings   Identity  May do  Watches  Autonomy  Learning|
|-------------------------------------------------------------------|
| Shadow dream  [ on ]   Shadow changes nothing.                     |
| Health: Fresh . last dream 6h ago         (Waiting: Turn autonomy on)|
| Measures (last 30 days)  [30 v]                                    |
|  Approved without edits   78%  (39 of 50)                          |
|  Override recurrence      Not enough data yet (no_data)            |
|  Dollars per merged task  4.10 USD (12 tasks)                      |
|  Median wait for you      2h 10m (50 proposals)                    |
|  Rating vs replay         Not enough data yet (too_few)            |
| Reports                                                            |
|  Sep 29  clean    30 turns   3 items   0.42 USD   [Open]           |
|  Sep 22  partial  22 turns   2 items   0.31 USD   [Open]           |
+-------------------------------------------------------------------+
Phone: one column of cards, 44 px touch targets.
```

```text
+-------------------------------------------------------------------+
| < Reports   Sep 29 . clean . window Sep 22 to Sep 29 . 30 turns     |
| Item 1  note_add   Gate: pass   Replay: unmeasured (14 held-out)    |
|  "Ask before proposing tasks on the Review column"                  |
|  Cited turns: T-118  T-131   Rating: (useful) (not useful) (harmful)|
| Item 2  context_diff  Gate: refused (thin_evidence)                 |
| Considered, not proposed   Reported by the agent, not verified      |
|  - Raise the ceiling                                                |
| Retire or supersede   (none this report)                            |
+-------------------------------------------------------------------+
Phone: full-screen page; rating is a segmented control.
```

## Acceptance

- Trigger matrix: each admission condition blocks the start and stores no row;
  two schedulers on PostgreSQL start one dream; an expired lease becomes
  `lease_lost` and a late completion changes nothing.
- The episode's session has exactly one tool and every registered action from
  it is refused; a dream turn is excluded from every window and count.
- Parse limits, gate order, replay cap of 5, statuses `ok`, `clean`,
  `partial`, `failed`; no item is written anywhere a turn reads.
- Rating upsert with the last commit winning and the newest rating per item;
  agreement counted only per `006.4`.
- Health at 36 and 72 hours with the fix text; the Learning section in loaded,
  empty, loading and failed states on desktop and 390 px, reader read-only.

## Validation

- `make -C apps/backend test` for the touched packages, with `synctest` for any timer and no `time.Sleep`; store conformance on SQLite and PostgreSQL for each new table or column.
- `cd apps && pnpm --filter @kandev/web` typecheck, lint and Vitest for the touched modules, `cd apps/web && pnpm run i18n:check`, and the Playwright spec of this work order (plan verification strategy) on desktop and `mobile-chrome`, with the `auth` project for reader cases.
- `python3 scripts/list-docs.py validate` if a specification changes.
