---
status: draft
system: tasks
requirements:
  - REQ-TASKS-QUORUM-CORE-001
  - REQ-TASKS-QUORUM-RECORDING-001
  - REQ-TASKS-QUORUM-VERDICT-001
  - REQ-TASKS-QUORUM-REEVALUATION-001
  - REQ-TASKS-QUORUM-SLATE-001
  - REQ-TASKS-QUORUM-DIAGNOSTICS-001
  - REQ-TASKS-QUORUM-CONCURRENCY-001
  - REQ-TASKS-QUORUM-BINDING-001
  - REQ-TASKS-QUORUM-REGRESSION-001
---


# Workflow quorum decision recording System Design



## Purpose and boundaries



The task system owns quorum semantics, participant canonicalization, decision persistence, guarded transition application, and diagnostic projections. Office and MCP surfaces are transports over that engine-owned contract.



## Requirement mapping

| Requirement | Design source |
| --- | --- |
| `REQ-TASKS-QUORUM-CORE-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-QUORUM-RECORDING-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-QUORUM-VERDICT-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-QUORUM-REEVALUATION-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-QUORUM-SLATE-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-QUORUM-DIAGNOSTICS-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-QUORUM-CONCURRENCY-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-QUORUM-BINDING-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-QUORUM-REGRESSION-001` | Extracted from the legacy design sections below. |

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
vocabulary alone changes nothing on the human path; AC-TASKS-QUORUM-VERDICT-001.3 is what makes AC-TASKS-QUORUM-VERDICT-001.2
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
`resolveDeciderRole` — the role-resolution precedent AC-TASKS-QUORUM-RECORDING-001.4 cites — is blind in
exactly the same way: an approver whose row sits at Work cannot be resolved as
an approver while the task is at Approval. Slate membership, role resolution,
and the `participant_id` written on a decision must therefore all use one
canonical-row rule, which AC-TASKS-QUORUM-SLATE-001.3 defines.

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
one under AC-TASKS-QUORUM-CONCURRENCY-001.2; the tool is deliberately NOT idempotent across calls, and
AC-TASKS-QUORUM-CONCURRENCY-001.7 states this as contract so a builder does not invent a client-supplied
key. Idempotency in this feature is confined to transition application
(AC-TASKS-QUORUM-CONCURRENCY-001.6, AC-TASKS-QUORUM-CONCURRENCY-001.4).

**Scoping.** The tool takes no `task_id`. It resolves the task and step from the
calling session, matching every other Office tool. An agent cannot record a
decision on a task other than the one its session is bound to.

**Return.** On success the tool returns exactly these seven fields, observed by
AC-TASKS-QUORUM-RECORDING-001.11:

| Field | Meaning |
|---|---|
| `decision` | the recorded verdict |
| `role` | the caller's resolved role, per AC-TASKS-QUORUM-RECORDING-001.4 |
| `step_id` | the validated step the row landed on, per AC-TASKS-QUORUM-BINDING-001.2 |
| `decision_id` | the decision row id, per AC-TASKS-QUORUM-CONCURRENCY-001.8 |
| `decided_at` | the row's `decided_at` value as persisted |
| `transition_applied` | `true` meaning the task left the step, per AC-TASKS-QUORUM-CONCURRENCY-001.10 — NOT that it was admitted at the target |
| `guards` | the AC-TASKS-QUORUM-REEVALUATION-001.14 per-guard entries for the validated step, identical in shape to AC-TASKS-QUORUM-DIAGNOSTICS-001.4's |

The agent needs `transition_applied` to know whether its work at this step is
finished, and `step_id` to detect the AC-TASKS-QUORUM-BINDING-001.2 race in which its decision landed on
a step the task has since left. `decision_id` and `decided_at` are returned
because AC-TASKS-QUORUM-REEVALUATION-001.12 requires `publishDecisionRecorded` to keep publishing
`decision_id` and `created_at` off the `DecisionRecord`, and once the persist
moves behind the AC-TASKS-QUORUM-REEVALUATION-001.10 entry point those two values are no longer office's to
generate.

