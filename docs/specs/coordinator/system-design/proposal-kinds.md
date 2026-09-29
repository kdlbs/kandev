---
id: coordinator-proposal-kinds-design
title: Resume, message and move proposals design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-PROPOSAL-KINDS-001
  - REQ-COORDINATOR-PROPOSAL-KINDS-002
  - REQ-COORDINATOR-PROPOSAL-KINDS-003
  - REQ-COORDINATOR-PROPOSAL-KINDS-004
  - REQ-COORDINATOR-PROPOSAL-KINDS-005
---

# Resume, message and move proposals System Design

## Purpose and boundaries

This design adds a `kind` to the phase-1 proposal record and an executor per
kind (ADR D23), so resume, message and move reuse the phase-1 propose,
claim, approve, reject, recovery, routes, events and card. It also adds the
two direct manager actions on Needs you and the Queue: stall **Resume** and
Ready to merge **Send it back**.

The create path of [proposals](proposals.md) stays the `create_task`
executor, unchanged except for `starts_agent`. Policy checks are in
[permissions](permissions.md), log rows in [activity log](activity-log.md),
and the `standing_order_ids` check in
[standing orders](standing-orders.md#citations).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PROPOSAL-KINDS-001` | [Store](#store), [Propose](#propose) |
| `REQ-COORDINATOR-PROPOSAL-KINDS-002` | [Propose](#propose), [Create with a start](#create-with-a-start) |
| `REQ-COORDINATOR-PROPOSAL-KINDS-003` | [Approve](#approve), [At most once](#at-most-once) |
| `REQ-COORDINATOR-PROPOSAL-KINDS-004` | [Cards](#cards) |
| `REQ-COORDINATOR-PROPOSAL-KINDS-005` | [Direct manager actions](#direct-manager-actions) |

## Store

`coordinator_proposals` gains, additively on both dialects:

| Column | Type | Notes |
| --- | --- | --- |
| `kind` | text not null default 'create_task' | `create_task`, `resume`, `message`, `move` |
| `target_task_id` | text null | set for resume, message, move |
| `standing_order_ids` | text not null default '[]' | JSON array, at most 5 |
| `starts_agent` | boolean not null default false | create and move: the target step was agent-starting at propose |
| `outcome_json` | text null | kind-specific result, set with `approved` |

Partial unique index `coordinator_proposals_open_target` on
`(coordinator_id, kind, target_task_id) WHERE kind <> 'create_task' AND
status IN ('pending','approving','failed')`, supported by both SQLite and
PostgreSQL. Existing rows read as `create_task` (`001.6`).

`spec_json` per kind:

| Kind | Spec |
| --- | --- |
| `resume` | `{task_id, rationale}` |
| `message` | `{task_id, text, rationale}` |
| `move` | `{task_id, workflow_id, from_step_id, to_step_id, rationale}` (`from_step_id` as seen at propose, for the card title) |

## Executors

`internal/coordinator/kinds.go`:

```go
type KindExecutor interface {
    Kind() Kind
    Action() Action                               // policy action
    ValidatePropose(ctx, c Coordinator, spec json.RawMessage) (json.RawMessage, error)
    ValidateEdits(base, edits json.RawMessage) (json.RawMessage, error)
    Execute(ctx context.Context, claim Claim) (Outcome, error)
    ReRunsOnStaleClaim() bool                     // true only for create_task
}
```

`Claim` carries the proposal id, claim token, frozen spec and coordinator;
`Outcome` carries `task_id` and `outcome_json`. A registry maps kind to
executor; an unknown stored kind is treated as a failed read (500, no
write) by approve and is skipped, with an error log, by the startup pass and
the sweep ([At most once](#at-most-once)). The phase-1 approve steps 3 to 6 call `Execute` in place of the
direct create call; everything before the claim and the fenced completion
after it stay as [proposals](proposals.md#approve) specifies. This
per-kind `Execute` is the seam phase 3 reuses.

## Propose

Three MCP actions, `coordinator.propose_resume`, `coordinator.propose_message`
and `coordinator.propose_move`, back the three tools. Each resolves the
coordinator from the principal, passes the guard of
[permissions](permissions.md#guard), then:

0. Dedupe read, before any validation: parse `task_id` (absent or not a
   string is 400 naming `task_id`) and read an open proposal of the same
   `(coordinator_id, kind, target_task_id)`. When one exists, return it and
   insert nothing, whatever the other arguments are and whether the target
   still passes step 1 (an archived target, or a task already on the
   proposed step, still returns the open proposal; its approval settles it
   as [Approve](#approve) says). This is what `001.5` means by "return that
   proposal"; step 2 repeats the lookup under the lock for concurrent calls.
   A returned existing proposal is the same result shape as a new one plus
   `deduplicated: true`, so the coordinator can tell it did not create a
   proposal; its `status` shows whether it is `pending`, `approving` or
   `failed`. A `failed` proposal is open, so it blocks a new proposal for the
   same target until a manager rejects it or approves it again; this is
   intended, because two cards for one target would let two approvals run.
1. `ValidatePropose`, which reads through the task, session and workflow
   services:
   - target task: exists, not archived, `workspace_id` equals the
     coordinator's, origin not `coordinator`, workflow watched (`001.2`);
   - resume: the primary session is a resumable session
     ([requirements](../requirements/proposal-kinds.md#terminology)), read as:
     state neither `COMPLETED` nor `CREATED`; `HasLiveExecution(sessionID)`
     false (the liveness check the task-level stall detection uses); and one
     of an executor record present (`GetExecutorRunningBySessionID` returns a
     row), state `FAILED` or `CANCELLED`, or
     `models.HasInterruptedRecoveryPending(session.Metadata)` true. These are
     exactly the sessions `orchestrator.ResumeTaskSession` accepts without an
     error, so a proposal Execute would always refuse is never stored
     (`001.4`);
   - message: primary session state in `STARTING`, `RUNNING`, `IDLE`,
     `WAITING_FOR_INPUT`, `COMPLETED`; `text` trimmed, 1 to 4,000 code
     points (`001.1`, `001.4`);
   - move: `step_id` in the task's workflow, not the task's current step, the
     step's `CompleteTaskOnEnter` false, and, while `start_agent` is `denied`,
     `StartsAgentOnEnter` false (`001.3`). The row stores `starts_agent =
     StartsAgentOnEnter(step)`, so a move into an agent-starting step (allowed
     only with `start_agent` `requires_approval`) is flagged like a create.
     `StartsAgentOnEnter(steps, stepID)` is new in `eligibility.go`: true when
     the step has `AutoStartOnEnter` or feeds, through `pull_from_step_id`
     links, a step with `AutoStartOnEnter` (the two
     agent-starting clauses `EligibleStep` already holds, moved into it so
     `EligibleStep` calls it and cannot drift). A move does not apply the
     `IsStart`/`AllowManualMove` placement clause of `EligibleStep`: a manager
     approves the destination by name, and Review-style steps that refuse
     drops are exactly what a move targets;
   - `rationale` per the phase-1 rule, and the shape of
     `standing_order_ids` (a JSON array of strings, at most 5, no
     duplicate). Whether each cited order is active is not checked here.
2. In the phase-1 locked transaction: look up an open proposal of the same
   `(coordinator_id, kind, target_task_id)`; when found, return it and
   insert nothing. Otherwise check that every `standing_order_ids` entry is
   an active order of this coordinator, read inside this transaction
   ([standing orders](standing-orders.md#citations)), refusing naming
   `standing_order_ids`; then count open proposals (all kinds) against 25 and
   insert `pending` with the `proposed` activity row. A unique-index
   violation (possible only if the lock were bypassed) re-reads and returns
   the existing row (`001.5`).
3. Publish `coordinator.updated`.

`propose_task_kandev` gains the `start_agent` branch of
[Create with a start](#create-with-a-start) and `standing_order_ids`. The
create propose has no dedupe step: phase 1 has no propose-time
deduplication key ([proposals](proposals.md#propose)), and the external id
belongs to the task created at approval, not to the propose call. So its
order is: the shape of `standing_order_ids` (at most 5, no duplicate) is
checked with the other fields in phase-1 step 1, before the transaction;
the active-order check runs inside the phase-1 locked transaction of step
2, before the count against 25. A repeated create call is a new proposal,
and a repeated call with malformed citations is refused naming
`standing_order_ids` like the first.

## Create with a start

With `start_agent` `requires_approval`, the create validator relaxes only the
agent-starting clauses of `EligibleStep`: it accepts a step that exists, is
the workflow's start step or allows manual moves, and has `CompleteTaskOnEnter`
false, whether or not `StartsAgentOnEnter` is true, and stores
`starts_agent = StartsAgentOnEnter(step)` (`002.2`). A step that fails the
placement clause, or is unknown, is refused as in phase 1. With `start_agent`
`denied` the phase-1 rule applies unchanged (`002.1`).

Approving a `starts_agent` proposal is the start decision: the create
request carries the `auto_start_on_create` marker, so `handleTaskCreated`
evaluates the step's `on_enter` `auto_start_agent` as it does for a person's
create. For such a proposal the pre-create `EligibleStep` check of phase 1
becomes the same relaxed check (step exists, placement clause, not a Done
step), and the approve re-check of [permissions](permissions.md#approve-re-check)
requires `start_agent` not `denied` at approval (`002.3`). A proposal stored
with `starts_agent = false` keeps the phase-1 pre-create check, so a step
that became agent-starting still fails it.

## Approve

Steps 1 to 6 of [proposals](proposals.md#approve) run for every kind, with
these differences for a kind other than `create_task`:

- Step 1's status decision is unchanged and comes first, so an `approved` or
  `rejected` row is 409 whatever the body carries, and an `approving` row with
  a body that carries edits is 409. "Carries edits" for these kinds means at
  least one of the six names `title`, `description`, `workflow_id`, `step_id`,
  `repository_id`, `text` is present, whatever its value, `null` included.
- Step 1's `failed` external-id lookup runs only for `create_task`; a
  `failed` resume, message or move row goes straight to step 2, and its
  Approve is a new claim with a new `Execute`.
- Step 2 is the kind's `ValidateEdits` and nothing else. It does not re-run
  `ValidatePropose`: the target, session and step checks are `Execute`'s
  job, so an archived target settles `failed` (`003.1`) and a task already on
  the destination settles `approved` with `noop` (`003.3`) instead of a 400.
- Step 4's pre-create `EligibleStep` check is create-only; the kind's
  `Execute` is what runs after the claim. The policy re-check and the claim
  are unchanged.

Edits (`003.4`), checked in step 2 on a `pending` or `failed` row: for
`message`, the body may carry `text`; a body carrying any of the other five
names is 400 `not_editable`. `text` is trimmed with `strings.TrimSpace`,
counted in code points, and refused naming `text` when `null` or empty after
trimming or over 4000; the trimmed text replaces the stored text and is what
is delivered. For `resume` and `move` a body carrying any of the six names is
400 `not_editable`; an empty body or `{}` carries none. A 400 leaves the row
unchanged. For `create_task`, `text` is an unknown field and is ignored, as
in phase 1. A `failed` message proposal takes edits on top of
`final_spec_json` as phase 1 does.

After the claim commits, `Execute` runs:

| Kind | Execute |
| --- | --- |
| `resume` | Re-read the task (archived: fail `task_archived`) and its primary session (not a resumable session as in [Propose](#propose), including a session that gained a live execution: fail `not_resumable`). Call `orchestrator.ResumeTaskSession(ctx, taskID, sessionID)` and classify its result: a returned execution settles `approved`, outcome `{session_id}`, including when the call joined a concurrent manual resume of the same session (the orchestrator returns that attempt's execution, and the session is resuming, which is what the manager asked for); `(nil, nil)`, which the orchestrator returns when admission defers the resume, settles `approved` with outcome `{session_id, deferred: true}` and the row detail "It will resume when there is room"; any error, including `ErrResumeAttemptCancelled`, fails with the error text. The "already owned by another caller" error exists only for a call with a continuation and cannot occur here. |
| `message` | Re-read the task and session: archived fails `task_archived`; no primary session, or one that is `CREATED`, `FAILED` or `CANCELLED`, fails `not_accepting`; a full queue fails `queue_full`; any other delivery error fails with its text. Deliver through `TaskMessenger.DeliverQueued` ([Message delivery](#message-delivery)). Outcome `{session_id}`. |
| `move` | Re-read the task and check, in this order, stopping at the first that applies: 1. archived: fail `task_archived`; 2. workflow changed: fail `task_left_workflow`; 3. destination step missing: fail `step_missing`; 4. task already on the destination: the no-op below, whatever the step's settings now are; 5. `CompleteTaskOnEnter` now true: fail `step_is_done`; 6. `StartsAgentOnEnter` true while the proposal's `starts_agent` is false: fail `step_starts_agent`; 7. any session of the task `STARTING` or `RUNNING` (the seam and states the [undo](activity-log.md#undo) move uses, the two states the task service blocks moves on): fail `agent_running`. The approve re-check has already refused `starts_agent` true with `start_agent` `denied`, before the claim, so that refusal wins over check 4: a proposal stored with `starts_agent` true whose task has since reached the destination is refused 409 `policy_denied` while `start_agent` is `denied`, and the card offers Reject only ([permissions](permissions.md#approve-re-check)). Otherwise record `from_step_id` = the task's current step (a fenced write of `outcome_json`, see [At most once](#at-most-once)), then `UndoTaskService.MoveTaskWithOptions(ctx, taskID, workflowID, toStepID, 0, UndoMoveOptions{ExpectedWorkflowID: workflowID})` (the seam undo uses, `undo_seam.go`, with `SkipStepPrompt` false because check 6 already refused an agent-starting step the proposal did not disclose), so a person moving the task to another workflow after check 2 is refused rather than dragged back. If the fenced write matches zero rows the claim was already settled: the move call is not made, the request logs at warn `execute_settle_fenced` and returns 200 with the current row; a write error settles `failed` with the error text and does not make the move call. Results of the seam call, which returns `admitted` and the coordinator sentinels (the adapter maps the task service's `ErrWorkflowResolutionConflict` and the workflow `ErrMoveConflict` to `ErrMoveConflict`): `ErrWIPLimitExceeded` fails `step_full`; `ErrMoveConflict` fails `moved`; any other error, including a session-blocked refusal for a session that started after check 7, fails with the error text; a move accepted with `admitted` false (the task queued behind the step's limit) settles `approved` with outcome `{from_step_id, to_step_id, queued: true}` and the row detail "It is queued behind the step's limit"; otherwise outcome `{from_step_id, to_step_id}`. `from_step_id` is the step read by check 1 to 7; a person moving the task within its workflow between that read and the move call is not fenced (the workflow fence covers a change of workflow only), the window is one request, and Execute logs at info the step it found, as undo does. |

A move whose task already sits on the destination (check 4) makes no call,
because it moves nothing and starts nothing, even when the step has since
become a Done or agent-starting step, and completes with outcome `{from_step_id: to_step_id, to_step_id, noop: true}`;
its `approved` row has detail "It was already there" and is not undoable
([activity log](activity-log.md#undo)). A destination that stopped being
agent-starting since propose is not a failure: the move starts nothing more
than the card warned about. The completion update is fenced by the claim token and
writes `outcome_json` and the `approved` row; a failure writes `failed`, the
`error` and the `failed` row ([activity log](activity-log.md#writes)).

## Message delivery

`coordinator.TaskMessenger` is an interface the backend wires to the same
dispatch path `handleMessageTask` uses (`internal/mcp/handlers/handlers.go`),
extracted into an exported function of that package so both callers share
it: queued delivery, `interruptIfBusy = false`, target pinned to the primary
session read in `Execute`. The prompt is wrapped in a `<kandev-system>`
attribution block naming the coordinator and the approving manager, instead
of a sender task. `Execute` refuses before calling when the session is
`CREATED`, `FAILED`, `CANCELLED` or missing, so delivery never creates a
session or starts one that never ran (`003.2`). Delivery to an `IDLE`,
`WAITING_FOR_INPUT` or `COMPLETED` session goes through the same queued path
as `message_task_kandev`, which may relaunch the agent to process the
message. That relaunch is part of what the manager approves when approving
the message; it is not a `start_agent` decision and the `start_agent` policy
is not consulted for a message. A full message queue
(`messagequeue.QueueFullErrorCode`) fails with `queue_full`.

## At most once

`create_task` keeps the phase-1 re-claim, which is safe because the reserved
external id makes the create idempotent. The other kinds have no such key,
so every recovery caller of [proposals](proposals.md#recovery) (approve on a
stale claim, the startup pass and the one-minute sweep) takes this branch
when `ReRunsOnStaleClaim()` is false:

```sql
UPDATE coordinator_proposals
   SET status='failed', error='outcome_unknown', claim_token=NULL, updated_at=now
 WHERE id=? AND status='approving' AND claimed_at < ?
```

with the `failed` activity row in the same transaction, and never calls
`Execute`. The card renders `outcome_unknown` as "It may or may not have
run; check the task" (`003.5`). A later Approve on that `failed` card is a
new claim by a person and runs `Execute` once more. An approve of a stale
non-create claim therefore returns 200 with the failed row, as phase 1 returns
the current row after a re-claim, not a re-run.

Recovery of a non-create claim runs whether or not `features.coordinatorPhase2`
is on: settling `failed` is a safety action and calls no `Execute`. A row whose
stored kind has no registered executor (a row written by a newer binary) is
left untouched by the startup pass and the sweep, which log it at error
`unknown_kind` each pass; only an approve request reads it, as a 500 with no
write ([Executors](#executors)).

Resume and message have no write to fence before their call. Their protection
is the deadline: `Execute` starts immediately after the claim commits and is
bounded at 60 seconds, so the 2-minute stale window cannot pass before the call
returns unless the callee ignores its context, which is the sweep-during-
execution row below.

Resume is such a callee for its launch: `ResumeTaskSession` runs the launch on
an attempt context detached from the caller's (`context.WithoutCancel`), so the
60-second deadline bounds only the wait for the result. A launch slower than
the 2-minute window (a cold container or pod start) can finish after the sweep
settled the row, or after the deadline settled it `outcome_unknown`. This is
accepted: the session is then resuming, the card says "It may or may not have
run; check the task", and the late result writes nothing. Resume gets no longer
claim window, because a window that outlasts every launch would leave a crashed
approval `approving` for as long.

`Execute` of a non-create kind runs under a context deadline of 60 seconds,
below the 2-minute stale window, so a sweep can only beat a live `Execute`
that ignores its context. The interleavings and their results:

| Interleaving | Result |
| --- | --- |
| `Execute` returns before the deadline | Fenced completion settles `approved` or `failed` with the error text. |
| The deadline fires first | A nil error from `Execute` settles `approved` even when the deadline fired (the action reported success). An error settles `failed` with `outcome_unknown`, the same code and card copy as a stale claim, when `errors.Is(err, context.DeadlineExceeded)` or when the Execute context's `Err()` is `DeadlineExceeded` at return; any other error settles `failed` with its text. The settle runs on a context detached from the Execute deadline (`context.WithoutCancel` with its own 5-second bound), so an expired Execute context cannot fail the settle and leave the row to the sweep. |
| The sweep settles `outcome_unknown` while `Execute` is still running | The row is `failed`. The late fenced completion matches zero rows, writes nothing, logs at warn `execute_settle_fenced`, and its caller returns 200 with the current `failed` row. The action ran once; a manager who approves again starts a new claim and may repeat it, which the card copy "check the task" warns about. |
| Crash after `Execute` and before the settle | The startup pass settles `failed` with `outcome_unknown`; no `Execute` runs. |
| Reject while `approving` | 409 with the row (reject takes only `pending` or `failed`); the claim fence decides between two approves and between approve and reject. |

The move's `from_step_id` is written to `outcome_json` by a claim-token-fenced
update before the move call is made, and the settle statements of the sweep
never touch `outcome_json`, so a move that ran under a swept claim still
records where the task came from. The row is `failed`, not `approved`, so it
offers no undo.

## Direct manager actions

These are not proposals and write no activity row.

- **Stall Resume** (`005.1`). The stall card on Needs you, while the
  phase-2 flag is on and the viewer is a manager, shows **Resume** as the
  primary button when the stall's task has a resumable session, computed by
  the classification input from the session state and executor record. It
  sends the same resume request the task page's manual resume sends
  (`useManualResumeSession` in
  `apps/web/hooks/domains/session/use-session-resumption.ts` is private to
  the active-session hook, so the request builder and launch call it uses,
  `buildResumeRequest` and `launchSession`, are the shared seam), which goes
  through the orchestrator's resume with the user's own authority. The stall
  clears through the phase-1 stall rules when the session changes state.
- **Open the PR** (`005.2`). A link to the pull request URL the Queue row
  already has, `target="_blank"`, `rel="noopener noreferrer"`.
- **Send it back** (`005.3`). An inline note form (1 to 4,000 characters,
  counted in code points) that sends the user's queued message to the task's
  primary session through the existing task-page message action, as the
  manager. Offered only when the row's session accepts a message; a send
  failure shows the error inline and keeps the text.
- The Ready to merge group header shows "Merging a pull request is always
  human." (`005.4`). No merge control exists anywhere in the coordinator UI.

## Cards

`ProposalCard` (`apps/web/app/coordinator/proposal-card/proposal-card.tsx`)
switches on `kind` for its title and body; state handling (pending,
approving, failed, settled, stale Retry) is shared:

```text
Resume KAN-418                        Policy: Resume a task requires approval
Its agent stopped 2h ago with no error; resuming picks up the last turn.
Shaped by: Standing order 2
[Approve]  [Reject]

Message KAN-409                       Policy: Message a task requires approval
> Please rebase on main before continuing.
Rationale: the branch is 40 commits behind.
[Approve]  [Edit]  [Reject]

Move KAN-411 from Build to Review     Policy: Move a task requires approval
Approving this starts an agent.
[Approve]  [Reject]
```

- Task identifiers link to the task (`004.1`).
- "Shaped by" labels come from the proposal's `standing_order_ids` and the
  store's orders, as [standing orders](standing-orders.md#shaped-by-ui)
  specifies (`004.2`).
- "Approving this starts an agent" shows when the proposal's stored
  `starts_agent` is true, for a create or a move (`004.3`); it is never
  computed from the live workflow steps store, so the card shows what was
  proposed and a later step change fails the move rather than widening it.
- Edit only on message cards, editing the text (`004.4`).
- The chat transcript attaches the card to the tool call of any propose
  tool by the returned `proposal_id`, as phase 1 does for create.
- With the phase-2 flag off, the client hides non-create proposals and the
  count excludes them ([coordinators](coordinators.md#phase-2)).

## Security

- Every kind passes the same guard, policy re-check and workspace and
  Watches checks; the target task id is validated server-side.
- Message text is untrusted; it is delivered inside an attribution block
  that marks it as coordinator-authored and manager-approved.
- A resume, message or move never runs without a person's approval, and
  never runs twice without a second approval.

## Observability

Execute logs at info with kind, proposal, coordinator and task ids and the
outcome or failure code. `outcome_unknown` settles log at warn.

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
