---
id: coordinator-outcomes
title: Coordinator outcomes and overrides
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
---

# Coordinator outcomes and overrides Requirements

## Overview

A decision is only the start of what happened. Phase 3.1 back-fills what came
of each proposal (approved, edited, rejected, returned or undone, and for a
created task whether it merged, was reopened or failed, what it cost and how
long it took) and captures every manager override as a feedback observation.
Every later rung of the learning loop learns from these facts, never from the
agent's opinion of itself. Grading and capture only read stored rows and
write their own; like the ledger they run whether or not
`features.coordinatorPhase31` is on
([turn ledger](turn-ledger.md#req-coordinator-turn-ledger-005-release-gating-and-retention)).
The measures of the second half of this document are read only with the flag.

## Terminology

- **Outcome row:** the single `coordinator_outcomes` row of one proposal.
- **Grading:** computing and storing an outcome row from stored facts.
- **Task result:** for a proposal that created a task, one of `open`, `done`,
  `merged`, `failed` or `dropped`: `merged` when a pull request linked to the
  task is merged, else `done` when the task sits in a step that completes it,
  `failed` when its latest session failed and the task is not done,
  `dropped` when the task is archived unfinished, else `open`.
- **Override:** a manager's rejection of a proposal, an edit before
  approval, an undo of an approved action, or a move of a card back to an
  earlier step after the coordinator created or moved it.
- **Feedback observation:** the stored record of one override.
- **Manager:** a person with `workspace.manage` in the coordinator's workspace.
- **Pattern key:** the triple of override kind, proposal kind and reason
  code.

## Requirements

### REQ-COORDINATOR-OUTCOMES-001: Outcome grading

**Intent:** Each decision has a recorded result that no one has to reconstruct.

#### Acceptance criteria

- **AC-COORDINATOR-OUTCOMES-001.1:** While `features.coordinator` and
  `features.coordinatorPhase2` are on, the system shall keep one outcome row
  per decided proposal, holding the proposal, its turn id, its decision
  (`approved`, `edited`, `rejected`, `returned` or `undone`), the decision
  time, the names of the fields a manager edited (never their values), the
  reject reason code (never its text), and, for a created task, its task
  result, cost, number of reopenings and the time from approval to merge.
  `edited` means approved with edits; `undone` supersedes `approved` and
  `edited` once the action is undone.
- **AC-COORDINATOR-OUTCOMES-001.2:** The system shall grade a proposal after a
  step transition of a task the coordinator created or moved, when the
  proposal is decided, and on a sweep every 24 hours over every proposal
  decided in the last 400 days whose row is not final. A row is final when its decision is `rejected`, `returned` or `undone`, when its proposal created no task, and when its task result is `merged` or `dropped`. Grading the
  same proposal any number of times, or from two grading paths at once, shall
  leave one row with the values derivable from the stored facts.
- **AC-COORDINATOR-OUTCOMES-001.3:** A final row shall not change again; `done` and `failed` are re-graded and a
  task that leaves `done` shall raise the reopening count by one, once per
  observed transition, however many graders observe it.
- **AC-COORDINATOR-OUTCOMES-001.4:** Cost shall be the sum of priced usage
  rows of the created task's sessions, and shall be unknown, never zero, when
  the task has no usage rows or one is unpriced. Time to merge shall be the
  time from the proposal's approval to the first observed merge, and empty
  until the task is merged.
- **AC-COORDINATOR-OUTCOMES-001.5:** When a task, a proposal or the usage of a
  task is read with an error, the grader shall keep the row's earlier values,
  count `coordinator_outcome_grade_failed_total{reason}` and retry on the next
  pass; a proposal without a decision shall have no row; a deleted task shall
  leave the outcome row with the task result it last had.
- **AC-COORDINATOR-OUTCOMES-001.6:** Grading shall never alter a proposal, an
  activity row, a task or a decision, and a grading failure shall not fail
  any of them.

### REQ-COORDINATOR-OUTCOMES-002: Override capture

**Intent:** Every time a manager corrects the coordinator, that is kept.

#### Acceptance criteria

- **AC-COORDINATOR-OUTCOMES-002.1:** When a manager rejects a proposal,
  approves it with edits, or undoes an approved action, and when a manager
  moves a card that the coordinator created or moved to an earlier step of its
  workflow, the system shall store one feedback observation holding the
  coordinator, the override kind (`rejected`, `edited`, `undone` or
  `moved_back`), the proposal, the turn that made it, the acting user, the
  reason code and the time. It shall store no free text.
- **AC-COORDINATOR-OUTCOMES-002.2:** The same override shall never produce two
  observations: an observation is unique per proposal, kind and, for
  `moved_back`, the moving step transition, however many times its source is
  read or a grader runs.
- **AC-COORDINATOR-OUTCOMES-002.3:** Only a manager's override shall be
  stored. An override by a person who is not a manager, by a coordinator
  principal or by the system shall store no observation and shall count in
  `coordinator_override_ignored_total{reason}`; an authorisation read that
  fails shall be treated as not a manager.
- **AC-COORDINATOR-OUTCOMES-002.4:** A card counts as moved back only when its
  destination step is earlier in the workflow's step order than the step the
  coordinator's approved action left it in, and the move happened after that action. When several proposals of the coordinator created or moved the card, the observation shall name the newest approved one. A move is observed only when its step transition history records a user as the mover, so a card that never had a session is not observed. A move to a later step, a move within the step, and a move of a card
  the coordinator did not create or move shall store nothing.
- **AC-COORDINATOR-OUTCOMES-002.5:** A rejection reason typed by a manager or
  a coded one shall map to one reason code of a closed set, `duplicate`,
  `wrong_target`, `wrong_timing`, `not_wanted`, `too_broad`, `other` and
  `none`; text with no code shall map to `other` and no reason to `none`.
- **AC-COORDINATOR-OUTCOMES-002.6:** Observations shall be readable by
  workspace members only through the measures of `003` and the shadow dream
  report ([shadow dream](shadow-dream.md)), never by the coordinator
  itself.

### REQ-COORDINATOR-OUTCOMES-003: Measures

**Intent:** A manager can see whether the coordinator is getting better.

#### Acceptance criteria

- **AC-COORDINATOR-OUTCOMES-003.1:** While the phase 3.1 flag is effective, the
  system shall return, for a coordinator and a window of 1 to 90 days
  (default 30), these measures, each with the counts it was computed from:
  the approval-without-edit rate (proposals approved and never edited, of
  those decided in the window); override recurrence (observations of the
  window whose pattern key also appears among observations in the 30 days
  before that observation, of all observations of the window); dollars per
  merged task (the priced cost of the coordinator's turns started in the
  window, over tasks it created that merged in the window); the median time a
  proposal waited for a decision (approval or rejection time minus creation
  time); and the agreement between owner ratings of shadow items and the
  replay verdicts on the same items ([shadow dream](shadow-dream.md)).
- **AC-COORDINATOR-OUTCOMES-003.2:** A measure with a zero denominator, with
  an unknown cost among its inputs, or (for the agreement) with fewer than 5
  rated items shall be returned as `null` with the reason (`no_data`,
  `cost_unknown`, `too_few`), never as `0`.
- **AC-COORDINATOR-OUTCOMES-003.3:** A window outside 1 to 90, empty, repeated
  or not an integer shall be refused with 400 naming `days`. Any workspace
  member shall read the measures of their own workspace's coordinator; a
  coordinator of another workspace shall be not found; a coordinator
  principal shall be refused.
- **AC-COORDINATOR-OUTCOMES-003.4:** The coordinator's Learning section
  ([shadow dream](shadow-dream.md#req-coordinator-shadow-dream-005-screens))
  shall show the five measures with their counts, and "Not enough data yet"
  for a `null` measure with its reason, and shall show a loading placeholder
  and a Try again failure state that do not show a measure value.

## Out of scope

- Any use of an outcome or observation to change the coordinator's
  configuration, notes, standing orders or trust (phase 3.5).
- Promoting a recurring override into a standing-order candidate (phase 3.5).
- Overrides by anyone other than a manager.
- Comparing coordinators with one another.
