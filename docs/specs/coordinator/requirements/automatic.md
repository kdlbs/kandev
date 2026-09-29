---
id: coordinator-automatic
title: The first automatic action class
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# The first automatic action class Requirements

## Overview

Decision D13 allows no `automatic` write before phase 2's "What it did" log has
recorded how managers decide. Phase 3 makes exactly one class raisable to
`automatic`: creating a task on an eligible step (`create_task`). A manager
raises it per coordinator through phase 2's D17 permission settings, and only
when that coordinator's own record for the class earns it. Everything else
stays `requires approval` or `denied`. An automatic create still creates one
ordinary task and never starts an agent (D15).

## Terminology

- **Action class:** one D17 action, as phase 2's permission settings name it.
  `create_task` is the class of `propose_task_kandev`.
- **Decided row:** a log row recording a manager's decision on one proposal of
  the class (approved, approved with edits, rejected or returned). An
  automatic approval is not a decided row; a manager's later decision on a
  proposal whose automatic approval failed is one.
- **Evidence window:** the 30 days ending at the moment of the check.
- **Class review:** a manager's record that they reviewed one coordinator's
  evidence window for one class.
- **Automatic approval:** an approval the system makes because the class is
  `automatic`, recorded in the log with the authorization `automatic` (its
  decider) and the manager who raised the setting.
- Other terms are defined in [proposals](proposals.md#terminology) and the
  [system README](../README.md#terms).

## Mockup

The source mockup (`mockup-v2.1`, outside this repository) shows "Raise to
automatic" (in the built settings page the raise is phase 2's **Automatic**
option of May do, saved with the page's Save; see
[integration](integration.md#requirements)) gated "until the last 30 days of this class has been reviewed",
the record "Raised to automatic, recorded against you", and merge and Move to
Done as classes that cannot be raised. The plan's ASCII preview UI-06 is the
reference.

## Requirements

### REQ-COORDINATOR-AUTOMATIC-001: Which class may be raised

**Intent:** Only a class with a reversible, agent-free effect may become
automatic, and only `create_task` in phase 3.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-001.1:** The system shall accept `automatic` only
  for the `create_task` class. Setting any other class to `automatic`,
  including merging, moving to Done, starting, resuming, stopping, moving,
  messaging, archiving, deleting and improvement proposals, shall be refused
  with 400 naming the class, changing nothing.
- **AC-COORDINATOR-AUTOMATIC-001.2:** The permission settings shall show every
  class other than `create_task` with "Cannot be raised" in place of a raise
  control.

### REQ-COORDINATOR-AUTOMATIC-002: Earning the raise

**Intent:** The raise is gated on 30 days of this coordinator's own record,
reviewed by a person.

Mockup:

- Plan UI-06: the eligibility list and Review.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-002.1:** The system shall treat `create_task` as
  eligible for one coordinator only when all hold: its earliest decided row
  of the class is at least 30 days old; the evidence window holds at least 20
  decided rows of the class; at least 90% of them were approved without edits;
  no task created from its proposals in the evidence window was undone
  through the log; and a class review for it was recorded within the last 7
  days.
- **AC-COORDINATOR-AUTOMATIC-002.2:** A manager shall be able to record a class
  review; the record shall store the reviewer, the time, the evidence window
  and its row count, all computed by the server at the time of the review. A reader or a coordinator principal shall be refused.
- **AC-COORDINATOR-AUTOMATIC-002.3:** The permission settings shall list each
  eligibility condition as Met or Not met with its current value, and
  **Review the last 30 days** shall open the log filtered to this coordinator
  and class before **Mark as reviewed** is offered.
- **AC-COORDINATOR-AUTOMATIC-002.4:** Setting `create_task` to `automatic`
  while it is not eligible shall be refused with 409 naming the first unmet
  condition, changing nothing. When the log cannot be read, the class shall be
  treated as not eligible.

### REQ-COORDINATOR-AUTOMATIC-003: What automatic does

**Intent:** An automatic create is the approval a manager would have given,
bounded and recorded.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-003.1:** When the coordinator calls
  `propose_task_kandev`, the call passes validation and `create_task` is
  `automatic` for it, the system shall store the proposal and approve it
  through the same approve path as a manager, with no edits and decider
  `automatic`, and return `{proposal_id, status: "approved", task_id}`. The
  created task shall start no agent.
- **AC-COORDINATOR-AUTOMATIC-003.2:** At most 10 automatic approvals per
  coordinator per rolling 24 hours, counting each automatic approval that
  ended `failed`: beyond that, the proposal shall stay `pending` for a
  manager and the tool result shall say that the automatic limit was
  reached.
- **AC-COORDINATOR-AUTOMATIC-003.3:** When the automatic approval fails, the
  proposal shall be left `failed` exactly as a manager's failed approval is,
  for a manager to approve, edit, reply to or reject.
- **AC-COORDINATOR-AUTOMATIC-003.4:** Every automatic approval shall be a log
  row with the authorization `automatic` and the manager who raised the setting as its actor.

### REQ-COORDINATOR-AUTOMATIC-004: Lowering

**Intent:** The raise is easy to take back and is taken back when it goes
wrong.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-004.1:** A manager shall be able to lower
  `create_task` to `requires approval` at any time, with no eligibility check.
  Once the lower is stored, every proposal not yet claimed shall wait for a
  manager: the automatic path re-reads the setting under its lock
  immediately before the claim. At most the one claim already past that
  re-read when the lower is stored completes automatically.
- **AC-COORDINATOR-AUTOMATIC-004.2:** When a task created by an automatic
  approval (not a manager's approval of a proposal whose automatic approval
  failed) is undone through the log, the system shall lower that
  coordinator's `create_task` to `requires approval` and record the lowering
  with the undo as its reason.
- **AC-COORDINATOR-AUTOMATIC-004.3:** A coordinator's runtime session shall
  never raise or lower any class, on any transport.

## Out of scope

- Any second automatic class. Each later class is its own decision, chosen the
  same way.
- Workspace-wide or instance-wide automatic settings.
- The log, its undo and the D17 settings storage themselves, which are phase 2
  work; this document adds the raise gate and the automatic path on top of
  them.
