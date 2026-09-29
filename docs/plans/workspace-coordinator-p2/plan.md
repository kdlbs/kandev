---
created: 2026-09-29
status: draft
requirements:
  - REQ-COORDINATOR-COORDINATORS-007
  - REQ-COORDINATOR-COORDINATORS-008
  - REQ-COORDINATOR-COORDINATORS-009
  - REQ-COORDINATOR-PERMISSIONS-001
  - REQ-COORDINATOR-PERMISSIONS-002
  - REQ-COORDINATOR-PERMISSIONS-003
  - REQ-COORDINATOR-PERMISSIONS-004
  - REQ-COORDINATOR-STANDING-ORDERS-001
  - REQ-COORDINATOR-STANDING-ORDERS-002
  - REQ-COORDINATOR-STANDING-ORDERS-003
  - REQ-COORDINATOR-STANDING-ORDERS-004
  - REQ-COORDINATOR-GOALS-001
  - REQ-COORDINATOR-GOALS-002
  - REQ-COORDINATOR-GOALS-003
  - REQ-COORDINATOR-ACTIVITY-LOG-001
  - REQ-COORDINATOR-ACTIVITY-LOG-002
  - REQ-COORDINATOR-ACTIVITY-LOG-003
  - REQ-COORDINATOR-ACTIVITY-LOG-004
  - REQ-COORDINATOR-ACTIVITY-LOG-005
  - REQ-COORDINATOR-PROPOSAL-KINDS-001
  - REQ-COORDINATOR-PROPOSAL-KINDS-002
  - REQ-COORDINATOR-PROPOSAL-KINDS-003
  - REQ-COORDINATOR-PROPOSAL-KINDS-004
  - REQ-COORDINATOR-PROPOSAL-KINDS-005
  - REQ-COORDINATOR-COPILOT-EVERYWHERE-001
  - REQ-COORDINATOR-COPILOT-EVERYWHERE-002
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/permissions.md
  - ../../specs/coordinator/system-design/standing-orders.md
  - ../../specs/coordinator/system-design/goals.md
  - ../../specs/coordinator/system-design/activity-log.md
  - ../../specs/coordinator/system-design/proposal-kinds.md
  - ../../specs/coordinator/system-design/copilot-everywhere.md
legacy_specs: []
---

# Implementation Plan: Workspace Coordinator, Phase 2 (Control)

## Overview

Phase 2 gives managers control over each coordinator while keeping phase 1's
two rules: a person approves every write, and nothing wakes the coordinator
unattended. It adds D17's per-action settings with the tool profile derived
from them, Watches, standing orders, a goal with baselines, the "What it
did" log with Undo, resume, message and move proposals, stall Resume and the
Ready to merge actions, guided setup, and the copilot on every workspace
page. Everything ships behind `features.coordinatorPhase2`, on top of
`features.coordinator`.

The decisions are in
[ADR-2026-09-29-coordinator-phase-2-control](../../decisions/2026-09-29-coordinator-phase-2-control.md)
(gate G2). The phase-1 package is
[workspace-coordinator](../workspace-coordinator/plan.md); this plan builds
on it and redraws nothing.

**What phase 3 reads.** Phase 3 (Autonomy) is designed in parallel. It relies
on the log table and summary (task 03), `coordinator.Service.Policy` and the
stored settings (tasks 01 and 02), and the per-kind `Execute` seam (task
04). This plan does not specify phase 3's wake, containment or cost ceiling.

## Order

Shared interface first, including the one log writer; then the backends in
parallel, each depending on task 01 alone where it can; then their screens.

**Preconditions from phase 1.** Phase 2 is built ahead of phase 1's merge,
from the phase-1 integration branch. Every work order waits for phase-1
[task 12, review follow-ups](../workspace-coordinator/task-12-review-follow-ups.md)
to pass review: task 01 follows its `config_revision` migration and
increments that column in `resetConversation`, task 02 derives the tool
list from a surface that task adds a tool to, and task 04 extends its
stale-claim sweep. Task 09 also waits for phase-1
[task 08](../workspace-coordinator/task-08-proposals-ui.md), whose proposal
card it extends. Task 10 waits for phase-1
[task 11, panel swap](../workspace-coordinator/task-11-panel-swap.md): it
adds `RightSidePanel` and its `mobileFullScreen` opt-in, which task 10
renders and which do not exist in the code before it.

