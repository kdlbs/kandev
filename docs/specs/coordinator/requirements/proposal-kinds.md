---
id: coordinator-proposal-kinds
title: Resume, message and move proposals
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Resume, message and move proposals Requirements

## Overview

Phase 1 had one proposal kind: create a task. Phase 2 adds resume a task,
message a task and move a task, on the same record, routes and card. Each
still needs a manager's approval. Stall cards gain Resume, and tasks whose
pull request is ready gain Open the PR and Send it back. Nothing here merges
a pull request or moves a task to Done.

## Terminology

- **Kind:** `create_task`, `resume`, `message` or `move`.
- **Target task:** the existing task a resume, message or move acts on.
- **Open proposal:** a proposal in `pending`, `approving` or `failed`.
- **Done step:** a workflow step that completes the task on entry.
- **Resumable session:** the task's primary session exists, is neither
  `COMPLETED` nor `CREATED`, has no live agent execution (the case a stall
  reports), and is one the orchestrator's resume accepts: it has an executor
  record, or its state is `FAILED` or `CANCELLED`, or an interrupted recovery
  is pending on it.
- **Agent-starting step:** a step whose `on_enter` actions include
  `auto_start_agent`, or that feeds, through `pull_from_step_id` links, a step
  that does.
- **Session that accepts a message:** the task's primary session is
  `STARTING`, `RUNNING`, `IDLE`, `WAITING_FOR_INPUT` or `COMPLETED`. A
  session that never started (`CREATED`), a `FAILED` or `CANCELLED` session,
  and no session do not accept one.
- **Ready to merge:** the phase-1 Queue group of tasks whose pull request is
  ready.
