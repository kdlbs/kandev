---
status: draft
created: 2026-08-19
owner: Kandev
---

# Workflow Quorum Decision Recording

## Why

A `move_to_step` guarded by `wait_for_quorum` never fires. An Office task
driven through Work → Review with a reviewer attached runs the reviewer, gets a
substantive review back, and then sits at Review permanently. It is
indistinguishable, from every surface a human can see, from a hung card.

Verified 2026-08-19 against task `7611f8da-4fe3-4bb2-a5a2-ebe6f90130f2`
(workspace `95542bf3`) in `~/.kandev/data/kandev.db`:

```
sqlite> SELECT COUNT(*) FROM workflow_step_decisions;
0
```

Zero rows in the entire store, ever.

## Verified current state

The originating report named two causes. Only the first survives inspection.
Investigation surfaced four defects, three of which the report did not name.

### The evaluation path is intact (originally reported as broken)

`wait_for_quorum` **is** evaluated on the path that runs these workflows. The
claim that the orchestrator ignores the guard is false; the orchestrator
delegates to the engine, which owns the semantics.

| Link | Evidence |
|---|---|
| Guard evaluated on the transition loop | `apps/backend/internal/workflow/engine/engine.go:275` |
| All five thresholds implemented | `apps/backend/internal/workflow/engine/quorum.go:88-119` |
| Office dispatches into that engine | `apps/backend/internal/office/engine_dispatcher/dispatcher.go:108` |
| Office shares the orchestrator's engine instance | `apps/backend/internal/backendapp/main.go:1520,1527` |
| Participant + decision stores wired in production | `apps/backend/internal/backendapp/main.go:1514-1515` |
| `clear_decisions` / `queue_run_for_each_participant` callbacks wired | `apps/backend/internal/orchestrator/workflow_callbacks.go:49,56` |

The persistence layer is likewise complete: `RecordStepDecision` supersedes
prior verdicts inside a transaction (`phase2_sqlite.go:522-578`),
`ListStepDecisions` returns oldest-first (`:583-600`), and `AddTaskParticipant`
writes `decision_required = 1` for every reviewer and approver
(`office/repository/sqlite/participants.go:84-87`).

The engine's evaluation is therefore correct **given its inputs**. With zero
decision rows, `all_approve` and `any_reject` both correctly evaluate false and
the card correctly stays put. The defects are all on the input side.

### Defect 1 — an agent has no tool to record a decision

The MCP tool registry (`apps/backend/internal/mcp/server/server.go:892-923`)
enables exactly four groups for `SurfaceOfficeTask`: `plan` (4 tools),
`related-tasks` (1), `office-documents` (3), and the capability-gated
`user-question` (1). Nine tools, none of which records a decision.

`POST /api/v1/office/tasks/:id/approve` and `.../request-changes` exist
(`office/dashboard/handler.go:91-92`) and are reachable from the human UI
(`apps/web/components/task/simple/components/approval-action-bar.tsx:122-123`),
but no agent can reach either. A reviewer agent's only means of expressing a
verdict is a free-text comment, which nothing parses.

### Defect 2 — the human path writes a verdict the evaluator cannot read

Office records `changes_requested` (`office/models/models.go:653`), surfaced in
the frontend contract as `"approved" | "changes_requested"`
(`apps/web/lib/api/domains/office-extended-api.ts:297`).

The evaluator counts only `approved` and `rejected`
(`workflow/engine/quorum.go:25-26,100-103`).

A human clicking **Request changes** therefore writes a row that satisfies
neither `all_approve` (it is not an approval) nor `any_reject` (it is not
literally `rejected`). The task stays at Review with a rejection on file. This
is live today on the human path and is independent of Defect 1.

The vocabulary is only half of it. `resolveParticipantID`
(`office/dashboard/decisions.go`) stamps every human decision with the constant
`userParticipantSentinel = "user"`, because the singleton user has no
`agent_profile_id` and therefore no `workflow_step_participants` row.
`latestDecisionsPerParticipant` (`workflow/engine/quorum.go`) discards any
decision whose `participant_id` is absent from the required slate, and that
slate is built from participant-row UUIDs. **A human decision is therefore
discarded before its verdict string is ever inspected.** Correcting the
vocabulary alone changes nothing on the human path; AC-43 is what makes AC-10
reachable.

### Defect 3 — recording a decision does not re-evaluate the transition

`recordTaskDecision` (`office/dashboard/decisions.go:225-265`) persists the row,
publishes `OfficeTaskDecisionRecorded`, logs activity, and queues follow-up
agent runs. It never re-evaluates the guarded transition.

`Engine.RecordParticipantDecision` (`workflow/engine/quorum.go:152-200`) does
exactly that re-evaluation and has **zero production callers** — only tests.

So even once a verdict is recorded with a vocabulary the evaluator understands,
the transition fires only if some unrelated turn later completes on that task.
When the reviewer has already finished, nothing re-triggers.

### Defect 4 — participant rows are pinned to the step they were attached at

`AddTaskParticipant` (`office/repository/sqlite/participants.go:59-88`) writes
`step_id` = the task's step *at attach time*. No code ever updates a
participant's `step_id`; the only `UPDATE workflow_step_participants` statements
in the tree touch `agent_profile_id`.

`requiredParticipants` filters on `state.CurrentStepID`
(`workflow/engine/quorum.go:70-83`). A participant attached at one step is
invisible at every other step, and an empty slate fails closed
(`quorum.go:57-60`).

Live evidence from the verified task — the approver exists only at Work:

| participant | step | role | decision_required |
|---|---|---|---|
| `afbd72cd` | Review | reviewer | 1 |
| `8eefba93` | **Work** | approver | 1 |
| `3cb7cd33` | Work | reviewer | 1 |

Review's reviewer slate is non-empty, so Defects 1–3 are what block this task
today. But Approval's approver slate is **empty**, so fixing Review alone moves
the card one step and strands it again.

Two further facts make this wider than the slate. First, `AddTaskParticipant`'s
idempotency probe is itself scoped to `step_id`, so re-attaching the same agent
after a step change **creates a second row** for the same
`(task_id, role, agent_profile_id)` — the two-row shape is how the live data
above came to exist, not a hypothetical. Second, `ListAllTaskParticipants`
(`office/repository/sqlite/participants.go`) is *also* step-scoped, so
`resolveDeciderRole` — the role-resolution precedent AC-4 cites — is blind in
exactly the same way: an approver whose row sits at Work cannot be resolved as
an approver while the task is at Approval. Slate membership, role resolution,
and the `participant_id` written on a decision must therefore all use one
canonical-row rule, which AC-44 defines.

### Defect 5 — every failure is silent and identical

The engine holds no logger (`workflow/engine/engine.go:131-144`) and `quorum.go`
emits nothing. `evaluateTransitionGuard` returns `(false, nil)` for all of:
store not wired, empty required slate, threshold genuinely unmet, and unknown
guard variant. From outside, a card legitimately awaiting two more approvals is
byte-for-byte identical to a card whose guard can never be satisfied. This is
the diagnostic gap that let the bug survive.

### Live workflow configuration

`Office Default` (`7561d51f`), steps Backlog(0) Work(1) Review(2) Approval(3)
Done(4). Both guarded steps carry the guard nested inside the action's `config`,
which `ConfigTransitionGuard` accepts (`workflow/engine/types.go:533`):

```json
"on_turn_complete": [
  {"type": "move_to_step", "config": {"step_id": "<Approval>",
     "wait_for_quorum": {"role": "reviewer", "threshold": "all_approve"}}},
  {"type": "move_to_step", "config": {"step_id": "<Work>",
     "wait_for_quorum": {"role": "reviewer", "threshold": "any_reject"}}}
]
```

Approval is the same shape with `role: approver`.

## What

Five behaviors, matching the five defects.

1. An agent holding `reviewer` or `approver` on the current step can record an
   approve or reject verdict with a reason, via an MCP tool, and it persists to
   `workflow_step_decisions`.
2. A rejection recorded through any surface — agent tool or human UI — is
   counted by `any_reject`. This needs both a vocabulary fix and a seat fix: the
   human decider has no participant row at all, so a rejection counts as a veto
   without requiring a quorum seat, while approvals still require one.
3. Recording a verdict through any surface immediately re-evaluates the guarded
   transitions for that task's current step.
4. A participant with `decision_required` is counted at the step where the
   decision is being taken, not only at the step where they were attached.
5. A guarded transition that does not fire says why, distinguishably, on a
   surface a human reads.

Ownership follows the engine. The engine already owns guard semantics,
threshold arithmetic, and the required-participant slate; this feature adds no
second implementation in the orchestrator. The MCP tool and the HTTP handler are
transports that call into the engine's decision API. AC-57 names that API and
the ports that make it reachable, because "call into the engine" is not today a
thing `office/dashboard` can do. There are two such ports and they are named
separately: AC-57a for the write side (record a decision, re-evaluate) and
AC-57d for the read side (evaluate every guard read-only, for the AC-24b
diagnostic surface). Behavior 5 is unbuildable without the second one.