```text
phase-1 task 12 ──> task-01 shared interface, log writer
                      ├──> task-02 policy, Watches enforcement ──┬──> task-04 proposal kinds backend ──> task-09 proposal kinds UI
                      ├──> task-03 activity log backend ─────────┤        (also after 05)                (also after 05)
                      │      ├──> task-08 What it did UI         └──> task-06 May do, Watches ──┐
                      │      └──> task-12 goals backend ──┐              (also after 11)        ├──> task-07 guided setup
                      ├──> task-05 standing orders ───────┴──> task-11 sections row, orders, goal ┘
                      │                                   └──> (also feeds task-04, task-09 above)
                      └──> task-10 copilot everywhere (also after phase-1 task 11)
```

| Work order | Package | Size | Depends on | Result |
| --- | --- | --- | --- | --- |
| [task-01](task-01-shared-interface.md) | WP-6/7 | M | phase-1 task 12 | Flag, tables, columns, types, route shapes, typed client, the log writer |
| [task-02](task-02-policy-enforcement.md) | WP-7 | L | 01 | Settings routes, derived and bound tool profile, guard, auto-approve, Watches filter |
| [task-03](task-03-activity-log-backend.md) | WP-6 | M | 01 | Rows written with every proposal change; list, summary, undo, read tool, retention |
| [task-05](task-05-standing-orders-backend.md) | WP-7 | M | 01 | Standing order routes, instructions, last applied |
| [task-10](task-10-copilot-everywhere.md) | WP-9 | M | 01; phase-1 task 11 | Launcher and panel on board, task page and Inbox; page chip |
| [task-04](task-04-proposal-kinds-backend.md) | WP-8 | L | 02, 03, 05 | Resume, message and move proposals with at-most-once execution |
| [task-08](task-08-what-it-did-ui.md) | WP-6 | M | 03 | What it did in the Queue with Undo |
| [task-12](task-12-goals-backend.md) | WP-10 | M | 01, 03 | Goal routes, instructions, baselines and measures |
| [task-11](task-11-orders-goal-sections.md) | WP-7, WP-10 | M | 05, 12 | Sections row, Standing orders and Goal sections, goal note |
| [task-09](task-09-proposal-kinds-ui.md) | WP-8 | M | 04, 05; phase-1 task 08 | Cards per kind, Shaped by, stall Resume, Ready to merge actions, Make it a standing order |
| [task-06](task-06-may-do-watches.md) | WP-7 | M | 02, 03, 11 | May do and Watches sections, list summary, Watches filtering in Needs you |
| [task-07](task-07-guided-setup.md) | WP-7 | M | 06, 11 | Guided Add coordinator with atomic setup route |

Sizes: S under 1 day, M 1 to 3 days, L 3 to 7 days. Every acceptance
criterion of the seven phase-2 requirement documents, and the phase-2
criteria added to [coordinators](../../specs/coordinator/requirements/coordinators.md),
is owned by exactly one work order's frontmatter. Where a criterion spans
surfaces, the owner's body names the work orders that build the others.
Phase 3 can start against task 01's shapes and task 03's summary.

**Sizing, from phase 1's record.** Every work order carries 5 to 17
acceptance criteria in one layer. Phase 1 measured the cost of each end:
its smallest card (2 criteria) still took 3 hours and 2 Build rounds, so a
card below 5 criteria is merged into a neighbour; its two cards with 30 and
41 criteria took 16 to 17 hours and up to 8 Build rounds, so no card here
exceeds 17. Its cards of 8 to 19 criteria in one layer took 6.5 to 9 hours
in 3 or 4 rounds. The rounds tracked ordering and concurrency rather than
criterion count: a 2-criterion recovery card and a 10-criterion proposal UI
each needed 5. Tasks 02 and 04 carry that class here (a policy tightened
inside a running conversation; at-most-once execution after a crash), so
each keeps a single such problem, and each states its interleaving test
before code.

## Backend

- Store (task 01): `coordinators.policy_json`, `policy_revision`,
  `watch_scope`; `coordinator_watches`; `coordinator_activity`;
  `coordinator_standing_orders`; `coordinator_goals`; proposal columns
  `kind`, `target_task_id`, `standing_order_ids`, `starts_agent`,
  `outcome_json` and the open-target partial index. Upgrade tests on both
  dialects from the phase-1 schema. `resetConversation`, shared by every
  change that must start the next conversation fresh.
- Log: the `activity.go` writer in the caller's transaction and refusal
  coalescing (task 01); hooks, list, summary, undo routes, `list_coordinator_activity_kandev`,
  daily retention.
