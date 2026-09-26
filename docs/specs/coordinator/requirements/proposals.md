---
id: coordinator-proposals
title: Task proposals
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-26
last_updated: 2026-09-26
---

# Task proposals Requirements

## Overview

A coordinator's only write is a proposal to create one task. A manager approves
it (optionally with edits) or rejects it, in the copilot or on Needs you.
Approving creates exactly one ordinary task and never starts an agent.

## Terminology

- **Proposal status:** one of `pending`, `approving`, `approved`, `rejected`,
  `failed`. `approved` and `rejected` are settled; the others are unsettled.
- **Open proposal:** a proposal in `pending`, `approving` or `failed`. The
  "pending proposal count" shown by the badge and carried by
  `coordinator.updated` is the number of open proposals.
- **Eligible step:** a step of the target workflow that has no
  `auto_start_agent` on-enter action and is either the workflow's start step
  or allows manual moves.
- **Stale claim:** an `approving` proposal whose claim is older than two
  minutes.
- Other terms are defined in the [system README](../README.md#terms).

## Mockup

The phase 1 mockup screenshots are the visual reference for this document's
user-facing criteria; each requirement below cites the ones it covers. Where
a screenshot and an acceptance criterion differ, the criterion governs. The
prototype banner, the demo controls and the `P1` and `WC-` labels are mockup
chrome, not product; the data is seeded fiction.

- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](../../../plans/workspace-coordinator/assets/p1-01-needs-you.png)
- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](../../../plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png)

## Requirements

### REQ-COORDINATOR-PROPOSALS-001: Proposing a task

**Intent:** The coordinator can suggest work but not create it.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](../../../plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png): a proposal written from chat.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSALS-001.1:** When a coordinator session calls
  `propose_task_kandev` with a title, description, rationale, workflow and
  optional source task, step and repository that pass validation, the system
  shall store one `pending` proposal owned by that coordinator, emit
  `coordinator.updated`, return `{proposal_id, status}` and create no task.
- **AC-COORDINATOR-PROPOSALS-001.2:** When the step is omitted, the system shall
  use the workflow's start step.
- **AC-COORDINATOR-PROPOSALS-001.3:** The system shall refuse the call, storing
  nothing, when the title is empty after trimming or longer than 60
  characters; the description or rationale is longer than 10,000 characters;
  the source task, workflow, step or repository is not in the coordinator's
  workspace; the step does not belong to the workflow; or the step is not an
  eligible step. The error shall name the field.
- **AC-COORDINATOR-PROPOSALS-001.4:** When the coordinator already has 25
  open proposals, the system shall refuse a new one and say so; concurrent
  proposals shall never leave more than 25 open.
- **AC-COORDINATOR-PROPOSALS-001.5:** Two identical calls shall create two
  proposals; the tool has no deduplication.

### REQ-COORDINATOR-PROPOSALS-002: Approving

**Intent:** Approval creates the task exactly once.

**User story:** As a workspace manager, I want to approve a proposed task, so
that it appears on its board without an agent starting.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](../../../plans/workspace-coordinator/assets/p1-01-needs-you.png): the Needs you count and the sidebar badge that include the open proposal.
- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](../../../plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png): the proposal card in the chat transcript with Approve, Edit and Reject.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSALS-002.1:** When a manager approves a `pending` or
  `failed` proposal, the system shall claim it as `approving`, freeze the final
  spec, create one task in the target workflow and step with the frozen title,
  description and repository, and set the proposal `approved` with the task id
  and the approving user.
- **AC-COORDINATOR-PROPOSALS-002.2:** The created task shall be an ordinary task
  of its board, not a coordinator task, and no agent shall start for it,
  whatever on-enter actions its step has.