## Tool contract

Pinned here because Build would otherwise invent it and Review would relitigate it.

**Name.** `record_step_decision_kandev`, registered under a new
`office-decisions` group in the profile registry
(`apps/backend/internal/mcp/server/server.go:892-923`), enabled for
`SurfaceOfficeTask` only.

**Arguments.**

| Argument | Required | Shape |
|---|---|---|
| `decision` | yes | `"approved"` or `"rejected"` |
| `reason` | yes | non-empty string |

No idempotency argument. A retry is a new verdict that supersedes the previous
one under AC-27; the tool is deliberately NOT idempotent across calls, and
AC-45 states this as contract so a builder does not invent a client-supplied
key. Idempotency in this feature is confined to transition application
(AC-30, AC-46).

**Scoping.** The tool takes no `task_id`. It resolves the task and step from the
calling session, matching every other Office tool. An agent cannot record a
decision on a task other than the one its session is bound to.

**Return.** On success the tool returns exactly these seven fields, observed by
AC-64:

| Field | Meaning |
|---|---|
| `decision` | the recorded verdict |
| `role` | the caller's resolved role, per AC-4 |
| `step_id` | the validated step the row landed on, per AC-37 |
| `decision_id` | the decision row id, per AC-66 |
| `decided_at` | the row's `decided_at` value as persisted |
| `transition_applied` | `true` meaning the task left the step, per AC-48 — NOT that it was admitted at the target |
| `guards` | the AC-57d per-guard entries for the validated step, identical in shape to AC-24b's |

The agent needs `transition_applied` to know whether its work at this step is
finished, and `step_id` to detect the AC-37 race in which its decision landed on
a step the task has since left. `decision_id` and `decided_at` are returned
because AC-57b requires `publishDecisionRecorded` to keep publishing
`decision_id` and `created_at` off the `DecisionRecord`, and once the persist
moves behind the AC-57a entry point those two values are no longer office's to
generate.

There is deliberately NO scalar `required_count` / `received_count` pair. A step
may configure several guards, and AC-4's approver-wins precedence can resolve the
caller to `approver` while every guard at that step names `reviewer` — which is
`Office Default`'s Review exactly, per `## Live workflow configuration`. In that
case AC-42 means the decision is never counted by that guard at all, so a single
pair of counts is either ambiguous or actively misleading. `guards` gives the
agent the count against each guard that exists, which is the question it was
actually asking.

**Reason column.** The reason is written to `workflow_step_decisions.comment`,
the column every Office reader already projects (`office/dashboard/decisions.go:141,428`).
It is NOT written to `note`, which `Engine.RecordParticipantDecision` currently
uses (`workflow/engine/quorum.go:180`) and which no Office surface reads. Any
engine-side decision API used by this feature writes `comment` for the
human-facing reason. A reason written only to `note` would be invisible in the
task timeline — this is the concrete bug this paragraph exists to prevent.

## Acceptance criteria

### Recording a decision (agent surface)

- **AC-1** WHEN an Office agent session is attached to the task's current step
  with role `reviewer` or `approver` and `decision_required = 1`, THE SYSTEM
  SHALL expose an MCP tool that records a decision, accepting a verdict of
  `approved` or `rejected` and a required non-empty reason.
- **AC-2** WHEN that tool is called with a valid verdict, THE SYSTEM SHALL write
  one row to `workflow_step_decisions` with `task_id` = the calling session's
  task, `step_id` = that task's current `workflow_step_id`, `role` = the
  caller's resolved role, `decider_type` = `agent`, `decider_id` = the caller's
  agent profile id, and `participant_id` = the id of the **seat** the participant occupies, as
  defined by AC-50 — that is, AC-44 canonicalization followed by AC-20 collapse,
  not AC-44 alone. The written `participant_id` SHALL be the same id
  the quorum evaluator uses for that participant in the required slate; a
  decision whose `participant_id` is not the canonical id is a defect, because
  `latestDecisionsPerParticipant` silently discards it.
- **AC-3** WHEN the caller is not a participant on the task with
  `decision_required = 1` for `reviewer` or `approver`, THE SYSTEM SHALL reject
  the call with a permission error and write no row.
- **AC-4** WHEN the caller holds both `reviewer` and `approver`, THE SYSTEM
  SHALL record the decision under `approver`, matching the existing
  approver-wins precedence in `resolveDeciderRole`
  (`office/dashboard/decisions.go`).
- **AC-4a** THE SYSTEM SHALL resolve the caller's role over the same
  participant population that AC-50 builds, not over rows scoped to the
  task's current step alone. `ListAllTaskParticipants` is step-scoped today, so
  an approver attached at an earlier step is currently unresolvable at the step
  where the approval is due; role resolution and slate membership SHALL NOT
  disagree about who participates.
- **AC-5** WHEN the verdict is any value other than `approved` or `rejected`,
  THE SYSTEM SHALL reject the call with a validation error and write no row.
- **AC-6** WHEN the reason is empty or whitespace-only, THE SYSTEM SHALL reject
  the call with a validation error and write no row.
- **AC-7** WHEN the task has no `workflow_step_id` bound, THE SYSTEM SHALL
  reject the call with an error naming the unbound step and write no row.
- **AC-8** THE SYSTEM SHALL NOT expose the decision tool to sessions whose
  profile does not enable `SurfaceOfficeTask`.

- **AC-55** THE SYSTEM SHALL validate the decision tool's preconditions in this
  order: (1) the task has a bound `workflow_step_id` (AC-7), (2) the caller
  resolves to a participant with `decision_required = 1` for `reviewer` or
  `approver` (AC-3), (3) the verdict is `approved` or `rejected` (AC-5), (4) the
  reason is non-empty (AC-6). The step check precedes the permission check
  because `AddTaskParticipant` silently no-ops when a task has no bound step
  (`office/repository/sqlite/participants.go`), so an unbound-step task has zero
  participant rows **by construction**; a permission-first implementation would
  report AC-3's opaque permission error on the most likely real trigger for AC-7,
  a task created before a workflow step was assigned. The order is stated because
  both preconditions are false at once in that case and the two errors differ in
  diagnostic value.

- **AC-64** WHEN the decision tool returns successfully, THE SYSTEM SHALL return
  the seven fields enumerated in `## Tool contract` — `decision`, `role`,
  `step_id`, `decision_id`, `decided_at`, `transition_applied` and `guards` —
  and SHALL NOT return a scalar `required_count` or `received_count`.
  `guards` SHALL equal the AC-57d snapshot's `Guards` for the validated step,
  in the same order, so the tool and the AC-24b endpoint report the same
  arithmetic. `guards` SHALL be an empty list, not an error and not omitted,
  when the validated step configures no guarded transition.
  WHEN the post-write AC-57d snapshot cannot be computed — the AC-15 case, where
  the write succeeded and the re-evaluation errored — THE SYSTEM SHALL return
  `guards` as an empty list and `transition_applied` as `false`, and SHALL still
  report the call as successful per AC-15, the failure being surfaced through
  AC-24 rather than to the agent. A diagnostic read that failed SHALL NOT put the
  recorded verdict at risk.
  This AC exists because without it only two of the returned fields — `step_id`
  and `transition_applied`, via AC-37 and AC-46 — were observable by any
  acceptance criterion, so a wrong `role` or a wrong count could ship untested.

### Verdict vocabulary

- **AC-9** THE SYSTEM SHALL treat `changes_requested` as a rejection wherever
  `rejected` is treated as one, so that `any_reject` is satisfied by either.
- **AC-10** WHEN a human records a decision through
  `POST /tasks/:id/request-changes`, THE SYSTEM SHALL produce a stored verdict
  that satisfies `any_reject` for the guard evaluating at that step, whatever
  role that guard names. AC-9 alone does not achieve this; AC-43 and AC-58 are
  the other halves.
- **AC-43** THE SYSTEM SHALL satisfy `any_reject` when, among decisions
  recorded at the evaluating step for the guard's role, the LAST row per decider
  identity `(decider_type, decider_id)` under the AC-26 ordering is a rejection
  — whether or not that decider's `participant_id` is in the required slate. A
  rejection is a veto: one is sufficient and it does not require a quorum seat.
  This is what makes a singleton-user rejection countable, since that decider
  has no `agent_profile_id` and therefore no participant row, and is written
  with the `userParticipantSentinel` participant id.
- **AC-43b** THE SYSTEM SHALL key the AC-43 veto scan on the latest verdict per
  decider, NOT on the presence of any rejection row. A decider who rejects and
  then records an approval at the same step SHALL NOT continue to veto. Stated
  because the naive reading — scan for a rejection — reintroduces the defect
  named under `## Persistence guarantees`: the engine's `DecisionInfo`
  projection carries no `superseded_at`, so "non-superseded" is not a predicate
  the evaluator can express, and only last-row-wins is.