- Policy (task 02): `policy.go`, `toolprofile.go`, `CoordinatorToolPolicy`
  in `internal/mcp/profile` with launch-metadata transport, the guard's
  bound-list and live-policy checks, the Watches filter, workflow-deletion
  subscriber and the approve re-check.
- Kinds (task 04): `kinds.go` executor registry, three propose actions and
  tools, `TaskMessenger` extracted from `handleMessageTask`, the
  at-most-once recovery branch, `starts_agent` creates.
- Standing orders (task 05): routes, prompt section, last applied.
- Goals (task 12): routes, prompt section, baselines, measures.
- Setup route (task 07); list summary and the Watches projection (task 06).

## Frontend

- Typed client and types (task 01).
- Coordinator page Sections row, Standing orders and Goal sections and the
  goal note (task 11); May do, Watches and list summary (task 06); guided
  setup (task 07).
- What it did (task 08); proposal cards per kind, stall Resume, Ready to
  merge actions and the standing-order offer (task 09).
- Workspace copilot host, switcher, one-right-panel store and page chip
  (task 10).

## ASCII UI previews

Structural choices (sections, control order, which actions exist, which
states render) are requirements; spacing and exact copy are illustrative.
All copy goes through `t()` in six locales. Components are Kandev's existing
primitives. Readers see every view without its write controls.

### UI-01: What it did (entry: Queue, below the groups)

Desktop. Scrolls with the Queue; the filter row sticks at the section top.

```text
What it did                                        Action class [All      v]
+------------+-----------------------------------+-------------+--------------------------+-------------------+
| When       | Action                            | Action class| How it was authorised    | Undo              |
+------------+-----------------------------------+-------------+--------------------------+-------------------+
| 2 min ago  | Created KAN-431 Retry webhooks    | Create task | Requires approval        | [Undo]            |
|            |                                   |             | Approved by Ana, with edits                  |
| 1 h ago    | Moved KAN-411 Build -> Review     | Move task   | Approved by Ana          | It has moved since|
| 3 h ago    | Messaged KAN-409                  | Message task| Approved by Ana          | No undo           |
| 5 h ago    | Refused: not allowed by May do    | Message task| Denied  x 3              |                   |
| yesterday  | Proposed resume of KAN-418        | Resume task | Requires approval        |                   |
| 2 days ago | Created KAN-377                   | Create task | Approved by Ana          | Undone by Ana, 1d |
+------------+-----------------------------------+-------------+--------------------------+-------------------+
                                  [Load more]
Empty: "It has not done anything yet."   Filtered empty: "Nothing matches this filter."
```

This sketch is non-normative for row wording: the Action cell shows the row's
detail with no invented verb, refused rows show the reason text of their code,
"It has moved since" is a temporary message under a still-clickable Undo, and
refused message or resume rows read "No undo". The copy table and acceptance
criteria of the activity-log system design win.

Phone: each row becomes a card (When and class on the first line, action,
authorisation, then Undo as a full-width button).

Criteria: `AC-COORDINATOR-ACTIVITY-LOG-002.*`, `003.1`, `003.6`.

### UI-02: Coordinator page sections and May do (entry: Settings, Coordinators, Configure)

```text
< All coordinators                                    Planner
[Identity] [Watches] [May do] [Standing orders] [Goal]
What this coordinator may ask for. Nothing runs until a manager approves.
  Create a task    ( ) Denied (*) Requires approval ( ) Automatic   12 approved, 2 rejected
  Start an agent   (*) Denied ( ) Requires approval ( ) Automatic   Nothing yet
  Message a task   ( ) Denied (*) Requires approval ( ) Automatic   3 approved, 0 rejected
  Move a task      ( ) Denied (*) Requires approval ( ) Automatic   Nothing yet
  Resume a task    ( ) Denied (*) Requires approval ( ) Automatic   1 approved, 1 rejected
  Stop a task      (*) Denied   Stopping is not available yet.
  Automatic arrives when the coordinator can act on its own.
  Merge a pull request   Always human
  Move a task to Done    Always human
  Review the last 30 days
Saving a change starts the next conversation fresh.          [Discard] [Save]
```

Automatic radios are disabled. Phone: the section tabs scroll horizontally;
each action is a stacked group with a segmented control.

Criteria: `AC-COORDINATOR-COORDINATORS-009.1`,
`AC-COORDINATOR-PERMISSIONS-001.6` to `001.9`, `004.3`.

### UI-03: Watches section

```text
[x] Watch every board, including new ones
-- switch off --
[ ] Watch every board, including new ones
  Product        In scope   [Take this board out of scope]
  Platform       Out        [Put this board in scope]
  Release        In scope   [Take this board out of scope]
Error (last board): "It must watch at least one board."
No board left after a deletion: "This coordinator watches no board." + Choose boards
```

