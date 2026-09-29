---
created: 2026-09-29
status: draft
requirements:
  - REQ-COORDINATOR-WAKE-001
  - REQ-COORDINATOR-WAKE-002
  - REQ-COORDINATOR-WAKE-003
  - REQ-COORDINATOR-WAKE-004
  - REQ-COORDINATOR-WAKE-005
  - REQ-COORDINATOR-WAKE-006
  - REQ-COORDINATOR-CONTAINMENT-001
  - REQ-COORDINATOR-CONTAINMENT-002
  - REQ-COORDINATOR-CONTAINMENT-003
  - REQ-COORDINATOR-SPEND-001
  - REQ-COORDINATOR-SPEND-002
  - REQ-COORDINATOR-SPEND-003
  - REQ-COORDINATOR-SPEND-004
  - REQ-COORDINATOR-RELAY-001
  - REQ-COORDINATOR-RELAY-002
  - REQ-COORDINATOR-RELAY-003
  - REQ-COORDINATOR-AUTOMATIC-001
  - REQ-COORDINATOR-AUTOMATIC-002
  - REQ-COORDINATOR-AUTOMATIC-003
  - REQ-COORDINATOR-AUTOMATIC-004
  - REQ-COORDINATOR-IMPROVEMENTS-001
  - REQ-COORDINATOR-IMPROVEMENTS-002
  - REQ-COORDINATOR-IMPROVEMENTS-003
  - REQ-COORDINATOR-INTEGRATION-001
  - REQ-COORDINATOR-INTEGRATION-002
  - REQ-COORDINATOR-INTEGRATION-003
  - REQ-COORDINATOR-INTEGRATION-004
  - REQ-COORDINATOR-INTEGRATION-005
  - REQ-COORDINATOR-INTEGRATION-006
  - REQ-COORDINATOR-INTEGRATION-007
  - REQ-COORDINATOR-INTEGRATION-008
system_design:
  - ../../specs/coordinator/system-design/wake.md
  - ../../specs/coordinator/system-design/containment.md
  - ../../specs/coordinator/system-design/spend.md
  - ../../specs/coordinator/system-design/relay.md
  - ../../specs/coordinator/system-design/automatic.md
  - ../../specs/coordinator/system-design/improvements.md
  - ../../specs/coordinator/system-design/integration.md
legacy_specs: []
---

# Implementation Plan: Workspace Coordinator, Phase 3 (Autonomy)

## Overview

Phase 3 lets a manager turn on autonomy for one coordinator. The coordinator
is then woken, once per episode, by questions, permissions, stalls, errors and
completions on the tasks it created, and runs an unattended turn in its one
existing conversation. A level-triggered backstop recovers dropped events,
busy never forks the conversation, a containment check and a cost ceiling
bound every unattended turn, and both fail closed. Managers answer questions
and permissions on the Needs you card, reply to a proposal with a condition,
raise `create_task` to `automatic` once a coordinator's log earns it, and
review improvement proposals that cite unattended runs.