- **AC-43c** THE SYSTEM SHALL extend the engine's `DecisionInfo` projection to
  carry `decider_type`, `decider_id`, `role`, and `comment`. AC-43 needs decider
  identity, AC-42 needs `role`, and AC-38 needs `comment`; the projection today
  carries only `participant_id`, `decision`, and `note`, so none of the three is
  satisfiable without this. The underlying columns already exist and
  `ListStepDecisions` already selects them.
- **AC-43a** THE SYSTEM SHALL count only slate members, per AC-50, toward
  `all_approve`, `all_decide`, `majority_approve`, and `n_approve:<N>`. An
  approval is a quorum contribution and does require a seat. AC-43 is scoped to
  `any_reject` alone, so a non-slate approval never advances a task.
- **AC-58** THE SYSTEM SHALL evaluate the AC-43 veto over decisions with
  `decider_type = user` WITHOUT filtering on the guard's role, while continuing
  to filter agent decisions by role per AC-42. The singleton human's stored
  `role` SHALL remain whatever `resolveDeciderRole` returns for a user caller —
  today the constant `approver`, which `## Out of scope` leaves unchanged — and
  the veto SHALL NOT depend on it.
  Without this, AC-10 is unsatisfiable at every reviewer-guarded step, which is
  the step the motivating card in `## Why` is stuck at: `resolveDeciderRole`
  sets `hasApprover = true` unconditionally for `decider_type = user` and
  returns on the approver-wins branch, so a human clicking **Request changes**
  at `Office Default`'s Review — whose guards name `role: reviewer` — writes
  `role = approver`, and AC-42 discards it before AC-43 ever sees it.
  The alternative, stamping the human's decision with the evaluating guard's
  role, is rejected rather than merely unchosen: a decision is recorded once
  while a step may configure several guards, so "the guard's role" is not well
  defined at write time. The human is the operator of the board rather than a
  seated participant, so a human rejection is a veto over the step, not a
  role-scoped quorum contribution. AC-43a already denies a seatless approval any
  effect, so this asymmetry touches rejections only.
  Boundary, stated so it is a decision rather than an accident: when a step
  configures `any_reject` guards under two DIFFERENT roles, one human rejection
  satisfies both at once, because the veto is role-agnostic by construction.
  That is intended — the human is the operator of the board and is rejecting the
  step, not a role's share of it — and AC-17 still means only the first
  satisfied transition in configured order is applied. `Office Default` does not
  configure this shape; nothing forbids it.
- **AC-11** THE SYSTEM SHALL continue to accept and store verdict strings
  outside the recognized set without error, counting them toward `all_decide`
  and toward neither `all_approve` nor `any_reject` — preserving the documented
  free-form behavior at `workflow/engine/quorum.go:22-24`.
- **AC-12** THE SYSTEM SHALL preserve the existing wire value written by the
  human `request-changes` path, so the frontend DTO union
  `"approved" | "changes_requested"` continues to typecheck unchanged.

### Re-evaluation on record

- **AC-13** WHEN a decision is recorded through any surface, THE SYSTEM SHALL
  re-evaluate the current step's `on_turn_complete` guarded transitions before
  returning to the caller, using the task's active session as defined in AC-16
  — including on the human HTTP path, which has no calling session of its own.
- **AC-14** WHEN that re-evaluation finds a satisfied guard, THE SYSTEM SHALL
  apply the transition, and the recording call SHALL still report success.
- **AC-15** WHEN that re-evaluation fails or errors, THE SYSTEM SHALL keep the
  recorded decision, report the recording as successful, and surface the
  re-evaluation failure through the observability required by AC-24.
- **AC-16** THE SYSTEM SHALL resolve the re-evaluation session as
  `GetActiveTaskSessionByTaskID` already defines it
  (`task/repository/sqlite/session.go`): the task's most recently started
  session whose state is one of `CREATED`, `STARTING`, `RUNNING`, or
  `WAITING_FOR_INPUT`, ordered by `started_at` descending, limit one. A task is
  "unresolvable" for this purpose exactly when that query returns no row.
- **AC-16a** WHEN no session is resolvable under AC-16, THE SYSTEM SHALL record
  the decision and skip re-evaluation without erroring, matching the existing
  blank-session behavior in `Engine.RecordParticipantDecision`, and SHALL report
  the skip under AC-23 so a card that recorded a verdict but never re-evaluated
  is distinguishable from one that re-evaluated and found the threshold unmet.
- **AC-17** WHEN two guarded transitions are both satisfied, THE SYSTEM SHALL
  apply the first in the step's configured `on_turn_complete` action order,
  preserving first-transition-wins (`workflow/engine/engine.go:268-283`).

- **AC-47** THE SYSTEM SHALL run the AC-13 re-evaluation with the engine in
  **committing mode** (`EvaluateOnly = false`), so the engine applies a satisfied
  transition itself rather than returning a payload for a transport to apply.
  Consequently the AC-30 operation id SHALL be marked applied only after the
  transition has been applied, or deliberately abandoned under AC-46 — never
  before. This is stated because the two precedents in tree disagree and only one
  is safe: `Engine.RecordParticipantDecision` passes `EvaluateOnly: true` today
  while `office/engine_dispatcher` does not, and `HandleTrigger` calls
  `markOperationApplied` on its success path only. Under committing mode an apply
  failure returns before the mark, so a retry can still land; under evaluate-only
  the id is marked while no transition has been applied, and every retry then
  returns `Idempotent: true` with the transition lost permanently — the exact
  stuck-card failure this feature exists to eliminate. The existing
  `EvaluateOnly: true` on `RecordParticipantDecision` SHALL therefore change.
  Keeping the apply inside the engine is also what `## What` means by ownership
  following the engine. "Committing mode" here names APPLY-OWNERSHIP — the
  engine performs the apply and marks the operation id after it — and NOT reuse
  of the full trigger path; AC-65 bounds what the re-evaluation is allowed to
  run.

- **AC-65** THE SYSTEM SHALL scope the AC-13 re-evaluation to the step's guarded
  transition actions ONLY. The AC-57a entry point SHALL evaluate those
  `on_turn_complete` actions for which `isTransitionAction` holds and which
  carry a `wait_for_quorum` guard, in configured order, and apply at most one,
  per AC-17 and AC-46. It SHALL NOT execute non-transition action callbacks, and
  it SHALL NOT be implemented as a plain
  `HandleTrigger(TriggerOnTurnComplete)` call.
  This is stated because the obvious implementation is exactly the wrong one.
  `evaluateActions` guards only actions for which `isTransitionAction` holds and
  routes EVERY other action to `executeCallback` — and that call is not gated on
  `EvaluateOnly`, unlike `applyTransition` and `PersistData`. So a
  HandleTrigger-based re-evaluation re-runs whatever else the step configures on
  `on_turn_complete` — `queue_run`, `clear_decisions`,
  `queue_run_for_each_participant`, `run_code_review`, `create_child_task`,
  `switch_workflow`, `set_workflow_data` are all registered kinds — once per
  RECORDED DECISION rather than once per completed turn. AC-30's operation id is
  deliberately unique per decision, so the id-dedup does not suppress the repeat
  either. `Office Default` configures only two `move_to_step` actions on
  `on_turn_complete`, so this is invisible on the shipped workflow and on every
  regression in `### Regression`; it would first appear on a workflow that pairs
  a quorum guard with a side-effect action, as `clear_decisions` and
  `queue_run_for_each_participant` already are elsewhere in this same workflow's
  `on_enter`. A reviewer vote is not a turn completion, and this AC is what keeps
  the two from being conflated.
  Boundary: when the current step configures no guarded transition at all, the
  re-evaluation SHALL apply nothing and report `transition_applied = false`
  without erroring. A decision is legitimately recordable at such a step — AC-3
  gates on participation, not on the presence of a guard — and treating "nothing
  to evaluate" as a failure would reject valid verdicts.

- **AC-57** THE SYSTEM SHALL implement the AC-50 slate construction, the AC-51
  seat matching, the AC-4/AC-4a role resolution and the decision write EXACTLY
  ONCE, in the engine, against the engine's `ParticipantStore` and
  `DecisionStore` ports, and SHALL expose them to both transports through a
  single engine decision entry point. Neither the MCP tool nor
  `office/dashboard` SHALL carry its own copy. This is stated because the
  ownership sentence in `## What` is not satisfiable as written today:
  `DashboardService`'s only engine connection is
  `engineDispatcher shared.WorkflowEngineDispatcher`, whose entire interface
  surface is one `HandleTrigger` method — no slate read, no role resolution, no
  decision write. The office decision path therefore runs through
  `recordTaskDecision` -> `resolveDeciderRole` -> `s.repo.ListAllTaskParticipants`
  in `office/repository/sqlite`, a DIFFERENT Go package from the
  `workflow/repository` one backing the engine's `ParticipantStore`, while both
  read and write the same `workflow_step_participants` table. Left unstated, a
  builder may reasonably implement AC-50's four steps a second time on the
  office side — two canonicalizations over one table, which is precisely the
  read-side/write-side divergence AC-44 exists to prevent.