Criteria: `AC-COORDINATOR-PERMISSIONS-003.6`, the notice of `003.5`.

### UI-04: Standing orders section and the reject offer

```text
Rules it follows in every conversation. They never grant a permission.
  Standing order 1   Prefer small cards.
                     Added 12 Sep . Last applied 2 h ago        [Retire this order]
  Standing order 2   Never propose work on the release board on Fridays.
                     Added 20 Sep . Never applied               [Retire this order]
  [Add standing order]
Empty: "No standing orders yet."
Toast: "Standing order 2 retired."  [Undo]            (10 s)
Toast after Reject with a reason:
  "Rejected. Keep the reason as a standing order?"  [Make it a standing order]  (10 s)
```

Criteria: `AC-COORDINATOR-STANDING-ORDERS-001.4` to `001.6`, `004.1`, `004.2`.

### UI-05: Goal section and the goal note

```text
Goal section
  Milestone [Ship the billing beta         ]  Due [2026-10-31]
  Exit criteria
    [x] Invoices render for all plans
    [ ] Stripe webhooks retried
    + Add criterion
  [Mark milestone met]
  Since this goal was set (20 Sep)
    Open tasks          23 -> 19   down
    Approved (7 days)   No baseline
    Rejected (7 days)   No baseline

Goal note, top of Needs you
  No goal:  "No goal is set, so this list is ordered by urgency alone." [Set a goal]
  Active:   "Ship the billing beta . Due 31 Oct . 1 of 2 criteria met"
  Met:      "Ship the billing beta was met on 28 Oct." [Set the next goal]
```

Criteria: `AC-COORDINATOR-GOALS-001.9`, `002.1` to `002.3`, `003.4`.

### UI-06: Guided setup (entry: + Add coordinator)

```text
Add coordinator
 (1) Who runs it  (2) What it watches  (3) What it is for  (4) What it knows
 (5) What it may do  (6) Review
------------------------------------------------------------------------------
 (6) Review: What it wrote
 +------------------+-------------------------------+---------------------+
 | Setting          | Value                         | Owned from now on by|
 +------------------+-------------------------------+---------------------+
 | Name             | Planner                       | Identity   [Change] |
 | Watches          | Product, Release              | Watches    [Change] |
 | Goal             | Ship the billing beta         | Goal       [Change] |
 | May do           | 4 actions require approval    | May do     [Change] |
 +------------------+-------------------------------+---------------------+
                                                   [Back]  [Finish]
Steps 3 and 4 show [Skip this step].
```

Phone: the step list collapses to "Step 3 of 6" with the step name.

Criteria: `AC-COORDINATOR-COORDINATORS-008.1` to `008.5`.

### UI-07: Proposal kinds, stall Resume, Ready to merge (Needs you and Queue)

```text
Resume KAN-418                        Policy: Resume a task requires approval
  Its agent stopped with no error 2 h ago.   Shaped by: Standing order 2
  [Approve] [Reject]
Message KAN-409                       Policy: Message a task requires approval
  > Please rebase on main before continuing.
  [Approve] [Edit] [Reject]
Move KAN-411 from Build to Review     Policy: Move a task requires approval
  Approving this starts an agent.
  [Approve] [Reject]
Failed:  "It may or may not have run; check the task."  [Approve] [Reject]

Stall card:     KAN-420 stopped 3 h ago ...          [Resume]  Ask about this
Ready to merge (Queue)   Merging a pull request is always human.
  KAN-402 Add SSO   PR #88 ready                     [Open the PR]
    Or send it back with a note                      [Send it back]
      [ note ....................................... ]  [Send]
```

Criteria: `AC-COORDINATOR-PROPOSAL-KINDS-004.*`, `005.*`,
`AC-COORDINATOR-STANDING-ORDERS-003.2`.

### UI-08: Copilot on every workspace page (entry: launcher, bottom right)

