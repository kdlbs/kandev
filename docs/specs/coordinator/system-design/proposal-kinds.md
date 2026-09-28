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
| `starts_agent` | boolean not null default false | |
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
write). The phase-1 approve steps 3 to 6 call `Execute` in place of the
direct create call; everything before the claim and the fenced completion
after it stay as [proposals](proposals.md#approve) specifies. This
per-kind `Execute` is the seam phase 3 reuses.

## Propose

Three MCP actions, `coordinator.propose_resume`, `coordinator.propose_message`
and `coordinator.propose_move`, back the three tools. Each resolves the
coordinator from the principal, passes the guard of
[permissions](permissions.md#guard), then:

1. `ValidatePropose`, which reads through the task, session and workflow
   services:
   - target task: exists, not archived, `workspace_id` equals the
     coordinator's, origin not `coordinator`, workflow watched (`001.2`);
   - resume: primary session exists, state not `COMPLETED`, and no
     running executor record (`GetExecutorRunningBySessionID` returns none)
     (`001.4`);
   - message: primary session state in `STARTING`, `RUNNING`, `IDLE`,
     `WAITING_FOR_INPUT`, `COMPLETED`; `text` trimmed, 1 to 4,000 code
     points (`001.1`, `001.4`);
   - move: `step_id` in the task's workflow, not the task's current step, the
     step's `CompleteTaskOnEnter` false, and, while `start_agent` is `denied`,
     `EligibleStep` true (`001.3`);
   - `rationale` per the phase-1 rule; `standing_order_ids` per
     [standing orders](standing-orders.md#citations).
2. In the phase-1 locked transaction: look up an open proposal of the same
   `(coordinator_id, kind, target_task_id)`; when found, return it and
   insert nothing. Otherwise count open proposals (all kinds) against 25 and
   insert `pending` with the `proposed` activity row. A unique-index
   violation (possible only if the lock were bypassed) re-reads and returns
   the existing row (`001.5`).
3. Publish `coordinator.updated`.

`propose_task_kandev` gains the `start_agent` branch of
[Create with a start](#create-with-a-start) and `standing_order_ids`.

## Create with a start

With `start_agent` `requires_approval`, the create validator accepts a step
where `EligibleStep` is false, provided `CompleteTaskOnEnter` is false, and
stores `starts_agent = true` (`002.2`). With `start_agent` `denied` the
phase-1 rule applies (`002.1`).

Approving a `starts_agent` proposal is the start decision: the create
request carries the `auto_start_on_create` marker, so `handleTaskCreated`
evaluates the step's `on_enter` `auto_start_agent` as it does for a person's
create. The pre-create `EligibleStep` check of phase 1 is skipped for such a
proposal, and the approve re-check of [permissions](permissions.md#approve-re-check)
requires `start_agent` not `denied` at approval (`002.3`). A proposal stored
with `starts_agent = false` keeps the phase-1 pre-create check, so a step
that became agent-starting still fails it.

## Approve

Edits (`003.4`): for `message`, the body may carry `text` (validated as in
propose); for `resume` and `move`, a body carrying any edit field is 400
`not_editable` before the claim. A `failed` message proposal takes edits on
top of `final_spec_json` as phase 1 does.

After the claim commits, `Execute` runs:

| Kind | Execute |
| --- | --- |
| `resume` | Re-read the task (archived: fail `task_archived`) and its primary session (not resumable: fail `not_resumable`). Call `orchestrator.ResumeTaskSession(ctx, taskID, sessionID)`; its error fails with the error text. Outcome `{session_id}`. |
| `message` | Re-read the task and session (archived or not accepting: fail). Deliver through `TaskMessenger.DeliverQueued` ([Message delivery](#message-delivery)). Outcome `{session_id}`. |
| `move` | Re-read the task: archived (`task_archived`), workflow changed (`task_left_workflow`), destination step missing (`step_missing`), `CompleteTaskOnEnter` now true (`step_is_done`), or agent-starting while `start_agent` is `denied` (`step_starts_agent`) each fail. Record `from_step_id` = the task's current step, then `taskSvc.MoveTask(ctx, taskID, workflowID, toStepID, 0)`. Outcome `{from_step_id, to_step_id}`. |

A move whose task already sits on the destination completes with that
outcome and no call. The completion update is fenced by the claim token and
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
session or starts one that never ran (`003.2`). A full message queue
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
non-create claim therefore returns the failed row, not a re-run.

## Direct manager actions

These are not proposals and write no activity row.

- **Stall Resume** (`005.1`). The stall card on Needs you, while the
  phase-2 flag is on and the viewer is a manager, shows **Resume** as the
  primary button when the stall's task has a resumable session, computed by
  the classification input from the session state and executor record. It
  calls the task page's existing manual resume
  (`useManualResumeSession` in
  `apps/web/hooks/domains/session/use-session-resumption.ts`), which goes
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

`ProposalCard` (`apps/web/app/coordinator/needs-you/proposal-card.tsx`)
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
- "Approving this starts an agent" shows when `starts_agent` is true, or for
  a move whose destination step is not eligible (`004.3`), computed from the
  workflow steps store.
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