- **AC-57a** THE SYSTEM SHALL extend `shared.WorkflowEngineDispatcher` with an
  additive **write-side** method that records a decision and performs the AC-13
  re-evaluation, and SHALL wire it at the existing dispatcher construction site
  (`backendapp/main.go`, where `officeenginedispatcher.New` is already built
  from the shared engine instance and the task repo). `HandleTrigger` and every
  existing caller of it SHALL be unchanged. This is the write half of the
  minimum port change that makes AC-57 buildable; without naming it, "call into
  the engine's decision API" has no referent. It is the FIRST of exactly two
  additive methods this feature adds to that interface — AC-57d names the
  second, read-side one. Earlier wording said "one additive method", which read
  as forbidding the read-side method that AC-24b cannot be built without.
- **AC-57b-i** THE AC-57a entry point's return SHALL carry the decision row id
  and the persisted `decided_at`, because AC-57b promises
  `publishDecisionRecorded` is preserved unchanged and that function publishes
  `"decision_id": d.ID` and `"created_at": d.CreatedAt` off the
  `DecisionRecord`. Once the persist moves behind the entry point, office can no
  longer source those two values itself. Without this the builder must either
  widen the entry point's return undocumented or locally synthesise an id and a
  timestamp, and a locally taken timestamp need not equal the row's stored
  `decided_at` — publishing an event whose `created_at` disagrees with the row
  it describes. AC-64 observes both fields on the tool surface and they are the
  same two values.
  Concretely: the entry point SHALL stamp `decided_at` once, persist that value,
  and return THAT value — never re-read the clock to build the response. Two
  clock reads around one write are the defect this clause exists to forbid; they
  would put `created_at` on the published event ahead of the row every time,
  which is invisible in review and wrong in every timeline that joins the two.
- **AC-57b** THE SYSTEM SHALL confine the office-side change to the resolve,
  persist and re-evaluate core of `recordTaskDecision`. Its surrounding
  behavior — `publishDecisionRecorded`, `logDecisionActivity`,
  `runReactivityForDecision`, and the `DecisionRecord` shape returned to the
  handler — SHALL be preserved, and every other reader of
  `ListAllTaskParticipants` (task inbox, scheduler reactivity, the task-detail
  handler) SHALL keep today's step-scoped behavior, as `## Out of scope`
  already states. This bounds the refactor: the card repoints one function's
  core, it does not migrate `office/dashboard` onto the engine wholesale.
- **AC-57c** WHEN the engine decision entry point is not wired — `main.go`
  already returns early and logs "workflow engine not initialised; office engine
  dispatcher disabled" when `orchestratorSvc.WorkflowEngine()` is nil, leaving
  `DashboardService.engineDispatcher` nil — THE SYSTEM SHALL reject the decision
  call with an error and write no row, on BOTH transports. It SHALL NOT fall back
  to a second office-side implementation of AC-50, which would resurrect exactly
  the divergence AC-57 forbids, and it SHALL NOT silently persist a decision that
  can never be counted. This mirrors the existing `decisionStoreNotWiredErr`
  guard in `recordTaskDecision` rather than inventing a new failure mode.
- **AC-57d** THE SYSTEM SHALL extend `shared.WorkflowEngineDispatcher` with a
  second additive method, **read-only**, that evaluates every guarded
  `on_turn_complete` transition configured at a task's current step and returns
  one entry per guard together with the AC-62 `reevaluation_blocked` value:

  `EvaluateStepQuorum(ctx, taskID) (QuorumSnapshot, error)`, where
  `QuorumSnapshot` carries `StepID`, `ReevaluationBlocked bool`, and
  `Guards []QuorumGuardState`, and each `QuorumGuardState` carries
  `TargetStepID`, `Role`, `Threshold`, `RequiredCount`, `ReceivedCount`,
  `Satisfied`, `Reason` and `Error`.

  This method SHALL apply no transition, write no row, execute no action
  callback, emit no AC-24 log record and increment no AC-24a counter. It SHALL
  reuse the AC-50 slate construction, the AC-51 seat matching and the AC-52
  reason precedence — it is a second CALLER of that logic, never a second
  implementation of it, per AC-57.

  **Ordering.** `Guards` SHALL be returned in the step's configured
  `on_turn_complete` action order, so that AC-61's selection rule and AC-17's
  first-transition-wins read off one and the same order. This is the named
  ordering AC-61 depends on; without it AC-61's "first such entry" has no
  referent.

  **Nil / empty / error.** A task with no bound `workflow_step_id` SHALL return
  an empty `Guards`, `ReevaluationBlocked = false` and no error, which is what
  AC-24c requires. A step with a bound id but no guarded transition SHALL
  likewise return an empty `Guards` and no error; AC-61 aggregates both to
  `clear`. A store error SHALL surface as a per-guard `evaluation_error` entry
  per AC-23, not as a method-level error, so one failing guard does not blank
  the others. A method-level error is reserved for the not-wired case.

  **Not wired.** When the dispatcher is nil, per the AC-57c condition, the
  AC-24b handler SHALL return an error rather than falling back to an
  office-side evaluation — the read side carries the same no-fallback rule as
  the write side, and for the same reason.

  **Unknown task.** An unresolvable `taskID` SHALL surface as a method-level
  error, distinct from the unbound-step case of AC-24c which is a successful
  empty result; the AC-24b handler SHALL map it exactly as the sibling
  `GET /tasks/:id/decisions` route already maps an unknown task. Stated because
  AC-24c's "return empty rather than error" is easy to over-apply to a task that
  does not exist at all, which would report a missing task as a healthy one.

  **Idempotency and concurrency.** The method is a pure read: two concurrent
  calls compute independently from committed state and neither observes nor
  mutates the other. It needs no operation id.

  AC-24b's HTTP payload SHALL be a direct projection of this snapshot, so the
  endpoint and the tool's `guards` field cannot drift apart.

  Without this AC there is no legal way to build AC-24b at all: AC-57 forbids
  `office/dashboard` from carrying its own copy of the slate machinery, and
  `DashboardService`'s only engine connection is
  `engineDispatcher shared.WorkflowEngineDispatcher`, whose declared surface is
  `HandleTrigger` alone. AC-24b, AC-24c, AC-25, AC-60, AC-61 and AC-62 all
  depend on this method existing.

  **Implementation note, not a requirement:** the in-tree precedent for adding a
  dispatcher capability is a separate optional interface plus a type assertion
  (`handledWorkflowEngineDispatcher` in `office/dashboard/service_tasks.go`
  reaching `HandleTriggerHandled`), rather than widening the shared interface.
  Either shape satisfies this AC; the requirement is that the method exist and
  be reachable from the AC-24b handler, not which of the two idioms carries it.

### Participant slate

- **AC-18** WHEN a quorum guard evaluates at a step, THE SYSTEM SHALL count a
  **per-task** participant row (`task_id != ''`) with `decision_required = 1`
  and a matching role even when that row was attached under a different step of
  the same workflow.
- **AC-18a** THE SYSTEM SHALL NOT extend cross-step counting to template-level
  rows (`task_id = ''`). A template row is bound to one step by workflow design,
  so it remains visible only at its own step. Without this, a Work-step template
  reviewer would be pulled into a Review-step quorum.
- **AC-44** THE SYSTEM SHALL identify a participant by the natural key
  `(task_id, role, agent_profile_id)`, and SHALL select one **canonical row**
  per key: the row whose `step_id` equals the evaluating step when such a row
  exists, otherwise the row with the lowest `id` in ASCII order among that key's
  rows. `workflow_step_participants` carries no timestamp column, so `id` ASC is
  the tiebreak — "most recent" is not expressible here. The same rule SHALL be
  used by slate construction (AC-18), by role resolution (AC-4a), and by the
  `participant_id` written on a decision (AC-2), so that the read side and the
  write side cannot disagree. AC-44 is step 3 of AC-50; the id that finally
  identifies a participant is the one surviving AC-50 step 4, because AC-20's
  template-versus-per-task collapse keys on `(role, agent_profile_id)` and can
  still discard an AC-44 canonical row.
- **AC-19** WHEN both a step-specific row and a differently-stepped row exist
  for the same `(task_id, role, agent_profile_id)`, THE SYSTEM SHALL count the
  participant exactly once, as the AC-44 canonical row. This shape is produced
  in normal operation: `AddTaskParticipant`'s idempotency probe is scoped to
  `step_id`, so re-attaching the same agent after a step change inserts a second
  row rather than matching the first.
- **AC-20** THE SYSTEM SHALL continue to honor template-level rows
  (`task_id = ''`) with per-task rows taking precedence on
  `(role, agent_profile_id)`, per `phase2_sqlite.go:20-27`.
- **AC-21** WHEN the required slate for the guard's role is empty AND the
  guard's threshold is an approve-style one (`all_approve`, `all_decide`,
  `majority_approve`, `n_approve:<N>`), THE SYSTEM SHALL NOT fire the
  transition, and SHALL report the empty slate distinctly from an unmet
  threshold per AC-23. The scoping to approve-style thresholds is required by
  AC-59; an unscoped empty-slate short-circuit makes the AC-43 veto unreachable.