- **AC-COORDINATOR-PROPOSALS-002.3:** When a manager approves with edits, the
  system shall merge the edited title, description, workflow, step and
  repository into the spec and validate it as in
  `AC-COORDINATOR-PROPOSALS-001.3`; an invalid edit shall be refused with 400
  and leave the proposal unchanged. A field left out shall stay unchanged; a
  field sent as null shall be refused with 400 naming it; an empty step shall
  mean the workflow's start step and an empty repository shall mean no
  repository; an empty title or workflow shall be refused with 400; a changed
  workflow without a step shall use the new workflow's start step. Edits made
  to a `failed` attempt shall carry into the next approve. The proposal's
  status shall be checked before any edit is validated, so an approve of a
  proposal that is not `pending` or `failed` shall answer as
  `AC-COORDINATOR-PROPOSALS-002.5` and `AC-COORDINATOR-PROPOSALS-002.9` say,
  whatever the edits contain.
- **AC-COORDINATOR-PROPOSALS-002.4:** When two approvals of one proposal race,
  exactly one shall claim it; the other shall receive 409 with the current
  proposal, and exactly one task shall exist.
- **AC-COORDINATOR-PROPOSALS-002.5:** When a manager approves an `approving`
  (unless stale), `approved` or `rejected` proposal, the system shall return
  409 with the current proposal.
- **AC-COORDINATOR-PROPOSALS-002.6:** When task creation fails, the system shall
  set the proposal `failed` with the error; a later approve shall claim it
  again.
- **AC-COORDINATOR-PROPOSALS-002.7:** When the process stops after a claim or
  after the task was created, the next startup, or the next approve, or the
  next proposal read by a caller holding `workspace.manage` that reads the
  stale claim, shall complete the approval with the same single task, using
  the frozen spec without validating it again; a read
  by a caller holding only `workspace.read`, or a read the browser marks as
  cross-site or same-site (`Sec-Fetch-Site`), shall change nothing; when two readers see the same stale claim, exactly one shall
  complete it, using the spec frozen by the first claim and keeping the first
  approver as the approving user. A list read by a caller holding
  `workspace.manage` shall recover every stale claim among the rows it
  returns before answering, and a recovery that fails shall not fail the
  read.
- **AC-COORDINATOR-PROPOSALS-002.8:** No caller other than the coordinator
  service shall be able to create a task with an external id starting
  `coordinator-proposal:`, and no caller shall be able to release such an
  external id from its task.
- **AC-COORDINATOR-PROPOSALS-002.9:** When a manager approves with edits a
  proposal that is `approving`, stale or not, the system shall return 409 with
  the current proposal and apply no edit.
- **AC-COORDINATOR-PROPOSALS-002.10:** When a claim goes stale and another
  reader re-claims it, the original claimer's later completion shall change
  nothing.
- **AC-COORDINATOR-PROPOSALS-002.11:** When a proposal is deleted with its
  coordinator or workspace while its approval is in flight, the approval shall
  return 404, the task it created shall stay on its board, and no proposal row
  shall be recreated.
- **AC-COORDINATOR-PROPOSALS-002.12:** When an earlier approval attempt of the
  same proposal already created the task, whether or not that creation had
  finished, the approval shall complete with that task and create no second
  one.

### REQ-COORDINATOR-PROPOSALS-003: Rejecting

**Intent:** A manager can say no, with a reason.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](../../../plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png): Reject on the proposal card.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSALS-003.1:** When a manager rejects a `pending` or
  `failed` proposal with an optional reason of at most 500 characters, the
  system shall set it `rejected` with the reason and the rejecting user and
  create nothing. A reason that is absent, null, or empty after trimming shall
  store no reason, and the proposal shall show none.
- **AC-COORDINATOR-PROPOSALS-003.2:** When a manager rejects a proposal in any
  other status, the system shall return 409 with the current proposal.
- **AC-COORDINATOR-PROPOSALS-003.3:** When a reject and an approve race, exactly
  one shall succeed and the other shall receive 409.