- Other terms are defined in [proposals](proposals.md#terminology) and
  [permissions](permissions.md#terminology).

## Mockup

- [`docs/plans/workspace-coordinator-p2/assets/p2-01-queue-what-it-did.png`](../../../plans/workspace-coordinator-p2/assets/p2-01-queue-what-it-did.png): Queue, including the Ready to merge row.

## Requirements

### REQ-COORDINATOR-PROPOSAL-KINDS-001: Proposing

**Intent:** The coordinator asks to resume, message or move a task.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSAL-KINDS-001.1:** A coordinator session whose tool
  profile holds them shall have `propose_resume_kandev(task_id, rationale)`,
  `propose_message_kandev(task_id, text, rationale)` and
  `propose_move_kandev(task_id, step_id, rationale)`, each also accepting
  `standing_order_ids`. `rationale` shall follow the phase-1 create rule and
  `text` shall be 1 to 4000 characters after trimming.
- **AC-COORDINATOR-PROPOSAL-KINDS-001.2:** Unless AC 001.5 returns an open
  proposal, each tool shall refuse, naming the field, a target task that is not a watched, unarchived task of the
  coordinator's workspace or that is a coordinator conversation task.
- **AC-COORDINATOR-PROPOSAL-KINDS-001.3:** `propose_move_kandev` shall refuse,
  naming `step_id`, a step that is not in the task's workflow, the step the
  task is in, a Done step, and an agent-starting step while `start_agent` is
  `denied`. It shall store the proposal with `starts_agent` true when the
  step is agent-starting at propose, and false otherwise.
- **AC-COORDINATOR-PROPOSAL-KINDS-001.4:** Unless AC 001.5 returns an open
  proposal, `propose_resume_kandev` shall refuse
  a task without a resumable session, and `propose_message_kandev` a task
  without a session that accepts a message, naming `task_id`.
- **AC-COORDINATOR-PROPOSAL-KINDS-001.5:** When the coordinator already has an
  open proposal of the same kind for the same target task, the tool shall
  return that proposal, marked as already open, and create nothing, whether or
  not the target or the other arguments would now be refused;
  concurrent calls shall leave one. A `failed` proposal is open until a manager
  rejects it or approves it again.
- **AC-COORDINATOR-PROPOSAL-KINDS-001.6:** A stored proposal shall have its
  kind; a phase-1 proposal and one created by `propose_task_kandev` shall be
  `create_task`. Resume, message and move proposals shall count toward the
  limit of 25 open proposals.

### REQ-COORDINATOR-PROPOSAL-KINDS-002: Creating into an agent-starting step

**Intent:** Starting an agent is a separate decision from creating a task.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSAL-KINDS-002.1:** While `start_agent` is `denied`, the
  create rules of phase 1 shall apply unchanged, including the eligible-step
  rule.
- **AC-COORDINATOR-PROPOSAL-KINDS-002.2:** While `start_agent` is
  `requires_approval`, `propose_task_kandev` shall accept an agent-starting
  step that is not a Done step, and store the proposal with `starts_agent`
  true.
- **AC-COORDINATOR-PROPOSAL-KINDS-002.3:** When a manager approves a proposal
  with `starts_agent` true while `start_agent` is `denied`, the approval
  shall be refused with 409 `policy_denied`, whatever the task's current step.

### REQ-COORDINATOR-PROPOSAL-KINDS-003: Approving other kinds

**Intent:** Approval does exactly the reviewed action, once.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSAL-KINDS-003.1:** When a manager approves a resume
  proposal, the system shall resume the target task's session and settle
  `approved`. When the task is archived or has no resumable session at
  approval, it shall settle `failed` with the reason.
- **AC-COORDINATOR-PROPOSAL-KINDS-003.2:** When a manager approves a message
  proposal, the system shall deliver the text, as edited, to the target
  task's primary session as a queued message and settle `approved`. It shall
  never create a session or start one that never ran; a task without a
  session that accepts a message at approval shall settle `failed`.
- **AC-COORDINATOR-PROPOSAL-KINDS-003.3:** Unless AC 002.3 refuses the
  approval, when a manager approves a move
  proposal, the system shall record the step the task is in, move the task to
  the proposed step and settle `approved`. When the task is archived, has
  left its workflow, or the step is gone or has become a Done step, it shall
  settle `failed` with the reason; the same applies when a session of the task
  is starting or running (`agent_running`), the step is at its work-in-progress
  limit (`step_full`), or the task moved to another workflow meanwhile
  (`moved`). When the step has become agent-starting since a proposal stored
  with `starts_agent` false, it shall settle `failed` with
  `step_starts_agent` and move nothing. When the task is already in the
  proposed step, it shall move nothing and settle `approved` with a row that is
  not undoable, and that outcome wins over the step having since become a Done
  step or agent-starting.
- **AC-COORDINATOR-PROPOSAL-KINDS-003.4:** Approving a resume, message or move
  proposal shall accept an edit only of the message text; any other edit
  shall be refused with 400 `not_editable`.
- **AC-COORDINATOR-PROPOSAL-KINDS-003.5:** A resume, message or move proposal
  shall run at most once. When its claim goes stale, the system shall settle
  it `failed` with "It may or may not have run; check the task" and shall not
  run it again; Approve on the failed card shall run it again only as a new
  explicit approval. An execution that outlives its claim shall not change the
  settled row.

### REQ-COORDINATOR-PROPOSAL-KINDS-004: Cards

**Intent:** A manager sees what approving each kind will do.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSAL-KINDS-004.1:** A resume card shall be titled
  "Resume <task identifier>", a message card "Message <task identifier>"
  with the text, and a move card "Move <task identifier> from <step> to
  <step>". Each shall link to the task and show the rationale.
- **AC-COORDINATOR-PROPOSAL-KINDS-004.2:** Every card shall show "Policy:
  <action> requires approval" and, for each cited order, the Shaped by label
  of [standing orders](standing-orders.md#req-coordinator-standing-orders-003-shaped-by-and-last-applied).
- **AC-COORDINATOR-PROPOSAL-KINDS-004.3:** A create or move card with
  `starts_agent` true shall say "Approving this starts an agent" next to
  Approve. The warning shall come from the stored proposal, not from the
  step's current settings.
- **AC-COORDINATOR-PROPOSAL-KINDS-004.4:** A message card shall offer Edit for
  the text; resume and move cards shall offer no Edit.

### REQ-COORDINATOR-PROPOSAL-KINDS-005: Stall Resume and PR-ready actions

**Intent:** A manager acts on a stall or a ready pull request in one click,
and merging stays human.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSAL-KINDS-005.1:** While the phase-2 flag is on, a
  stall card whose task has a resumable session shall show **Resume** as its
  primary action to a manager; it shall resume the session directly, and the
  stall shall clear through the phase-1 rules.
- **AC-COORDINATOR-PROPOSAL-KINDS-005.2:** While the phase-2 flag is on, each
  Ready to merge row shall show **Open the PR**, opening the pull request in
  a new tab, and "Or send it back with a note" with **Send it back**.
- **AC-COORDINATOR-PROPOSAL-KINDS-005.3:** **Send it back** shall open a note
  of 1 to 4000 characters and, on send, deliver the note to the task's
  primary session as a queued message from the manager; it shall be offered
  only to a manager and only when the task has a session that accepts a
  message.
- **AC-COORDINATOR-PROPOSAL-KINDS-005.4:** The Ready to merge group shall say
  "Merging a pull request is always human." and no card, row or approval
  shall merge a pull request.

## Out of scope

- A stop proposal kind.
- Editing a move's destination or a resume before approval.
- Undoing a message or a resume.
- Starting an agent on an existing task other than through a move.
