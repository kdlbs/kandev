---
id: coordinator-proposals-design
title: Task proposals design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-09-26
requirements:
  - REQ-COORDINATOR-PROPOSALS-001
  - REQ-COORDINATOR-PROPOSALS-002
  - REQ-COORDINATOR-PROPOSALS-003
  - REQ-COORDINATOR-PROPOSALS-004
  - REQ-COORDINATOR-PROPOSALS-005
---

# Task proposals System Design

## Purpose and boundaries

Proposals are the coordinator's only write. The proposal service stores them,
validates them against the workspace, and on approval creates one ordinary
task through the task service with an idempotent external id. The task system
owns task creation; this design only calls it.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PROPOSALS-001` | [Store](#store), [Propose](#propose) |
| `REQ-COORDINATOR-PROPOSALS-002` | [Approve](#approve), [Edits](#edits), [Recovery](#recovery), [Reserved prefix](#reserved-prefix) |
| `REQ-COORDINATOR-PROPOSALS-003` | [Reject](#reject) |
| `REQ-COORDINATOR-PROPOSALS-004` | [Routes](#routes), [Events](#events), [Client store](#client-store) |
| `REQ-COORDINATOR-PROPOSALS-005` | [Cards](#cards) |

## Store

Table `coordinator_proposals` in the coordinator store:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID |
| `coordinator_id` | text not null | indexed with `status, created_at, id` |
| `workspace_id` | text not null | |
| `status` | text not null | `pending`, `approving`, `approved`, `rejected`, `failed` |
| `spec_json` | text not null | title, description, rationale, workflow, step, repository, source task |
| `final_spec_json` | text null | frozen at claim |
| `claimed_at` | timestamp null | set with `approving` |
| `claim_token` | text null | UUID generated per claim; fences completion |
| `task_id` | text null | set with `approved` |
| `error` | text null | set with `failed`, at most 1,000 characters |
| `reject_reason` | text null | at most 500 characters |
| `decided_by` | text null | user id |
| `created_at`, `updated_at` | timestamp not null | UTC |

## Propose

The `coordinator.propose_task` MCP action resolves the coordinator from the
principal, then in `internal/coordinator/proposals.go`:

1. Validates the fields of `AC-COORDINATOR-PROPOSALS-001.3` through the task,
   workflow and repository services. The step defaults to the workflow's start
   step. Eligibility reads the step's `on_enter` actions and `allow_manual_move`.
2. In one transaction, first takes a per-coordinator lock, then counts open
   proposals of the coordinator, refuses at 25, and inserts `pending`. On
   PostgreSQL the lock is `SELECT id FROM coordinators WHERE id = ? FOR
   UPDATE`, so concurrent proposes for one coordinator serialise on the
   coordinator row and a second transaction counts after the first commits;
   a missing row ends the transaction as 404. On SQLite the transaction is
   opened with `BEGIN IMMEDIATE`, taking the single write lock before the
   count. A store test on both dialects runs 30 concurrent proposes against a
   coordinator with 0 open proposals and asserts exactly 25 rows and 5
   refusals.
3. After commit publishes `coordinator.updated`.

There is no deduplication key.

## Approve

`POST .../proposals/:pid/approve` (`workspace.manage`), body optional edits
(see [Edits](#edits)):

1. Read the proposal (404 when it is absent or belongs to another coordinator
   or workspace) and decide by its status, before any edit is looked at or
   validated. The body "carries edits" when it has at least one of the five
   [Edits](#edits) fields, whatever their values; an empty body or `{}` carries
   none.
   - `approved` or `rejected`: 409 with the row.
   - `approving` and the body carries edits: 409 with the row, stale or not;
     the edits are neither validated nor applied.
   - `approving` with a claim that is not stale: 409 with the row.
   - `approving` with a stale claim and no edits: take the
     [stale re-claim](#stale-re-claim), which completes with the spec frozen
     by the first claim and does not validate it again.
   - `pending` or `failed`: continue with step 2.
2. Build the candidate spec: the base is `final_spec_json` when the row has
   one (a `failed` attempt), else `spec_json`; merge the edits and validate as
   in propose. 400 leaves the row unchanged. `spec_json` is never rewritten.
3. Claim, with a new UUID `T`: `UPDATE ... SET status='approving',
   claimed_at=now, claim_token=T, final_spec_json=?, decided_by=?, error=NULL
   WHERE id=? AND status IN ('pending','failed')`. When it matches no row, a
   concurrent request changed the row after step 1: re-read it; a row that is
   gone (its coordinator or workspace was deleted) returns 404, any other row
   returns 409 with that row. After a claim commits, publish
   `coordinator.updated`, so every open surface shows "Approval in progress"
   before the create runs.
4. Create the task through the task service with the frozen spec, external
   id `coordinator-proposal:<id>` (with `AllowReservedExternalID`), origin the
   regular board origin, and no `start_agent`, no `prepare_session` and no
   `auto_start_on_create` metadata marker. Branch on the returned
   `CreateTaskResult.Outcome`:
   - `CreateTaskOutcomeCreated`: call `Service.SettleExternalID(ctx,
     task.ID, "coordinator-proposal:<id>")`, as the MCP and HTTP create
     handlers do. `settled=true`: complete with `task.ID`. `settled=false`
     (identity lost, which only a direct database change can cause, since
     release of the prefix is refused): complete with the survivor's id and
     log at warn. An error wrapping `taskrepo.ErrTaskNotFound` (the task was
     deleted while this create ran): fail with "The created task was deleted
     before approval completed" and leave `task_id` unset: the task no
     longer exists, so nothing was in fact created. Any other settle error:
     log it at warn and return the error to the caller without touching the
     row (step 5 does not run for this branch); the row stays `approving`
     with its existing claim, since the task exists but is not confirmed
     settled. The next recovery (the next startup pass, or the claim's own
     staleness after two minutes triggering a stale re-claim) retries the
     create, which the external id makes idempotent, and completes through
     the `Found*` branch below.
   - `CreateTaskOutcomeFoundSettled`: an earlier attempt created the task;
     complete with its id.
   - `CreateTaskOutcomeFoundUnsettled`: an earlier attempt created the task
     and stopped before settling. Complete with its id, as the external-id
     contract requires ("proceed with the returned task id"); never settle,
     release or re-create it, and log at info.
   - A create error: fail with it.
5. Complete: `UPDATE ... SET status='approved', task_id=?, claim_token=NULL
   WHERE id=? AND status='approving' AND claim_token=T`. Fail: the same fence
   sets `failed` with the error (truncated to 1,000 characters) and clears
   `claim_token`. When either update matches zero rows, re-read the row:
   - the row exists (another claimer re-claimed it after this one went
     stale): log at info and return 200 with the current row, writing
     nothing; any task this request created is the one the other claimer's
     create returns as Found;
   - the row is gone (its coordinator or workspace was deleted): log at info
     with the task id and return 404; the task stays on its board.
6. Publish `coordinator.updated`; return the proposal.

### Stale re-claim

`UPDATE ... SET claimed_at=now, claim_token=T WHERE id=? AND
status='approving' AND claimed_at < now - 2 minutes`. It never writes
`final_spec_json` or `decided_by`, and it does not re-validate the frozen
spec's fields (title, description, workflow and repository existence): the
completion uses the spec frozen by the first claim and keeps the first
approver as `decided_by`. It does re-run the [step-eligibility
check](#no-agent-starts) against the frozen spec's workflow and step
immediately before step 4's create call, because eligibility can have changed
in the time between the original claim and this stale re-claim (an
`on_enter` `auto_start_agent` action added to the target step, or a new
`pull_from_step_id` feeder link formed into an auto-starting step), and
recovery must not create the task on a step that is no longer eligible. When
the recheck fails, the re-claim still commits (the row leaves `approving`
either way), but recovery sets the proposal `failed` with a descriptive error
("the target step is no longer eligible") and never calls the task service,
the same failure shape as the existing case where a frozen spec the task
service no longer accepts (its workflow was deleted, for example) fails at
the create in step 4 and sets the proposal `failed` with that error. An
approve request whose body carries edits never reaches the re-claim (step 1
refuses it with 409), so edits are never silently dropped; recovery callers
never send edits. A re-claim that commits publishes `coordinator.updated`,
then runs the eligibility recheck and steps 4 to 6 with its token. When it
matches no row, re-read the row: gone returns 404 (for a recovery reader, no
action); otherwise another reader won the re-claim and the request returns
409 with the current row (for a recovery reader, no action).

### No agent starts

The create request carries no session request and no
`auto_start_on_create` marker, and `handleTaskCreated`
(`internal/orchestrator/event_handlers_workflow.go`) evaluates the target
step's `on_enter` `auto_start_agent` only for a task carrying that marker
(`models.HasAutoStartOnCreateIntent`), so the direct-create path never starts
an agent whatever the target step's own actions are.

That marker is create-time only: `finalizeCreatedTask`
(`internal/task/service/service_tasks.go`) also runs feeder/WIP-limit
reconciliation synchronously after every create (`pullTasksFromNewFeederWork`
→ `promoteNextQueuedTask` → `promoteSameStepQueuedTask` /
`promoteFeederQueuedTask`, `internal/task/service/service_workflow.go`), and
the resulting `task.moved` / `task.queue_promoted` events reach
`handleTaskMovedNoSession` / `handleTaskQueuePromotedWithAutoStartOnCreateClaimed`
(`internal/orchestrator/event_handlers_workflow.go`), neither of which checks
`HasAutoStartOnCreateIntent`, by design, since those handlers represent a
task entering a step via an ordinary transition, which is meant to auto-start
regardless of how the task was created. A task created on a step that feeds
an available auto-start step would therefore auto-start immediately despite
carrying no marker, defeating this guarantee.

Eligibility closes this at the placement boundary instead of the create-time
boundary: an **eligible step** (defined in
[requirements/proposals.md](../requirements/proposals.md#terminology)) is
also refused when it is a feeder, directly or through a chain of
`pull_from_step_id` links, of any step with an `on_enter` `auto_start_agent`
action. Validated at propose, again at claim time, and again by a [stale
re-claim](#stale-re-claim) immediately before it creates the task (so a step
or feeder graph changed after proposing, after the original claim, or during
the recovery window is caught each time), this means a proposal can only ever
land somewhere a manager could place a task by hand *and leave it there*,
never somewhere the workflow itself would immediately relocate it into an
auto-starting step. task-01 implements the feeder-graph walk inside its
step-eligibility store method, which takes the workflow's full step graph,
not just the candidate step, so it can check reachability; task-03 calls it
at propose time and task-07 calls it again at claim time and stale re-claim.

### Edits

The approve body fields are `title`, `description`, `workflow_id`, `step_id`
and `repository_id`. For each:

| Sent as | Effect |
| --- | --- |
| absent | unchanged |
| JSON `null` | 400 naming the field; nothing changes |
| `""` for `title` | 400 (empty after trimming) |
| `""` for `description` | clears the description |
| `""` for `workflow_id` | 400 (a workflow is required) |
| `""` for `step_id` | clears the step, so the workflow's start step is used |
| `""` for `repository_id` | clears the repository; the task has none |
| a value | replaces the field, then the spec is validated |

When `workflow_id` changes and `step_id` is absent, the step resets to the new
workflow's start step. `rationale` and `source_task_id` are not editable; like
any unknown field they are ignored. Strings are trimmed before validation. An
empty body, or `{}`, approves the base spec unchanged.

## Reject

`POST .../proposals/:pid/reject` (`workspace.manage`), body `{reason?}`, in
the same order as approve: read the proposal (404 when absent or of another
coordinator or workspace); a status other than `pending` or `failed` returns
409 with the row; then the reason is read and trimmed, and a reason over 500
characters returns 400 with the row unchanged; then `UPDATE ... SET status='rejected',
reject_reason=?, decided_by=? WHERE id=? AND status IN ('pending','failed')`.
When it matches no row, re-read the row: gone returns 404, any other row
returns 409 with that row.

The reason is handled as follows:

| Sent as | Effect |
| --- | --- |
| absent, JSON `null`, `""` or only whitespace | `reject_reason` stored as SQL `NULL`; the API returns `reject_reason: null` and the card shows no reason |
| a string of 1 to 500 code points after trimming | stored trimmed |
| over 500 code points after trimming | 400 naming `reason`; nothing changes |
| any other JSON type | 400 naming `reason`; nothing changes |

Unknown body fields are ignored. An empty body or `{}` rejects with no reason.
Reject is allowed from `failed` so that a
proposal that cannot be created can be closed, matching the failed-card
design (UI-03 in the [plan](../../../plans/workspace-coordinator/plan.md)).
This departs from the source analysis plan (`implementation-plan.md`
revision 9, section 6.5.4, kept outside this repository), which allowed
reject from `pending` only. The approve claim and the reject share the
same condition, `WHERE id=? AND status IN ('pending','failed')`, and each sets
a status outside that set (`approving` or `rejected`). The row's `status` is
therefore an atomic compare-and-swap: whichever UPDATE commits first matches
one row, and the other then matches none, re-reads and returns 409 with the
winner's row. A racing approve and reject cannot both succeed.

## Recovery

A claim is stale after two minutes. Two callers run recovery on an
`approving` row with a stale claim: the startup pass and an approve request.
A proposal read, single or list, never writes; it always returns rows as
stored. Recovery takes the stale re-claim `UPDATE` in [Approve](#approve),
which refreshes `claimed_at` and sets a new `claim_token`, so of a startup
pass and an approve racing on one stale claim exactly one wins it, and a slow
original claimer's completion no longer matches the token. It then re-runs
the step-eligibility check against the frozen spec's workflow and step
([No agent starts](#no-agent-starts)); a step that is no longer eligible sets
the proposal `failed` with a descriptive error and skips the create entirely.
Otherwise it runs steps 4 to 6: the idempotent create returns the task the
first attempt made, if any. Recovery keeps `final_spec_json` and `decided_by`
from the first claim, and never touches a session.

## Reserved prefix

The prefix is enforced in the task service, so every entry point inherits it:

- `CreateTaskRequest` gains `AllowReservedExternalID bool` tagged `json:"-"`,
  so no HTTP or MCP body can set it. `Service.CreateTask`, after
  `NormalizeExternalID`, refuses a value starting `coordinator-proposal:` with
  `ErrExternalIDInvalid` (400 at the HTTP and MCP handlers) unless the flag is
  set. Only the coordinator service sets it.
- `Service.ReleaseTaskExternalID` refuses a value starting
  `coordinator-proposal:` with `ErrExternalIDInvalid`, so
  `DELETE /api/v1/workspaces/:id/tasks/by-external-id`
  (`httpReleaseTaskExternalID`) returns 400 and the id stays bound to its
  task. The coordinator service never releases these ids.
- Task-service tests cover create through HTTP and MCP with the prefix (400),
  create with the flag (created), and release with the prefix (400, binding
  unchanged).

## Routes

| Route | Scope | Result |
| --- | --- | --- |
| `GET .../coordinators/:cid/proposals?status=pending\|all` | `workspace.read` | open, `created_at` asc then id asc; or newest 50, desc; 400 naming `status` for any other value |
| `GET .../coordinators/:cid/proposals/:pid` | `workspace.read` | one proposal |
| `POST .../proposals/:pid/approve` | `workspace.manage` | proposal, 400, 403, 404 or 409 |
| `POST .../proposals/:pid/reject` | `workspace.manage` | proposal, 400, 403, 404 or 409 |

Routes live under `/api/v1/workspaces/:id/`. A proposal of another
coordinator or workspace is 404. Both reads need only `workspace.read` and
never write: stale-claim recovery runs only in the startup pass and on
approve ([Recovery](#recovery)). The coordinator list response carries
`open_proposals` per coordinator for the sidebar badge.

## Events

Every proposal write publishes `coordinator.updated` after it commits, with
`{workspace_id, coordinator_id, open_proposals}`: insert, claim, stale
re-claim, completion, failure and reject. A write that matched zero rows
publishes nothing. The forwarder in
`gateway/websocket/coordinator_notifications.go` sends it to clients
subscribed to the workspace, as other workspace notifications do.

## Client store

`hooks/domains/coordinator/use-proposals.ts` keeps proposals keyed by id per
coordinator. It loads `status=pending` on mount and refetches on
`coordinator.updated`. A merge never replaces a settled status (`approved`,
`rejected`) with an unsettled one, so a late list response cannot revive a
decided card.

A `status=pending` refetch alone cannot satisfy
`AC-COORDINATOR-PROPOSALS-004.4` for a proposal this client already knows
about: once it settles, the pending list simply stops containing it, so a
browser that had it cached as `pending`/`approving` would otherwise keep
showing that stale state forever instead of the settled one. On each
`coordinator.updated`, after merging the fresh `status=pending` response, the
hook also fetches by id (`GET .../proposals/:pid`) every locally cached id
that response no longer contains, and merges each result the same way (never
un-settling a settled card). This is bounded by how many ids the client has
ever locally held for its coordinator, not by the 25-open cap. A Needs-you
item that settles and was never cached beyond the list is not affected: it is
correct for it to simply stop being a Needs-you item.

The chat transcript's `ProposalCard` (see [Cards](#cards)) is attached to a
specific `proposal_id` that can outlive the pending window entirely: a
reloaded second browser may never have fetched the pending list at the
moment a proposal it is displaying was still open. It therefore does not rely
on `use-proposals.ts`'s list-keyed cache: it fetches its own `proposal_id` by
id on mount and again on every `coordinator.updated` for its coordinator,
independent of whether that id is in the current pending list, which is what
lets it show "Approved: <card>" or "Rejected: <reason>"
(`AC-COORDINATOR-PROPOSALS-005.8`) after a reload with no other proposal
ever having been listed.

## Cards

- One `ProposalCard` renders pending, approving, failed and settled states; it
  is used on Needs you and inside the copilot transcript.
- In the transcript the card is attached to the tool-call message of
  `propose_task_kandev` by the returned `proposal_id`.
- Edit and Reject forms render in place on the Needs-you card; from the chat
  card they navigate to Needs you and open the same form, focus moving to it.
- The decision toast reads the item count from the classification of
  [needs-you](needs-you.md#classification) after the store update.
- Readers get the card without actions.

## Security

- Decisions authorise at the backend by workspace scope; the principal of the
  MCP action is resolved server-side.
- A proposal read never writes, so no request, from any site, can trigger
  recovery or otherwise mutate a proposal through a read route.
- Spec strings are untrusted and rendered as text.
- The created task is ordinary; the coordinator gains no authority over it.

## Observability

Propose, claim, approve, fail, reject and recovery log at info with the
proposal, coordinator and workspace ids; failures carry the error.

## Related decisions

- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
