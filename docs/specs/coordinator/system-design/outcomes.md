---
id: coordinator-outcomes-design
title: Coordinator outcomes and overrides design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-OUTCOMES-001
  - REQ-COORDINATOR-OUTCOMES-002
  - REQ-COORDINATOR-OUTCOMES-003
---

# Coordinator outcomes and overrides System Design

## Purpose and boundaries

Outcomes back-fills what came of each decided proposal. Overrides captures each
manager correction. Both only read stored rows and write their own tables; they
sit beside the proposal, approval and task paths as observers and never
change them. Both record whether or not the phase 3.1 flag is on
([turn ledger](turn-ledger.md#gating)); the measures read side is flagged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-OUTCOMES-001` | [Outcome rows](#outcome-rows), [Grading paths](#grading-paths), [Task result](#task-result) |
| `REQ-COORDINATOR-OUTCOMES-002` | [Override capture](#override-capture), [Reason codes](#reason-codes) |
| `REQ-COORDINATOR-OUTCOMES-003` | [Measures](#measures) |

## Tables

`coordinator_outcomes`: `proposal_id` (primary key), `coordinator_id`,
`turn_id` (nullable), `kind`, `decision` (`approved`, `edited`, `rejected`,
`returned`, `undone`), `automatic` (bool, true when the decision was the
automatic-approval class), `decided_at`, `edited_fields` (JSON array of names),
`reason_code`, `task_id` (nullable), `task_result` (nullable), `cost_subcents`
(nullable), `reopen_count`, `approved_at`, `merged_at` (nullable),
`last_step_id`, `final` (bool), `graded_at`. Index on `(coordinator_id,
decided_at, proposal_id)` and on `(final, graded_at)`. `approved_at` is the
`created_at` of the proposal's approved created or moved activity row
(`coordinator_activity` with `action_class` `create_task` or `move`, `outcome`
`approved`, joined by `proposal_id`), and NULL when the proposal has none.

`coordinator_feedback`: `id`, `coordinator_id`, `kind` (`rejected`, `edited`,
`undone`, `moved_back`), `proposal_id`, `turn_id`, `user_id`, `reason_code`
(`none` for `moved_back`), `proposal_kind` (the proposal's `kind`), `to_step_id`
(destination step of a `moved_back`, empty otherwise, informational),
`transition_key` (empty except `moved_back`), `created_at` (the decision time for decisions; for `moved_back` the history row's `created_at`, so a late scan or retry does not move the observation in the recurrence window or retention). Unique index on
`(proposal_id, kind, transition_key)`, and an index on `(coordinator_id,
created_at)`. A third table, `coordinator_moveback_seen` (`history_row_id`
primary key, `coordinator_id`, `seen_at`), remembers which step history rows were
judged once (see [override capture](#override-capture)).

**Lifecycle.** Rows of all three tables are deleted with their coordinator and
with its workspace, inside the same transactions as the other phase 3 rows
(`deleteCoordinatorPhase3Rows`, defined in `store_wake.go` and called from `store.go` and `store_workspace_delete.go`). Retention is 400
days: the daily sweep deletes outcome rows by `decided_at`, feedback rows by
`created_at` and seen rows by `seen_at` older than 400 days, in batches of 200. No text column exists, so `002.1`
("no free text") holds structurally.

## Outcome rows

An outcome row is derived, never edited: `Grade(proposalID)` recomputes the
whole row from the proposal, its decision record, the task and its usage, and
upserts with `INSERT ... ON CONFLICT (proposal_id) DO UPDATE` (`001.2`). A
recompute that reads the same facts writes the same values, so two graders
racing produce one row with one value. The only non-derivable field is
`reopen_count`, handled below.

`decided_at` is the time of the first recorded decision and is written only by
the insert: the insert takes the queue entry's `decidedAt` (the hook time) when it is non-zero, and a row created by the sweep or the task-transition path
(below) stores the proposal's `updated_at`, since `coordinator_proposals` has no
decision time column. The queue is a keyed set: enqueueing an id already
queued keeps the queued entry but takes the new `decidedAt` when the queued one
is zero, so a hook time is never lost to an earlier sweep entry. A re-grade and an undo never change it (an undo changes
`decision` to `undone` only). `UndoActivity` enqueues with `decidedAt` zero, so an
undo that grades first (the approval's enqueue was dropped) stores the proposal's `updated_at` like the sweep does, never the undo time. The upsert is guarded: `DO UPDATE ... WHERE
coordinator_outcomes.final = false OR excluded.decision = 'undone' OR
(coordinator_outcomes.turn_id IS NULL AND excluded.turn_id IS NOT NULL)`. The `SET`
list is per column: `decision`, `edited_fields`, `reason_code`, `task_result`, `cost_subcents`,
`approved_at`, `merged_at`, `last_step_id`, `final` and `graded_at` take the
recomputed value only when `coordinator_outcomes.final = false`, else keep the
stored one (`CASE WHEN coordinator_outcomes.final THEN coordinator_outcomes.x ELSE
excluded.x END`); `decision` alone also takes `'undone'` on a final row, and
`turn_id` is `COALESCE(coordinator_outcomes.turn_id, excluded.turn_id)`. `decided_at`
and `reopen_count` are never in the `SET` list; `reopen_count` is written only by the
reopen increment under [Task result](#task-result), and the insert writes it as 0. So an undo of a `move` row, of a `merged` or `dropped`
row, or one that races the archive of its created task (`reverseCreate` commits
before `markUndone`, so a grade may already have stored `dropped`) still lands
as `undone`. `turn_id` is read from `coordinator_proposals.turn_id` on every
grade. `decision` is `edited` when `edited_fields` is
non-empty and `approved` otherwise, for a proposal whose status is `approved`;
`rejected`, `returned` and `undone` come from the status and the activity row
as [grading paths](#grading-paths) derives them, which is the one rule.

`edited_fields` is the sorted list of top-level JSON keys whose value in
`final_spec_json` differs from `spec_json`; a NULL `final_spec_json` means no
edits. One function (`outcomes.EditedFields`) computes it and the hook payload
uses the same function, so an `edited` observation and an `approved` row cannot
disagree; values are never read into the row (`001.1`). An edit whose approved
execution ends `failed` leaves the proposal open: no decision has taken effect, so the hook fires no `edited` observation and no row exists (`AC-002.1`). The reject
reason code comes from [reason codes](#reason-codes). A proposal with no
decision has no row (`001.5`).

## Grading paths

No decision event exists today, so the decision paths gain a post-commit hook: an
optional `DecisionObserver` on the coordinator service (nil-safe, a no-op when
nothing registers) called by every path that decides a proposal (a decided proposal has status `approved`, `rejected` or `returned`; `pending`, `approving` and `failed` are open and have no outcome row, `001.5`):
`ApproveProposal` and `finishClaim` (`approve.go`), the automatic class
(`approveAutomatically`, `automatic_approve.go`, with `automatic = true`),
`RejectProposal` (`reject.go`), `ReturnProposalTx` (`store_reply.go`, decision
`returned`) and `UndoActivity`/`markUndone` (`undo.go`), after their
transaction commits, with `{proposalID, decision, automatic, actor,
editedFields, reasonCode}`. Override capture uses the whole payload. Grading
uses only `proposalID` and the hook time: the grader queue entry is
`{proposalID, decidedAt}` with `decidedAt` zero on every entry that does not come
from the hook, and **`Grade` derives every other value from stored rows**, so a
sweep-run grade and a hook-run grade write the same row. The derivation:
`automatic` is `coordinator_proposals.decided_automatically`; `decision` is
`rejected` or `returned` from the status, `undone` when the proposal's created or
moved activity row has `undone_at` set, `edited` when `edited_fields` is
non-empty and `approved` otherwise; `edited_fields` is the sorted names that
differ between `spec_json` and `final_spec_json`; `reason_code` is
`reasons.Code` of `reject_reason` (the text is read and dropped, never copied).
A call the observer makes never fails the decision, and a panic in it is
recovered and counted.

Three paths call `Grade` through one in-process queue (`grader.Enqueue(id)`,
keyed set so a proposal is queued once at a time, a worker pool of 1):

1. **Decision:** the `DecisionObserver` hook above.
2. **Task transition:** the grader observes `task.moved` (`events.TaskMoved`, a
   step change) and `task.state_changed` for tasks present in
   `coordinator_outcomes.task_id` or in `coordinator_activity.target_task_id` with
   `action_class` `create_task` or `move` and `outcome` `approved`; it resolves the proposals of the task and enqueues
   each.
3. **Sweep:** every 24 hours (and at start) it enqueues, from
   `coordinator_proposals` left-joined to `coordinator_outcomes`, decided
   proposals (status `approved`, `rejected` or `returned`, never `pending`, `approving` or `failed`) whose `updated_at` is within 400 days and
   that have no outcome row or whose row has `final = false`, in batches of 200
   ordered by `(graded_at NULLS FIRST, proposal_id)`; the set also includes final rows whose `turn_id` is NULL while the proposal's is not. A proposal decided before
   recording began therefore gets a row on the first sweep within that window.

The grader worker, the sweep ticker and the delayed moved-back retry timers are
started and stopped by the coordinator service's lifecycle (no goroutine
outlives `Stop`, checked with goleak); a pending retry timer is not a queue
entry until it fires, so it does not count against the 1000, and a restart
loses it, which the daily moved-back scan recovers for tasks whose coordinator action is within the scan's 30 days; a move-back on an older card whose retry is lost is accepted as unobserved. The queue never blocks the publisher; a full queue (1000) drops the enqueue and
counts `coordinator_outcome_grade_failed_total{reason="queue_full"}`; the sweep
recovers it. A read error keeps the earlier row, counts the reason
(`task_read`, `proposal_read`, `usage_read`), and the next pass retries
(`001.5`).

## Task result

A decided proposal made automatically has `automatic = true`; the measures and
replay treat it as a system decision, not a manager one (see [Measures](#measures)
and [replay](replay.md)).

`TaskResult(task)` is a pure function of the task's state, its linked pull
request states, its step's completion flag and its latest session state, in
this precedence (Terminology of outcomes): merged (any linked pull request
merged), done (the step completes tasks), failed (latest session failed and
not done), dropped (archived and not done or merged), else open. A task that
was deleted keeps the last stored result (`001.5`).

`final` is set when the decision is `rejected`, `returned` or `undone`, when the
proposal kind creates no task, or when the task result is `merged` or `dropped`
(`001.3`); such a row is graded once at decision and the sweep skips it, so for
decided proposals that have a row, the sweep re-grades only non-final rows (approved task-creating proposals whose task is not yet merged
or dropped, within 400 days); the sweep set of [grading paths](#grading-paths) also holds proposals with no row and the final rows whose `turn_id` is still empty. A `merged`
row's `merged_at` is set once from the earliest merged timestamp the linked pull
requests carry (not the time a grader saw it) and never moves; a task that
was already deleted when first graded stores `task_result` NULL and a final
row, so it is not re-swept.

**Reopen count.** `Grade` runs in one transaction per proposal. The row is
locked before the stored result is read: SQLite's write transaction serializes
graders, and PostgreSQL takes `pg_advisory_xact_lock(hashtext(proposal_id))`
first (covering the first insert as well). It reads the stored result,
computes the new one, and when the stored result is `done` and the new one is `open` or `failed`
it adds one to `reopen_count` in that same transaction before writing the new
result, only when the stored row is not final (an undone row holding `done` never counts a reopening); the second grader then reads the already-updated result and adds nothing
(`001.3`). No other statement writes `reopen_count`, and `task_result` is written only by the guarded upsert after this step.

**Cost.** `cost_subcents` is the sum of priced usage rows of sessions of the
created task; when the task has no usage rows or one row is unpriced the
column is NULL, never 0 (`001.4`). Time to merge is `merged_at - approved_at`,
computed on read.

## Override capture

`overrides.Capture` observes:

- the `DecisionObserver` hook with decision `rejected`, `edited` or `undone`;
- `task.moved` for tasks the coordinator created or moved (the same activity
  lookup as the grader). The event only triggers a scan of that task; it is never
  matched to a history row. The scan reads the step-transition history rows
  (`SessionStepHistory`: `trigger`, nullable `actor_id`, `created_at`, row id) of
  every session of the task with `created_at` later than the coordinator action's
  time, in order `(created_at, id)`, and treats each row as one candidate whose
  `transition_key` is the row id. The mover is `actor_id`: a person only when it
  is a user id of the workspace. Engine, agent, undo, plugin and queue-promotion
  moves store a nil `actor_id`, so a nil or non-user value is counted
  `coordinator_override_ignored_total{reason="actor_unknown"}` and stores
  nothing. The table is keyed by session and a row is written only when the task had a
  primary or active session at the moment of the move (the workflow service writes
  nothing for a sessionless move), so a card moved while none of its sessions was
  active has no row and `moved_back` is a signal for moves made with an active
  session only. A test moves a card with only a completed session and asserts no
  observation and no error.
  History rows are written asynchronously and may follow the event, so a scan
  that finds no unseen row later than the event that triggered it (a history row of the task with `created_at` at or after the event time and
  absent from `coordinator_moveback_seen`) is retried by the in-process queue after 2 minutes, 10
  minutes and 1 hour, and a separate daily **moved-back scan** (own pass, not
  the outcome sweep set, so `final` rows are included) scans tasks with a
  coordinator created or moved activity row in the last 30 days, in batches of
  200 ordered by `task_id`. Every path reads the same rows and inserts under the
  same key, so any of them may run first.

A candidate is judged once, in this order: (1) the position check of `moved_back` below (a step no longer in the workflow, an equal or later position, or a row not later than the coordinator action ends here: nothing stored, nothing counted, the candidate marked seen); (2) the actor: a nil or non-user `actor_id` is counted `actor_unknown` and marked seen; (3) the manager check. So the ignored counter counts only user moves back, never engine or forward moves. Marking seen inserts the row id into `coordinator_moveback_seen`
(`ON CONFLICT DO NOTHING`) in the same transaction as the feedback insert or the
ignored count, and a candidate already seen is skipped, so rescans neither
recount `coordinator_override_ignored_total` nor store an observation for a
user promoted to manager later. Every judged candidate is marked seen except one whose manager check errored
(`authz_error`): it stays unseen, is counted, and is retried by the next scan that reaches its task. Each candidate that reaches step (3) goes through the manager check the automatic class already uses
for its raiser (`automatic_approve.go`, `workspace.manage` in the coordinator's
workspace). A false result
or an error stores nothing and counts `coordinator_override_ignored_total{reason}`
with reason `not_manager`, `principal`, `system`, `actor_unknown` or `authz_error`
(`002.3`); for decisions the kind comes from the observer's actor, for moves from the history row above. A manager
override inserts one feedback row with `INSERT ... ON CONFLICT DO NOTHING` on
the unique index (`002.2`), so redelivery, a grader run or a second observer
never duplicates one.

`moved_back` compares the destination step's position with the position of the
step the coordinator's approved action left the card in (for a `move` the
proposal's `to_step_id`; for `create_task` the `step_id` of `final_spec_json`,
`spec_json` when that is NULL, and the workflow's start step when it is empty), both read from the
workflow's step order when the candidate is processed, and requires the row time
to be later than the action's time (`002.4`). `transition_key` is the history
row id, so two different moves back both count while one row seen twice does
not. The observation names the approved proposal, whose activity row is not undone (`undone_at` NULL), of the coordinator that
created or moved the card with the greatest action time (the `created_at` of its
created or moved activity row) not later than the row's `created_at`, ties by
`proposal_id` descending; that choice is fixed by the row, so every path names the same
proposal and the unique key `(proposal_id, kind, transition_key)` admits one
observation per row (`002.2`). A step no longer in the workflow, or an equal or
later position, stores nothing.

## Reason codes

`reasons.Code(coded, text)` returns one of `duplicate`, `wrong_target`,
`wrong_timing`, `not_wanted`, `too_broad`, `other`, `none`. A coded value in
that set wins; text with no code maps to `other`; empty maps to `none`; the text
itself is dropped after mapping and never persisted in either table (`002.5`).

## Access

Neither table has a tool, and the coordinator tool profile has no route that
reads them (`002.6`). Readers are the measures and the report builder, both
behind workspace-member authorisation on the HTTP layer.

## Measures

`GET /coordinators/:id/measures?days=N` (flag-gated, 404 otherwise) accepts one
`days` of 1 to 90, default 30; empty, repeated, non-integer or out of range is
400 naming `days`; a coordinator of another workspace is not found; a
coordinator principal is 403 (`003.3`). The response holds the five measures,
each `{value, numerator, denominator, null_reason}`:

| Measure | Computation |
| --- | --- |
| Approval without edit | the subset of the denominator rows with `decision` in (`approved`, `undone`) and empty `edited_fields`, over rows with `automatic = false` and `decision` in (`approved`, `edited`, `rejected`, `undone`) and `decided_at` in the window (`returned` is not counted) |
| Override recurrence | feedback rows of the window whose pattern key also appears in the previous 30 days before that row, over all rows of the window; the pattern key is `(kind, reason_code, proposal_kind)` read from the feedback row (`reason_code` is `none` for `moved_back`); "before" is strict: another row of the same coordinator and key with `(created_at, id)` less than the row's and `created_at` within 30 days before it, never the row itself |
| Dollars per merged task | priced cost of coordinator turns started in the window (joined from usage), over tasks with `merged_at` in the window |
| Median wait | median of `decided_at - proposal.created_at` over manager-decided proposals (`automatic = false`) whose `decision` is `approved`, `edited`, `rejected` or `undone` (an undone proposal was first approved); `returned` proposals are not counted, as no decision was taken |
| Agreement | items rated and replayed, per shadow dream 006.4 |

The window is the rolling `days` x 24 hours ending at the request time (UTC); dollars are `cost_subcents / 10000`. A zero denominator gives `null` with `no_data`; an unknown cost among the inputs
`cost_unknown`; fewer than 5 rated items `too_few` (`003.2`). Dollars per merged task includes the cost of dream turns, since they are
coordinator turns, counted by turn start time, and of replay runs (their
`cost_subcents` by result row `created_at`); shadow items count for agreement
by their dream's start time. The median for an
even count is the mean of the two middle values rounded down to the second;
computed in Go from a bounded set (at most 2000 proposals in the window, newest
first, the count reported).

## Screens

The five measures render in the Learning section
([shadow dream](shadow-dream.md#learning-section)): a value with its counts, "Not
enough data yet" with the reason for `null`, a loading placeholder and a Try
again failure state that shows no value (`003.4`).

## Error handling

| Failure | Behaviour |
| --- | --- |
| Grade read error | Keep earlier row, count, retry next pass |
| Duplicate observer | Unique index absorbs |
| Authz read error | Treated as not manager, counted |
| Measures query error | 500 with the standard error; the screen shows Try again |

## Testing

Grade idempotence under two goroutines; final rows never change; reopen once
under concurrent graders; cost NULL vs zero; override uniqueness and
redelivery; moved-back ordering matrix; the reason-code table; the measures
ordering and null reasons; the median parity case; window validation matrix.
