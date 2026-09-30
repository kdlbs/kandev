---
id: coordinator-turn-ledger
title: Coordinator turn ledger
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
---

# Coordinator turn ledger Requirements

## Overview

Until phase 3.1 the coordinator records decisions, not turns: a turn that
proposes nothing leaves no trace, an attended turn has no row of its own, and
nothing says which model, configuration or prompt produced a decision. Phase
3.1 (Record and measure) records every turn, attended or unattended, with a
stamp of what produced it, so that outcomes, overrides and replays have
something to stand on. Recording carries no risk and learns nothing, so it
ships ahead of the rest of phase 3.1: it runs whether or not
`features.coordinatorPhase31` is on; the tool, routes and screens that read the
ledger sit behind that flag. This document is the ledger, its stamp, the
board snapshot each turn freezes, its query tool and the release gating. The
outcome side is [outcomes](outcomes.md), the harness that consumes the data is
[replay](replay.md).

## Terminology

- **Turn:** one prompt-to-completion cycle of the coordinator's conversation
  session (a `task_session_turns` row), or of a dream episode
  ([shadow dream](shadow-dream.md)).
- **Ledger row:** the `coordinator_turns` row of one turn.
- **Trigger:** what started the turn: `message` (a manager's message), `wake`
  (an unattended delivery, [wake](wake.md#terminology)) or `dream`.
- **Verdict:** what the turn came to: `acted`, `proposed`, `nothing_needed`,
  `blocked` or `needs_you`; defined by `AC-COORDINATOR-TURN-LEDGER-001.3`.
- **Stamp:** the six values that identify what produced a turn: agent profile,
  model, harness version, configuration revision, policy revision and prompt
  hash.
- **Board snapshot:** the frozen, minimal board state a turn started from,
  identified by a content hash.
- **Recording:** the write side of the ledger (rows, calls, snapshots, stamp).
  It is independent of the release toggle of the read side.
- **Phase 3.1 flag:** `features.coordinatorPhase31`
  (`KANDEV_FEATURES_COORDINATOR_PHASE31`).
- Other terms are defined in [wake](wake.md#terminology) and the
  [system README](../README.md#terms).

## Requirements

### REQ-COORDINATOR-TURN-LEDGER-001: One row per turn

**Intent:** Every turn leaves a row, including a turn that changed nothing.

#### Acceptance criteria

- **AC-COORDINATOR-TURN-LEDGER-001.1:** While `features.coordinator` and
  `features.coordinatorPhase2` are on, when a turn starts on a coordinator's
  conversation session, the system shall insert one ledger row holding the
  coordinator, the session and session turn, the trigger, the stamp, the
  board snapshot hash and the watch set in force. Starting the same session
  turn twice, or a redelivered start event, shall leave exactly one row.
- **AC-COORDINATOR-TURN-LEDGER-001.2:** The trigger shall be `wake` when the session turn is the one an unattended-turn row of phase 3 is bound to or has reserved (the row carries the wake kinds it delivered, at most 20), `dream` when it is a shadow dream episode, and `message` otherwise; a manager's message during an open delivery shall not make its turn a `wake`. A turn that has no row when it completes (a start event that was lost) shall get one at completion with the stamp read then, and its trigger derived by the same rule from the unattended-turn row in any status. A delivery that never started a turn shall have no ledger row.
- **AC-COORDINATOR-TURN-LEDGER-001.3:** When a turn completes, the system
  shall set the ledger row's finish time, the turn outcome (`completed`, `failed`, `cancelled`, `interrupted`, `stopped_at_ceiling`, `stopped_by_pause` or `unknown`, read from the unattended-turn row when there is one and from the completion event otherwise) and one verdict, chosen by this precedence: `blocked` when the turn failed, was
  cancelled, was interrupted, was stopped by the ceiling or by Pause, or when every guarded call of the turn was refused and none created a
  proposal; `acted` when an automatic approval of phase 3 executed in the turn;
  `proposed` when the turn created at least one proposal; `needs_you` when an
  unattended turn created no proposal and a question or permission wake it
  delivered is still unanswered; `nothing_needed` otherwise. An attended turn
  is never `needs_you`.
- **AC-COORDINATOR-TURN-LEDGER-001.4:** The system shall record each guarded
  Kandev call of a turn as a call row holding the action name, the target task
  id when the call has one, and whether the guard allowed or refused it, and
  shall never record an argument, a result body or any text. A turn shall hold
  at most 100 call rows; further calls shall be dropped and the row shall
  report that its digest is truncated. Calls of one turn shall read in the
  order they were recorded, ties broken by the call row id.
- **AC-COORDINATOR-TURN-LEDGER-001.5:** When two writers complete the same turn
  at once, or a completion arrives for a row already completed, the system
  shall keep the first completion's values and change nothing.
- **AC-COORDINATOR-TURN-LEDGER-001.6:** If recording a row, a call, a snapshot
  or a completion fails, the system shall log it, count it in
  `coordinator_ledger_write_failed_total{stage}` and let the turn, the
  proposal or the decision that triggered it proceed unchanged. A failed recording shall never fail, delay or alter a turn: no recording step shall wait on a database write in the path of a turn, a guarded call or a decision.
- **AC-COORDINATOR-TURN-LEDGER-001.7:** At startup and once a day, the system shall settle every ledger row that has been unfinished for more than 24 hours with the outcome `interrupted` and the verdict `blocked`, and shall change no row that is finished.

### REQ-COORDINATOR-TURN-LEDGER-002: The stamp

**Intent:** A turn says what produced it, from what actually ran.

#### Acceptance criteria

- **AC-COORDINATOR-TURN-LEDGER-002.1:** The system shall read the agent profile
  id, configuration revision and policy revision from the coordinator row at
  turn start, and the prompt hash as the SHA-256 of the standing instructions a conversation opened at that moment would receive, rendered from the coordinator's current context, standing orders and goal. It is the hash of the current configuration, not of the text a still-open conversation was opened with.
- **AC-COORDINATOR-TURN-LEDGER-002.2:** The system shall read the model from
  what the provider reported for that turn's usage, normalised by trimming
  and lower-casing, and shall never fill it from the agent profile's
  configured model. Before any usage is reported the model is empty; when the turn completes, and once more when the turn's usage is priced later, the row shall be updated, only while the model is still empty, with the reported model. A turn with several models shall carry the
  first reported.
- **AC-COORDINATOR-TURN-LEDGER-002.3:** The harness version shall be the
  Kandev build version and the agent type the provider-reported usage
  carries, joined as `<agent type>@<build version>`; it is empty when no
  agent type was reported.
- **AC-COORDINATOR-TURN-LEDGER-002.4:** Tokens and cost shall not be stored on
  the row: the system shall join them from the usage ledger by session turn
  when read, and shall report cost as unknown, never zero, when the turn has
  no usage rows or one of them is unpriced.
- **AC-COORDINATOR-TURN-LEDGER-002.5:** An empty stamp value shall be stored
  and returned as empty, and shall never equal another empty value for any
  purpose that compares stamps.

### REQ-COORDINATOR-TURN-LEDGER-003: Links from the log and proposals

**Intent:** A decision can be traced to the turn that made it.

#### Acceptance criteria

- **AC-COORDINATOR-TURN-LEDGER-003.1:** When a proposal is created or an
  activity row is written by a coordinator's guarded call, the system shall
  set the proposal's and the row's turn id to the ledger row of the turn that
  is active on the calling session, and leave it empty when no such turn can
  be determined or its read fails.
- **AC-COORDINATOR-TURN-LEDGER-003.2:** An unattended-turn row of phase 3 shall
  carry the id of its ledger row, set when the ledger row is inserted. A
  ledger row shall be linked to at most one unattended-turn row.
- **AC-COORDINATOR-TURN-LEDGER-003.3:** A proposal or activity row created
  before recording began, or outside a turn (a manager's approval, an undo),
  shall have no turn id and every reader shall treat that as unknown, not as
  an error.

### REQ-COORDINATOR-TURN-LEDGER-004: The query tool

**Intent:** The coordinator reads its own history instead of carrying it in
context.

#### Acceptance criteria

- **AC-COORDINATOR-TURN-LEDGER-004.1:** While the phase 3.1 flag is effective,
  a coordinator conversation opened afterwards shall have
  `list_coordinator_turns_kandev`, in every policy, and one opened earlier
  shall not until it is reopened. The tool shall write nothing.
- **AC-COORDINATOR-TURN-LEDGER-004.2:** The tool shall accept `since` (an
  RFC 3339 time, default 30 days before now, at most 400 days), `verdict`,
  `trigger`, `task` (a task id) and `limit` (1 to 50, default 20), and shall
  return rows of its own coordinator only, newest first by start time, ties by
  row id descending, with `next_before` to continue. A value outside its
  range, an unknown verdict or trigger, or a malformed time shall be refused
  with the phase-1 validation error naming the argument.
- **AC-COORDINATOR-TURN-LEDGER-004.3:** Each returned row shall carry its id,
  trigger, start and finish time, verdict, stamp, the count and ids of its
  proposals, its call digest (names and target task ids) and its tokens and
  cost per `002.4`, and shall carry no message text, proposal text or task
  title.
- **AC-COORDINATOR-TURN-LEDGER-004.4:** Target task ids in a returned digest,
  and a `task` filter, shall be limited to the coordinator's watch set
  ([permissions](permissions.md#req-coordinator-permissions-003-watches),
  [Projects](permissions.md#req-coordinator-permissions-005-projects)): a `task`
  filter outside it shall return the phase-1 not-found error, and a digest
  entry outside it shall be returned with its target omitted.
- **AC-COORDINATOR-TURN-LEDGER-004.5:** When the ledger read fails, the tool
  shall return the phase-1 unavailable error and shall not return a partial
  page.

### REQ-COORDINATOR-TURN-LEDGER-005: Release gating and retention

**Intent:** Recording ships before the rest of phase 3.1 without changing
anything a person or the agent can see.

#### Acceptance criteria

- **AC-COORDINATOR-TURN-LEDGER-005.1:** The system shall have the release
  toggle `features.coordinatorPhase31`, off in every shipped profile,
  effective only while `features.coordinator`, `features.coordinatorPhase2`
  and `features.coordinatorPhase3` are also on, and requiring a restart.
- **AC-COORDINATOR-TURN-LEDGER-005.2:** While the phase 3.1 flag is not
  effective, the system shall still record the ledger, its calls, board
  snapshots, outcomes and overrides
  (`AC-COORDINATOR-TURN-LEDGER-001.1`, `AC-COORDINATOR-OUTCOMES-001.1`,
  `AC-COORDINATOR-OUTCOMES-002.1`), and shall add no tool, route, screen, prompt
  text, admission check or behaviour to the coordinator, any decision or any
  page; every read route of phase 3.1 shall answer 404 and stored data shall be
  kept.
- **AC-COORDINATOR-TURN-LEDGER-005.3:** At startup and once a day, the system
  shall delete ledger rows and their call rows older than 400 days, and board
  snapshots older than 90 days, in batches of 500 rows, each batch in its own
  short transaction, and shall delete a coordinator's rows with the
  coordinator. It shall run while recording runs, whatever the phase 3.1
  flag.
- **AC-COORDINATOR-TURN-LEDGER-005.4:** The ledger tables shall be added by
  migrations that are additive and replayable on SQLite and PostgreSQL; a
  database upgraded from the phase 3 schema shall keep every existing row and
  behave as before.

### REQ-COORDINATOR-TURN-LEDGER-006: The board snapshot

**Intent:** A replay can see the board a decision was made against.

#### Acceptance criteria

- **AC-COORDINATOR-TURN-LEDGER-006.1:** At turn start the system shall freeze
  the coordinator's board state as its open tasks within the watch set (not archived and not in a step that completes tasks; at most 200, newest updated first, ties by task id), its open proposals (at most 50, oldest first, ties by id) and, for each task, only its task id, workflow step id, state, last update time and the kinds of the coordinator's pending proposals that target it, and shall store it once per content hash; an identical board shall
  share one stored snapshot. When more items exist than the cap, the snapshot
  shall say so.
- **AC-COORDINATOR-TURN-LEDGER-006.2:** The snapshot shall hold no task title,
  description, message or proposal text. A replay reads those from the
  original records at replay time, and a record that is gone shall skip that
  replay case with the reason `input_gone` ([replay](replay.md)).
- **AC-COORDINATOR-TURN-LEDGER-006.3:** When the snapshot cannot be built or
  stored, the ledger row shall be written with an empty snapshot hash and the
  turn shall proceed (`AC-COORDINATOR-TURN-LEDGER-001.6`).

## Out of scope

- A screen that browses the ledger: phase 3.1 shows it through the tool and
  through the measures and reports of [outcomes](outcomes.md) and
  [shadow dream](shadow-dream.md).
- Recording the agent's own non-Kandev tool calls, message text or reasoning.
- Storing tokens or cost on the ledger row.
- Any use of the ledger by a wake, admission or approval decision.
- Recording turns of any conversation other than a coordinator's.
