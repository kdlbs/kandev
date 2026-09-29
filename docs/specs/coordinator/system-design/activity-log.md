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
andand, on a move row, `from_step_id` (the proposal outcome's id, null when
absent). The server sends user ids (`actor_user_id`, `undone_by`) and no
display names or step names: the client resolves both ([What it did
UI](#what-it-did-ui), `002.8`, `003.10`), so the list never reads the user
service or the step store and a failure of either cannot affect it.

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
without `actor_user_id` and `undone_by`, and
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
it with N = 30; goal baselines read the approved and rejected counts for 7
days through the same service function ([goals](goals.md#baselines)).

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

`apps/web/app/coordinator/queue/what-it-did.tsx` and `what-it-did-row.tsx`,
below the phase-1 Queue groups, fed by
`hooks/domains/coordinator/use-activity.ts`. It renders only with
`features.coordinatorPhase2` on and mounts after the groups.

### Data

One list state per (coordinator, class filter): loaded rows, the last
page's `next_cursor`, a request generation, the message map and a status
(`loading | loaded | failed`).

- **Reads.** The first page loads on mount. **Load more** appends the page
  after the last loaded row's cursor and is ignored while a Load more is in
  flight. Every other refresh is a **re-read**: it fetches page 1, then follows
  `next_cursor` once for each further page that was loaded (so a list of three
  pages is re-read as three pages), and replaces the loaded rows and the cursor
  only when the last page has arrived. A re-read therefore never merges: rows
  are exactly what the server returned, in server order (`created_at DESC, id
  DESC`), the cursor is the last re-read page's, and a row that moved beyond
  the re-read span simply drops off the end. A failed re-read keeps the rows shown and sets no banner; only the
  first load and Load more have a failure state (below). The client sends no `limit` on any page (the route's 50). A re-read
  stops at a null cursor. A re-read that starts supersedes an in-flight Load more (its
  response is dropped) and Load more is disabled while a re-read is in flight,
  so rows gain no gap or duplicate. An event or reconnect during or after a failed first load is
  ignored; Retry decides.
- **Triggers.** A `coordinator.updated` event for the coordinator, a
  reconnect of the WebSocket, and every undo outcome that says the row's state
  may have changed (200, `already_undone`, `not_undoable`, 404) trigger a
  re-read. Re-reads never overlap: one triggered while another is in flight
  queues at most one trailing re-read. A filter change or a change of the selected coordinator discards
  the list and message map and starts a first load.
- **Stale responses.** Each first load, re-read and Load more carries the
  generation current when it started; the generation increments on a filter
  or coordinator change and on unmount. A response of an older generation is
  dropped whole, and an undo response for a row no longer listed, or under a
  newer generation, sets no message.

- `?class=` preselects the filter (May do's link uses it); an unrecognised
  value selects All and is left in the address until the filter changes. A
  filter change replaces the current history entry (no new entry) with
  `?class=<class>`, and removes `class` when All is chosen. The filter offers All, the six classes and "Unknown action".
- Names: the workspace member list (`listWorkspaceMembers`), read once per
  section mount and again once when a row's person is absent from the loaded
  list and the last read was more than 30 seconds ago (a member added after
  mount; the second read keeps the loaded state), gives `user_id` to `display_name`.
  The member list is the only name source: a person who holds access through
  an inherited role has no member row, and "a former member" therefore means
  "not in the workspace member list", an accepted limit. State is `loading | failed |
  loaded`; `loading` and `failed` render the no-person outcome forms in both
  the outcome line and the Undo cell ("Undone, <time>"), and `loaded` with the
  id absent renders "a former member" (`002.8`). A member with an empty or
  missing `display_name`, and a null `actor_user_id`, render the no-person
  form in every state.
- Step names and task availability come from `useCoordinatorTasks` through
  the Queue's `useCoordinatorAttention` result, which is extended to also
  return `tasks`, `loadedAt` and `error`; the Queue passes them to the section
  as props, so the section calls no hook of its own for them (a second instance would call `useAllWorkflowSnapshots`):
  `stepNameByWorkflowStep` (keyed `${workflowId}:${stepId}`) is flattened once
  into a step-id to title map (step ids are unique across workflows);  a
  `from_step_id` absent from the map is unknown. A `target_task_id` in `tasks` is available. The set becomes trusted the first time a result arrives with `loadedAt` set
  and `error` false, held in a section-level flag that a later failure does not clear; the hook sets `loadedAt` together with `error` true on a
  partial failure, so `loadedAt` alone never trusts the set (`002.9`).
- **List states.** While the first load is in flight the section shows
  `activityLoading` and no empty text. A failed first load (any error, including 403 and 404) shows
  `activityLoadFailed` with a **Retry** button (`activityRetry`) and never the
  empty text. A failed Load more keeps the rows and button and shows
  `activityLoadMoreFailed` beneath the list until the next attempt.

With status `loaded` and no rows the text is `activityEmpty` for All,
`activityFilteredEmpty` for a class.

### Rows

Columns When, Action, Action class, How it was authorised, Undo. The When cell
shows `created_at` (for a coalesced refusal too, so the column agrees with the
list order) as `formatRelative` (`lib/i18n/formats.ts`); its exact time is `formatDateTime` in the viewer's
locale and time zone, shown in a tooltip that opens on hover and on keyboard
focus (the cell is focusable); a refusal with `refusal_count` above 1
adds `activityLastRepeat` with the `updated_at` time. An empty or unparsable timestamp shows nothing. The Action cell text is chosen in this order: a refused row shows its
reason text; a rejected or failed row shows `detail` with its prefix
(`activityRejectedDetail` / `activityFailedDetail`), or `activityRejected` /
`activityFailed` alone when the detail is empty; any other row shows `detail`,
or the class label when it is empty. The text (line-clamped to two lines, the full text as the title
attribute, rendered as text) with the identifier link after it, separated by a single space (the link wraps
to its own line when the detail fills the clamp): the
identifier links to the task when `target_task_identifier` is set, "Open
task" when only the snapshots hold it, "Task no longer available" as plain
text under `002.9`, nothing when the row has no `target_task_id` (an identifier without an id is not shown). Phone width
stacks each row as a card (When and class, Action, authorisation, then a
full-width Undo).

Undo cell, first match wins: "Undone by <name>, <time>" when `undone_at` is
set (`003.6`); **Undo** for a manager when `undoable` is true; "No undo" on
every row of class `message` or `resume` whatever its outcome and for readers
too; nothing otherwise, including the `undone` outcome row, whose undoer shows
in How it was authorised. A row's failure message (`003.11`) renders below the
Undo button in a `role="status"` region, and renders even when the cell
would otherwise be empty.

### Copy table

All keys are in the `coordinator` namespace, six locales, each a whole
sentence with interpolation only for names, times, counts and codes (never a
noun phrase spliced into a sentence). The one exception is `activityFormerMember`,
which is a name value substituted for `{{name}}` in the outcome keys, translated
per locale as a short noun phrase that reads correctly after "by" in
`activityApprovedBy`, `activityRejectedBy`, `activityUndoneBy` and
`activityUndoneByAt`; a locale whose grammar cannot do that translates those
keys so they read correctly with it. "x" is the ASCII letter. No em dash.

| Key | English |
| --- | --- |
| `activityTitle` | What it did |
| `activityEmpty` | It has not done anything yet. |
| `activityFilteredEmpty` | Nothing matches this filter. |
| `activityFilterLabel` | Action class |
| `activityFilterAll` | All |
| `activityClassCreateTask` / `StartAgent` / `Message` / `Move` / `Resume` / `Stop` / `Unknown` | Create task / Start agent / Message task / Move task / Resume task / Stop task / Unknown action |
| `activityAuthRequires` / `activityAuthDenied` | Requires approval / Denied |
| `activityAuthDeniedRepeated` | Denied x {{count}} |
| `activityApprovedBy` / `ApprovedByEdited` | Approved by {{name}} / Approved by {{name}}, with edits |
| `activityApproved` / `ApprovedEdited` | Approved / Approved, with edits |
| `activityRejectedBy` / `activityRejected` | Rejected by {{name}} / Rejected |
| `activityFailed` | Failed |
| `activityUndoneBy` / `activityUndone` | Undone by {{name}} / Undone |
| `activityFormerMember` | a former member (the `{{name}}` value) |
| `activityRejectedDetail` / `activityFailedDetail` | Rejected: {{detail}} / Failed: {{detail}} |
| `activityRefusedBindingInvalid` | Its tool settings could not be read. |
| `activityRefusedNotInProfile` | It called something it is not allowed to use. |
| `activityRefusedPolicyDenied` | A manager has set this action to Denied. |
| `activityRefusedCode` / `activityRefused` | Refused. {{code}} / Refused. |
| `activityTaskGone` / `activityOpenTask` | Task no longer available / Open task |
| `activityUndoAction` / `activityNoUndo` | Undo / No undo |
| `activityUndoneByAt` / `activityUndoneAt` | Undone by {{name}}, {{time}} / Undone, {{time}} |
| `activityUndoTitle` | Undo this? |
| `activityUndoConfirmCreate` / `...CreateNoId` | the two create sentences of `003.10` |
| `activityUndoConfirmMove` / `...MoveNoStep` / `...MoveNoId` / `...MoveNoIdNoStep` | the four move sentences of `003.10` |
| `activityUndoConfirm` / `activityUndoCancel` | Undo / Cancel |
| `activityConflictMoved` (moved, archived, unknown or absent reason) | It has moved since |
| `activityConflictAgentRunning` | An agent is working on it. Stop it, then undo. |
| `activityConflictStepDeleted` | The step it came from no longer exists. |
| `activityConflictStepDone` | The step it came from is now a finishing step. |
| `activityConflictStepFull` | The step it came from is full. |
| `activityNotUndoable` | This can no longer be undone |
| `activityUndoFailed` | Undo failed. Try again. |
| `activityGone` | This action is no longer listed. |
| `activityLoadMore` | Load more |
| `activityLoading` | Loading what it did |
| `activityLoadFailed` / `activityRetry` | What it did could not be loaded. / Retry |
| `activityLoadMoreFailed` | More could not be loaded. Try again. |
| `activityLastRepeat` | Last repeated {{time}} |

Traditional Chinese comes from `pnpm run i18n:zh-hant`.

### Undo flow

Undo opens the dialog (`003.10`). The dialog is built on the base
`AlertDialog` with `enterConfirms` false: Enter activates whichever button has
focus (Cancel on open), so the dialog has no default confirm, Escape, a backdrop click and Cancel
close it and send nothing, and focus returns to the row's Undo button (or to
the section heading when the row is gone or its Undo became text). Confirming closes the dialog and
sends `POST activity/:rid/undo`, marking that row's Undo `aria-disabled` (it
keeps focus and ignores activation) until the response settles and, after a
200, until the re-read it triggers settles, so a row has at most one request in flight; another row's Undo may
run at the same time. Results:

- 200: re-read; the returned original row is not used (the re-read supplies
  it); no message. If that re-read fails, Undo is clickable again (a second Undo
  answers 409 `already_undone`).
- 409 `already_undone`: re-read, no message.
- 409 `undo_conflict`: the text by `reason`; an unknown or absent reason (a code the client predates, or no `reason`) reads
  `activityConflictMoved`. No re-read, so the row stays and Undo stays
  clickable.
- 409 `not_undoable`: `activityNotUndoable`, re-read at once.
- 404: `activityGone` in the section notice, re-read at once.
- Anything else, including a network error, a 403 and a 409 with an
  unrecognised `code`: `activityUndoFailed`, no re-read, Undo stays clickable.

The message map is `Map<rowId, {text, survives}>` plus the section notice (one more entry, same lifecycle, no row). `not_undoable` and
404 set `survives` true. `survives` is consumed only by the first re-read that starts after the
refusal settles (an in-flight one that began earlier, and a trailing one
already queued, count: the trailing one is that re-read), when it settles
(success, failure or superseded by a newer generation): the message stays
through it and `survives` becomes false. A re-read triggered by anything else (an event, a
reconnect) neither clears nor consumes it while `survives` is true. Any
message with `survives` false is cleared by the next re-read that completes
with success (a failed one leaves it), a
filter or coordinator change, or a new confirmed Undo of that row (`003.11`);
Load more clears nothing. A second failure on one row replaces its message.

## Phase 3 contract

Phase 3 may rely on, and phase 2 will not change without an ADR:

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
insert statement. It counts succeeded insert statements (a later rollback is
not subtracted); no identifier is a label.

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
