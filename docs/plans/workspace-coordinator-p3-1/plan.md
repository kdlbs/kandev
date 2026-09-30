---
created: 2026-09-30
status: draft
requirements:
  - REQ-COORDINATOR-TURN-LEDGER-001
  - REQ-COORDINATOR-TURN-LEDGER-002
  - REQ-COORDINATOR-TURN-LEDGER-003
  - REQ-COORDINATOR-TURN-LEDGER-004
  - REQ-COORDINATOR-TURN-LEDGER-005
  - REQ-COORDINATOR-TURN-LEDGER-006
  - REQ-COORDINATOR-OUTCOMES-001
  - REQ-COORDINATOR-OUTCOMES-002
  - REQ-COORDINATOR-OUTCOMES-003
  - REQ-COORDINATOR-REPLAY-001
  - REQ-COORDINATOR-REPLAY-002
  - REQ-COORDINATOR-REPLAY-003
  - REQ-COORDINATOR-REPLAY-004
  - REQ-COORDINATOR-REPLAY-005
  - REQ-COORDINATOR-SHADOW-DREAM-001
  - REQ-COORDINATOR-SHADOW-DREAM-002
  - REQ-COORDINATOR-SHADOW-DREAM-003
  - REQ-COORDINATOR-SHADOW-DREAM-004
  - REQ-COORDINATOR-SHADOW-DREAM-005
  - REQ-COORDINATOR-SHADOW-DREAM-006
  - REQ-COORDINATOR-PAUSE-001
  - REQ-COORDINATOR-PAUSE-002
  - REQ-COORDINATOR-PAUSE-003
  - REQ-COORDINATOR-PERMISSIONS-005
system_design:
  - ../../specs/coordinator/system-design/turn-ledger.md
  - ../../specs/coordinator/system-design/outcomes.md
  - ../../specs/coordinator/system-design/replay.md
  - ../../specs/coordinator/system-design/shadow-dream.md
  - ../../specs/coordinator/system-design/pause.md
  - ../../specs/coordinator/system-design/watch-projects.md
legacy_specs: []
---

# Implementation Plan: Workspace Coordinator, Phase 3.1 (Record and measure)

## Overview

Phase 3.1 records and measures; it learns nothing and applies nothing. It adds a turn ledger with a stamp and a query tool, outcome grading and manager override capture, a replay harness whose regression guard and improvement judge are proven by a planted suite in CI, a shadow dream report with no write tools, a Pause control, and Watches by project (repository sets and repositories).

Recording (work orders 01 and 02) ships ahead of the flag. Everything that reads it, changes the tool profile, adds a route or a screen, or starts an episode sits behind `features.coordinatorPhase31` (effective only with `features.coordinator`, `coordinatorPhase2` and `coordinatorPhase3`), off in every shipped profile. The decisions, the wake-source amendment and the flag boundary are in [ADR-2026-09-30-coordinator-phase-3-1-record-and-measure](../../decisions/2026-09-30-coordinator-phase-3-1-record-and-measure.md). Source proposal: Part 1 of the post-phase-3 proposal (revision 2).

## Gate