- **AC-22** WHEN a decision is on file for a participant who WAS in the required
  slate and has since been removed, THE SYSTEM SHALL ignore that decision for
  the counting thresholds, preserving the documented mid-flight removal
  behavior in `applyThreshold`. This governs removed slate members only; it does
  NOT govern deciders who never held a seat, whose rejections are handled by
  AC-43 and whose approvals are excluded by AC-43a.

- **AC-49** THE SYSTEM SHALL extend the engine's `ParticipantStore` port with a
  task-scoped read — `ListTaskParticipants(ctx, taskID)`, returning every
  per-task row (`task_id = ?`) for that task irrespective of `step_id` — and
  SHALL leave `ListStepParticipants(ctx, stepID, taskID)` unchanged. AC-18's
  cross-step counting is not expressible through the existing port, whose only
  production query is `WHERE step_id = ? AND (task_id = '' OR task_id = ?)`;
  widening that predicate in place would also pull template rows across steps,
  which AC-18a forbids. A task with no per-task rows SHALL yield an empty list, not an
  error, matching `ListStepParticipants`'s documented empty-is-valid contract.
  This is the participant-side twin of AC-43c — the projection and the port both
  have to be named, or the builder invents them.
- **AC-50** THE SYSTEM SHALL build the required slate for a guard, at the
  evaluating step, in exactly this order:
  1. **Gather** — per-task rows for the task via AC-49 (any step), plus
     template rows (`task_id = ''`) at the evaluating step only, per AC-18a.
  2. **Filter** — keep rows whose `role` matches the guard and whose
     `decision_required` is true, per `requiredParticipants`.
  3. **Canonicalize** — collapse to one row per `(task_id, role,
     agent_profile_id)` by AC-44.
  4. **Collapse** — collapse across `task_id` to one row per
     `(role, agent_profile_id)`, the per-task row winning, per AC-20.

  The row surviving step 4 is THE **seat**; its `id` is the slate id that AC-2
  writes and AC-26 counts. The order is stated because AC-44 keys on
  `(task_id, role, agent_profile_id)` while AC-20 keys on
  `(role, agent_profile_id)`: running AC-44 alone seats one agent twice when it
  holds both a template row and a per-task row, inflating `totalRequired` so
  `all_approve` can never be met, and running AC-20 alone discards AC-44's
  cross-step canonicalization. Boundary: a row whose `agent_profile_id` is empty
  has no identity to canonicalize on and SHALL be kept as its own seat, keyed by
  its row `id`, at both step 3 and step 4 — collapsing two such rows into one
  seat would under-count `totalRequired` and let `all_approve` fire on a single
  approval.
- **AC-51** WHEN counting decisions against the slate, THE SYSTEM SHALL map each
  decision to at most one AC-50 seat: by `participant_id` when that id is a seat
  id, otherwise by the seat whose `(role, agent_profile_id)` equals the
  decision's `(role, decider_id)` when `decider_type` = `agent`. A decision that
  maps to no seat is not counted toward any approve-style threshold, per AC-43a;
  `any_reject` does not use this mapping at all, per AC-43. The fallback exists
  because a participant's AC-44 canonical row can CHANGE after that participant
  has already decided — AC-19 states the two-row shape arises in normal
  operation, so attaching a current-step row for an agent who decided while only
  an earlier-step row existed flips the seat id, and an id-only match would
  silently discard a real verdict, which is the discard class AC-2 exists to
  prevent. AC-22 is unaffected: a participant removed from the slate entirely has
  no seat under either match, so its decision is still ignored.
- **AC-59** WHEN the required slate is empty, THE SYSTEM SHALL still evaluate an
  `any_reject` guard and SHALL NOT short-circuit it. The empty-slate fail-closed
  of AC-21 exists because `all_approve` is `approveCount == totalRequired`,
  which is vacuously TRUE at `0 == 0` — an empty slate would otherwise advance a
  task nobody reviewed. `any_reject` is `rejectCount > 0` and cannot fire
  vacuously, so the same guard rail is unnecessary there and actively harmful:
  it makes the AC-43 seatless veto unreachable in exactly the case where every
  decider is seatless. Implementation consequence, stated because this is not a
  free change: `evaluateWaitForQuorum` returns at `len(required) == 0` before
  `ListStepDecisions` is called, so the empty-slate check must become
  threshold-aware rather than remaining a precondition.

### Distinguishing waiting from stuck

- **AC-23** WHEN a guarded transition does not fire, THE SYSTEM SHALL record
  exactly one reason code from this closed set:
  `threshold_not_met`, `slate_empty`, `decision_store_unwired`,
  `participant_store_unwired`, `guard_variant_unrecognized`,
  `threshold_unrecognized`, `threshold_unsatisfiable`, `evaluation_error`, and
  `session_unresolvable`.
  `decision_store_unwired` and `participant_store_unwired` are separate because
  `evaluateWaitForQuorum` SHALL nil-check them separately — today it does not,
  the current code being a single combined
  `if e.decisions == nil || e.participants == nil { return false, nil }`, so
  splitting that branch in two is a REQUIRED change of this feature and not an
  existing property to be relied on. Left unsplit, the two codes are
  indistinguishable in practice and AC-23's taxonomy silently loses a member;
  `threshold_unsatisfiable` carries AC-40, the case where the threshold can
  never be met by any future decision and the card is stuck rather than waiting;
  `evaluation_error` carries a genuine error returned by
  `ListStepParticipants` or `ListStepDecisions`; `session_unresolvable` carries
  AC-16a. `threshold_unrecognized` carries AC-53, a threshold string
  the evaluator does not recognize at all. The set is closed rather than "at
  minimum" so that AC-24a's expvar keys are enumerable and a new reason cannot
  appear unlabelled. More than one condition can hold at once; AC-52 fixes which
  single code is reported.
- **AC-24** THE SYSTEM SHALL emit, each time a guarded transition does not fire
  **on the engine's transition-evaluation path**, a structured log record
  carrying the task id, step id, guard role,
  threshold, required-participant count, decisions-received count, the AC-23
  reason code, and an `error` field which is populated when and only when the
  reason is `evaluation_error`. The engine holds no logger today, so wiring one
  is part of this feature.
- **AC-24a** THE SYSTEM SHALL expose counts of not-fired guard evaluations under
  `/debug/vars`, keyed by the AC-23 reason, following the existing `workflow_*`
  expvar convention documented in the root `CLAUDE.md`. Like AC-24, this counts
  **engine-path evaluations only**: the AC-24b diagnostic endpoint evaluates
  guards read-only and SHALL emit no log record and increment no counter. Stated
  because AC-54 requires that endpoint to evaluate every guard live, so the two
  ACs otherwise overlap: were diagnostic reads counted, these counters would
  measure UI polling rather than workflow health, and a card left open in a
  browser would out-count a genuinely stuck one.
- **AC-24b** THE SYSTEM SHALL expose guard state for a task at
  `GET /api/v1/office/tasks/:id/quorum`, a sibling of the existing
  `GET /api/v1/office/tasks/:id/decisions` route
  (`office/dashboard/handler.go`) and carrying the same authorization as that
  route. The response SHALL contain one entry per guarded `on_turn_complete`
  transition configured at the task's current step, each with: `target_step_id`,
  `role`, `threshold`, `required_count`, `received_count`, `satisfied` (a
  boolean), `reason` (an AC-23 code, present if and only if `satisfied` is
  `false` — see AC-60), and `error` (populated only for `evaluation_error`). A
  task at a step with no guarded transition SHALL return an empty list, not a
  404. Entries are computed live per AC-54, which also defines the payload's
  top-level `reevaluation_blocked` field. The payload SHALL be a direct
  projection of the AC-57d snapshot — that is the only path by which this
  handler may reach the slate machinery, since AC-57 forbids it from carrying
  its own copy — preserving AC-57d's ordering. AC-61 defines how the card
  renders when entries disagree.
- **AC-24c** WHEN the task has no bound `workflow_step_id`, the AC-24b endpoint
  SHALL return an empty list rather than an error, so a diagnostic read never
  fails on the state it exists to diagnose.
- **AC-25** WHEN the task's AC-24b entries aggregate to `awaiting` under AC-61,
  the Office task detail view SHALL render the task as awaiting decisions —
  showing `received_count` of `required_count` for the role of the entry that
  AC-61 selects — and SHALL NOT render it as failed or errored. When they
  aggregate to `stuck`, the view SHALL render a visually distinct stuck state
  naming the selected entry's AC-23 reason. This is the whole diagnostic gap:
  the two states are byte-identical today. AC-61 exists because AC-24b returns a
  LIST and `Office Default`'s Review configures two guards, so "the reason" is
  not well defined without an aggregation rule.

