---
id: coordinator-activity-log-design
title: What it did (activity log) design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-ACTIVITY-LOG-001
  - REQ-COORDINATOR-ACTIVITY-LOG-002
  - REQ-COORDINATOR-ACTIVITY-LOG-003
  - REQ-COORDINATOR-ACTIVITY-LOG-004
  - REQ-COORDINATOR-ACTIVITY-LOG-005
---

# What it did (activity log) System Design

## Purpose and boundaries

This design adds the `coordinator_activity` table (ADR D20), the writes
that fill it from the proposal and guard paths, the list, summary and undo
routes, the coordinator's read tool and the What it did section of the
Queue. It is the source phase 3 reads for its first automatic choice; the
contract phase 3 relies on is listed in [Phase 3 contract](#phase-3-contract).

Proposal state changes are designed in [proposals](proposals.md) and
[proposal kinds](proposal-kinds.md); this design only adds the row each one
writes. Refusals come from the guard of [permissions](permissions.md#guard).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-ACTIVITY-LOG-001` | [Store](#store), [Writes](#writes), [Refusals](#refusals) |
| `REQ-COORDINATOR-ACTIVITY-LOG-002` | [Routes](#routes), [What it did UI](#what-it-did-ui) |
| `REQ-COORDINATOR-ACTIVITY-LOG-003` | [Undo](#undo), [What it did UI](#what-it-did-ui) |
| `REQ-COORDINATOR-ACTIVITY-LOG-004` | [Read tool](#read-tool) |
| `REQ-COORDINATOR-ACTIVITY-LOG-005` | [Retention](#retention), [Summary](#summary), [Routes](#routes) |

## Store

`coordinator_activity` in the coordinator store, both dialects:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID; the tiebreak after `created_at` |
| `coordinator_id` | text not null | |
| `workspace_id` | text not null | |
| `action_class` | text not null | one of the six actions, or `unknown` |
| `outcome` | text not null | `proposed`, `approved`, `rejected`, `failed`, `refused`, `undone` |
| `authorization` | text not null | `requires_approval` or `denied` |
| `target_task_id` | text null | the task acted on or created |
| `proposal_id` | text null | |
| `actor_user_id` | text null | the manager for approved, rejected, undone and, when known, failed; NULL when authentication is off or the caller is internal |
| `reason_code` | text null | refusal code, rejection reason code, failure code |
| `detail` | text not null default '' | at most 1,000 characters, truncated on write |
| `edited` | boolean not null default false | approved with edits |
| `refusal_count` | integer not null default 1 | |
| `undone_at` | timestamp null | |
| `undone_by` | text null | the undoing manager; NULL, never an empty string, when authentication is off |
| `undo_of_id` | text null | on an `undone` row, the row it reverses |
| `created_at`, `updated_at` | timestamp not null | UTC |

Every time bound in this design (the list cursor, the summary window, the
retention cutoff and the refusal cutoff) is computed in Go in UTC and bound in
the same time encoding `created_at` is written with, so the SQLite text
comparison and the PostgreSQL timestamp comparison agree.

Indexes: `(coordinator_id, created_at DESC, id DESC)` for the list and
summary, `(coordinator_id, action_class, created_at)` for the filtered list,
`(created_at)` for retention, `(workspace_id)` for the workspace-deletion
delete. Coordinator delete and the workspace deletion
transaction delete the coordinator's rows (`005.1`).

`internal/coordinator/activity.go` is the only writer. Rows are never
updated except `undone_at`, `undone_by`, `updated_at` by `MarkUndone` and
`refusal_count`, `updated_at` by coalescing (`001.4`). A test scans the
package's non-test sources for every string literal containing `UPDATE
coordinator_activity` and fails unless its SET columns are exactly one of
those two sets, and for every `DELETE FROM coordinator_activity` outside
retention and the two deletion transactions. `Detail` is truncated on write
to 1,000 runes (code points), without an ellipsis. `MarkUndone` stores NULL
for an empty `undone_by`. `Service.Record` and
`RecordRefusal` write nothing when `phase2` is false, and their signatures
are in [coordinators](shared-interface.md#shared-interface).

## Writes

Each write takes the caller's transaction (`tx`), so the row and the state
change commit together or not at all; a failed insert fails the
transaction and the caller returns the error (`001.6`).

| Site | Row |
| --- | --- |
| propose insert (every kind) | `proposed`, `requires_approval`, detail from the kind's one-line title |
| completion update to `approved` | `approved`, actor = `decided_by`, `edited` = `final_spec_json` differs from `spec_json` on any edited field, target = created or target task |
| reject | `rejected`, actor, `reason_code` and `detail` both hold the reason text as the manager gave it (1 to 500 characters, so no truncation applies) |
| failure update to `failed` | `failed`, `reason_code` = the failure code (`approval_failed` for a created task; each other kind defines its own closed set in [proposal kinds](proposal-kinds.md)), detail = the error, actor = the approving manager when known, else NULL |
| undo | `undone` row (its `detail` copies the reversed row's) plus the marker on the original ([Undo](#undo)) |

The completion and failure updates are the claim-fenced updates of
[proposals](proposals.md#approve); the insert runs only when that update
matched a row, so a losing claimer writes no row. A retry of a `failed`
proposal that later settles writes another row. The phase-1 stall paths
and a manager's direct Resume or Send it back write nothing (`001.5`).

A proposal's action class is its kind's action: `create_task`, `resume`,
`message` or `move`.

## Refusals

`Service.RecordRefusal(ctx, coordinatorID, workspaceID, actionClass,
reasonCode)` runs in its own transaction after the guard refuses:

1. `UPDATE coordinator_activity SET refusal_count = refusal_count + 1,
   updated_at = now WHERE id = (SELECT id FROM coordinator_activity WHERE
   coordinator_id = ? AND action_class = ? AND reason_code = ? AND outcome =
   'refused' AND created_at >= ? ORDER BY created_at DESC, id DESC
   LIMIT 1)`.
2. No row matched: insert a `refused`, `denied` row.

`?` is a cutoff computed in Go as `now.UTC().Add(-60 * time.Second)` and
bound in the same time encoding `created_at` is written with, so the SQLite
text comparison and the PostgreSQL timestamp comparison agree. Both run
under the per-coordinator lock, so two concurrent refusals produce one row
with count 2; a coordinator missing under the lock (deleted meanwhile)
writes nothing and is not an error. The action class of an unknown tool
name is `unknown`, assigned by the guard through `ActionForTool`
(`001.3`). A failure here is logged at warn; the refusal already happened (`001.6`).
After the transaction commits, a refusal that inserted or coalesced a row
publishes `coordinator.updated` for the coordinator, so the list refreshes
(`002.6`).

## Routes

Under `/api/v1/workspaces/:id/coordinators/:cid/`, registered only with the
phase-2 flag on:

| Route | Scope | Result |
| --- | --- | --- |
| `GET activity?class=&before=&limit=` | `workspace.read` | `{rows, next_cursor}` |
| `GET activity/summary?days=` | `workspace.read` | [Summary](#summary) |
| `POST activity/:rid/undo` | `workspace.manage` | the original row after undo |

The list orders `created_at DESC, id DESC`. `limit` is an integer from 1
to 50, 50 when absent; anything else, including empty, is 400 naming
`limit` (`002.7`). The route reads `limit + 1` rows and returns
`next_cursor` only when the extra row exists, else null. The cursor is the
unpadded base64url of the JSON `{"t": created_at as RFC 3339 with nanoseconds
in UTC, "i": id}`, bound to nothing else (a value that is not valid base64url,
not that JSON or has an unparsable `t` is 400 naming `before`; an empty or
repeated `before` or `limit` is 400 naming the field), so it works with
any `class` and `limit`; the next page is `WHERE (created_at, id) < (?, ?)`
written as the expanded comparison for SQLite. Coalescing changes only
`updated_at` and `refusal_count`, so it never reorders a page. `class`
accepts one action class or `unknown`; absent or empty means All; other
values or a repeated `class` are 400 naming `class` (`002.3`). A bad cursor
is 400 naming `before`. The 409 bodies of undo are `{code, reason?}`
without a row; the client refetches. A coordinator of another workspace is 404 (`005.3`). Rows carry
`undoable`, computed at read time (outcome `approved`, class `create_task`
or `move`, `undone_at` null, and not a move whose proposal outcome has
`noop: true`, and everything undo needs is readable per `003.9`: a create
row has a `target_task_id`, a move row's proposal exists and its
`outcome_json` parses with both `from_step_id` and `to_step_id`; the page's
move proposals are read in one query by id), `target_task_identifier` (the task's identifier such as KAN-431, read
through the seam's `GetTask`, null when the task is gone or the read fails)
and, on a move row, `from_step_name` (the proposal outcome's `from_step_id`
resolved through the seam's `GetStep`, null when the step is gone or the read
fails; both read once per row of the page, a failed read is logged at warn
and never fails the list), `actor_name` resolved from `actor_user_id` through the user
service at read time, and `undone_by_name` resolved from `undone_by` the
same way (null while the id is null). The row also carries `actor_missing` and
`undone_by_missing`, true only when the id is set and the user no longer
exists (the name is then null); the client shows the translated "A former
member" for them, so the server sends no display text. When the user service
fails, the names are null and the flags false for that page, a warn is
logged and the list still returns.

## Undo

`POST activity/:rid/undo`, managers only:

0. Take the in-process mutex keyed by the row id. The wait honours the
   request context; when the context ends first, the request returns the
   context error and writes nothing. The map entry is dropped when its last
   holder or waiter leaves.
1. Read the row (404 when absent or of another coordinator). Not
   `approved`, class not `create_task` or `move`, a move whose outcome
   has `noop: true`, or not readable per `003.9`: 409 `not_undoable`.
   `undone_at` set: 409 `already_undone`.
2. **Create.** Call `ArchiveTask(target_task_id)`, which also stops any
   agent working on the task (the dialog says so). `ErrTaskAlreadyArchived`
   and not found count as done. Any other error: 500, nothing written.
3. **Move.** Read the task and the proposal's `outcome_json.from_step_id`
   and `to_step_id` ([proposal kinds](proposal-kinds.md#approve)), then
   check in this order, stopping at the first that applies (`003.7`); a `GetTask`, step or `HasActiveSession` read error that is not a not-found is 500 with nothing
   written, and a not-found task is `archived`:
   a. task archived, or not found: 409 `undo_conflict` reason `archived`;
   b. task on `from_step_id` (a retry after a failed marker step, or a person
      who moved it back): counts as reversed; skip the call and go to the
      marker step (top-level step 4);
   c. task on `to_step_id`. Read the from step through the seam (`GetStep`),
      then: from step not found, 409 `undo_conflict` reason `step_deleted`;
      from step completing on enter, `step_done`; any session of the task
      starting or running (seam `HasActiveSession`, the same two states the
      task service blocks moves on), `agent_running`. Otherwise call
      `MoveTaskWithOptions(task, workflow, from_step_id, 0, opts)` with
      `opts.ExpectedWorkflowID` = the workflow just read, and
      `opts.EntryOptions.SkipStepPrompt` = true only when the from step
      auto-starts an agent (a step that does not needs no option: no prompt
      runs, and the task service refuses entry options for a step with no
      auto-start and no session), so no agent starts (`003.8`). Results:
      `ErrWIPLimitExceeded`, 409 reason `step_full`; the move accepted but the
      task queued behind the step's limit (result `WIPAdmitted` false), counts
      as moved back and the marker step (top-level step 4) runs; `ErrWorkflowResolutionConflict` or
      `ErrMoveConflict`, 409 reason `moved`; a session-blocked refusal that the
            pre-check missed (a session started in the window, reported by the task
      service as an unsentineled error) is not classified and, like any other
      error, is 500 with nothing written, so a retry re-checks. A move still
      pending on the task is not read: the move-conflict refusal above covers
      an optioned move, and a plain move (a from step that does not
      auto-start) clears the pending marker, which is accepted (`003.7`);
   d. any other step: 409 `undo_conflict` reason `moved`.

   The observed step is not fenced against a person moving the task in
   the window between the read and the move: the task service exposes no
   step-level compare-and-move to a caller without a Host command, and the
   window is one request. A manager who clicked Undo asked for the task to go
   back, so that residual race is accepted; the undo is logged at info with
   the step it found.

   An outcome with `noop: true` was rejected as `not_undoable` in step 1.
4. In one `withCoordinatorLock` transaction: `UPDATE ... SET undone_at=now,
   undone_by=? WHERE id=? AND undone_at IS NULL` (`undone_by` NULL with
   authentication off); zero rows matched is 409 `already_undone` when the row still exists and 404
when retention has deleted it meanwhile (the reversal stands); one row inserts the
   `undone` row with `undo_of_id` (`actor_user_id` NULL likewise). A
   coordinator deleted between the reversal and this step makes the lock
   return not found: 404, the reversal stands and nothing else is written.
5. Publish `coordinator.updated`.

The reversal (steps 2 and 3) and the marker (step 4) are two commits, as
`003.2` states: the task service owns its own transaction. When step 4's
transaction fails, the route returns 500, the task stays reversed and the
row stays undoable; the next Undo finds the reversal done (archive is
idempotent, and a task on `from_step_id` counts as moved back) and only runs
step 4. D20's same-transaction rule governs proposal decisions; undo is not one, so
the two commits above are the contract (`001.6`). The `undone` row carries the original row's `action_class`,
`target_task_id` and `proposal_id`, authorization `requires_approval`, and
the manager as `actor_user_id`.

Two concurrent undos of a create both reach step 2; archive is idempotent,
and step 4's compare-and-set lets one win (`003.4`). Two concurrent undos of
one row are serialised by an in-process mutex keyed by the row id, held
around steps 0 to 4, so the second re-reads `undone_at` set and returns 409
before calling the task service. It is not a database lock, because steps 2
and 3 write through the task service, which on SQLite would wait on a write
lock held by this request. A second backend process is not a supported
deployment; if one existed, step 4's compare-and-set still records one
undo, and a second move undo would find the task on `from_step_id`, skip
the call and lose that compare-and-set. Undo of a message or
resume is `not_undoable`; the UI shows "No undo" (`003.1`).

### Task service seam

`internal/coordinator/undo.go` defines the one interface undo uses, which task
04 reuses: `ArchiveTask(ctx, id)`, `GetTask(ctx, id)` (identifier, archived time,
workflow id, workflow step id), `MoveTaskWithOptions(ctx, id, workflowID,
stepID, position, opts)` (returning whether the task was admitted),
`GetStep(ctx, stepID)` (name, workflow id, auto-start, completes-on-enter, or
`ErrStepNotFound`) and `HasActiveSession(ctx, taskID)`. The backend wiring
adapts the task service for the first four and the workflow service's
`GetStep` (mapping `ErrWorkflowStepNotFound` to `ErrStepNotFound`) and the
task's session list for the last two; tests use a fake.

## Read tool

`list_coordinator_activity_kandev(limit?, before?)` is registered for every
phase-2 coordinator session ([permissions](permissions.md#tool-profile)).
The MCP action `coordinator.list_activity` resolves the coordinator from the
principal only; it takes no coordinator or workspace argument, so it cannot
read another coordinator's rows (`004.1`). It returns the list route's rows
without `actor_user_id`, `undone_by`, `actor_name` or `undone_by_name`, and
with both `created_at` and `updated_at` (`004.2`; a coalesced refusal's
`updated_at` is the time of its latest repeat), default limit
20, and refuses a limit outside 1 to 50 or a bad cursor naming the field
(`004.3`). A principal with no coordinator binding (a phase-1 conversation)
or whose coordinator was deleted gets a not-found refusal and no rows.

## Summary

`GET activity/summary?days=N`, N in 1 to 90, default 30, other values 400
naming `days`:

```json
{
  "days": 30,
  "earliest_row_at": "2026-08-02T10:11:00Z",
  "classes": {
    "create_task": {"proposed": 14, "approved": 12, "approved_with_edits": 3,
                    "rejected": 2, "failed": 0, "refused": 0, "undone": 1}
  }
}
```

One grouped query over `created_at >= now - N days`; `refused` sums
`refusal_count`; every class appears with zeros. `approved` counts every
`approved` row, edited or not; `approved_with_edits` is the subset with
`edited` true, so it is never larger than `approved`. `undone` counts
`undone` rows under the class they carry, the class of the row they
reverse, so only `create_task` and `move` can be non-zero. `unknown` appears
as a class only when it has a row created in the window.
A coordinator that does not exist is `ErrNotFound` (404 on the route, and the
`goals` and May do callers receive the error). `earliest_row_at` is the
oldest row of the coordinator at any age, `null` with none. May do reads
it with N = 30; goal baselines read the approved and rejected counts for the 7
days before the goal is set through `Store.ActivityCountsIn` on the locked
handle, not through this service function ([goals](goals.md#baselines)).

## Retention

With `features.coordinatorPhase2` on at startup, a ticker of a fixed 24-hour
interval is started with the other coordinator background work, and one run
happens in the startup pass, in its own goroutine so it never delays
readiness. The flag needs a restart to change, so a running ticker never
re-reads it and a restart with the flag off starts no ticker. A run deletes
`created_at < cutoff` in batches of 500 selected `ORDER BY created_at, id
LIMIT 500`, each batch its own short transaction, so a proposal write waits
for at most one batch. The cutoff (now minus 400 days) is computed once per
run. Runs never overlap: a run that starts while another is in progress
returns at once. It stops on context cancel. A batch error is logged at warn
and the run ends; the next run retries (`005.1`). With the flag off neither
the ticker nor the startup run starts, so no row is deleted by age while
phase-2 data is kept ([coordinators](coordinators.md#phase-2),
`AC-COORDINATOR-COORDINATORS-007.3`); the first run after a restart with the
flag on deletes whatever is past 400 days by then. Deleting a coordinator
deletes its rows whatever the flag.

## What it did UI

`apps/web/app/coordinator/queue/what-it-did.tsx`, below the phase-1 Queue
groups, fed by `hooks/domains/coordinator/use-activity.ts`:

- Loads the first page on mount; **Load more** appends by cursor; a
  `coordinator.updated` event refetches the first page and merges by id.
- A class filter select (All, the six classes and "Unknown action"); `?class=`
  in the URL preselects it, which May do's link uses; an unrecognised value
  selects All.
- Columns When, Action, Action class, How it was authorised, Undo, as in
  `AC-COORDINATOR-ACTIVITY-LOG-002.4`. How it was authorised shows
  "Requires approval" or "Denied", plus "Approved by <name>" ("Approved" when
  the actor is null and not missing, "Approved by A former member" when
  `actor_missing`), "with edits", "x N". A refused row's Action cell shows the
  reason text of its `reason_code` (`002.4`), keyed by code in all six
  locales, with "Refused." plus the code for a code with no text.
- Undo column: **Undo** for managers on rows with `undoable` true; "No undo"
  on every row whose action class is `message` or `resume`, whatever its
  outcome (`proposed`, `approved`, `rejected`, `failed` or `refused`), and
  for readers too; "Undone by <name>, <time>" on the original row that was
  reversed (`undone_at` set), in place of Undo, with `<name>` from
  `undone_by_name` and `<time>` from `undone_at` (`003.6`); nothing on any
  other row, including the separate row whose outcome is `undone`, which
  shows the undoer through `actor_name` in How it was authorised (`002.4`)
  (`003.1`). The 409 `undo_conflict` message shows inline by its `reason`:
  "It has moved since" for `moved`, `archived` and any unknown or absent reason, "An agent is working on
  it. Stop it, then undo." for `agent_running`, "The step it came from no
  longer exists." for `step_deleted`, "The step it came from is now a
  finishing step." for `step_done`, "The step it came from is full." for
  `step_full` (`003.7`). A 404 from undo shows "This action is no longer
  listed." and refetches. "Undone by" shows "Undone,
  <time>" when `undone_by_name` is null because `undone_by` is null.
  Undo first opens a confirmation dialog titled "Undo this?" whose text
  names the effect: for a created task "The task <identifier> will be
  archived. Any agent working on it will be stopped.", for a move "The task <identifier> will move back to
  <from step name>." ("the step it came from" when `from_step_name` is null; "the task" when `target_task_identifier` is null) It has two buttons, **Undo** and **Cancel**, with
  Cancel focused on open. Cancel or Escape closes it and sends nothing;
  Undo sends the request and closes it, and the outcomes below apply.
  The other two refusals: 409 `already_undone` (a double click, or another
  manager undid it first) shows no error; the list refetches, so the row
  shows "Undone by <name>, <time>" of whoever undid it. 409 `not_undoable`
  (reachable only from a direct API call, since `undoable` is computed
  from fields undo never changes; the UI handles it defensively) shows "This can no longer be undone" inline in place of
  the button and refetches the list. Any other error (500, network) shows
  "Undo failed. Try again." inline and keeps the Undo button.
- Empty states from `002.5`. Readers see no Undo (`002.6`).
- Phone width stacks each row as a card with the same fields.

## Phase 3 contract

Phase 3 may rely on, and phase 2 will not change without a new ADR:

- the `coordinator_activity` columns and outcome values above;
- a row per proposal decision written in the same transaction as it;
- `Service.ActivitySummary(ctx, coordinatorID, days)` and its route, with
  `approved` including `approved_with_edits` and `undone` counted under the
  reversed row's class;
- retention of at least 400 days.

## Security

- Undo is `workspace.manage`; list and summary are `workspace.read`.
- `detail` is untrusted text from the coordinator's proposal and is rendered
  as text.
- The read tool never returns user identities.

## Observability

Undo logs at info with the row, coordinator and task ids and the result.
Retention logs the deleted count. `coordinator_activity_rows_total` (expvar,
labelled by `outcome`) is incremented by `InsertActivity` after a successful
insert statement. It counts insert statements that succeeded, so a caller's
later rollback is not subtracted; no identifier is a label.

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
