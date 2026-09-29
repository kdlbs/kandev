---
id: coordinator-integration-design
title: Phase 3 on phase 2 as built design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-INTEGRATION-001
  - REQ-COORDINATOR-INTEGRATION-002
  - REQ-COORDINATOR-INTEGRATION-003
  - REQ-COORDINATOR-INTEGRATION-004
  - REQ-COORDINATOR-INTEGRATION-005
  - REQ-COORDINATOR-INTEGRATION-006
  - REQ-COORDINATOR-INTEGRATION-007
  - REQ-COORDINATOR-INTEGRATION-008
---

# Phase 3 on phase 2 as built System Design

## Purpose and boundaries

This design fixes how the phase 3 designs ([wake](wake.md),
[containment](containment.md), [relay](relay.md), [automatic](automatic.md),
[improvements](improvements.md)) meet the phase 2 code that is now built. It
adds no new mechanism. Where a phase 3 design and this one differ, this one
wins, and the other design carries a pointer. Each ruling reuses the phase 2
read, writer or registry that already exists; the only additions to phase 2
code are the additive columns, values and call sites listed in
[Phase 2 touch points](#phase-2-touch-points). Phase 2's own documents are not
changed.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-INTEGRATION-001` | [Effective condition](#effective-condition) |
| `REQ-COORDINATOR-INTEGRATION-002` | [Watch set](#watch-set) |
| `REQ-COORDINATOR-INTEGRATION-003` | [Tool list](#tool-list) |
| `REQ-COORDINATOR-INTEGRATION-004` | [Log rows](#log-rows) |
| `REQ-COORDINATOR-INTEGRATION-005` | [Instructions, orders and goal](#instructions-orders-and-goal) |
| `REQ-COORDINATOR-INTEGRATION-006` | [Proposal statuses and kinds](#proposal-statuses-and-kinds) |
| `REQ-COORDINATOR-INTEGRATION-007` | [Settings layout](#settings-layout) |
| `REQ-COORDINATOR-INTEGRATION-008` | [Copilot everywhere](#copilot-everywhere) |

## Effective condition

Phase 3 is effective only while `features.coordinator`,
`features.coordinatorPhase2` and `features.coordinatorPhase3` are all on.
`internal/backendapp/coordinator.go` computes it once from the same
`cfg.Features` values it already passes to `initCoordinatorWiring`. This
replaces the two-flag rule of [wake](wake.md#flag-and-settings) and the ADR.
The reason is in phase 2's code: with phase 2 off, `Service.Record` writes no
activity row, `Service.Policy` reports the fixed phase 1 policy with scope
`all`, and `kindFilter` hides every proposal kind but `create_task`, so an
unattended row, a watch filter and an improvement could not exist.

**Registration seam (task 01).** Each phase 3 work order attaches to the
backend through one named function in `internal/backendapp/coordinator.go`,
declared by task 01 with an empty body. All eight have the one signature of
phase 2's `registerCoordinatorSubscribers`:
`func(router *gin.Engine, eventBus bus.EventBus, svc *coordinator.Service, log
*logger.Logger) func(context.Context, time.Time)`, named
`registerCoordinatorContainment`, `...Spend`, `...Wake`, `...Delivery`,
`...Relay`, `...Reply`, `...Automatic`, `...Improvements`. A function whose
work order adds a route registers it on `router` inside its body, the way
`registerCoordinatorConversation` does; a function with nothing to subscribe
or start returns a no-op hook. A work order that needs a further dependency
adds a `Service` setter called from its body, never a parameter. The wiring
appends the eight returned hooks, in the order listed, after phase 2's hooks
in the `hooks` slice of `registerCoordinatorRoutes`, only when the value
computed once as `phase3Effective` is true; it is false whenever any of the
three flags is off, and the eight functions are then never called, which is
what makes every phase 3 subscriber, ticker and route absent.
`registerCoordinatorWake` returns a hook whose first step is
`Store.PruneWakeState(ctx, t0)` ([wake](wake.md#store)); task 01 declares the
function with that hook body, so the prune is wired in task 01 and task 04
adds its route and subscribers to the same function without moving it.

**What task 01 observes.** The acceptance clauses of
`AC-COORDINATOR-WAKE-004.4` and `AC-COORDINATOR-INTEGRATION-001.1` are
observed in layers, each by the work order that adds the thing:

| Clause | Observed by |
| --- | --- |
| `phase3Effective` is true only for all three flags on (eight combinations) | task 01, a table test over `coordinator.go`'s computation |
| the eight registration functions are called only when effective | task 01, a test that swaps them for counters |
| PATCH fields ignored, GET/list keys omitted, Autonomy section hidden while not effective | task 01 |
| stored phase 3 rows survive an off/on cycle | task 01, a store test that writes rows, restarts the wiring with the flag off, then on |
| no wake stored, no backstop, no turn started | tasks 04 and 05, each in its own tests, because each adds that behaviour; nothing exists in task 01 to assert |
| every phase 3 route is 404 | each task that adds a route, in its own route test |

## Autonomy PATCH and deletion

Task 01's contract for the settings fields of [wake](wake.md#flag-and-settings) and for phase 3 rows.

**PATCH field rules (task 01).**

- Order of checks, as phase 2 does it: `httpPatchCoordinator` binds the JSON
  body first (malformed JSON is 400 for any caller, unchanged), then the
  service's scope check runs (a reader gets 403), then the two fields are
  validated. The two keys are bound as raw JSON values (`json.RawMessage`),
  never as typed fields, so a wrongly typed `autonomy_enabled` or
  `cost_ceiling_usd` cannot fail binding; while phase 3 is not effective both
  keys are dropped unread, so an invalid `cost_ceiling_usd` on an ineffective
  install returns 200 and stores nothing. A coordinator principal is refused
  by the MCP guard before any of this, with the refusal the guard gives every
  non-allowlisted action (no activity row, no new refusal shape).
- Presence: a key absent from the body leaves its column unchanged.
  `cost_ceiling_usd: null` clears the ceiling. `autonomy_enabled` accepts
  JSON `true` or `false` only; `null`, a string, a number or any other value
  is 400 naming `autonomy_enabled`, and nothing in the body is applied
  (the whole PATCH is one transaction, so every field fails together).
- A PATCH that sets a field to the value it already holds is a success, writes
  the same row and publishes nothing (phase 2's PATCH publishes no event, and
  task 01 adds none for an unchanged value); only the change-only publish
  below exists.
- An autonomy-off PATCH runs the supersede statement even when the row already
  reads off, because a wake can return to `pending` after autonomy went off
  (delivery `send_failed` or `interrupted`, see [Delivery](wake.md#delivery)) and no
  other writer supersedes it. The statement is idempotent: it matches zero
  rows when none are pending.
- The PATCH takes the [wake lock](wake.md#wake-lock) only when the body carries
  `autonomy_enabled: false`; every other PATCH keeps phase 2's transaction
  unchanged.
- After the PATCH commits, and only when the resulting `autonomy_enabled` or
  `cost_ceiling_subcents` differs from the row it read, the service publishes
  `coordinator.updated` with `autonomy_changed: true` and calls
  `Kick(coordinatorID)`. Both are post-commit and best effort: a failure is
  logged at warn and never changes the PATCH result. `Service` holds
  `kick func(ctx context.Context, coordinatorID string) error` (nil means no
  call), set by task 05 through `SetKick` (task 04 only calls it); task 01 declares the field and
  setter and no interface. A returned error and a panic (recovered in the
  PATCH path) both count as a failure. The `coordinator.updated`
  `autonomy_changed` field is added to the payload type in task 01 and
  published by this PATCH only (change-only); the other publish sites of
  [Autonomy read](wake.md#autonomy-read) belong to task 04 (wake insert) and
  task 05 (delivery, turn settle).

**Deletion of every phase 3 table.** Only `coordinator_wakes`
carries `workspace_id`; the other phase 3 tables carry `coordinator_id` or
only a parent id and gain no `workspace_id` column. Deletion therefore keys on
the coordinator, always (the tables exist and are deleted from whether or not
phase 3 is effective, so a coordinator deleted while the flag is off leaves no
orphan):

1. `coordinator_unattended_denials WHERE turn_id IN (SELECT id FROM
   coordinator_unattended_turns WHERE coordinator_id = ?)`, first, because
   denials have no `coordinator_id` and hang off their turn;
2. `coordinator_unattended_turns`, `coordinator_wakes`,
   `coordinator_class_changes`, `coordinator_class_reviews` and
   `coordinator_pending_changes`, each `WHERE coordinator_id = ?`;
3. then phase 2's existing `coordinatorOwnedTables` loop and the coordinator
   row.

`DeleteCoordinator` runs steps 1 and 2 inside its existing transaction
before that loop. `DeleteWorkspaceState` runs them once per workspace with
`coordinator_id IN (SELECT id FROM coordinators WHERE workspace_id = ?)`
(denials through the turn subquery of that set) before it deletes the
coordinators. `Store.PruneWakeState(ctx, now)` (task 01) is the retention pass. In one
transaction it deletes, in this order: the denial rows of turns with
`finished_at` older than 90 days, those turn rows, then wake rows with status
`delivered` or `superseded`, `updated_at` older than 30 days and whose task cannot
become an own task again ([wake](wake-recording.md#retention-and-own-tasks)). A wake with
status `pending`, and a turn with `finished_at` null (open), are
never deleted. It is idempotent (a second run at the same `now` deletes
nothing) and returns the two deleted counts. Its caller is the first step of
the hook `registerCoordinatorWake` returns, run once at startup and only
while phase 3 is effective; an error is logged at warn and never blocks
startup.

## Watch set

One read, `Service.EffectiveWatchSet(ctx, coordinatorID)`, is the only watch
filter. It returns the coordinator's Watches with deleted workflows omitted,
and `WatchSet.Contains(workflowID)` is false for the empty id, so a task with
no workflow is never watched. The MCP guard, `list_workflows` and
`proposeTarget` already use it; phase 3 adds no second filter and none of its
own paths reads `LoadWatchSet` (the raw read that keeps deleted ids).

A **watched own task** is a task of [`ListOwnTasks`](wake-recording.md#own-tasks) whose
`workflow_id` satisfies `Contains`. `ListOwnTasks` returns each task's
`workflow_id` (the join to `tasks` already reads the row), so the callers
filter in memory against one set read:

- **Recorder.** For each event, after `CoordinatorsOwningTask`, the recorder
  reads the set once per owning coordinator and calls `RecordWake` only when
  the task's workflow is in it. A read error drops the event (the backstop
  recovers it).
- **Backstop.** Step 3.1 reads the set once per coordinator per pass, before
  the per-task reads, and skips tasks outside it. A read error skips that
  coordinator's wake duties for the pass, logs at warn and increments
  `coordinator_backstop_skipped_total`, as a failed `ListOwnTasks` does
  (`AC-COORDINATOR-WAKE-002.3`). The visit set, the turn duties and the
  lowering retry read no watch set.
- **Delivery.** Step 2 reads the set once per delivery and treats a task
  outside it as ended for every kind, like an archived task
  ([Episode recheck](wake.md#episode-recheck)): its wakes are `superseded`.
  A read error leaves every wake of that delivery `pending` and delivers
  none.
- **Adding to the set.** No hook is needed: a workflow added to the set, or a
  task moved into one, is a task the next backstop pass reads as watched, so
  its episodes that have no stored wake are stored then
  (`AC-COORDINATOR-INTEGRATION-002.3`). An episode whose wake was superseded
  while the task was unwatched stays superseded: the wake key is unique on the
  episode and the insert is `ON CONFLICT DO NOTHING`, so re-watching neither
  revives it nor stores a second one (`AC-COORDINATOR-WAKE-001.2`); only a new
  episode, with a new key, wakes. Removing a workflow supersedes its pending
  wakes at the next delivery step 2, not at the moment of the change.

The unattended turn's reads and proposals go through the phase 2 guard as an
attended turn's do: a read naming `workflow_id` or `task_id` is checked with
`coordinatorWatchesFields`, `list_workflows` is filtered by
`coordinatorWatchFilter`, and a propose target is checked by `proposeTarget`.
A failed set read refuses, so an unattended turn cannot see an unwatched board
the attended one cannot. Phase 3 adds a test that runs the guard's table
against a delivery-started session, in
[Tool list](#tool-list).

## Tool list

A coordinator conversation is opened with its bound tool list
(`BoundCoordinatorToolNames`, from `ToolNames(policy, phase2)` at open time,
stamped in the conversation task's metadata and carried to the runtime as the
`CoordinatorToolPolicy` binding). The guard refuses any tool not on it, and
agentctl builds its exact-name approval allowlist from the same list
(`manager_permission_policy.go`). Rulings:

- **Delivery uses that list.** Delivery sends into the existing conversation
  and passes no tool, binding or allowlist option. The turn message names no
  tool. There is no second list for an unattended turn.
- **Containment only removes.** Admission check 2 can hold a turn, and the
  denial below can refuse a request; neither adds a tool or approves a
  request.
- **The denial is required, not defensive.** agentctl auto-approves only a
  request whose tool name is on the allowlist and leaves every other request
  pending for a person (`manager.go`, permission handling). In an attended
  turn the manager answers it; in an unattended turn nobody would. The
  `UnattendedPermissionHandler` of
  [containment](containment.md#unattended-permissions) therefore denies at
  once any request that reaches `handlePermissionRequest` during a turn whose
  `session_turn_id` matches the open unattended row, counts it in the turn's
  `denied_permissions` and `coordinator_unattended_denials`, and never waits.
  The allowlist stays the only approval path.
- **The improvement tool.** `ToolNames` gains a third input, `phase3`, and adds
  `propose_improvement_kandev` only when phase 3 is effective at the time the
  conversation is opened. The binding validator and the allowlist derive from
  the list, so no separate registration is needed. A conversation opened
  earlier keeps its list until it is reopened. While phase 3 is effective, a
  call to the tool from such a conversation is refused `not_in_profile` and
  logged as a refusal (`AC-COORDINATOR-INTEGRATION-003.3`). While phase 3 is
  not effective, the tool is outside the guard's allowed coordinator actions,
  so a call from any conversation, including one whose list still holds the
  tool, is the phase 1 unknown-action error of the guard's check 0, stores
  nothing and writes no row
  ([permissions](permissions.md#guard), `AC-COORDINATOR-IMPROVEMENTS-001.1`).
  Changing the list mid-conversation is not done, because the binding is the
  conversation's identity.
- **Test.** The guard table of phase 2 runs against a session started by
  `Deliver`, asserting the same refusals and the same bound list as an
  attended session ([delivery](wake.md#delivery)).

## Log rows

`coordinator_activity` gains one nullable column, `unattended_turn_id`, by the
`phase2ColumnMigrations` pattern (`ALTER TABLE ... ADD COLUMN`, replayable).
`activityColumns`, `ActivityRow`, `InsertActivity` and the scan gain it, the
activity DTO carries it, and the list read joins nothing. The column has no foreign key, because turn rows are pruned after 90 days
while activity rows are kept 400 days; a pruned turn leaves the mark and
nothing to link to.

**Unattended rows.** The two writers that act for the coordinator principal,
the propose handlers and `RecordRefusal` (called by the guard), resolve the
coordinator's open unattended turn through `currentUnattendedTurn(ctx, tx,
coordinatorID)`: the open `coordinator_unattended_turns` row whose
`session_turn_id` equals the conversation session's active turn id, the same
comparison [Turn end](wake.md#turn-end) uses to tell an unattended turn from a
manager's. It stamps `unattended_turn_id` on the row it writes: the
`proposed` row, a `refused` row, and the row of an automatic approval made
inside the propose call. Rows a manager's request writes (approve, reject,
reply, undo) are never stamped, even while an unattended turn is open. A read
that fails stamps nothing and logs at warn. A refusal coalesces into an
existing row only when both rows carry the same
`unattended_turn_id` (both null counts as the same). The turn id, not a wake
id, is stored because one turn delivers up to 20 wakes; the turn's wakes are
`coordinator_wakes.turn_id`.

**Returned.** `ActivityOutcome` gains `returned`, accepted by
`ActivityRow.validate`. The reply route writes it in the same locked
transaction as the status update, through the phase 2 `settleDecision`
pattern: class `kindAction(kind)` (or `improvement`), authorization
`requires_approval`, actor the replying manager, `proposal_id`, detail the
reply text (truncated by `InsertActivity` to 1,000 characters), `edited`
false. It is not undoable, like a `message` or `resume` row.

**Automatic.** `ActivityAuthorization` gains `automatic`. The approve path
writes the `approved` row, and the `failed` row of a failed attempt, with
authorization `automatic` and actor the raising manager when the claimed
proposal has `claimed_automatically = 1`; every other approve path is
unchanged, so a manager's approval of a proposal whose automatic approval
failed is a `requires_approval` row. This is the record the
[DecisionLog adapter](automatic.md#phase-2-interfaces-consumed) keys on, so no
`decider` column is added: phase 2's authorization column already records why
an act was allowed to proceed.

**Class.** `ActionImprovement Action = "improvement"` is an activity-only
class. `validActivityClass` accepts it; `AllActions`, `Validate` and the six
D17 settings do not know it, so it is never a policy setting and never
raisable. The activity filter's class list and the class label gain it.

**Copy** (`coordinator` namespace, six locales, no em dash; extends
[what-it-did-ui](what-it-did-ui.md#copy-table), which is unchanged):

| Key | English |
| --- | --- |
| `activityAuthAutomatic` | Automatic |
| `activityApprovedAutomatically` / `...NoName` | Approved automatically, raised by {{name}} / Approved automatically |
| `activityReturnedBy` / `activityReturned` | Returned by {{name}}, with a condition / Returned, with a condition |
| `activityReturnedDetail` | Condition: {{detail}} |
| `activityClassImprovement` | Improvement |
| `activityUnattended` | During an unattended turn |

`outcomeLine` gains the `returned` case and the automatic form of `approved`;
`authorizationLine` gains `automatic`; the unattended mark renders as a
second muted line on the row's Action cell and on the phone card; the Undo
cell shows "No undo" for class `improvement` whatever its outcome. The mapping
from a row to `Decision` for task 09 is in
[automatic](automatic.md#phase-2-interfaces-consumed).

## Instructions, orders and goal

Phase 2 builds the coordinator's instructions once, when a new session's first
prompt is sent (`orchestrator/coordinator_prompt.go`, from the context, the
active standing orders and the active goal), and a change to the context, an
order or the goal resets the conversation (`resetConversation`), so the next
session is built from the new state. An unattended turn is a later prompt in
that session, so:

- It carries the instructions its session was built with, with the same three
  sections. Delivery adds no orders, goal or context to the wake message; the
  message stays the events list of [Transcript](wake.md#transcript).
- `MarkApplied(tx, coordinatorID, orderIDs, at)` runs where phase 2 runs it,
  in the transaction that stores a proposal citing `standing_order_ids`
  (`ProposeTask` and the kind propose path). An unattended turn that proposes
  with citations marks them there; a turn that proposes nothing marks nothing.
  Delivery, admission and the turn's start or end mark nothing.
- After a reset the conversation is gone until a manager opens the copilot, so
  admission check 5 holds the wakes as `no_conversation`; the replacement
  session gets the new instructions at its first prompt, attended or not.

## Proposal statuses and kinds

**`returned`** is a new proposal status value (`coordinator_proposals.status`
has no CHECK, and `openProposalStatuses` and the open-target index predicate
already exclude any other value, so a returned row is not open, not counted
toward the 25 and does not hold its task's target slot).

- The reply route's conditional update is `WHERE id = ? AND status =
  'pending'`. A `failed` proposal is refused with 409 because approve's retry
  of a failed create reconciles a task the failed attempt may already have
  created (the idempotent external id), and a reply would orphan it.
  This narrows the `pending or failed` of [relay](relay.md#reply-route).
- Not claimable, not swept: `ApproveProposal` and `approveKind` switch on the
  status and default to an unknown-status error; task 08 adds `returned` to
  the settled-conflict case of both (409 with the current proposal), and to
  reject's status rule. `ListApprovingClaimedBefore` selects only
  `approving`, so the stale-claim pass ignores it with no change.
- A revised proposal is a new row: `propose_task_kandev` with `in_reply_to`
  inserts a new `pending` `create_task` row carrying `in_reply_to`; the
  returned row is never reopened. `in_reply_to` is valid only for a returned
  `create_task` proposal of the same coordinator (the phase 2 kind value is
  `create_task`, not the `task` of earlier drafts).

**`improvement`** is a proposal kind registered in `registerKinds` beside
resume, message and move, with a `KindExecutor`:

| Method | Value |
| --- | --- |
| `Kind()` / `Action()` | `improvement` / `ActionImprovement` |
| `ValidatePropose` | not used: the tool has no `task_id`, so `propose_improvement_kandev` validates in [improvements](improvements.md#tool) and inserts through `InsertProposalWith` with a null target |
| `ValidateEdits` | refuses any edit, 400 naming `edits` |
| `Execute` | inserts the pending change with `ON CONFLICT (proposal_id) DO NOTHING` and returns `Outcome` with no task id |
| `ReRunsOnStaleClaim()` | **false** |

`ReRunsOnStaleClaim() == true` is not usable: `reclaimStaleAndProceed`
returns an error for it, so recovery of the row would fail. With false, a
claim left `approving` past the window settles `failed` with
`outcome_unknown` and no second run (`settleStaleKind`), and a manager's
**Approve** on the failed card claims it again, which runs the same idempotent
insert (`AC-COORDINATOR-INTEGRATION-006.5`). `knownProposalKind`,
`KindExecutors` and `kindAction` gain the kind; `recheckPolicy` returns nil
for it (no D17 setting governs it, and it must not inherit `create_task`'s,
which `kindAction`'s default would otherwise apply). Null `target_task_id`
rows do not collide in the open-target index, so the 25-open limit is the only
bound on open improvements. The automatic path never sees this kind.

## Settings layout

The coordinator page's Sections row (`coordinator-sections.tsx`) gains a sixth
entry `autonomy` after `goal` (added by task 01 with an empty body, so it is
visible and translated from the start; task 06 fills it), present in `SLUGS` and `entries` only while
phase 3 is effective (the flag the web already reads for the Autonomy
section, task 01), so a stored `?section=autonomy` opens Identity
otherwise. Its label and help are `sectionAutonomy` "Autonomy" and
`sectionAutonomyHelp` "Let this coordinator act on its own tasks while nobody
is watching, within a cost ceiling.", in six locales. It holds
[the controls](spend.md#screens) and "Changes waiting for you" of task 10, and
the section entry keeps phase 2's `useCoordinatorSection` behaviour.

**Automatic option.** May do keeps one radio group per action and no second
control. For every action but `create_task`, when phase 3 is effective the
Automatic option stays disabled and its note reads `mayDoAutomaticCannot`
"Cannot be raised" instead of "Not available yet"
(`AC-COORDINATOR-AUTOMATIC-001.2`). For `create_task` it is enabled for a
manager only while the eligibility read of
[automatic](automatic.md#class-reviews) says eligible, with `mayDoAutomaticNotEligible`
"Not eligible yet. See the list below." otherwise, and the eligibility list,
**Review the last 30 days** and **Mark as reviewed** render under the radio.
Choosing Automatic and saving is the raise; choosing Requires approval and
saving is the lower. The plan's **Raise to automatic** and **Lower** are
therefore the option and Save, and the record "Raised to automatic by <you>
at <time>" (`mayDoAutomaticRaised`) renders from the class change history.

**Backend.** `Validate(p)` becomes `Validate(p, phase3 bool)`: with phase 3
effective it accepts `automatic` for `create_task`; for any other action, or
with phase 3 not effective, it keeps returning `automatic_not_available`
naming the action. The change hook of [automatic](automatic.md#raise-gate)
then decides `create_task`, so only an eligible coordinator's raise is stored.

## Copilot everywhere

The "Woken by" entry is a transcript renderer keyed on
`metadata.coordinator_wake_turn_id`, part of the conversation's message list.
Every surface that renders that message list (the copilot panel on any page,
the popover and the conversation task view) gets it from the one message
component, with no page-specific branch, and the turn read that supplies the
denied count is the autonomy read (`AC-COORDINATOR-INTEGRATION-008.1`).

## Phase 2 touch points

| File (phase 2) | Additive change | Owner |
| --- | --- | --- |
| `store_phase2_schema.go` | column `coordinator_activity.unattended_turn_id`; table `coordinator_class_changes` ([automatic](automatic.md#phase-2-interfaces-consumed)) | task 01 |
| `activity.go` | `returned` outcome, `automatic` authorization, `improvement` class, `unattended_turn_id` in row, columns, insert and scan, and the activity DTO field omitted while phase 3 is not effective; refusal coalescing key | task 01 (values, columns, DTO), 05 (stamping) |
| `activity_service.go`, `activity-text.ts`, `en/coordinator.json` and five locales | class list, copy table above | task 08 (returned, unattended), 09 (automatic), 10 (improvement) |
| `policy.go`, `settings.go` | `Validate(p, phase3)`; change hook call and `LowerClass` in the locked transaction | task 09 |
| `approve.go`, `approve_kinds.go`, `undo.go` | `returned` cases; `AuthAutomatic` rows; `recheckPolicy` skip; post-commit `OnUndo` | tasks 08, 09, 10 |
| `kinds.go`, `models.go`, `decision_phase2.go` | improvement executor and kind value | task 10 |
| `toolprofile.go`, `mcp/profile/coordinator_policy.go` | `phase3` input of `ToolNames` | task 10 |
| `mcp/handlers/coordinator_authorization.go` | `actionClass` maps the improvement tool to `ActionImprovement` | task 10 |
| `coordinator-sections.tsx`, `may-do-section.tsx` | Autonomy entry (slug, label, help, six locales, empty body) | task 01 |
| `coordinator-sections.tsx`, `may-do-section.tsx` | Autonomy section controls; Automatic option states | tasks 06, 09 |

## Security

- Every addition is behind phase 3 being effective; no phase 2 route,
  scope or guard rule is loosened. The guard still refuses every phase 3 write
  for a coordinator principal, and the class change history and reviews are
  writable only by a manager or the system.
- A denied permission during an unattended turn never becomes an approval;
  the allowlist is the only approval path.

## Observability

`coordinator_unattended_denials` (task 02) counts the denials; the
`coordinator_backstop_skipped_total` counter counts a failed watch read.
No new label carries a task, coordinator or session id.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