- **AC-52** WHEN more than one AC-23 condition holds at once, THE SYSTEM SHALL
  report the first that applies in this order, which mirrors the order the code
  discovers them: `guard_variant_unrecognized`, `decision_store_unwired`,
  `participant_store_unwired`, `evaluation_error`, `slate_empty`,
  `threshold_unrecognized`, `threshold_unsatisfiable`, `threshold_not_met`.
  AC-23 requires exactly one code, and AC-21 and AC-40 both fire for an empty
  slate with any positive `n_approve:<N>`, since every positive N exceeds a slate
  of zero. `slate_empty` wins: it is the more actionable diagnosis, and
  `evaluateWaitForQuorum` already returns at `len(required) == 0` before the
  threshold is examined. The ninth code, `session_unresolvable`, is not in this
  ordering because it is not a guard-evaluation outcome at all — it is the
  recording-time skip of AC-16a, surfaced through AC-24/AC-24a and, on the
  AC-24b payload, through the separate field AC-54 defines. `slate_empty` is
  reachable only for approve-style thresholds, per AC-21 as scoped and AC-59; an
  `any_reject` guard over an empty slate reports `threshold_not_met`, because a
  seatless rejection can still arrive and so the card is waiting, not stuck.
- **AC-53** WHEN the guard's threshold is a string that is neither one of the
  five recognized thresholds nor prefixed `n_approve:`, THE SYSTEM SHALL not fire
  the transition and SHALL report AC-23 reason `threshold_unrecognized`, which
  AC-25 renders as stuck. `applyThreshold` returns false from its final
  fall-through for such a value today, indistinguishably from
  `threshold_not_met`, so a mistyped threshold would render as "awaiting
  decisions" forever — the exact diagnostic gap this feature exists to close.
  `guard_variant_unrecognized` is reserved for a guard whose variant is not
  `wait_for_quorum` at all, matching `evaluateTransitionGuard`'s fail-closed
  branch.
- **AC-54** THE SYSTEM SHALL compute AC-24b's per-guard entries by evaluating
  each guarded `on_turn_complete` transition configured at the task's current
  step **live, at request time, and independently** — not by replaying the
  engine's transition loop, and not by reading a stored evaluation. Independent
  evaluation is required because `evaluateActions` short-circuits, evaluating a
  guard only while `targetStepID == ""`, so a replay cannot enumerate every
  guard the way AC-24b requires. A per-guard `reason` SHALL therefore never be
  `session_unresolvable`, which is a recording-time skip rather than an
  evaluation outcome; the AC-24b payload SHALL instead carry a top-level
  `reevaluation_blocked` boolean, defined by AC-62, and AC-61 SHALL treat it as
  stuck-distinct. The field SHALL be `false` when the task has no decisions at
  its current step, so an untouched task is never rendered as stuck.

- **AC-60** THE SYSTEM SHALL set an AC-24b entry's `satisfied` to `true` when
  that guard's threshold is met at request time, and SHALL omit `reason`
  entirely for such an entry. AC-23's set is a taxonomy of why a guard did NOT
  fire and SHALL NOT gain a "satisfied" member, so AC-24a's expvar keys stay
  enumerable. The state is reachable rather than theoretical: under AC-16a a
  verdict is recorded, re-evaluation is skipped, and the task sits at the step
  with its quorum already met. AC-16a is the whole of it. An earlier draft also
  claimed the AC-46 abandon as a second route; it is not one, and the claim is
  removed rather than left to send a builder hunting: AC-46 abandons PRECISELY
  when the task's `workflow_step_id` no longer equals the evaluated step — the
  race winner has already moved it — and AC-24b evaluates at the task's CURRENT
  step, so the abandoned guard is not in the returned list at all.
- **AC-61** THE SYSTEM SHALL aggregate a task's AC-24b entries to exactly one
  card-level state: `stuck` when `reevaluation_blocked` is `true` or when ANY
  entry is unsatisfied with a `reason` other than `threshold_not_met`;
  otherwise `awaiting` when any entry is unsatisfied; otherwise `clear`. When
  the state is `stuck` the SELECTED entry is the first such entry in the step's
  configured `on_turn_complete` action order, matching AC-17's
  first-transition-wins; when `awaiting`, it is the first unsatisfied entry in
  that same order. Stuck outranks awaiting because a stuck guard is the
  actionable diagnosis and MUST NOT be masked by a sibling that is merely
  waiting — a mistyped `n_approve:<N>` reporting `threshold_unsatisfiable`
  beside a healthy `any_reject` reporting `threshold_not_met` is the concrete
  case, and rendering that card as "awaiting decisions" forever is the exact
  defect AC-53 was added to close. Boundary: an EMPTY entry list aggregates to
  `clear`, which is the correct rendering for both a step with no guarded
  transition and the AC-24c unbound-step case — neither is stuck, and neither is
  waiting on a decision.
- **AC-62** THE SYSTEM SHALL compute `reevaluation_blocked` live at request time
  as the conjunction of: the task has at least one non-superseded decision at
  its current step, AND the AC-16 session query returns no row. It SHALL NOT be
  read from a stored flag.
  The first conjunct SHALL be evaluated as "`ListStepDecisions` returns a
  non-empty list for (task, current step)". That is equivalent, and it is the
  form the engine can actually express: a row is superseded only when a
  replacement is inserted for the same decider, so a step holding at least one
  row always holds at least one non-superseded row. Stating the equivalence
  matters because the engine's `DecisionInfo` projection carries no
  `superseded_at` — the same limitation AC-43b turns into last-row-wins — so a
  literal reading of "non-superseded" would be a predicate the evaluator cannot
  compute.
  This is a CURRENT-STATE predicate, not a history of AC-16a skips, and the
  difference is deliberate. A stored "the last recording skipped" flag would keep
  rendering a card as stuck after a session returned and the card became healthy,
  and reading a stored evaluation is the very thing AC-54 forbids for the
  per-guard entries. The live predicate also needs no ordering rule, no tiebreak
  and no new column, so the "most recent decision" ambiguity does not arise; two
  concurrent reads compute the same value from committed state.
  Naming: the field is `reevaluation_blocked`, NOT `last_reevaluation_skipped`,
  because it asserts something different from the original and a reader must not
  carry the historical reading across the rename.

### Ordering, idempotency, concurrency

- **AC-26** THE SYSTEM SHALL order decisions for threshold evaluation by
  `decided_at` ascending, tie-broken by `id` ascending — the existing
  `ListStepDecisions` contract (`phase2_sqlite.go:594`) — and SHALL count only
  the last row per **AC-50 seat** under that ordering, decisions being matched to
  seats by AC-51. This clause governs the approve-style thresholds
  (`all_approve`, `all_decide`, `majority_approve`, `n_approve:<N>`) ONLY;
  `any_reject` is keyed on decider identity and needs no seat, per AC-43 and
  AC-43b. The scoping is explicit because an unqualified "last row per
  `participant_id`" is precisely the reading that reintroduces the
  discarded-human-rejection defect AC-43 exists to fix — production funnels every
  threshold through one `latestDecisionsPerParticipant` call today, so reusing
  this rule for `any_reject` is the path of least resistance.
- **AC-27** WHEN the same participant records a second decision for the same
  `(task_id, step_id)`, THE SYSTEM SHALL mark the prior row `superseded_at` and
  insert the new one in a single transaction, and the new verdict SHALL replace
  the old one for threshold purposes.
- **AC-28** WHEN two decisions are recorded concurrently for the same
  `(task_id, step_id)` by different participants, THE SYSTEM SHALL persist both,
  and at most one guarded transition SHALL be applied for the resulting step
  change. The engine's operation-id mechanism CANNOT provide this:
  `HandleTrigger` dedupes by exact `OperationID` match only
  (`isOperationAlreadyApplied`) and offers no mutual exclusion across distinct
  ids, and two concurrent decisions necessarily observe different decision sets
  and so compute different ids. Mutual exclusion SHALL therefore be enforced at
  apply time per AC-46, not by the operation id.
- **AC-46** WHEN a transition is applied after a quorum re-evaluation, THE
  SYSTEM SHALL apply it only if the task's `workflow_step_id` still equals the
  step the guard was evaluated against, and SHALL abandon the apply otherwise.
  AC-48 defines the port this needs and why the existing one cannot express it.
  The loser of a concurrent race SHALL report `transition_applied = false` with
  its decision still persisted, and SHALL NOT be reported as an error: losing
  the race is a normal outcome, not a failure.
- **AC-29** WHEN two decisions are recorded concurrently by the *same*
  participant for the same `(task_id, step_id)`, THE SYSTEM SHALL leave exactly
  one non-superseded row for that participant.
- **AC-30** WHEN a re-evaluation is triggered twice for the same recorded
  decision, THE SYSTEM SHALL apply the transition at most once, keyed on a
  **deterministic** operation id of the form
  `decision:<task_id>:<step_id>:<decision_row_id>`. Determinism is the whole
  point: `Engine.RecordParticipantDecision` currently builds its operation id
  with a `time.Now().UnixNano()` suffix, which yields a fresh id on every call
  and therefore deduplicates nothing. That existing construction SHALL NOT be
  reused as-is. The repo's convention for this primitive is a deterministic
  content-derived key — see `childCompletionOperationID`
  (`orchestrator/event_handlers_children_completed.go`).