Desktop, task page with the panel open (board is the same with "This
board: Product"):

```text
+---------------------------------------------+------------------------------+
| KAN-419 Retry failed invoices               | Coordinator: Planner    [v] x|
| ...task page...                             | Open the coordinator page    |
|                                             | ...conversation...           |
|                                             | [This task: KAN-419  x]      |
|                                             |  (Not watched by this        |
|                                             |   coordinator)               |
|                                             | [ Ask something...     ] [>] |
+---------------------------------------------+------------------------------+
Closed: launcher (o) bottom right. Board: opening the panel closes the task preview.
```

Phone: the panel is a full-screen sheet with Close; the chip sits above the
composer. Criteria: `AC-COORDINATOR-COPILOT-EVERYWHERE-001.*`, `002.*`.

## Mockup screenshots

The phase 2 screenshots from the analysis mockup (`shots-v2.1`, views v21-07,
v21-08, v21-03 and v21-04) are in [`assets/`](assets/). They are the visual
reference; the criteria govern, and the prototype banner, demo controls and
`P1` to `P4` and `WC-` labels are mockup chrome. The mockup's "Expand into a
Quick Chat tab" (v21-05) is not built (D11).

| Screenshot | Shows | Cited by | Work orders |
| --- | --- | --- | --- |
| [`assets/p2-01-queue-what-it-did.png`](assets/p2-01-queue-what-it-did.png) | Queue with What it did and Ready to merge | activity-log, proposal-kinds | 08, 09 |
| [`assets/p2-02-settings-coordinator-sections.png`](assets/p2-02-settings-coordinator-sections.png) | coordinator page Sections row | permissions, standing-orders, coordinators | 06, 11 |
| [`assets/p2-03-task-page-copilot-context.png`](assets/p2-03-task-page-copilot-context.png) | task page, panel, task chip | copilot-everywhere | 10 |
| [`assets/p2-04-board-copilot-context.png`](assets/p2-04-board-copilot-context.png) | board, panel, board chip | copilot-everywhere | 10 |

The goal section, goal note and setup have no screenshot; UI-05 and UI-06
are their reference.

## Mockup scenario to repo test

| Mockup spec (phase 2 view) | Repo test | Work order |
| --- | --- | --- |
| `18-v21-copilot-anywhere` (launcher on board and task page, chip) | `tests/coordinator/copilot-everywhere.spec.ts` | 10 |
| `18-v21-copilot-anywhere` (settings sections) | `tests/coordinator/configure-sections.spec.ts` | 06, 11 |
| `14-first-run-setup` (coordinated part) | `tests/coordinator/guided-setup.spec.ts` | 07 |
| `05-rule-on-a-proposal` (reject with a reason) | `tests/coordinator/proposal-kinds.spec.ts`, the standing-order offer | 09 |

Other phase-2 surfaces have no mockup scenario; their repo tests are new.
Repo tests drive the mock agent with `e2e:` scripts and assert product
surfaces, not agent wording.

## Verification strategy

- Go tests beside the code in `internal/coordinator`, `internal/mcp`,
  `internal/agent/runtime/lifecycle`, `internal/orchestrator/executor` and
  `internal/task`; store and upgrade conformance on SQLite and PostgreSQL.
- The guard's table test (phase 1) extends to every phase-2 tool, every
  policy setting and every Watches case.
- Vitest for the typed client, stores (`use-activity`, `use-standing-orders`,
  copilot host store), the page-context derivation and `attention.ts`'s
  Watches filter.
- Playwright in `apps/web/e2e/tests/coordinator/`, with the `auth` project
  for reader cases and `mobile-chrome` for phone views, axe scans on each new
  section.
- Every work order that adds copy runs `cd apps/web && pnpm run i18n:check`.
- Public docs `docs/public/coordinator.md` gain the phase-2 controls through
  `/docs-maintainer` with task 06.

## Risks

| Risk | Mitigation |
| --- | --- |
| A loosened setting reaches a running conversation | Loosening needs a new conversation (the save archives it); the guard reads the live policy for tightening; task 02 tests both directions |
| A binding forged through task metadata | Create and update paths refuse the `kandev.coordinator_` prefix; launch metadata strips and re-derives the key (task 02) |
| A resume or message runs twice after a crash | Non-create kinds never re-run on a stale claim; `outcome_unknown` asks for a new approval (task 04) |
| Undo moves a task a person already moved | Move undo requires the task to sit on the destination step (task 03) |
| Phase 3 needs a column phase 2 did not add | The log columns and summary are frozen as the phase 3 contract; additions are additive |
| `requirements/copilot.md` and `system-design/copilot.md` are at their size limits | Phase 2 copilot changes live in the copilot-everywhere pair |

## Definition of done (phase 2)

Both flags are on in e2e; the tests above pass; task 02's tests prove the
tool profile is the policy's output, that a denied action is refused and
logged, and that Kandev auto-approves only the bound tools; task 04's tests
prove each kind runs once per approval; the flag-off suite still passes the
phase-1 tests unchanged; public docs are updated.
