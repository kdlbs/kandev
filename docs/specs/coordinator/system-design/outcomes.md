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
`returned`, `undone`), `decided_at`, `edited_fields` (JSON array of names),
`reason_code`, `task_id` (nullable), `task_result` (nullable), `cost_micros`
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

`edited_fields` is the sorted list of field names whose approved value differs
from the proposed; values are never read into the row (`001.1`). The reject
reason code comes from [reason codes](#reason-codes). A proposal with no
decision has no row (`001.5`).

## Grading paths

Three paths call `Grade` through one in-process queue (`grader.Enqueue(id)`,
keyed set so a proposal is queued once at a time, a worker pool of 1):

1. **Decision:** the proposal decision path publishes the existing
   `coordinator.proposal.decided` event; the grader observes it.
2. **Task transition:** the grader observes `task.step_changed` for tasks
   present in `coordinator_outcomes.task_id` or in
   `coordinator_activity.task_id` with the created or moved action; it
   resolves the proposals of the task and enqueues each.
3. **Sweep:** every 24 hours (and at start) it enqueues proposals decided in
   the last 400 days with `final = false`, in batches of 200 ordered by
   `(graded_at, proposal_id)`.

The queue never blocks the publisher; a full queue (1000) drops the enqueue and
counts `coordinator_outcome_grade_failed_total{reason="queue_full"}`; the sweep
recovers it. A read error keeps the earlier row, counts the reason
(`task_read`, `proposal_read`, `usage_read`), and the next pass retries
(`001.5`).

## Task result

`TaskResult(task)` is a pure function of the task's state, its linked pull
request states, its step's completion flag and its latest session state, in
this precedence (Terminology of outcomes): merged (any linked pull request
merged), done (the step completes tasks), failed (latest session failed and
not done), dropped (archived and not done or merged), else open. A task that
was deleted keeps the last stored result (`001.5`).

`final` is set when the result is `merged` or `dropped` (`001.3`). A `merged`
row's `merged_at` is set once from the first observed merged pull request
time and never moves.

**Reopen count.** `reopen_count` increments once per observed leave-done
transition. The grader keeps `last_step_id`; a grade that sees the previous
result `done` and the current result not `done`, in one conditional update
`SET reopen_count = reopen_count + 1, task_result = ? WHERE proposal_id = ?
AND task_result = 'done'`, counts it; the second grader finds the result no
longer `done` and adds nothing (`001.3`).

**Cost.** `cost_micros` is the sum of priced usage rows of sessions of the
created task; when the task has no usage rows or one row is unpriced the
column is NULL, never 0 (`001.4`). Time to merge is `merged_at - approved_at`,
computed on read.

## Override capture

`overrides.Capture` observes:

- `coordinator.proposal.decided` with decision `rejected`, `edited` or `undone`;
- `task.step_changed` for tasks the coordinator created or moved (the same
  activity lookup as the grader).

Each candidate calls `authz.IsManager(userID, workspaceID)`. A false result
or an error stores nothing and counts `coordinator_override_ignored_total{reason}`
with reason `not_manager`, `principal`, `system` or `authz_error`
(`002.3`); the actor kind comes from the event's principal field. A manager
override inserts one feedback row with `INSERT ... ON CONFLICT DO NOTHING` on
the unique index (`002.2`), so redelivery, a grader run or a second observer
never duplicates one.

`moved_back` compares the destination step's position with the position of the
step the coordinator's approved action left the card in, both from the
workflow's step order at the time of the event, and requires the move time to
be later than the action's time (`002.4`). `transition_key` is the step
transition's id, so two different moves back both count while one event
redelivered does not. A step no longer in the workflow, or an equal or later
position, stores nothing.

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
| Approval without edit | `decision = 'approved'` over decided proposals decided in the window |
| Override recurrence | feedback rows of the window whose pattern key also appears in the previous 30 days before that row, over all rows of the window |
| Dollars per merged task | priced cost of coordinator turns started in the window (joined from usage), over tasks with `merged_at` in the window |
| Median wait | median of `decided_at - proposal.created_at` |
| Agreement | items rated and replayed, per shadow dream 006.4 |

A zero denominator gives `null` with `no_data`; an unknown cost among the inputs
`cost_unknown`; fewer than 5 rated items `too_few` (`003.2`). The median for an
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