- **AC-45** THE SYSTEM SHALL NOT accept a client-supplied idempotency key on the
  decision tool. A repeated call is a new verdict and supersedes the previous one
  per AC-27. Consequence, stated so a builder does not quietly invent a key: an
  agent that retries after a timeout on a call that actually succeeded records a
  second verdict rather than a no-op. This is acceptable because the verdict is
  the same value and AC-27 leaves exactly one non-superseded row; it is NOT
  acceptable to paper over it with a synthesized key derived from the arguments,
  which would make a deliberate change-of-mind silently fail.
- **AC-66** THE SYSTEM SHALL generate the decision row id BEFORE the write and
  pass it in `DecisionInfo.ID`, following the existing office precedent
  (`recordTaskDecision` sets `ID: uuid.New().String()` on the row it hands to
  `RecordStepDecision`). It SHALL NOT follow `Engine.RecordParticipantDecision`,
  which leaves `ID` blank and lets the repository generate one. The
  `DecisionStore.RecordStepDecision(ctx, d DecisionInfo) error` signature is
  UNCHANGED by this feature.
  The two in-tree precedents disagree and only one is usable: the port returns
  no id, so a blank-`ID` write leaves AC-30's operation id
  `decision:<task_id>:<step_id>:<decision_row_id>` unconstructible, and AC-57b-i
  unsatisfiable since office would have no id to publish. Recovering the id by
  reading the row back after the write is NOT permitted — under the AC-28
  concurrency this must coexist with, a read-back can select a different
  decider's row for the same `(task_id, step_id)`. Pre-generation also makes the
  operation id available before the write, which is what lets AC-47 mark it
  applied strictly after the apply.
- **AC-31** WHEN a task re-enters a guarded step, THE SYSTEM SHALL clear that
  step's decisions before the round's participants are queued, so a new round
  starts with zero decisions — the existing `clear_decisions` ordering on
  `on_enter`. AC-56 makes this observable rather than assumed, and names what it
  means when it does not hold.

- **AC-48** THE SYSTEM SHALL add exactly one method to the engine's transition
  port — `ApplyTransitionIfAtStep(ctx, taskID, sessionID, expectedStepID,
  toStepID, trigger) (applied bool, err error)` — used ONLY by the AC-46 quorum
  apply, and SHALL leave the existing `ApplyTransition(...) error` and every
  transition that uses it semantically unchanged. `HandleResult` SHALL gain an
  additive `TransitionAbandoned` field so an abandoned apply is distinguishable
  from both a transition and an error. This is required because AC-46's outcome
  is not expressible today: `ApplyTransition` receives `fromStepID` and ignores
  it, its orchestrator implementation unconditionally sets
  `task.WorkflowStepID = toStepID`, and a single `error` return cannot separate
  "lost the race" from "the write failed". The compare-and-swap SHALL read the
  task's `workflow_step_id` inside the same transaction that performs the update
  — `updateTaskWithWorkflowStepAdmission` already opens one and already takes the
  workspace and workflow-step locks — so no new locking primitive is introduced
  and no non-quorum transition changes behavior. `applied = true` means the task
  left `expectedStepID`; it does NOT mean the task was WIP-admitted at the
  target, because a step at capacity queues the task there and that is still a
  completed transition.

### Step binding and threshold boundaries

- **AC-36** THE SYSTEM SHALL bind a decision to the task's `workflow_step_id`
  read during validation, and SHALL write the row inside the same transaction
  that supersedes the participant's prior verdict for that step.
- **AC-37** WHEN the task's step changes between validation and write, THE
  SYSTEM SHALL persist the decision against the **validated** step — not reject
  it, and not re-target it at the new step. The recorded row SHALL NOT be
  counted at the new step, the call SHALL report success, `transition_applied`
  SHALL be `false`, and the returned `step_id` SHALL be the validated step so
  the caller can see its verdict landed on a step the task has left. Persisting
  is chosen over rejecting because the reviewer's verdict is real work and
  discarding it would silently lose a review; the returned `step_id` is what
  makes the outcome detectable.
- **AC-38** THE SYSTEM SHALL write the caller's reason to
  `workflow_step_decisions.comment`.
- **AC-39** THE SYSTEM SHALL satisfy `majority_approve` only when approvals
  strictly exceed half the required slate, so a slate of 2 requires 2 approvals
  and a slate of 3 requires 2 — preserving `approveCount*2 > totalRequired`
  (`workflow/engine/quorum.go:113`).
- **AC-40** WHEN `n_approve:<N>` names an N greater than the required slate size,
  THE SYSTEM SHALL never fire that transition and SHALL report AC-23 reason
  `threshold_unsatisfiable` rather than an error. It is deliberately NOT
  `threshold_not_met`: no future decision can satisfy it, so it is a stuck card,
  and AC-25 requires stuck to look different from waiting.
- **AC-41** WHEN `n_approve:<N>` names a non-numeric or non-positive N, THE
  SYSTEM SHALL not fire the transition and SHALL report AC-23 reason
  `threshold_unrecognized`. The threshold family is recognized here and only its
  parameter is malformed, so this is not a guard-variant failure;
  `guard_variant_unrecognized` is reserved for the case AC-53 names.
- **AC-42** WHEN a decision exists for a role other than the guard's role at the
  same step, THE SYSTEM SHALL ignore it for that guard — EXCEPT for the
  `decider_type = user` veto of AC-58, which is deliberately role-agnostic.
  AC-42 governs seated, role-scoped quorum contributions; the human veto is not
  one.

### Regression

- **AC-32** GIVEN a task at Review with exactly one reviewer participant
  (`decision_required = 1`) and one recorded `approved` decision from that
  reviewer, WHEN quorum is evaluated, THE SYSTEM SHALL move the task to
  Approval.
- **AC-33** GIVEN the same task with one recorded rejection instead, WHEN quorum
  is evaluated, THE SYSTEM SHALL move the task to Work.
- **AC-34** GIVEN two reviewer participants and one approval, WHEN quorum is
  evaluated under `all_approve`, THE SYSTEM SHALL leave the task at Review and
  report it as awaiting one further decision.
- **AC-35** GIVEN a workflow whose steps carry no guard, THE SYSTEM SHALL
  transition exactly as it does today.
- **AC-63** GIVEN a task at Review, whose guards name `role: reviewer`, and a
  human recording **Request changes** through `POST /tasks/:id/request-changes`,
  WHEN quorum is evaluated, THE SYSTEM SHALL move the task to Work — even though
  that decision is stored with `role = approver`. This is the regression that
  makes AC-10 and AC-58 observable end to end, and it is the exact path that is
  broken in production today.

- **AC-56** GIVEN a task moved off a guarded step by `any_reject` under AC-33
  and later returned to that step, WHEN quorum is next evaluated there, THE
  SYSTEM SHALL evaluate against zero decisions from the prior round. This AC
  exists to make AC-31's premise observable rather than assumed: `clear_decisions`
  on `on_enter` is depended upon but verified by no other AC, and the only
  `on_enter` trigger dispatch in the tree is inside `SwitchWorkflowCallback` —
  the orchestrator's callback registry wires a `DispatchTriggerFn` for
  `switch_workflow` only. If this AC fails, this card is blocked on the sibling
  `on_enter` action-dispatch feature and SHALL be reported as blocked; it SHALL
  NOT be closed by adding a second decision-clearing path inside this feature.
  Without it, a rejection persists at the guarded step and re-fires `any_reject`
  on the next completed turn, so AC-32 and AC-33 cannot both hold across a round
  trip.

## Failure modes

