---
id: coordinator-activity-log
title: What it did (activity log)
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# What it did (activity log) Requirements

## Overview

Every action a coordinator proposes, and every decision on it, leaves a row
in its activity log: proposed, approved, rejected, failed, refused or undone.
The Queue shows the log as "What it did", with Undo where an action can be
reversed. The coordinator reads its own log through one tool, so it can say
what it did. The log's rows and its summary are also the evidence phase 3
reads when it offers the first automatic setting.

## Terminology

- **Activity row:** one log entry, for one coordinator, of one action class.
- **Action class:** one of the six actions of
  [permissions](permissions.md#terminology).
- **Outcome:** `proposed`, `approved`, `rejected`, `failed`, `refused` or
  `undone`.
- **Authorization:** how the row was authorised: `requires_approval` for a
  proposal and its decisions, `denied` for a refusal.
- **Undoable row:** an `approved` row of a created task, or of a move that
  changed the task's step, that has not been undone. A move approved while
  the task was already in the proposed step is not undoable.
- Other terms are defined in the [system README](../README.md#terms).

## Mockup

- [`docs/plans/workspace-coordinator-p2/assets/p2-01-queue-what-it-did.png`](../../../plans/workspace-coordinator-p2/assets/p2-01-queue-what-it-did.png): Queue with What it did, columns When, Action, Action class, How it was authorised and Undo.

## Requirements

### REQ-COORDINATOR-ACTIVITY-LOG-001: Recording

**Intent:** Nothing a coordinator does is missing from its log.

#### Acceptance criteria

- **AC-COORDINATOR-ACTIVITY-LOG-001.1:** While the phase-2 flag is on, when a
  proposal is created, the system shall write a `proposed` row in the same
  transaction, with the proposal id, its action class, its target task when
  it has one and a one-line detail of at most 1000 characters.
- **AC-COORDINATOR-ACTIVITY-LOG-001.2:** When a proposal's claim settles
  `approved`, the system shall write an `approved` row in the same
  transaction as the settle, with the approving manager's user id and
  `edited` true when the manager changed the proposal before approving. When
  a proposal is rejected, it shall write a `rejected` row with the manager's
  user id and the reason code or text. When an execution settles `failed`, it
  shall write a `failed` row with the error detail. A retry that settles again
  shall write a new row for the new outcome.
- **AC-COORDINATOR-ACTIVITY-LOG-001.3:** When the guard refuses a coordinator
  action (`AC-COORDINATOR-PERMISSIONS-002.2`), the system shall write a
  `refused` row with authorization `denied`, the action class and a reason
  code. A refusal of the same coordinator, action class and reason code
  within 60 seconds of an existing refused row shall increase that row's
  `refusal_count` and `updated_at` instead of adding a row. A refused call
  that names no known action shall be recorded under the action class
  `unknown`.
- **AC-COORDINATOR-ACTIVITY-LOG-001.4:** Activity rows shall never be updated
  except to mark them undone or to add a refusal count, and never deleted
  except by retention (`AC-COORDINATOR-ACTIVITY-LOG-005.1`).
- **AC-COORDINATOR-ACTIVITY-LOG-001.5:** A manager's direct task action (Resume
  on a stall card, Send it back on a Queue row, Undo excepted) shall write no
  activity row, because the coordinator did not propose it.
- **AC-COORDINATOR-ACTIVITY-LOG-001.6:** When a log write fails, the state
  change it records shall not commit; the caller shall receive the error.

### REQ-COORDINATOR-ACTIVITY-LOG-002: What it did

**Intent:** A manager reads what the coordinator did, newest first.

**User story:** As a workspace manager, I want one list of everything my
coordinator asked for and what came of it, so that I can trust it or tighten
it.

Mockup:

- [`docs/plans/workspace-coordinator-p2/assets/p2-01-queue-what-it-did.png`](../../../plans/workspace-coordinator-p2/assets/p2-01-queue-what-it-did.png): the What it did section.

#### Acceptance criteria

- **AC-COORDINATOR-ACTIVITY-LOG-002.1:** While the phase-2 flag is on, the
  Queue shall show a What it did section for the selected coordinator, with
  the columns When, Action, Action class, How it was authorised and Undo.
- **AC-COORDINATOR-ACTIVITY-LOG-002.2:** The system shall return activity rows
  ordered by `created_at` descending, then `id` descending, 50 per page, with
  a cursor for the next page; the section shall show **Load more** while a
  next page exists.
- **AC-COORDINATOR-ACTIVITY-LOG-002.3:** The section shall offer a filter by
  action class, including All; the route shall accept at most one action
  class and refuse any other value with 400.
- **AC-COORDINATOR-ACTIVITY-LOG-002.4:** Each row shall show the relative time
  (exact time on hover), the action's detail with the target task's
  identifier linking to the task, the action class, the outcome with "with
  edits" when edited and "x N" when a refusal repeated, and the actor's name
  for approved, rejected and undone rows.
- **AC-COORDINATOR-ACTIVITY-LOG-002.5:** With no row, the section shall say "It
  has not done anything yet."; with no row for the chosen filter, it shall
  say that nothing matches the filter.
- **AC-COORDINATOR-ACTIVITY-LOG-002.6:** A reader shall see the section
  without Undo controls. The list shall update when a `coordinator.updated`
  event arrives for the coordinator.

### REQ-COORDINATOR-ACTIVITY-LOG-003: Undo

**Intent:** A manager reverses a reversible action from the log.

#### Acceptance criteria

- **AC-COORDINATOR-ACTIVITY-LOG-003.1:** An undoable row shall show **Undo**
  to a manager. Rows of a message or resume shall show "No undo"; other rows
  shall show nothing in the Undo column.
- **AC-COORDINATOR-ACTIVITY-LOG-003.2:** When a manager undoes an approved
  create, the system shall first archive the created task, and then, in one
  transaction, mark the row undone with `undone_at` and `undone_by` and write
  an `undone` row pointing to it. A created task already archived shall
  count as archived. When that transaction fails after the reversal, the
  undo shall return the error and the row shall stay undoable; a retry shall
  find the reversal already done and only mark the row.
- **AC-COORDINATOR-ACTIVITY-LOG-003.3:** When a manager undoes an approved
  move, the system shall move the task back to the step it left if the task
  is still in the step it was moved to, and then record the undo as in
  `AC-COORDINATOR-ACTIVITY-LOG-003.2`. A task already back in the step it
  left, unarchived, shall count as moved back. When the task is in any other
  step or was archived, the system shall refuse with 409 `undo_conflict` and
  the row shall say "It has moved since".
- **AC-COORDINATOR-ACTIVITY-LOG-003.4:** Undoing a row that is already undone,
  or two undos at once, shall reverse the action at most once; the second
  shall return 409 `already_undone`. Undoing a row that is not undoable shall
  return 409 `not_undoable`.
- **AC-COORDINATOR-ACTIVITY-LOG-003.5:** A reader's undo shall be refused with
  403 and change nothing.
- **AC-COORDINATOR-ACTIVITY-LOG-003.6:** An undone row shall show "Undone by
  <name>, <relative time>" in place of Undo.

### REQ-COORDINATOR-ACTIVITY-LOG-004: The coordinator reads its log

**Intent:** The copilot can answer "what did you do?".

#### Acceptance criteria

- **AC-COORDINATOR-ACTIVITY-LOG-004.1:** A coordinator session shall have
  `list_coordinator_activity_kandev` with an optional `limit` of 1 to 50
  (default 20) and an optional `before` cursor. It shall return only the
  calling coordinator's rows, in the order of
  `AC-COORDINATOR-ACTIVITY-LOG-002.2`, with the cursor for the next page.
- **AC-COORDINATOR-ACTIVITY-LOG-004.2:** The tool shall return each row's time,
  action class, outcome, target task id, proposal id, detail, edited flag,
  refusal count and undone time, and shall not return user ids or names.
- **AC-COORDINATOR-ACTIVITY-LOG-004.3:** A `limit` outside 1 to 50, or a cursor
  that does not parse, shall be refused naming the field.

### REQ-COORDINATOR-ACTIVITY-LOG-005: Retention and summary

**Intent:** The log stays bounded and gives settings and phase 3 counts.

#### Acceptance criteria

- **AC-COORDINATOR-ACTIVITY-LOG-005.1:** While the phase-2 flag is on, at
  startup and once a day, the system shall delete activity rows older than
  400 days, in batches, without
  blocking proposal writes. While the flag is off no row is deleted by age
  (`AC-COORDINATOR-COORDINATORS-007.3`). Rows of a deleted coordinator shall
  be deleted with it, whatever the flag.
- **AC-COORDINATOR-ACTIVITY-LOG-005.2:** The system shall return, for a
  coordinator and a window of 1 to 90 days (default 30), per action class,
  the counts of rows created in the window by outcome, `proposed`,
  `approved` (every approval, edited or not), `approved` with edits (the
  subset of `approved`), `rejected`, `failed`, `refused` (summing refusal
  counts) and `undone` (counted under the action class of the row undone), and the earliest row time the coordinator
  has. A window outside 1 to 90 shall be refused with 400.
- **AC-COORDINATOR-ACTIVITY-LOG-005.3:** Any workspace member shall read the
  log and the summary; neither shall be available for another workspace's
  coordinator.

## Out of scope

- Undoing a message or a resume.
- Exporting the log.
- Logging actions of people who are not acting on a proposal.
- Choosing automatic settings from the summary (phase 3).
