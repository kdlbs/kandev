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
  [permissions](permissions.md#terminology), or `unknown` for a refused call
  that names no known action (`AC-COORDINATOR-ACTIVITY-LOG-001.3`).
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
  Two writes are excepted: a refusal row, because the refusal already
  happened and stands, so its write failure is logged at warn and nothing is
  rolled back; and the undo marker (`AC-COORDINATOR-ACTIVITY-LOG-003.2`),
  which follows the reversal it records in a second commit.

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
  class and refuse any other value with 400. An absent or empty `class`
  means All; a repeated `class` is refused with 400 naming `class`. The
  filter offers All, the six classes and "Unknown action" (`unknown`), so a
  refused call that named no known action can be found; an unrecognised
  `?class=` value in the page address selects All.
- **AC-COORDINATOR-ACTIVITY-LOG-002.4:** Each row shall show the relative time
  (exact time on hover), the Action cell, the action class and the How it
  was authorised cell. The Action cell shows the row's `detail` as the
  coordinator or manager wrote it (an approved or proposed row's detail is the
  proposal's one-line title, shown with no prefix and no invented verb), and
  the target task's identifier linking to the task when the row has one; a row with an empty detail shows the action class label in its place. The Action cell text order is in the system design. The
  Action cell prefixes a rejected row's detail (the manager's reason) with
  "Rejected: " and a failed row's detail (the error) with "Failed: ", showing
  "Rejected" or "Failed" alone when that detail is empty; an `undone` row
  carries the reversed row's detail with no prefix. A refused row shows, in
  place of a detail, the reason text of its code: `binding_invalid` "Its tool
  settings could not be read.", `not_in_profile` "It called something it is
  not allowed to use.", `policy_denied` "A manager has set this action to
  Denied."; any other code shows "Refused. " followed by the code, and a
  refused row with no code shows "Refused.". The How it was authorised cell
  shows the authorization line, "Requires approval" or "Denied" (with " x N"
  after "Denied" when `refusal_count` is above 1), and, for an approved,
  rejected, failed or undone row, a second outcome line: "Approved by <name>"
  (with ", with edits" when edited), "Rejected by <name>", "Failed" or
  "Undone by <name>". With no person recorded (authentication off) the
  outcome line has no "by <name>" ("Approved", "Rejected", "Undone"); when the
  recorded person is not a member any more it reads "by a former member"; a
  proposed or refused row has no second line. The full texts and their forms
  are the copy table of the system design.
- **AC-COORDINATOR-ACTIVITY-LOG-002.8:** The server sends user ids and no
  display names. The client shall resolve each `actor_user_id` and
  `undone_by` to a name from the workspace member list, read once when the section mounts and at most once more when a row's person is absent and the last read was more than 30 seconds ago (the member state stays loaded during the second read). While the member list has
  not loaded, or when reading it failed, a row whose person is recorded shall
  show the no-person form of its outcome, so a transient failure never reads
  "a former member"; only a member list that loaded and does not hold the id
  shall show "a former member", which therefore means "not in the workspace member list" (inherited-role managers have no row). Two members with the same display name show
  the same name.
- **AC-COORDINATOR-ACTIVITY-LOG-002.9:** The text "Task no longer available"
  shall show, with no link, only on a row that has a `target_task_id` whose
  task cannot be found: the workspace's task snapshots have loaded without error at least once (a later failure does not withdraw it) and do not hold that id, and the row has no `target_task_identifier`.
  A row with no `target_task_id` (a proposed, rejected or failed create, a
  refusal, an `unknown` class row) shows nothing in its place, and a row
  whose snapshots have not loaded or failed to load shows the link when the
  server sent an identifier and nothing otherwise. A row whose task the
  snapshots hold but that has no `target_task_identifier` links to it with the
  text "Open task".
- **AC-COORDINATOR-ACTIVITY-LOG-002.10:** A refresh of the list (a
  `coordinator.updated` event, a WebSocket reconnect, or an undo outcome of
  200, `already_undone`, `not_undoable` or 404) shall re-read every page the section has loaded (superseding an in-flight Load more, which is disabled meanwhile), replace the rows and the next cursor with what the
  server returned in the order of `AC-COORDINATOR-ACTIVITY-LOG-002.2`, and
  keep at most one refresh in flight (one trailing refresh may queue). A
  response for a filter or coordinator that is no longer selected shall be
  dropped and change nothing. A filter change shall write `?class=` to the
  address by replacing the current history entry, and remove `class` for All.
- **AC-COORDINATOR-ACTIVITY-LOG-002.11:** While the first page is loading the
  section shall say so and not show the empty text; when the first page
  fails to load it shall say "What it did could not be loaded." with a
  **Retry** button and not show the empty text; when Load more fails the
  loaded rows and the Load more button stay and "More could not be loaded.
  Try again." shows. A failed refresh keeps the rows already shown.
- **AC-COORDINATOR-ACTIVITY-LOG-002.5:** With no row, the section shall say "It
  has not done anything yet."; with no row for the chosen filter, it shall
  say that nothing matches the filter. The empty text shows only once the first load has succeeded with no rows: "It has not done anything yet." when the filter is All, the filtered text otherwise. The client sends no `limit`.
- **AC-COORDINATOR-ACTIVITY-LOG-002.7:** The list route shall take `limit`
  as an integer from 1 to 50, default 50 when absent, and refuse any other
  value (0, negative, above 50, empty, not an integer) with 400 naming
  `limit`. It shall return `next_cursor` only when at least one older row
  exists, so the last page has a null cursor. A cursor shall be valid with
  any `class` and `limit`; one that does not parse, is empty or is repeated
  is refused with 400 naming `before`; a repeated `limit` is refused with 400
  naming `limit`.
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
  count as archived. Archiving stops any agent working on the task, and the
  confirmation says so. When that transaction fails after the reversal, the
  undo shall return the error and the row shall stay undoable; a retry shall
  find the reversal already done and only mark the row.
- **AC-COORDINATOR-ACTIVITY-LOG-003.3:** When a manager undoes an approved
  move, the system shall move the task back to the step it left if the task
  is still in the step it was moved to, and then record the undo as in
  `AC-COORDINATOR-ACTIVITY-LOG-003.2`. A task already back in the step it
  left, unarchived, shall count as moved back. When the task is in any other
  step or was archived, the system shall refuse with 409 `undo_conflict` and
  the row shall say "It has moved since".
- **AC-COORDINATOR-ACTIVITY-LOG-003.7:** The checks of an undo of a move run in
  this order and the first that applies decides: the task is archived or
  missing (`archived`); the task is already in the step it left (counts as
  moved back, no refusal); the task is in the step it was moved to, and then
  the step it left no longer exists (`step_deleted`), that step is now a
  completing step (`step_done`), the task has an agent session that is
  starting or running (`agent_running`), or that step is at its task limit
  and refuses the task (`step_full`); the task is in any other step
  (`moved`). Each refusal is 409 `undo_conflict` carrying that reason,
  changes nothing and leaves the row undoable. Only an optioned move refuses on a move still pending on the task (the task
  service's move conflict, reason `moved`); undo of a move to a step that
  does not auto-start is a plain move, which clears a pending move marker,
  and that is accepted. The row shall say "It has moved since" for
  `moved`, `archived` and any reason the client does not know or that is absent, "An agent is working on it. Stop it, then undo."
  for `agent_running`, "The step it came from no longer exists." for
  `step_deleted`, "The step it came from is now a finishing step." for
  `step_done` and "The step it came from is full." for `step_full`. When the
  step it left queues the task because it is at its limit, the task counts as
  moved back.
- **AC-COORDINATOR-ACTIVITY-LOG-003.8:** Moving a task back shall not start an
  agent: the move enters the step without running its prompt, so the task
  lands idle in the step it left.
- **AC-COORDINATOR-ACTIVITY-LOG-003.9:** A row shall count as undoable only
  when everything undo needs is readable: an `approved` create row with a
  target task id, or an `approved` move row whose proposal outcome holds both
  `from_step_id` and `to_step_id`. A row missing any of them, or whose
  proposal is gone or whose outcome does not parse, shall list `undoable`
  false and answer undo with 409 `not_undoable`.
- **AC-COORDINATOR-ACTIVITY-LOG-003.10:** Undo shall first open a dialog
  titled "Undo this?" with **Undo** and **Cancel**, Cancel focused on open, so that Enter with that initial focus activates Cancel (the dialog has no default confirm action); Cancel, Escape or a click outside closes it
  and sends nothing; Undo closes it and sends one
  request. Its text is one whole sentence chosen by the row, never a noun
  phrase inserted into a sentence: for a created task "The task <identifier>
  will be archived. Any agent working on it will be stopped." or, when the
  identifier is unknown, "This task will be archived. Any agent working on it
  will be stopped."; for a move "The task <identifier> will move back to <step
  name>.", "The task <identifier> will move back to the step it came from."
  when the step name is unknown, "This task will move back to <step name>."
  when the identifier is unknown, and "This task will move back to the step
  it came from." when neither is known. The step name is the title of the
  step whose id is the row's `from_step_id`, resolved on the client from the Queue's task snapshots; it is unknown when the id is null or not in
  the snapshots.
- **AC-COORDINATOR-ACTIVITY-LOG-003.11:** When undo answers 409
  `undo_conflict`, the row shall show the conflict text of `003.7` inline in
  its Undo cell below the Undo button, which stays clickable. Any other
  failure (`not_undoable`, 500, network, 404) shows its own text the same way
  except 404, which shows "This action is no longer listed." in a section
  notice above the list. The message clears when the next re-read completes with success (a `coordinator.updated` event, a reconnect, a successful retry of that row's undo), when the filter or coordinator changes, or when the row's Undo is confirmed again; a failed re-read and Load more do not clear it. A `not_undoable` or 404 re-reads at once and its message survives the first re-read that starts after the refusal settles, clearing on the next. A
  `409 already_undone` shows no message. Two rows can each hold a message at
  once; a second failure on one row replaces its message.
- **AC-COORDINATOR-ACTIVITY-LOG-003.4:** Undoing a row that is already undone,
  or two undos at once, shall reverse the action at most once; the second
  shall return 409 `already_undone`. Undoing a row that is not undoable shall
  return 409 `not_undoable`. Undoing a row retention has deleted meanwhile
  shall return 404, with the reversal standing.
- **AC-COORDINATOR-ACTIVITY-LOG-003.5:** A reader's undo shall be refused with
  403 and change nothing.
- **AC-COORDINATOR-ACTIVITY-LOG-003.6:** An undone row shall show "Undone by
  <name>, <relative time>" in place of Undo, or "Undone, <relative time>"
  when no person is recorded (authentication is off) and "Undone by a former
  member, <relative time>" when the recorded person is not in the loaded member list (`AC-COORDINATOR-ACTIVITY-LOG-002.8`); the row's outcome text follows
  `AC-COORDINATOR-ACTIVITY-LOG-002.4`.

### REQ-COORDINATOR-ACTIVITY-LOG-004: The coordinator reads its log

**Intent:** The copilot can answer "what did you do?".

#### Acceptance criteria

- **AC-COORDINATOR-ACTIVITY-LOG-004.1:** A coordinator session shall have
  `list_coordinator_activity_kandev` with an optional `limit` of 1 to 50
  (default 20) and an optional `before` cursor. It shall return only the
  calling coordinator's rows, in the order of
  `AC-COORDINATOR-ACTIVITY-LOG-002.2`, with the cursor for the next page.
- **AC-COORDINATOR-ACTIVITY-LOG-004.2:** The tool shall return each row's `created_at`
  and `updated_at` times, action class, outcome, target task id, proposal id, detail, edited flag,
  refusal count and undone time, and shall not return user ids or names.
- **AC-COORDINATOR-ACTIVITY-LOG-004.3:** A `limit` outside 1 to 50, or a cursor
  that does not parse, shall be refused naming the field.

### REQ-COORDINATOR-ACTIVITY-LOG-005: Retention and summary

**Intent:** The log stays bounded and gives settings and phase 3 counts.

#### Acceptance criteria

- **AC-COORDINATOR-ACTIVITY-LOG-005.1:** While the phase-2 flag is on, at
  startup and once a day, the system shall delete activity rows older than
  400 days, in batches of 500 rows, each batch in its own short transaction. The run repeats every 24 hours from startup. The flag is fixed for the
  life of the process (a change takes a restart), so a run needs no flag
  check of its own. While the flag is off at startup no run starts and no
  row is deleted by age
  (`AC-COORDINATOR-COORDINATORS-007.3`). Rows of a deleted coordinator shall
  be deleted with it, whatever the flag.
- **AC-COORDINATOR-ACTIVITY-LOG-005.2:** The system shall return, for a
  coordinator and a window of 1 to 90 days (default 30), per action class,
  the counts of rows created in the window by outcome, `proposed`,
  `approved` (every approval, edited or not), `approved` with edits (the
  subset of `approved`), `rejected`, `failed`, `refused` (summing refusal
  counts) and `undone` (counted under the action class of the row undone), and the earliest row time the coordinator
  has. The class `unknown` shall appear only when it has a row created in the
  window. A window outside 1 to 90, empty, repeated or not an integer shall be refused
  with 400 naming `days`.
- **AC-COORDINATOR-ACTIVITY-LOG-005.3:** Any workspace member shall read the
  log and the summary; neither shall be available for another workspace's
  coordinator.

## Out of scope

- Undoing a message or a resume.
- Exporting the log.
- Logging actions of people who are not acting on a proposal.
- Choosing automatic settings from the summary (phase 3).