| Condition | Behavior |
|---|---|
| Agent not a participant | Permission error; no row written (AC-3) |
| Verdict outside `approved`/`rejected` on the tool | Validation error; no row (AC-5) |
| Empty reason | Validation error; no row (AC-6) |
| Task has no bound step | Error naming the unbound step; no row (AC-7) |
| Decision store not wired | Guard does not fire; reason recorded (AC-23) |
| Required slate empty, approve-style threshold | Guard does not fire; reason `slate_empty` (AC-21, AC-23) |
| Required slate empty, `any_reject` | Guard still evaluated, not short-circuited; a seatless rejection vetoes (AC-59, AC-43) |
| Unknown guard variant | Guard does not fire; reason recorded (AC-23) |
| Re-evaluation errors after a successful write | Decision kept; success reported; failure surfaced (AC-15) |
| No resolvable session | Decision kept; re-evaluation skipped, no error (AC-16) |
| Duplicate decision from one participant | Prior superseded transactionally (AC-27) |
| Human rejection, no participant row | Counted by `any_reject` as a veto (AC-43) |
| Non-slate approval | Not counted toward any approve threshold (AC-43a) |
| Participant has rows at two steps | Canonical row chosen by AC-44; one seat (AC-19) |
| Task step changed between validate and write | Persisted against validated step; `transition_applied=false`; returned `step_id` is the validated step (AC-37) |
| Concurrent transition race lost | `transition_applied=false`, decision kept, not an error (AC-46) |
| Participant store not wired | Guard does not fire; reason `participant_store_unwired` (AC-23) |
| `ListStepDecisions` / `ListStepParticipants` errors | Guard does not fire; reason `evaluation_error` with `error` field (AC-23, AC-24) |
| No active session for re-evaluation | Decision kept, re-eval skipped, reason `session_unresolvable` (AC-16a) |
| Unrecognized threshold string | Guard does not fire; reason `threshold_unrecognized`; rendered as stuck (AC-53) |
| Empty slate AND `n_approve:<N>` unsatisfiable | Single reason `slate_empty` wins by AC-52 precedence |
| Seat id changed after a verdict was recorded | Verdict still counted, matched by decider identity (AC-51) |
| Re-evaluation apply fails | Operation id left unmarked so a retry can still land (AC-47) |
| Quorum apply loses the step compare-and-swap | `applied = false`, not an error; `TransitionAbandoned` set (AC-48, AC-46) |
| Target step at WIP capacity | Transition still applied; task queued at target (AC-48) |
| Decision on file and no resolvable session | AC-24b reports `reevaluation_blocked`; card renders stuck (AC-62, AC-61) |
| Human rejects at a reviewer-guarded step | Veto counts; role filter waived for `decider_type = user` (AC-58, AC-10) |
| Guard's threshold already met at request time | Entry reports `satisfied = true` and omits `reason` (AC-60) |
| One guard stuck, a sibling merely awaiting | Card renders stuck; stuck outranks awaiting (AC-61) |
| Diagnostic `GET /quorum` read | No AC-24 log record, no AC-24a counter increment (AC-24, AC-24a) |
| Engine decision entry point not wired | Both transports error; no row written; no office-side fallback (AC-57c) |
| Task at a step with no guarded transition | AC-24b returns empty list; card aggregates to `clear` (AC-61, AC-24c) |
| Task returns to a guarded step with prior decisions live | AC-56 fails; card is blocked on the `on_enter` card, not patched here |
| AC-24b read while the engine dispatcher is not wired | Endpoint errors; no office-side fallback evaluation (AC-57d, AC-57c) |
| One guard's store read errors, siblings healthy | That entry alone reports `evaluation_error`; the others are still returned (AC-57d, AC-23) |
| Decision recorded at a step that also configures a side-effect action on `on_turn_complete` | Only guarded transitions are evaluated; no callback re-fires (AC-65) |
| Tool called at a step with no guarded transition | `guards` is an empty list, not an error (AC-64) |
| Decision row id needed for the AC-30 operation id | Pre-generated caller-side into `DecisionInfo.ID`; never read back after the write (AC-66) |
| Office needs `decision_id` / `created_at` to publish | Both returned by the AC-57a entry point (AC-57b-i) |

## Persistence guarantees

- A decision write and its supersede of the prior verdict are one transaction; a
  reader never observes zero non-superseded rows for a participant that has
  decided (existing `RecordStepDecision` behavior).
- A recorded decision survives a re-evaluation failure (AC-15).
- Decisions cleared by `clear_decisions` on step re-entry are hard-deleted;
  decisions superseded by a new verdict are retained with `superseded_at` set —
  the existing split at `phase2_sqlite.go:686-697`.
- Threshold evaluation counts, per AC-50 seat, the LAST
  row under the AC-26 ordering — it does not filter on `superseded_at`.
  `ListStepDecisions` deliberately returns superseded rows so timelines can
  render full history, and the engine's `DecisionInfo` projection carries no
  `SupersededAt` field, so the engine could not filter on it even if it wanted
  to. Last-row-wins and non-superseded coincide whenever the supersede predicate
  and the `participant_id` key agree; AC-44 is what makes them agree, and
  without it the two diverge exactly in the multi-row case of AC-19.

## Out of scope

Each of the following is a deliberate exclusion, not an oversight.

- **Workspace `approvals` subsystem.** `POST /approvals/:id/decide` and its
  `"approved" | "rejected"` shape (`apps/web/lib/api/domains/office-api.ts:534`)
  are a separate feature from task step decisions. Untouched.
- **New thresholds.** `all_approve`, `all_decide`, `any_reject`,
  `majority_approve`, `n_approve:<N>` are the complete set. No new threshold.
- **Guard variants beyond `wait_for_quorum`.** `TransitionGuard` stays
  single-variant.
- **`on_enter` action dispatch.** Tracked by its own card, and this feature does
  not change it. It is a **hard dependency**, not merely an adjacent card:
  AC-31's decision-clearing is what stops a rejection recorded at a guarded step
  from re-firing `any_reject` every time the task returns there, and the only
  `on_enter` trigger dispatch in the tree today is inside
  `SwitchWorkflowCallback`. AC-56 turns that dependency into an observable
  regression so it fails loudly instead of shipping as an unbounded
  reject/return loop. This feature SHALL NOT grow a second decision-clearing
  path to compensate.
- **Decision revocation.** A participant may supersede a verdict by recording a
  new one; there is no withdraw-to-undecided operation.
- **Timeouts and escalation.** A quorum that is never reached waits
  indefinitely. No auto-advance, no reminder, no deadline.
- **Delegating or reassigning a pending decision.** Not addressed.
- **Kanban-workflow decisions.** The decision tool is Office-surface only
  (AC-8); Kanban keeps `step_complete_kandev`.
- **Backfilling the zero existing decision rows.** There are none to migrate.
- **Client-supplied idempotency keys on the decision tool.** Excluded by AC-45.
  Transition application is idempotent (AC-30); recording is not.
- **Giving the singleton human user a participant row.** AC-43 makes a human
  rejection count without one. Seating the user in the slate would change who
  `all_approve` waits for on every existing task, which is a larger contract
  change than this card carries.
- **The `allApproversApproved` / `pendingApprovers` close gate.** These drive the
  `task_ready_to_close` reactivity run and the `ApprovalsPendingError` 409 on
  `UpdateTaskStatus`, and they read through the same step-scoped
  `office/repository/sqlite` query, so they carry Defect 4's blindness: an
  approver attached at an earlier step is invisible to them. Excluded
  deliberately, and the consequence is stated rather than hidden — after this
  card, quorum can advance a task while that separate gate still blocks its
  close on an approver it cannot see. Folding it in would extend AC-50's slate
  to a second consumer with different semantics (per-role current-approval, not
  threshold arithmetic) on a card that AC-57 has already grown. It needs its own
  card.
  The residual is sharper than "quorum can advance a task while that gate still
  blocks", and is recorded in full so the follow-up card is scoped from fact: a
  task whose AC-24b entries all report `satisfied` renders as `clear` under
  AC-61 — the healthy rendering — and can still take a 409
  `ApprovalsPendingError` on close, naming an approver that nothing this card
  touches can help the operator locate, because that gate reads through the same
  step-scoped `ListAllTaskParticipants` query Defect 4 diagnoses. This card
  therefore converts one invisible stuck class into one visible-but-confusing
  class, which is an improvement but not a completion. The follow-up should be
  tracked, not deferred indefinitely.
- **Repointing `resolveDeciderRole` for non-decision callers.** AC-4a fixes role
  resolution on the decision path only. Other readers of
  `ListAllTaskParticipants` keep today's step-scoped behavior.
- **A generic transition-lock primitive.** AC-48 adds exactly one additive port
  method, used only by the AC-46 quorum apply; `ApplyTransition` and every
  transition that uses it keep today's semantics, and the compare-and-swap reuses
  the transaction and the workspace/step locks
  `updateTaskWithWorkflowStepAdmission` already takes. No engine-wide locking
  mechanism, and no behavior change to any non-quorum transition.
- **Repointing already-attached participant rows.** AC-18 changes how the slate
  is *read*; whether existing rows are also rewritten is an implementation
  choice the plan may make either way, provided AC-18–AC-22 hold.

## E2E surfaces

User-visible surfaces this touches:

- Office task detail approval action bar (`approval-action-bar.tsx`) — the
  Request-changes path changes behavior under AC-10.
- Office task board — a card at Review now advances on quorum (AC-32, AC-33).
- `GET /api/v1/office/tasks/:id/quorum` — new read endpoint (AC-24b, AC-24c),
  projecting the AC-57d engine snapshot.
- Office task detail awaiting-decisions rendering (AC-25), including the
  stuck-distinct rendering of `reevaluation_blocked` (AC-62) and the AC-61
  aggregation when two guards disagree.

E2E recommendation: one Playwright spec driving Work → Review → Approval with a
reviewer and an approver attached, asserting advance-on-approve, return-on-reject,
and the awaiting-decisions presentation of AC-25. The same spec covers AC-56 at
no extra cost: after the return-on-reject leg, drive the task back to the guarded
step and assert it does not immediately bounce again. Backend quorum arithmetic,
vocabulary, and concurrency are Go-test territory, not E2E.