There is deliberately NO scalar `required_count` / `received_count` pair. A step
may configure several guards, and AC-TASKS-QUORUM-RECORDING-001.4's approver-wins precedence can resolve the
caller to `approver` while every guard at that step names `reviewer` — this happens
when both seats sit at the task's current `workflow_step_id`, which `Office
Default`'s Review reaches whenever a thin workspace seats one agent in both
roles there (AC-OFFICE-REVIEW-SEATS-002.4/002.5), per `## Live workflow
configuration`. A caller whose approver seat instead sits at an earlier step,
such as Work, now resolves to `reviewer` at Review under
AC-TASKS-QUORUM-RECORDING-001.4's step-preferring rule and is counted
normally by that guard; approver-wins no longer reaches across steps. In the
same-step case, AC-TASKS-QUORUM-BINDING-001.7 means the decision is never
counted by that guard at all, so a single pair of counts is either ambiguous
or actively misleading. `guards` gives the agent the count against each guard
that exists, which is the question it was actually asking.

This precedence applies only to the agent decision surface. The human decision
path retains its existing `resolveDeciderRole` behavior and unconditional
approver precedence, as required by AC-TASKS-QUORUM-RECORDING-001.4a. The two
paths can therefore persist different roles for the same caller, task, and
step. Future implementations must preserve this boundary.

**Reason column.** The reason is written to `workflow_step_decisions.comment`,
the column every Office reader already projects (`office/dashboard/decisions.go:141,428`).
It is NOT written to `note`, which `Engine.RecordParticipantDecision` currently
uses (`workflow/engine/quorum.go:180`) and which no Office surface reads. Any
engine-side decision API used by this feature writes `comment` for the
human-facing reason. A reason written only to `note` would be invisible in the
task timeline — this is the concrete bug this paragraph exists to prevent.

## Failure modes

| Condition | Behavior |
|---|---|
| Agent not a participant | Permission error; no row written (AC-TASKS-QUORUM-RECORDING-001.3) |
| Verdict outside `approved`/`rejected` on the tool | Validation error; no row (AC-TASKS-QUORUM-RECORDING-001.6) |
| Empty reason | Validation error; no row (AC-TASKS-QUORUM-RECORDING-001.7) |
| Task has no bound step | Error naming the unbound step; no row (AC-TASKS-QUORUM-RECORDING-001.8) |
| Decision store not wired | Guard does not fire; reason recorded (AC-TASKS-QUORUM-DIAGNOSTICS-001.1) |
| Required slate empty, approve-style threshold | Guard does not fire; reason `slate_empty` (AC-TASKS-QUORUM-SLATE-001.6, AC-TASKS-QUORUM-DIAGNOSTICS-001.1) |
| Required slate empty, `any_reject` | Guard still evaluated, not short-circuited; a seatless rejection vetoes (AC-TASKS-QUORUM-SLATE-001.11, AC-TASKS-QUORUM-VERDICT-001.3) |
| Unknown guard variant | Guard does not fire; reason recorded (AC-TASKS-QUORUM-DIAGNOSTICS-001.1) |
| Re-evaluation errors after a successful write | Decision kept; success reported; failure surfaced (AC-TASKS-QUORUM-REEVALUATION-001.3) |
| No resolvable session | Decision kept; re-evaluation skipped, no error (AC-TASKS-QUORUM-REEVALUATION-001.4) |
| Duplicate decision from one participant | Prior superseded transactionally (AC-TASKS-QUORUM-CONCURRENCY-001.2) |
| Human rejection, no participant row | Counted by `any_reject` as a veto (AC-TASKS-QUORUM-VERDICT-001.3) |
| Non-slate approval | Not counted toward any approve threshold (AC-TASKS-QUORUM-VERDICT-001.6) |
| Participant has rows at two steps | Canonical row chosen by AC-TASKS-QUORUM-SLATE-001.3; one seat (AC-TASKS-QUORUM-SLATE-001.4) |
| Task step changed between validate and write | Persisted against validated step; `transition_applied=false`; returned `step_id` is the validated step (AC-TASKS-QUORUM-BINDING-001.2) |
| Concurrent transition race lost | `transition_applied=false`, decision kept, not an error (AC-TASKS-QUORUM-CONCURRENCY-001.4) |
| Participant store not wired | Guard does not fire; reason `participant_store_unwired` (AC-TASKS-QUORUM-DIAGNOSTICS-001.1) |
| `ListStepDecisions` / `ListStepParticipants` errors | Guard does not fire; reason `evaluation_error` with `error` field (AC-TASKS-QUORUM-DIAGNOSTICS-001.1, AC-TASKS-QUORUM-DIAGNOSTICS-001.2) |
| No active session for re-evaluation | Decision kept, re-eval skipped, reason `session_unresolvable` (AC-TASKS-QUORUM-REEVALUATION-001.5) |
| Unrecognized threshold string | Guard does not fire; reason `threshold_unrecognized`; rendered as stuck (AC-TASKS-QUORUM-DIAGNOSTICS-001.8) |
| Empty slate AND `n_approve:<N>` unsatisfiable | Single reason `slate_empty` wins by AC-TASKS-QUORUM-DIAGNOSTICS-001.7 precedence |
| Seat id changed after a verdict was recorded | Verdict still counted, matched by decider identity (AC-TASKS-QUORUM-SLATE-001.10) |
| Re-evaluation apply fails | Operation id left unmarked so a retry can still land (AC-TASKS-QUORUM-REEVALUATION-001.7) |
| Quorum apply loses the step compare-and-swap | `applied = false`, not an error; `TransitionAbandoned` set (AC-TASKS-QUORUM-CONCURRENCY-001.10, AC-TASKS-QUORUM-CONCURRENCY-001.4) |
| Target step at WIP capacity | Transition still applied; task queued at target (AC-TASKS-QUORUM-CONCURRENCY-001.10) |
| Decision on file and no resolvable session | AC-TASKS-QUORUM-DIAGNOSTICS-001.4 reports `reevaluation_blocked`; card renders stuck (AC-TASKS-QUORUM-DIAGNOSTICS-001.12, AC-TASKS-QUORUM-DIAGNOSTICS-001.11) |
| Human rejects at a reviewer-guarded step | Veto counts; role filter waived for `decider_type = user` (AC-TASKS-QUORUM-VERDICT-001.7, AC-TASKS-QUORUM-VERDICT-001.2) |
| Guard's threshold already met at request time | Entry reports `satisfied = true` and omits `reason` (AC-TASKS-QUORUM-DIAGNOSTICS-001.10) |
| One guard stuck, a sibling merely awaiting | Card renders stuck; stuck outranks awaiting (AC-TASKS-QUORUM-DIAGNOSTICS-001.11) |
| Diagnostic `GET /quorum` read | No AC-TASKS-QUORUM-DIAGNOSTICS-001.2 log record, no AC-TASKS-QUORUM-DIAGNOSTICS-001.3 counter increment (AC-TASKS-QUORUM-DIAGNOSTICS-001.2, AC-TASKS-QUORUM-DIAGNOSTICS-001.3) |
| Engine decision entry point not wired | Both transports error; no row written; no office-side fallback (AC-TASKS-QUORUM-REEVALUATION-001.13) |
| Task at a step with no guarded transition | AC-TASKS-QUORUM-DIAGNOSTICS-001.4 returns empty list; card aggregates to `clear` (AC-TASKS-QUORUM-DIAGNOSTICS-001.11, AC-TASKS-QUORUM-DIAGNOSTICS-001.5) |
| Task returns to a guarded step with prior decisions live | AC-TASKS-QUORUM-REGRESSION-001.6 fails; card is blocked on the `on_enter` card, not patched here |
| AC-TASKS-QUORUM-DIAGNOSTICS-001.4 read while the engine dispatcher is not wired | Endpoint errors; no office-side fallback evaluation (AC-TASKS-QUORUM-REEVALUATION-001.14, AC-TASKS-QUORUM-REEVALUATION-001.13) |
| One guard's store read errors, siblings healthy | That entry alone reports `evaluation_error`; the others are still returned (AC-TASKS-QUORUM-REEVALUATION-001.14, AC-TASKS-QUORUM-DIAGNOSTICS-001.1) |
| Decision recorded at a step that also configures a side-effect action on `on_turn_complete` | Only guarded transitions are evaluated; no callback re-fires (AC-TASKS-QUORUM-REEVALUATION-001.8) |
| Tool called at a step with no guarded transition | `guards` is an empty list, not an error (AC-TASKS-QUORUM-RECORDING-001.11) |
| Decision row id needed for the AC-TASKS-QUORUM-CONCURRENCY-001.6 operation id | Pre-generated caller-side into `DecisionInfo.ID`; never read back after the write (AC-TASKS-QUORUM-CONCURRENCY-001.8) |
| Office needs `decision_id` / `created_at` to publish | Both returned by the AC-TASKS-QUORUM-REEVALUATION-001.10 entry point (AC-TASKS-QUORUM-REEVALUATION-001.11) |

## Persistence guarantees

- A decision write and its supersede of the prior verdict are one transaction; a
  reader never observes zero non-superseded rows for a participant that has
  decided (existing `RecordStepDecision` behavior).
- A recorded decision survives a re-evaluation failure (AC-TASKS-QUORUM-REEVALUATION-001.3).
- Decisions cleared by `clear_decisions` on step re-entry are hard-deleted;
  decisions superseded by a new verdict are retained with `superseded_at` set —
  the existing split at `phase2_sqlite.go:686-697`.
- Threshold evaluation counts, per AC-TASKS-QUORUM-SLATE-001.9 seat, the LAST
  row under the AC-TASKS-QUORUM-CONCURRENCY-001.1 ordering — it does not filter on `superseded_at`.
  `ListStepDecisions` deliberately returns superseded rows so timelines can
  render full history, and the engine's `DecisionInfo` projection carries no
  `SupersededAt` field, so the engine could not filter on it even if it wanted
  to. Last-row-wins and non-superseded coincide whenever the supersede predicate
  and the `participant_id` key agree; AC-TASKS-QUORUM-SLATE-001.3 is what makes them agree, and
  without it the two diverge exactly in the multi-row case of AC-TASKS-QUORUM-SLATE-001.4.

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
  AC-TASKS-QUORUM-CONCURRENCY-001.9's decision-clearing is what stops a rejection recorded at a guarded step
  from re-firing `any_reject` every time the task returns there, and the only
  `on_enter` trigger dispatch in the tree today is inside
  `SwitchWorkflowCallback`. AC-TASKS-QUORUM-REGRESSION-001.6 turns that dependency into an observable
  regression so it fails loudly instead of shipping as an unbounded
  reject/return loop. This feature SHALL NOT grow a second decision-clearing
  path to compensate.
- **Decision revocation.** A participant may supersede a verdict by recording a
  new one; there is no withdraw-to-undecided operation.
- **Timeouts and escalation.** A quorum that is never reached waits
  indefinitely. No auto-advance, no reminder, no deadline.
- **Delegating or reassigning a pending decision.** Not addressed.
- **Kanban-workflow decisions.** The decision tool is Office-surface only
  (AC-TASKS-QUORUM-RECORDING-001.9); Kanban keeps `step_complete_kandev`.
- **Backfilling the zero existing decision rows.** There are none to migrate.
- **Client-supplied idempotency keys on the decision tool.** Excluded by AC-TASKS-QUORUM-CONCURRENCY-001.7.
  Transition application is idempotent (AC-TASKS-QUORUM-CONCURRENCY-001.6); recording is not.
- **Giving the singleton human user a participant row.** AC-TASKS-QUORUM-VERDICT-001.3 makes a human
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
  close on an approver it cannot see. Folding it in would extend AC-TASKS-QUORUM-SLATE-001.9's slate
  to a second consumer with different semantics (per-role current-approval, not
  threshold arithmetic) on a card that AC-TASKS-QUORUM-REEVALUATION-001.9 has already grown. It needs its own
  card.
  The residual is sharper than "quorum can advance a task while that gate still
  blocks", and is recorded in full so the follow-up card is scoped from fact: a
  task whose AC-TASKS-QUORUM-DIAGNOSTICS-001.4 entries all report `satisfied` renders as `clear` under
  AC-TASKS-QUORUM-DIAGNOSTICS-001.11 — the healthy rendering — and can still take a 409
  `ApprovalsPendingError` on close, naming an approver that nothing this card
  touches can help the operator locate, because that gate reads through the same
  step-scoped `ListAllTaskParticipants` query Defect 4 diagnoses. This card
  therefore converts one invisible stuck class into one visible-but-confusing
  class, which is an improvement but not a completion. The follow-up should be
  tracked, not deferred indefinitely.
- **Repointing `resolveDeciderRole` for non-decision callers.** AC-TASKS-QUORUM-RECORDING-001.5 fixes role
  resolution on the decision path only. Other readers of
  `ListAllTaskParticipants` keep today's step-scoped behavior.
- **A generic transition-lock primitive.** AC-TASKS-QUORUM-CONCURRENCY-001.10 adds exactly one additive port
  method, used only by the AC-TASKS-QUORUM-CONCURRENCY-001.4 quorum apply; `ApplyTransition` and every
  transition that uses it keep today's semantics, and the compare-and-swap reuses
  the transaction and the workspace/step locks
  `updateTaskWithWorkflowStepAdmission` already takes. No engine-wide locking
  mechanism, and no behavior change to any non-quorum transition.
- **Repointing already-attached participant rows.** AC-TASKS-QUORUM-SLATE-001.1 changes how the slate
  is *read*; whether existing rows are also rewritten is an implementation
  choice the plan may make either way, provided AC-TASKS-QUORUM-SLATE-001.1–AC-TASKS-QUORUM-SLATE-001.7 hold.

## E2E surfaces

User-visible surfaces this touches:

- Office task detail approval action bar (`approval-action-bar.tsx`) — the
  Request-changes path changes behavior under AC-TASKS-QUORUM-VERDICT-001.2.
- Office task board — a card at Review now advances on quorum (AC-TASKS-QUORUM-REGRESSION-001.1, AC-TASKS-QUORUM-REGRESSION-001.2).
- `GET /api/v1/office/tasks/:id/quorum` — new read endpoint (AC-TASKS-QUORUM-DIAGNOSTICS-001.4, AC-TASKS-QUORUM-DIAGNOSTICS-001.5),
  projecting the AC-TASKS-QUORUM-REEVALUATION-001.14 engine snapshot.
- Office task detail awaiting-decisions rendering (AC-TASKS-QUORUM-DIAGNOSTICS-001.6), including the
  stuck-distinct rendering of `reevaluation_blocked` (AC-TASKS-QUORUM-DIAGNOSTICS-001.12) and the AC-TASKS-QUORUM-DIAGNOSTICS-001.11
  aggregation when two guards disagree.

E2E recommendation: one Playwright spec driving Work → Review → Approval with a
reviewer and an approver attached, asserting advance-on-approve, return-on-reject,
and the awaiting-decisions presentation of AC-TASKS-QUORUM-DIAGNOSTICS-001.6. The same spec covers AC-TASKS-QUORUM-REGRESSION-001.6 at
no extra cost: after the return-on-reject leg, drive the task back to the guarded
step and assert it does not immediately bounce again. Backend quorum arithmetic,
vocabulary, and concurrency are Go-test territory, not E2E.
