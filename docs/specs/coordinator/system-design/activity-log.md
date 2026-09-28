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
| `actor_user_id` | text null | the manager for approved, rejected, undone |
| `reason_code` | text null | refusal code, rejection reason code, failure code |
| `detail` | text not null default '' | at most 1,000 characters, truncated on write |
| `edited` | boolean not null default false | approved with edits |
| `refusal_count` | integer not null default 1 | |
| `undone_at` | timestamp null | |
| `undone_by` | text null | |
| `undo_of_id` | text null | on an `undone` row, the row it reverses |
| `created_at`, `updated_at` | timestamp not null | UTC |

Indexes: `(coordinator_id, created_at DESC, id DESC)` for the list and
summary, `(coordinator_id, action_class, created_at)` for the filtered list,
`(created_at)` for retention. Coordinator delete and the workspace deletion
transaction delete the coordinator's rows (`005.1`).

`internal/coordinator/activity.go` is the only writer. Rows are never
updated except `undone_at`, `undone_by`, `updated_at` by undo and
`refusal_count`, `updated_at` by coalescing (`001.4`).

## Writes

Each write takes the caller's transaction (`tx`), so the row and the state
change commit together or not at all; a failed insert fails the
transaction and the caller returns the error (`001.6`).

| Site | Row |
| --- | --- |
| propose insert (every kind) | `proposed`, `requires_approval`, detail from the kind's one-line title |
| completion update to `approved` | `approved`, actor = `decided_by`, `edited` = `final_spec_json` differs from `spec_json` on any edited field, target = created or target task |
| reject | `rejected`, actor, `reason_code` = the reason (truncated to 1,000 in detail) |
| failure update to `failed` | `failed`, `reason_code` = the failure code, detail = the error |
| undo | `undone` row plus the marker on the original ([Undo](#undo)) |

The completion and failure updates are the claim-fenced updates of
[proposals](proposals.md#approve); the insert runs only when that update
matched a row, so a losing claimer writes no row. A retry of a `failed`
proposal that later settles writes another row. The phase-1 stall paths
and a manager's direct Resume or Send it back write nothing (`001.5`).

A proposal's action class is its kind's action: `create_task`, `resume`,
`message` or `move`.

## Refusals

`RecordRefusal(ctx, coordinatorID, workspaceID, actionClass, reasonCode)`
runs in its own transaction after the guard refuses:

1. `UPDATE coordinator_activity SET refusal_count = refusal_count + 1,
   updated_at = now WHERE id = (SELECT id FROM coordinator_activity WHERE
   coordinator_id = ? AND action_class = ? AND reason_code = ? AND outcome =
   'refused' AND created_at >= now - 60s ORDER BY created_at DESC, id DESC
   LIMIT 1)`.
2. No row matched: insert a `refused`, `denied` row.

Both run under the per-coordinator lock, so two concurrent refusals produce
one row with count 2. The action class of an unknown tool name is `unknown`
(`001.3`). A failure here is logged at warn; the refusal already happened.

## Routes

Under `/api/v1/workspaces/:id/coordinators/:cid/`, registered only with the
phase-2 flag on:

| Route | Scope | Result |
| --- | --- | --- |
| `GET activity?class=&before=&limit=` | `workspace.read` | `{rows, next_cursor}` |
| `GET activity/summary?days=` | `workspace.read` | [Summary](#summary) |
| `POST activity/:rid/undo` | `workspace.manage` | the original row after undo |

The list orders `created_at DESC, id DESC`; `limit` defaults to 50 and is
at most 50; the cursor is an opaque base64 of `(created_at, id)` and the
next page is `WHERE (created_at, id) < (?, ?)` written as the expanded
comparison for SQLite. `class` accepts one action class or `unknown`;
other values are 400 naming `class` (`002.3`). A bad cursor is 400 naming
`before`. A coordinator of another workspace is 404 (`005.3`). Rows carry
`undoable`, computed at read time (outcome `approved`, class `create_task`
or `move`, `undone_at` null, and not a move whose proposal outcome has
`noop: true`), and `actor_name` resolved from the user service at read time; a missing user
reads as "A former member".

## Undo

`POST activity/:rid/undo`, managers only:

1. Read the row (404 when absent or of another coordinator). Not
   `approved`, class not `create_task` or `move`, or a move whose outcome
   has `noop: true`: 409 `not_undoable`.
   `undone_at` set: 409 `already_undone`.
2. **Create.** Call `ArchiveTask(target_task_id)`. `ErrTaskAlreadyArchived`
   and not found count as done. Any other error: 500, nothing written.
3. **Move.** Read the task and the proposal's `outcome_json.from_step_id`
   and `to_step_id` ([proposal kinds](proposal-kinds.md#approve)), then
   check in this order, stopping at the first that applies:
   1. task archived, or not found: 409 `undo_conflict`;
   2. task on `from_step_id` (a retry after a failed step 4, or a person
      who moved it back): counts as reversed; skip the call and go to
      step 4;
   3. task on `to_step_id`: when `from_step_id` no longer exists, 409
      `undo_conflict`; otherwise `MoveTask(task, workflow, from_step_id,
      0)`, whose error is 500 with nothing written;
   4. any other step: 409 `undo_conflict`.

   An outcome with `noop: true` was rejected as `not_undoable` in step 1.
4. In one transaction: `UPDATE ... SET undone_at=now, undone_by=? WHERE id=?
   AND undone_at IS NULL`; zero rows is 409 `already_undone`; one row inserts
   the `undone` row with `undo_of_id`.
5. Publish `coordinator.updated`.

The reversal (steps 2 and 3) and the marker (step 4) are two commits, as
`003.2` states: the task service owns its own transaction. When step 4's
transaction fails, the route returns 500, the task stays reversed and the
row stays undoable; the next Undo finds the reversal done (archive is
idempotent, and a task on `from_step_id` counts as moved back) and only runs
step 4. The `undone` row carries the original row's `action_class`,
`target_task_id` and `proposal_id`, authorization `requires_approval`, and
the manager as `actor_user_id`.

Two concurrent undos of a create both reach step 2; archive is idempotent,
and step 4's compare-and-set lets one win (`003.4`). Two concurrent undos of
one row are serialised by an in-process mutex keyed by the row id, held
around steps 1 to 4, so the second re-reads `undone_at` set and returns 409
before calling the task service. It is not a database lock, because steps 2
and 3 write through the task service, which on SQLite would wait on a write
lock held by this request. A second backend process is not a supported
deployment; if one existed, step 4's compare-and-set still records one
undo, and a second move undo would find the task on `from_step_id`, skip
the call and lose that compare-and-set. Undo of a message or
resume is `not_undoable`; the UI shows "No undo" (`003.1`).

## Read tool

`list_coordinator_activity_kandev(limit?, before?)` is registered for every
phase-2 coordinator session ([permissions](permissions.md#tool-profile)).
The MCP action `coordinator.list_activity` resolves the coordinator from the
principal only; it takes no coordinator or workspace argument, so it cannot
read another coordinator's rows (`004.1`). It returns the list route's rows
without `actor_user_id`, `undone_by` or `actor_name` (`004.2`), default limit
20, and refuses a limit outside 1 to 50 or a bad cursor naming the field
(`004.3`).

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
as a class only when it has a row. `earliest_row_at` is the
oldest row of the coordinator at any age, `null` with none. May do reads
it with N = 30; goal baselines read the approved and rejected counts for 7
days through the same service function ([goals](goals.md#baselines)).

## Retention

While `features.coordinatorPhase2` is on, a daily ticker started with the
other coordinator background work (and one run in the startup pass)
deletes `created_at < now - 400 days` in batches of
500 by primary key, each batch its own short transaction, stopping on
context cancel. A batch error is logged at warn and the run ends; the next
run retries (`005.1`). With the flag off neither the ticker nor the startup
run starts, so no row is deleted by age while phase-2 data is kept
([coordinators](coordinators.md#phase-2), `AC-COORDINATOR-COORDINATORS-007.3`);
the first run after the flag returns deletes whatever is past 400 days by
then. Deleting a coordinator deletes its rows whatever the flag.

## What it did UI

`apps/web/app/coordinator/queue/what-it-did.tsx`, below the phase-1 Queue
groups, fed by `hooks/domains/coordinator/use-activity.ts`:

- Loads the first page on mount; **Load more** appends by cursor; a
  `coordinator.updated` event refetches the first page and merges by id.
- A class filter select (All plus the six classes); `?class=` in the URL
  preselects it, which May do's link uses.
- Columns When, Action, Action class, How it was authorised, Undo, as in
  `AC-COORDINATOR-ACTIVITY-LOG-002.4`. How it was authorised shows
  "Requires approval" or "Denied", plus "Approved by <name>", "with edits",
  "x N".
- Undo column: **Undo** for managers on rows with `undoable` true, "No undo" on
  approved message and resume rows, "Undone by <name>, <time>" on undone
  rows, and the 409 `undo_conflict` message "It has moved since" inline.
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
labelled by `outcome`) counts inserted rows; no identifier is a label.

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