- **AC-COORDINATOR-PROPOSALS-003.4:** The proposal's status shall be checked
  before the reason, so a reject of a proposal in any status other than
  `pending` or `failed` shall return 409 whatever the reason; a reason longer
  than 500 characters on a `pending` or `failed` proposal shall return 400 and
  change nothing. When the proposal does not exist, including one deleted with
  its coordinator or workspace during the request, approve and reject shall
  return 404.

### REQ-COORDINATOR-PROPOSALS-004: Reading proposals

**Intent:** Both surfaces show the same proposal state.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSALS-004.1:** Listing with `status=pending` shall return
  every open proposal of the coordinator ordered
  by `created_at` ascending, then id ascending; listing with `status=all` shall
  return the newest 50 in any status ordered by `created_at` descending, then id
  descending. `pending` is the default.
- **AC-COORDINATOR-PROPOSALS-004.2:** Readers shall be able to list and read
  proposals; approve and reject shall require `workspace.manage` and return 403
  otherwise.
- **AC-COORDINATOR-PROPOSALS-004.3:** Every change to a proposal shall emit
  `coordinator.updated` with the coordinator's current open proposal count,
  including the claim that makes it `approving`, so other surfaces show
  "Approval in progress" while the task is being created.
- **AC-COORDINATOR-PROPOSALS-004.4:** When a decision is made on one surface or
  in another browser, the other surfaces shall show the settled state after
  `coordinator.updated`, and a settled status shown by a client shall never be
  replaced by an unsettled one.

### REQ-COORDINATOR-PROPOSALS-005: Proposal cards

**Intent:** The manager decides in place, on either surface.

Mockup:

- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](../../../plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png): the chat proposal card.
- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](../../../plans/workspace-coordinator/assets/p1-01-needs-you.png): the Needs you screen the proposal item joins, with the badge on the sidebar entry.

#### Acceptance criteria

- **AC-COORDINATOR-PROPOSALS-005.1:** A `pending` proposal card shall show
  "Pending Approval", the title, workflow and step, and **Approve**, **Edit**
  and **Reject** for managers.
- **AC-COORDINATOR-PROPOSALS-005.2:** While a proposal is `approving`, the card
  shall say "Approval in progress. Edits are locked." with no actions.
- **AC-COORDINATOR-PROPOSALS-005.3:** A `failed` proposal card shall say "Could
  not create the task: <error>. Nothing was created." and keep Approve, Edit and
  Reject.
- **AC-COORDINATOR-PROPOSALS-005.4:** **Edit** on the Needs-you card shall open
  title, description, workflow, step (eligible steps only) and repository in
  place with **Approve with edits** and **Cancel**; an empty title shall be
  refused in place, and Cancel shall return focus to Edit.
- **AC-COORDINATOR-PROPOSALS-005.5:** **Reject** shall open an optional reason
  in place with **Confirm reject** and **Cancel**; Cancel shall return focus to
  Reject.
- **AC-COORDINATOR-PROPOSALS-005.6:** Edit and Reject on the chat card shall open
  the same forms on the Needs-you card.
- **AC-COORDINATOR-PROPOSALS-005.7:** After a decision the Needs-you card shall
  leave the list and a toast shall say "Approved. <card> created in <step>; no
  agent starts until you start it" or "Rejected. Nothing was created", followed
  by "Next: <n> items still need you" or "Next: nothing needs you. That is the
  working state."
- **AC-COORDINATOR-PROPOSALS-005.8:** A chat card shall show the settled state
  "Approved: <card>" or "Rejected: <reason>".
- **AC-COORDINATOR-PROPOSALS-005.9:** At a 390px-wide viewport the card actions
  and forms shall stack with touch targets of at least 44px and no horizontal
  scroll.

## Out of scope

- Undo of an approval (needs the phase 2 "What it did" log).
- Proposal classes other than creating a task, including messaging a running
  agent (phase 2).
- Replying to a proposal with a condition (phase 4).
- Any `automatic` write class (decision D13, gate G2 and phase 4).
- Expiry of pending proposals: they stay until decided or their coordinator is
  deleted.