Work orders 01, 02 and 06 need phase 3 merged (condition 1 of G3.1) and may start then. Work orders 03, 04 and 05 need all four conditions of [G3.1](../../decisions/2026-09-30-coordinator-phase-3-1-record-and-measure.md#g31-conditions).

## Order

```text
phase 3 --> 01 ledger --> 02 outcomes --> 03 replay --> 04 shadow dream
phase 3 --> 05 pause -------------------------------------^ (04 needs 05's gate)
phase 3 --> 06 projects   (anytime; before 01 if early, so the ledger records the watch set)
```

Critical path: 01, 02, 03, 04. Work orders 05 and 06 run in parallel with them. If 06 lands first, 01 records `project_scope` from the start; if 01 lands first, 06 moves the ledger snapshot and digest to `watch.Task`. Work orders touch disjoint files except `internal/backendapp/coordinator.go`, where 01 adds one named registration function per later work order, and the coordinator settings page, where 01 adds no section and 04 adds Learning.

**One PR** like phases 1 to 3: work orders are built and reviewed on their branches and merged into a phase 3.1 integration branch that ships as one pull request; work orders 01 and 02 may ship first as their own pull request: their recording half adds no visible behaviour and their reader half (tool, routes) is behind the flag.

| Work order | Package | Size | Depends on | Result |
| --- | --- | --- | --- | --- |
| [task-01](task-01-turn-ledger.md) | 3.1-1 | L | phase 3 merged | Ledger rows, stamp, call digest, board snapshot, links, tool, the flag |
| [task-02](task-02-outcomes-overrides.md) | 3.1-2 | M | 01 | Outcome rows, override observations, five measures |
| [task-03](task-03-replay-harness.md) | 3.1-3 | L | 01, 02 | Replay library, guard, judge, planted suite in CI |
| [task-04](task-04-shadow-dream.md) | 3.1-4 | L | 01, 02, 03, 05 | Shadow dream, report, ratings, health, Learning section |
| [task-05](task-05-pause.md) | 3.1-5 | S | phase 3 merged | Pause state, route, precondition, controls |
| [task-06](task-06-watch-projects.md) | 3.1-6 | M | phase 3 merged | Projects scope, filter, enforcement, settings and setup UI |

Sizes: S under 1 day, M 1 to 3 days, L 3 to 7 days. Every acceptance criterion of the new and amended requirement documents is owned by exactly one work order ([Traceability](#traceability)).

## Backend

- `internal/coordinator/ledger`, `outcomes`, `replay`, `dream`, `pause`, `watch` packages with tests beside the code.
- `internal/coordinator` store: ten additive tables and columns on `coordinators`, `coordinator_proposals`, `coordinator_activity` and `coordinator_unattended_turns` ([turn ledger](../../specs/coordinator/system-design/turn-ledger.md#tables)).
- The guarded-call layer (`ledger.Call`, `turn_id`), the wake backstop pass (dream tick, paused retry), wake delivery and `TryAutomaticApproval` (pause precondition), the repository and repository set deletion events (projects), the decision observer hook (outcomes), the spend reader's `ExtraSpend` term (replay).
- `internal/runtimeflags/registry.go` and root `profiles.yaml`: `features.coordinatorPhase31`.

## Frontend

- `apps/web/lib/api/domains/coordinator-api.ts`: pause, learning, measures, dreams, rating and the `projects` settings member.
- `apps/web/lib/coordinator/watch-filter.ts` and the attention module (`repositoryIds`).
- The Learning section and report detail, the Pause control (autonomy strip and Autonomy section), the Projects part of Watches, the guided-setup step and the copilot hint, all copy through `t()` in six locales; no em dash.

## ASCII UI previews

Structural choices are requirements; spacing and exact copy are illustrative. All views are behind `features.coordinatorPhase31`.

### UI-31-01: Learning section

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

### UI-31-02: Report detail

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

### UI-31-03: Pause control

```text
Needs you strip   Autonomy: Paused by Ada . 09:12 . 3 pending   [Resume]
                  Autonomy: Active . Last woke 12m ago . 3 pending [Pause]
Settings > Autonomy   Paused since 09:12 by Ada   [Resume]
  A paused coordinator keeps its queue. Turning autonomy off does not.
Phone: [ Pause ] full-width button at the end of the strip's second line.
```

### UI-31-04: Projects part of Watches

```text
Watches
  Boards: [ ] Watch every board, including new ones  (phase 2)
  Projects
   [ ] Watch every project, including new ones
       Include tasks with no repository [x]
       Sets:  Payments (repo-a, repo-b) In scope [Take this project out of scope]
              Mobile   (repo-c)         Out      [Put this project in scope]
       Repositories not in a set:  repo-d  Out  [Put this project in scope]
  Save is disabled: Keep at least one project in scope or include tasks with no repository.
```

## Traceability

| Criteria | Work order |
| --- | --- |
| `AC-COORDINATOR-TURN-LEDGER-001.1` to `7` (7); `AC-COORDINATOR-TURN-LEDGER-002.1` to `5` (5); `AC-COORDINATOR-TURN-LEDGER-003.1` to `3` (3); `AC-COORDINATOR-TURN-LEDGER-004.1` to `5` (5); `AC-COORDINATOR-TURN-LEDGER-005.1` to `4` (4); `AC-COORDINATOR-TURN-LEDGER-006.1` to `3` (3) | 01 |
| `AC-COORDINATOR-OUTCOMES-001.1` to `6` (6); `AC-COORDINATOR-OUTCOMES-002.1` to `6` (6); `AC-COORDINATOR-OUTCOMES-003.1` to `3` (3) | 02 |
| `AC-COORDINATOR-REPLAY-001.1` to `4` (4); `AC-COORDINATOR-REPLAY-002.1` to `6` (6); `AC-COORDINATOR-REPLAY-003.1` to `3` (3); `AC-COORDINATOR-REPLAY-004.1` to `4` (4); `AC-COORDINATOR-REPLAY-005.1` to `5` (4) | 03 |
| `AC-COORDINATOR-SHADOW-DREAM-001.1` to `6` (6); `AC-COORDINATOR-SHADOW-DREAM-002.1` to `7` (7); `AC-COORDINATOR-SHADOW-DREAM-003.1` to `5` (5); `AC-COORDINATOR-SHADOW-DREAM-004.1` to `5` (5); `AC-COORDINATOR-SHADOW-DREAM-005.1` to `6` (6); `AC-COORDINATOR-SHADOW-DREAM-006.1` to `4` (4); `AC-COORDINATOR-REPLAY-005.4`; `AC-COORDINATOR-OUTCOMES-003.4` | 04 |
| `AC-COORDINATOR-PAUSE-001.1` to `3` (3); `AC-COORDINATOR-PAUSE-002.1` to `7` (7); `AC-COORDINATOR-PAUSE-003.1` to `4` (4) | 05 |
| `AC-COORDINATOR-PERMISSIONS-005.1` to `9` (9) | 06 |

The phase 2 criteria that Projects extends (`AC-COORDINATOR-PERMISSIONS-003.x`, `004.x`) stay owned by their phase 2 work orders; work order 06 updates the tests that pin them.

## Verification strategy

- Go tests beside the code; store and upgrade conformance on SQLite and PostgreSQL for every new table and column, including an upgrade of a phase 3 database with rows; `synctest` for the lease, the 24-hour window and the daily jobs, never `time.Sleep`.
- Race tests: two recorders on one turn, two completions, two graders, two dream schedulers, Pause against a delivery, Pause against turn end.
- Import-boundary tests for `replay` and `dream` (no writer of proposals, activity, settings or conversation).
- The planted-regression suite in the ordinary backend run (`TestPlantedCandidates`).
- Vitest for `watch-filter.ts`, the API client and each new component's states.
- Playwright in `apps/web/e2e/tests/coordinator/`: `learning.spec.ts`, `pause.spec.ts`, `watch-projects.spec.ts`, with `mobile-chrome` 390 px checks and the `auth` project for reader cases; the e2e profile turns the flag on and the mock agent scripts a dream answer.
- `cd apps/web && pnpm run i18n:check` for every work order that adds copy.
- Public docs `docs/public/coordinator.md` gain Learning, Pause and Projects sections through `/docs-maintainer` with the last UI work order to merge.

## Risks

| Risk | Mitigation |
| --- | --- |
| Recording adds write load on every turn | One insert per turn and one per call, capped at 100 calls; asynchronous event subscription; failures swallowed |
| The dream's episode gains a write tool by a later change | The profile is a constant with a test that asserts its content and that every other action is refused |
| A planted candidate passes after a harness change | `TestPlantedCandidates` fails; the constants test shows an edited threshold |
| Too few decided turns for a meaningful replay | Items are `unmeasured`, never improvements; that is the designed result |
| Projects filter missed in one path | One function (`watch.Task`) and a test per enforcement path in the design table |

## Definition of done (phase 3.1)

The flag is on in e2e; every work order's checks pass; tests prove one ledger row per turn, one outcome row per proposal, one observation per override, a replay that is deterministic and blocks every planted regression, a dream episode that can call nothing but the ledger tool and writes nothing a turn reads, Pause stopping everything the coordinator does on its own while keeping its queue, and the projects filter in every path; with the flag off the coordinator behaves as phase 3 except for the recorded rows; public docs are updated.

## E2E decision input

User-visible surfaces touched: the Learning section and report detail, the Pause control on the autonomy strip and in settings, the Projects part of Watches, the guided-setup step and the copilot scope hint. Each has an e2e spec listed above.
