---
id: coordinator-integration
title: Phase 3 on phase 2 as built
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Phase 3 on phase 2 as built Requirements

## Overview

Phase 3 (autonomy) was specified while phase 2 (control) was still a design.
Phase 2 is now built, and it fixes answers that the phase 3 documents left
open: what a wake is filtered by, which tool list an unattended turn runs
with, what the "What it did" log says about an unattended or automatic act,
what an unattended turn is told, which proposal statuses and kinds exist, where
the Autonomy settings live, and where the "Woken by" entry renders. This
document states those rulings as testable behaviour. It changes no phase 2
requirement: every criterion below is either a consequence of a phase 2 rule
or an addition that phase 2 leaves room for.

## Terminology

- **Watch set:** a coordinator's effective Watches from phase 2
  (`REQ-COORDINATOR-PERMISSIONS` Watches): every workflow when its scope is
  `all`, otherwise the selected workflows that still exist. A task without a
  workflow is in no watch set.
- **Watched own task:** an own task ([wake](wake.md#terminology)) whose
  workflow is in the coordinator's watch set.
- **Bound tool list:** the list of Kandev tools stamped on a coordinator's
  conversation when it is opened, from the coordinator's policy at that time.
  The coordinator surface refuses any tool not on it, and the agent runtime's
  approval allowlist is built from it.
- **Unattended row:** a "What it did" row written while an unattended turn is
  open, carrying that turn's id.
- **Automatic row:** a "What it did" row for an approval made by the
  [automatic path](automatic.md), whose authorization is `automatic`.
- Other terms are defined in [wake](wake.md#terminology) and the
  [system README](../README.md#terms).

## Mockup

The plan's ASCII previews UI-04 (settings), UI-05 (reply), UI-06 (raising
`create_task`) and UI-08 (Sections row and log rows) are the reference for
this document's user-facing criteria.

## Requirements

### REQ-COORDINATOR-INTEGRATION-001: Phase 3 needs phase 2

**Intent:** Phase 3 reads phase 2's watch set, log and per-action settings, so
it never runs without them.

#### Acceptance criteria

- **AC-COORDINATOR-INTEGRATION-001.1:** While `features.coordinatorPhase2` is
  off, phase 3 shall be off as though `features.coordinatorPhase3` were off:
  no wake is stored, no backstop runs, no unattended turn starts, every phase 3
  route is 404 and the Autonomy section is hidden, and stored phase 3 rows are
  kept.

### REQ-COORDINATOR-INTEGRATION-002: Wakes follow the watch set

**Intent:** A coordinator is woken only about work it is watching, and the
unattended turn that follows sees no more than that.

#### Acceptance criteria

- **AC-COORDINATOR-INTEGRATION-002.1:** The system shall store a wake only for
  an episode on a watched own task, whether the episode arrives by an event or
  by the backstop. An episode on an own task whose workflow is not in the
  watch set, or that has no workflow, stores no wake. When the watch set cannot
  be read the system shall store no wake for that coordinator in that pass.
- **AC-COORDINATOR-INTEGRATION-002.2:** When delivery reads a pending wake
  whose task is no longer a watched own task, the system shall mark it
  `superseded`, as it does for a task that is archived or gone. When the watch
  set cannot be read, the wake shall stay `pending` and be excluded from that
  delivery.
- **AC-COORDINATOR-INTEGRATION-002.3:** Adding a workflow to the watch set, or
  moving an own task into a watched workflow, shall store wakes for that
  task's existing episodes at the next backstop pass, as turning autonomy on
  does.
- **AC-COORDINATOR-INTEGRATION-002.4:** During an unattended turn, the
  coordinator surface shall answer a read or proposal that names an unwatched
  workflow or task exactly as it does during an attended turn, and no wake path
  shall add its own filter or widen this one.

### REQ-COORDINATOR-INTEGRATION-003: One tool list, narrowed only by containment

**Intent:** An unattended turn has exactly the capability of the conversation
it runs in, or less.

#### Acceptance criteria

- **AC-COORDINATOR-INTEGRATION-003.1:** An unattended turn shall run with the
  bound tool list its conversation already has. Delivery shall not rebind,
  extend or replace it, and the wake message shall not name a tool that is not
  on it.
- **AC-COORDINATOR-INTEGRATION-003.2:** No unattended path (delivery,
  admission, containment or the permission denial) shall add a tool to the
  bound list or approve a permission request that the list's approval
  allowlist did not approve. Containment shall only remove capability; the
  immediate denial of `AC-COORDINATOR-CONTAINMENT-003.1` is the only thing it
  does to a request the allowlist left open.
- **AC-COORDINATOR-INTEGRATION-003.3:** A conversation opened before phase 3
  was on shall keep its bound list, so it has no `propose_improvement_kandev`
  tool until it is reopened; the tool shall be absent from its list and a call
  to it refused as not in the profile.

### REQ-COORDINATOR-INTEGRATION-004: The log names unattended and automatic acts

**Intent:** "What it did" says which acts no person watched and which no
person decided.

#### Acceptance criteria

- **AC-COORDINATOR-INTEGRATION-004.1:** A row written by the coordinator
  during an open unattended turn (a proposal, a refusal or an automatic
  approval) shall carry that turn's id, and the log shall mark it "During an unattended turn". A row
  written outside an unattended turn shall carry no turn id.
- **AC-COORDINATOR-INTEGRATION-004.2:** A manager's reply with a condition
  shall write a row with the outcome `returned`, the replying manager and the
  reply text as detail, and the log shall render it as "Returned with a
  condition".
- **AC-COORDINATOR-INTEGRATION-004.3:** An automatic approval, and the row of
  an automatic approval that failed, shall be written with the authorization
  `automatic` and the raising manager as actor. Every other row shall keep
  its existing authorization, including a manager's approval of a proposal
  whose automatic approval failed. The log reads that gate the raise shall
  exclude rows whose authorization is `automatic`.

### REQ-COORDINATOR-INTEGRATION-005: An unattended turn is told what an attended turn is told

**Intent:** Autonomy changes who starts a turn, not what the coordinator has
been instructed.

#### Acceptance criteria

- **AC-COORDINATOR-INTEGRATION-005.1:** An unattended turn shall carry the
  conversation's instructions as built when its session started, with the same
  sections (context, standing orders, goal), and the wake message shall carry
  no second copy of any of them.
- **AC-COORDINATOR-INTEGRATION-005.2:** A standing order shall be marked
  applied only where it is for an attended turn: when a proposal that cites it
  is stored. An unattended turn that stores a proposal citing an order marks it
  applied in the same transaction, and a turn that stores none marks nothing.
- **AC-COORDINATOR-INTEGRATION-005.3:** A change to an order, the goal or the
  context that resets the conversation shall apply to the next unattended
  turn exactly as to the next attended one, because that turn runs in the
  replacement conversation or, when there is none, is held with the reason
  `no_conversation`.

### REQ-COORDINATOR-INTEGRATION-006: A returned proposal and an improvement kind fit the store

**Intent:** The two phase 3 additions to proposals follow the rules phase 2's
kinds already follow.

#### Acceptance criteria

- **AC-COORDINATOR-INTEGRATION-006.1:** A reply with a condition shall set a
  proposal `returned` only from `pending`. A proposal in any other status,
  including `failed`, shall answer the reply with 409 and the current
  proposal, and a `failed` card shall keep Approve, Reject and Try again
  without **Reply with a condition**.
- **AC-COORDINATOR-INTEGRATION-006.2:** A `returned` proposal shall not be
  claimable by approve, shall be ignored by stale-claim recovery, shall not
  count toward the open-proposal limit, and shall not hold the open-target
  slot of its task.
- **AC-COORDINATOR-INTEGRATION-006.3:** A revised proposal shall be a new row
  with `in_reply_to`; the returned row shall never be reopened or edited.
- **AC-COORDINATOR-INTEGRATION-006.4:** An improvement shall be a proposal
  kind registered like message, move and resume, shown and counted with them,
  not governed by any of the six per-action settings, and never automatic.
- **AC-COORDINATOR-INTEGRATION-006.5:** An improvement approval left `approving`
  past the stale-claim window shall be settled `failed` with the reason
  `outcome_unknown` and no second execution, and a manager's Approve on that
  `failed` card shall run the approval again without storing a second pending
  change.

### REQ-COORDINATOR-INTEGRATION-007: Where autonomy is set and raised

**Intent:** Autonomy and the first automatic class are set where a manager
already looks for a coordinator's controls.

Mockup:

- Plan UI-08: the Sections row with Autonomy, and the log rows.

#### Acceptance criteria

- **AC-COORDINATOR-INTEGRATION-007.1:** While phase 3 is effective, the
  coordinator settings page shall show Autonomy as one more entry of the
  Sections row after Goal, holding the controls of `REQ-COORDINATOR-WAKE-004`,
  `REQ-COORDINATOR-SPEND-004` and `REQ-COORDINATOR-CONTAINMENT-002` and the
  changes waiting for the manager. While it is not effective the entry is
  absent and a link to it opens the first section.
- **AC-COORDINATOR-INTEGRATION-007.2:** The **Automatic** option of May do
  shall stay disabled with "Not available yet" for every action except
  `create_task`. For `create_task` it shall be enabled only for a manager
  while the coordinator is eligible ([automatic](automatic.md)), and the
  settings write shall refuse `automatic` for `create_task` with the first
  unmet condition otherwise. The settings write shall keep refusing
  `automatic` for every other action with `automatic_not_available`.

### REQ-COORDINATOR-INTEGRATION-008: The copilot shows the conversation everywhere

**Intent:** An unattended turn is visible wherever a manager reads the
conversation.

#### Acceptance criteria

- **AC-COORDINATOR-INTEGRATION-008.1:** The "Woken by" entry of
  `AC-COORDINATOR-WAKE-005.5` shall render in every surface that shows the
  coordinator conversation's transcript, with the same content and no
  behaviour that depends on the page.

## Out of scope

- Any change to phase 2's requirements, designs or built behaviour outside the
  additive columns, outcome and class values named in the
  [integration design](../system-design/integration.md).
- Waking on tasks outside the coordinator's own tasks. The watch set narrows
  own tasks; it does not add tasks ([wake](wake.md#out-of-scope)).
- An automatic class other than `create_task`, and an automatic improvement.
