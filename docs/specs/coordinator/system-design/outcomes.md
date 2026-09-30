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
decided_at, proposal_id)` and on `(final, graded_at)`.

`coordinator_feedback`: `id`, `coordinator_id`, `kind` (`rejected`, `edited`,
`undone`, `moved_back`), `proposal_id`, `turn_id`, `user_id`, `reason_code`,
`transition_key` (empty except `moved_back`), `created_at`. Unique index on
`(proposal_id, kind, transition_key)`. No text column exists, so `002.1`
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
decision time column. A re-grade and an undo never change it (an undo changes
`decision` to `undone` only). `decision` is `edited` when `edited_fields` is
non-empty and `approved` otherwise; a `returned` proposal is graded through the
same insert.

`edited_fields` is the sorted list of field names whose approved value differs
from the proposed; values are never read into the row (`001.1`). The reject
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
   `coordinator_outcomes.task_id` or in `coordinator_activity.task_id` with the
   created or moved action; it resolves the proposals of the task and enqueues
   each.
3. **Sweep:** every 24 hours (and at start) it enqueues, from
   `coordinator_proposals` left-joined to `coordinator_outcomes`, decided
   proposals (status `approved`, `rejected` or `returned`, never `pending`, `approving` or `failed`) whose `updated_at` is within 400 days and
   that have no outcome row or whose row has `final = false`, in batches of 200
   ordered by `(graded_at NULLS FIRST, proposal_id)`. A proposal decided before
   recording began therefore gets a row on the first sweep within that window.

The queue never blocks the publisher; a full queue (1000) drops the enqueue and
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
(`001.3`); such a row is graded once at decision and the sweep skips it, so the
sweep set is only approved task-creating proposals whose task is not yet merged
or dropped and whose decision is within 400 days. A `merged`
row's `merged_at` is set once from the first observed merged pull request
time and never moves.

**Reopen count.** `Grade` runs in one transaction per proposal (a write
transaction, so two graders serialize on the row). It reads the stored result,
computes the new one, and when the stored result is `done` and the new one is not
it adds one to `reopen_count` in that same transaction before writing the new
result; the second grader then reads the already-updated result and adds nothing
(`001.3`). No other statement writes `task_result` or `reopen_count`.

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
  nothing. The table is keyed by session, so a task that never had a session
  has no row and `moved_back` is a signal for tasks with a session only.
  History rows are written asynchronously and may follow the event, so a scan
  that finds no new row is retried by the in-process queue after 2 minutes, 10
  minutes and 1 hour, and a separate daily **moved-back scan** (own pass, not
  the outcome sweep set, so `final` rows are included) scans tasks with a
  coordinator created or moved activity row in the last 30 days, in batches of
  200 ordered by `task_id`. Every path reads the same rows and inserts under the
  same key, so any of them may run first.

Each candidate goes through the manager check the automatic class already uses
for its raiser (`automatic_approve.go`, `workspace.manage` in the coordinator's
workspace). A false result
or an error stores nothing and counts `coordinator_override_ignored_total{reason}`
with reason `not_manager`, `principal`, `system`, `actor_unknown` or `authz_error`
(`002.3`); for decisions the kind comes from the observer's actor, for moves from the history row above. A manager
override inserts one feedback row with `INSERT ... ON CONFLICT DO NOTHING` on
the unique index (`002.2`), so redelivery, a grader run or a second observer
never duplicates one.

`moved_back` compares the destination step's position with the position of the
step the coordinator's approved action left the card in, both read from the
workflow's step order when the candidate is processed, and requires the row time
to be later than the action's time (`002.4`). `transition_key` is the history
row id, so two different moves back both count while one row seen twice does
not. The observation names the approved proposal of the coordinator that
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
| Approval without edit | `decision = 'approved'` over manager-decided proposals (`automatic = false`) decided in the window |
| Override recurrence | feedback rows of the window whose pattern key also appears in the previous 30 days before that row, over all rows of the window; the pattern key is `(kind, reason_code, proposal kind)`, and for `moved_back` the destination step id replaces `reason_code` |
| Dollars per merged task | priced cost of coordinator turns started in the window (joined from usage), over tasks with `merged_at` in the window |
| Median wait | median of `decided_at - proposal.created_at` |
| Agreement | items rated and replayed, per shadow dream 006.4 |

A zero denominator gives `null` with `no_data`; an unknown cost among the inputs
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