Everything ships behind `features.coordinatorPhase3` (effective only with
`features.coordinator` and `features.coordinatorPhase2`,
[integration](../../specs/coordinator/requirements/integration.md)),
`prod: "false"`. The decisions and gate are in
[ADR-2026-09-29-coordinator-phase-3-autonomy](../../decisions/2026-09-29-coordinator-phase-3-autonomy.md);
the phase map is in
[ADR-2026-09-26-workspace-coordinator](../../decisions/2026-09-26-workspace-coordinator.md#phase-plan).
Phase 2 (control) is built; this plan consumes its D17 settings, "What it
did" log, watch set, tool list and proposal kinds as built, through the
adapters of
[automatic](../../specs/coordinator/system-design/automatic.md#phase-2-interfaces-consumed)
and the rulings of
[integration](../../specs/coordinator/system-design/integration.md), and does
not respecify them.

## Gate

No work order is built before G3 is met
([ADR G3 Status](../../decisions/2026-09-29-coordinator-phase-3-autonomy.md#g3-status)).
Task 09 additionally needs the ADR to record the 30-day log review that
confirms `create_task`; a different class re-plans task 09 only.

## Order

```text
G3 --> task-01 flag, schema, settings --+--> task-02 containment ----------+
                                        +--> task-03 spend ----------------+--> task-05 delivery, turn --> task-06 autonomy UI
                                        +--> task-04 recorder, backstop ---+                       |
                                        +--> task-07 answer in place     |                         v
                                        +--> task-08 reply with a condition -------------> task-10 improvements
                                        +--> task-09 automatic <---------+ (also needs 04; phase 2 merged + log review)
```

Critical path: G3, 01, 04, 05, 06. Tasks 02, 03, 04, 07 and 08 run in
parallel after 01, and task 09 after 01 and 04 (it fills the backstop's
lowering-retry hook). They touch disjoint files except
`internal/backendapp/coordinator.go`, where task 01 gives each later work
order its own named registration function (containment, spend, wake,
delivery, relay, reply, automatic, improvements), and the coordinator
settings page, where task 01 adds an empty Autonomy section with named slots.

**One PR**, as phase 1: work orders are built and reviewed on their branches
and merged into `coordinator/p3-integration`, which ships upstream as one pull
request once every work order passes.

| Work order | Package | Size | Depends on | Result |
| --- | --- | --- | --- | --- |
| [task-01](task-01-flag-schema-settings.md) | WP-11 | M | G3 | The flag, every phase 3 table and column, the autonomy and ceiling PATCH fields and their interlock; flag off 404s |
| [task-02](task-02-containment.md) | WP-11 | M | 01 | `containment.Check` with four fail-closed conditions; unattended permissions denied and counted |
| [task-03](task-03-spend.md) | WP-12 | M | 01 | Spend measured from the usage ledger; admission checks 3 and 4; the usage observer stops an open turn at the ceiling |
| [task-04](task-04-wake-recorder-backstop.md) | WP-11 | L | 01 | Wakes recorded once per episode from events and a 60-second backstop |
| [task-05](task-05-delivery-unattended-turn.md) | WP-11 | L | 02, 03, 04 | Admission, delivery, one open unattended turn, turn end and restart recovery; no fork |
| [task-06](task-06-autonomy-ui.md) | WP-11 | M | 05 | Autonomy strip, held item, settings Autonomy section, "Woken by" transcript entry |
| [task-07](task-07-answer-in-place.md) | WP-11 | M | 01 | Questions and permissions answered on the Needs you card |
| [task-08](task-08-reply-with-condition.md) | WP-11 | M | 01 | Reply with a condition, delivery to the conversation, revised proposals |
| [task-09](task-09-automatic.md) | WP-11 | L | 01, 04, phase 2 | `create_task` raisable to automatic under the eligibility gate; automatic approval; lowering on undo |
| [task-10](task-10-improvements.md) | WP-12 | M | 06, 08 | Improvement proposals, the card, pending changes applied in settings |

Sizes: S under 1 day, M 1 to 3 days, L 3 to 7 days. Every acceptance
criterion of the seven requirement documents is owned by exactly one work
order's frontmatter ([Traceability](#traceability)).

## Backend

- `internal/coordinator`: `wake_store.go`, `wake_recorder.go`,
  `wake_backstop.go`, `admission.go`, `delivery.go`, `turns.go`,
  `containment.go`, `spend.go`, `relay_handlers.go`, `reply.go`,
  `automatic.go`, `phase2.go` (adapters), `improvements.go`,
  `pending_changes.go`, with tests beside each.
- `internal/task/usage`: the optional `OnRecorded` observer (task 03).
- `internal/orchestrator`: an optional `UnattendedPermissionHandler` called
  from `handlePermissionRequest` beside `failAutomationRunOnPermission`, for
  requests agentctl's exact-name auto-approve did not grant (task 02).
- `internal/mcp/handlers`: `propose_improvement_kandev` and `in_reply_to` on
  `propose_task_kandev`; the guard's refused routes gain the phase 3 write
  routes (tasks 08, 10).
- Phase 2 touch points, all additive and listed with owners in
  [integration](../../specs/coordinator/system-design/integration.md#phase-2-touch-points):
  `coordinator_activity.unattended_turn_id`, the `returned`, `automatic` and
  `improvement` values, `coordinator_class_changes`, `Validate(p, phase3)`,
  the `SaveSettings` change hook, `LowerClass`, the post-commit `OnUndo`, the
  `returned` proposal status, the improvement `KindExecutor` and the
  `phase3` input of `ToolNames`.
- `internal/runtimeflags/registry.go` and root `profiles.yaml`: the flag
  (task 01), through `/runtime-feature-flags`.

## Frontend

- `apps/web/lib/api/domains/coordinator-api.ts`: autonomy, relay, reply,
  eligibility, reviews, runs and pending-change clients (each with its work
  order).
- `apps/web/app/coordinator/`: the autonomy input and strip, the held item in
  `lib/coordinator/attention.ts`, `question-answer.tsx`,
  `permission-answer.tsx`, the reply control and `improvement-card.tsx`.
- `apps/web/lib/permissions/respond.ts`: the permission response builder
  extracted from `use-permission-handlers.ts` (task 07).
- `apps/web/app/settings/workspace/[id]/coordinators/[coordinatorId]/`: the
  Autonomy section and "Changes waiting for you".
- Copy through `t()` in six locales; no em dash.

## ASCII UI previews

Structural choices are requirements; spacing and exact copy are
illustrative. Components are Kandev's existing primitives. All views are
behind `features.coordinatorPhase3`.

### UI-01: Autonomy strip (Needs you, autonomy on)

Desktop. The autonomy strip sits above the count strip; both are fixed above
the scrolling list.

```text
+------------+-------------------------------------------------------------------+
| Kandev     | Coordinator / Needs you                   [Configure]   Live      |
|            | Autonomy: Active . Last woke 12m ago . 3 pending                   |
| Home       |   Spend 4.20 of 10.00 USD in 24 h  [Inside]                        |
| Inbox    2 +--------------+-----------+-----------+------------------+         |
| Planner  4 | 4 Needs you  | 9 Working | 3 In rev. | 1 Ready to merge |         |
|            +--------------+-----------+-----------+------------------+         |
|            | (items as phase 1)                                                |
+------------+-------------------------------------------------------------------+

Held:        Autonomy: Held (Cost ceiling reached) . Last woke 2h ago . 5 pending
               Spend 10.40 of 10.00 USD in 24 h  [Over]
Unmeasured:  Autonomy: Held (Spend cannot be measured) ...
               Spend unknown: some usage is unpriced
Read error:  Autonomy state unavailable  [Try again]
```

Phone at 390px. The strip wraps into two lines above the sticky count strip.

```text
+--------------------------------+
| [=] Coordinator: Planner   [v] |
| Autonomy: Active . 3 pending   |
| Spend 4.20 / 10.00 USD [Inside]|
| 4 Needs you | 9 Working        |  count strip (sticky)
|--------------------------------|
| ...                            |
+--------------------------------+
```

Criteria: `AC-COORDINATOR-WAKE-006.1`, `006.4`,
`AC-COORDINATOR-SPEND-004.1`, `004.3`.

### UI-02: Held item and the "Woken by" entry

The held item is an ordinary Needs you card of kind `autonomy`, ordered after
error items of the same age.

```text
+----------------------------------------------------------------+
| Autonomy held  [Needs you]  1h 03m                             |
| Containment not in place: executor_isolated                     |
| Why it is here   5 events are waiting for the coordinator       |
| What clears it   Choose a Docker, remote Docker, Sprites or     |
|                  Kubernetes executor profile.                   |
| [Open settings]                                                 |
+----------------------------------------------------------------+
```

In the copilot transcript, an unattended turn's message:

```text
| (bolt) Woken by 3 events                           09:12     |
|   > KAN-418  question   KAN-421  stall   KAN-430  completed  |
|   1 permission denied (nobody to ask)                        |
```

Phone: the card stacks as phase 1 cards; **Open settings** is a full-width
44px target. The transcript entry is unchanged at 390px, the list wrapping.

Criteria: `AC-COORDINATOR-WAKE-006.2`, `006.3`, `005.5`,
`AC-COORDINATOR-CONTAINMENT-003.3`.

### UI-03: Answering in place (question and permission)

```text
+----------------------------------------------------------------+
| KAN-418  Build  [Decide now]  12m                [Ask about this] |
| The agent is waiting for your answer                             |
| [Answer here] [Open task]                                        |
+----------------------------------------------------------------+
expanded question (the Inbox's clarification component):
| Which database should the migration target?                      |
|  (1) Postgres 16   (2) SQLite   ( ) Other: [____________]        |
| [Send answer]                                        [Collapse]  |
expanded permission:
| Run: git push origin feature/x                                    |
| [Allow once] [Allow always] [Reject]                              |
failure: answer kept, "Could not send. [Try again]"; resolved elsewhere:
collapse and toast "This question is no longer waiting"
```

Phone: the expanded body takes the full card width with a 50vh scroll cap;
option buttons stack one per row. Readers see no **Answer here**.

Criteria: `AC-COORDINATOR-RELAY-001.*`, `002.*`.

### UI-04: Settings, Autonomy section

```text
Settings > Workspaces > Software Factory > Coordinators > Planner
...
Autonomy
  [ ] Let Planner act on its own tasks while nobody is watching
      (refused until a ceiling is set)
  Cost ceiling (USD per 24 hours)  [ 10.00 ]   [Save]
  Spend 4.20 of 10.00 USD in 24 h [Inside] . 7-day daily mean 3.10 USD
  Last unattended turn 0.42 USD
  Containment                                         [Check again]
    Met      Isolated executor (local_docker)
    Not met  Authentication on
             Turn on Kandev authentication (features.auth) and finish setup.
    Met      No Kandev tokens in the environment
    Met      No extra MCP servers
Changes waiting for you (task 10)
```

Phone: one column; the ceiling field and Save stack; each condition row is
two lines.

Criteria: `AC-COORDINATOR-WAKE-004.1`, `AC-COORDINATOR-SPEND-001.*`,
`004.2`, `AC-COORDINATOR-CONTAINMENT-002.3`.

### UI-05: Reply with a condition

```text
! create_task  Pending Approval
  <title, workflow, step>
  [Approve] [Edit] [Reject] [Reply with a condition]
expanded:
  Reply with a condition
  [ Only if it stays under 200 lines; split the migration.   ]  48/2000
  [Send reply] [Cancel]
returned:        Returned with your condition: Only if ...
not delivered:   Reply saved, not delivered  [Send again]
revised card:    Revised after your reply: "Only if ..."
```

Phone: actions wrap two per row; the textarea is full width.

Criteria: `AC-COORDINATOR-RELAY-003.*`.

### UI-06: Raising `create_task` to automatic (phase 2 permission settings)

```text
May do
  Create a card    ( ) Never  (o) Requires approval  ( ) Automatic (disabled)
    Eligibility
      Met      30 days of history (since 2026-11-02)
      Met      20 or more decisions (34)
      Not met  90% approved without edits (85%)
      Met      Nothing undone (0)
      Not met  Reviewed in the last 7 days
    [Review the last 30 days]  [Mark as reviewed]
  Merge            Automatic (disabled)  Cannot be raised
  Move to Done     Automatic (disabled)  Cannot be raised
eligible:          Automatic option enabled; choose it, then Save
after a raise:     Raised to automatic by you at 10:04
lower:             choose Requires approval, then Save
```

Phone: one column; the two buttons stack.

Criteria: `AC-COORDINATOR-AUTOMATIC-001.2`, `002.3`,
`AC-COORDINATOR-INTEGRATION-007.2`.

### UI-07: Improvement card

```text
+----------------------------------------------------------------+
| Improvement  [Changes coordinator context]  20m                 |
| Ask before proposing tasks on the Review column                 |
| Rationale: three proposals this week were rejected because ...  |
| Runs behind it                                                  |
|   09:12  completed  0.42 USD                                    |
|   Run record expired                                            |
|   KAN-418  Split the migration                                  |
| [Show the change]                                               |
|   - Propose tasks for any column.                               |
|   + Propose tasks for Build only; ask about Review.             |
| [Approve as a reviewable change] [Reject] [Reply with a condition] |
+----------------------------------------------------------------+
approved: Approved as a reviewable change. Nothing was applied. [Open settings]
settings: Changes waiting for you . <diff> . [Apply] [Discard]
conflict: The context changed since this was proposed. Discard it, or ask
          the coordinator to propose again.
```

Phone: the diff scrolls horizontally inside the card; buttons stack.

Criteria: `AC-COORDINATOR-IMPROVEMENTS-002.*`, `003.1`, `003.2`.

### UI-08: Sections row and log rows

```text
Sections: Identity | Watches | May do | Standing orders | Goal | Autonomy
          (Autonomy is present only while phase 3 is effective)

What it did
  10:04  create_task  Proposed  KAN-431 Split the migration
         During an unattended turn
  10:04  create_task  Approved automatically, raised by Dana   Undo
         During an unattended turn
  10:09  create_task  Returned by Dana, with a condition
         Condition: Only if it stays under 200 lines
  10:12  improvement  Proposed  Ask before proposing on Review   No undo
```

Phone: each row is a card; the unattended mark is a second muted line.

Criteria: `AC-COORDINATOR-INTEGRATION-004.1` to `004.3`, `007.1`.

## Traceability

| Criteria | Work order |
| --- | --- |
| `AC-COORDINATOR-WAKE-004.1`, `004.3`, `004.4`; `AC-COORDINATOR-SPEND-001.1`, `001.2`, `001.3` | 01 |
| `AC-COORDINATOR-CONTAINMENT-001.1`, `001.2`, `001.3`, `002.1`, `002.2`, `003.1`, `003.2` | 02 |
| `AC-COORDINATOR-SPEND-002.1`, `002.2`, `002.3`, `003.1`, `003.2`, `003.3` | 03 |
| `AC-COORDINATOR-WAKE-001.1` to `001.4`, `002.1` to `002.4` | 04 |
| `AC-COORDINATOR-WAKE-003.1` to `003.3`, `004.2`, `005.1` to `005.4`, `005.6` | 05 |
| `AC-COORDINATOR-WAKE-005.5`, `006.1` to `006.4`; `AC-COORDINATOR-SPEND-004.1` to `004.3`; `AC-COORDINATOR-CONTAINMENT-002.3`, `003.3` | 06 |
| `AC-COORDINATOR-RELAY-001.1` to `001.5`, `002.1` to `002.4` | 07 |
| `AC-COORDINATOR-RELAY-003.1` to `003.5` | 08 |
| `AC-COORDINATOR-AUTOMATIC-001.1` to `004.3` (all 13) | 09 |
| `AC-COORDINATOR-IMPROVEMENTS-001.1` to `003.3` (all 9) | 10 |
| `AC-COORDINATOR-INTEGRATION-001.1` | 01 |
| `AC-COORDINATOR-INTEGRATION-002.1`, `002.3` | 04 |
| `AC-COORDINATOR-INTEGRATION-002.2`, `002.4`, `003.1`, `003.2`, `004.1`, `005.1`, `005.2`, `005.3` | 05 |
| `AC-COORDINATOR-INTEGRATION-007.1`, `008.1` | 06 |
| `AC-COORDINATOR-INTEGRATION-004.2`, `006.1`, `006.2`, `006.3`; the rendering half of `004.1` (the mark; 05 stamps the turn id) | 08 |
| `AC-COORDINATOR-INTEGRATION-004.3`, `007.2` | 09 |
| `AC-COORDINATOR-INTEGRATION-003.3`, `006.4`, `006.5` | 10 |

The amended phase 1 criteria (`AC-COORDINATOR-COPILOT-002.1`,
`AC-COORDINATOR-NEEDS-YOU-002.4`, `002.5`) stay owned by their phase 1 work
orders; tasks 05, 06 and 07 update the phase 1 tests that pin them.

## Verification strategy

- Go tests beside the code; store and upgrade conformance on SQLite and
  PostgreSQL for every new table and column; `synctest` for the backstop
  ticker, cooldown and 24-hour windows, never `time.Sleep`.
- Race tests: two deliveries, delivery against autonomy off, ceiling stop
  against turn end, reply against approve, apply against discard.
- Vitest for `attention.ts`, the API client, `respond.ts`, and each new
  component's states.
- Playwright in `apps/web/e2e/tests/coordinator/`: `autonomy.spec.ts`,
  `answer-in-place.spec.ts`, `reply.spec.ts`, `automatic.spec.ts`,
  `improvements.spec.ts`, with `mobile-chrome` 390px checks and the `auth`
  project for reader cases. The e2e profile turns the flag on and allows
  `mock_remote` as isolated; tests drive the mock agent with `e2e:` scripts.
- `cd apps/web && pnpm run i18n:check` for every work order that adds copy.
- Public docs `docs/public/coordinator.md` gain an Autonomy section through
  `/docs-maintainer` with task 06, stating what containment does not cover.

## Risks

| Risk | Mitigation |
| --- | --- |
| Phase 2's settings or log differ from the interfaces task 09 assumes | Built phase 2 was checked: the adapter mapping and the capabilities phase 3 adds (class change history, change hook, `LowerClass`, `OnUndo`, `automatic` authorization) are in [automatic](../../specs/coordinator/system-design/automatic.md#phase-2-interfaces-consumed); a further mismatch re-plans task 09 only |
| The log review shows `create_task` is not the right first class | The ADR's evidence gate; task 09 alone re-plans |
| A usage row lands after the ceiling is crossed | Documented overshoot of one report, or one backstop period on a dropped call or failed cancel; backstop re-check every 60 s |
| Containment passes on an executor that is not isolated in practice (for example a Docker host that is the backend host with the socket mounted) | Residual recorded in the ADR; the fix text names the conditions checked, not a guarantee |
| Delivery races a manager's message into a busy session | Admission check 7 reads queued messages and the open turn; the partial unique index holds one open turn |
| The automation runner's fork is reused by mistake | Task 05 adds a test that no delivery path calls the automation or Office run services |

## Definition of done (phase 3)

The flag is on in e2e; every work order's checks pass; tests prove one wake
per episode across event, restart and backstop, one open unattended turn per
coordinator, no conversation created or repointed by any wake path,
containment and spend failing closed, attended turns unaffected by either,
and every phase 3 write refused to a coordinator principal; public docs are
updated.
