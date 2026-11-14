---
id: coordinator-automatic
title: The first automatic action class
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-11-07
---

# The first automatic action class Requirements

## Overview

Decision D13 allows no `automatic` write before phase 2's "What it did" log has
recorded how managers decide. Phase 3 made exactly one class raisable to
`automatic`: creating a task on an eligible step (`create_task`). The
2026-11-07 amendment (REQ-COORDINATOR-AUTOMATIC-005 to -008) adds `resume` and
`start_agent`. A manager raises a class per coordinator through phase 2's D17
permission settings, and only when that coordinator's own record for the class
earns it. Everything else stays `requires approval` or `denied`. An automatic
create creates one ordinary task; it starts an agent only when the proposal
sets `starts_agent` and `start_agent` is also `automatic` (D15 as amended).

## Terminology

- **Action class:** one D17 action, as phase 2's permission settings name it.
  `create_task` is the class of `propose_task_kandev`.
- **Decided row:** a log row recording a manager's decision on one proposal of
  the class (approved, approved with edits, rejected or returned). An
  automatic approval is not a decided row; a manager's later decision on a
  proposal whose automatic approval failed is one.
- **Evidence window:** the coordinator's evidence window setting (default 30
  days) ending at the moment of the check.
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
the record "Raised to automatic, recorded against you", and merge (except as [merge stewardship](merge.md) permits) and Move to
Done as classes that cannot be raised. The plan's ASCII preview UI-06 is the
reference.

## Requirements

### REQ-COORDINATOR-AUTOMATIC-001: Which class may be raised

**Intent:** Only a class on the closed raisable allowlist may become
automatic: `create_task` in phase 3, plus `resume` and `start_agent` from the
2026-11-07 amendment. The allowlist is closed, not a denylist.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-001.1:** *(Amended 2026-11-07, see
  [REQ-COORDINATOR-AUTOMATIC-005](automatic-classes.md#req-coordinator-automatic-005-the-raisable-classes-amended).)*
  The system shall accept `automatic` only for the `create_task`, `resume` and
  `start_agent` classes. Setting any other class to `automatic`, including
  merging, moving to Done, stopping, moving, messaging, archiving, deleting
  and improvement proposals, shall be refused with 400 naming the class,
  changing nothing.
- **AC-COORDINATOR-AUTOMATIC-001.2:** *(Amended 2026-11-07.)* The permission
  settings shall show every class other than `create_task`, `resume` and
  `start_agent` with "Cannot be raised" in place of a raise control.

### REQ-COORDINATOR-AUTOMATIC-002: Earning the raise

**Intent:** The raise is gated on 30 days of this coordinator's own record,
reviewed by a person.

Mockup:

- Plan UI-06: the eligibility list and Review.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-002.1:** *(Amended 2026-11-07: thresholds are
  the coordinator's [eligibility settings](automatic-classes.md#req-coordinator-automatic-006-per-coordinator-eligibility-settings),
  shown here at their defaults, and the class may be any raisable class.)*
  The system shall treat a raisable class as eligible for one coordinator
  only when all hold: its earliest decided row of the class is at least the
  evidence window (default 30 days) old; the evidence window holds at least
  the minimum decisions (default 20) decided rows of the class; at least the
  approved-unedited floor (default 90%) of them were approved without edits;
  the class's no-undo condition holds (for `create_task`, no task created
  from its proposals in the evidence window was undone through the log; for
  `resume` and `start_agent`, see
  [AC-COORDINATOR-AUTOMATIC-008.4](automatic-lowering.md#req-coordinator-automatic-008-lowering-for-classes-that-cannot-be-undone)); and a class review for it was recorded within the review
  max age (default 7 days).
- **AC-COORDINATOR-AUTOMATIC-002.2:** A manager shall be able to record a class
  review; the record shall store the reviewer, the time, the evidence window
  and its row count, all computed by the server at the time of the review. A reader or a coordinator principal shall be refused.
- **AC-COORDINATOR-AUTOMATIC-002.3:** The permission settings shall list each
  eligibility condition as Met or Not met with its current value, and
  **Review the last <window>** (the label names the configured evidence window, default 30 days) shall open the log filtered to this coordinator
  and class before **Mark as reviewed** is offered.
- **AC-COORDINATOR-AUTOMATIC-002.4:** Setting a raisable class to `automatic`
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
  `automatic`, and return `{proposal_id, status: "approved", task_id}`. *(Amended
  2026-11-07.)* The created task shall start no agent unless the proposal sets
  `starts_agent` and `start_agent` is `automatic` too
  ([AC-COORDINATOR-AUTOMATIC-005.3](automatic-classes.md#req-coordinator-automatic-005-the-raisable-classes-amended));
  in that case the approval takes the manager's `starts_agent` path
  (eligible-starting-step check and auto-start metadata) unchanged, and a
  proposal with `starts_agent` set while `start_agent` is not `automatic`
  stays `pending` for a manager.
- **AC-COORDINATOR-AUTOMATIC-003.2:** At most the coordinator's daily
  automatic limit (default 10) of automatic approvals per coordinator, across
  all classes, per rolling 24 hours, counting each automatic approval that
  ended `failed`: beyond that, the proposal shall stay `pending` for a
  manager and the tool result shall say that the automatic limit was
  reached.
- **AC-COORDINATOR-AUTOMATIC-003.3:** When the automatic approval fails, the
  proposal shall be left `failed` exactly as a manager's failed approval is,
  for a manager to approve, edit or reject. A reply with a condition is not
  available on a `failed` proposal
  (`AC-COORDINATOR-INTEGRATION-006.1`).
- **AC-COORDINATOR-AUTOMATIC-003.4:** Every automatic approval shall be a log
  row with the authorization `automatic` and the manager who raised the setting as its actor.

### REQ-COORDINATOR-AUTOMATIC-004: Lowering

**Intent:** The raise is easy to take back and is taken back when it goes
wrong.

#### Acceptance criteria

- **AC-COORDINATOR-AUTOMATIC-004.1:** A manager shall be able to lower any
  raisable class to `requires approval` at any time, with no eligibility check.
  Once the lower is stored, every proposal not yet claimed shall wait for a
  manager: the automatic path re-reads the setting under its lock
  immediately before the claim. At most the one claim already past that
  re-read when the lower is stored completes automatically.
- **AC-COORDINATOR-AUTOMATIC-004.2:** When a task created by an automatic
  approval (not a manager's approval of a proposal whose automatic approval
  failed) is undone through the log, the system shall lower that
  coordinator's `create_task` to `requires approval` and record the lowering
  with the undo as its reason. *(Amended 2026-11-07.)* When that proposal set
  `starts_agent`, the system shall also lower `start_agent` with the same
  reason.
- **AC-COORDINATOR-AUTOMATIC-004.3:** A coordinator's runtime session shall
  never raise or lower any class, on any transport.

## Out of scope

- The classes and settings added by the 2026-11-07 amendment, which
  [automatic-classes](automatic-classes.md) owns, including what stays out of
  scope there.
- Workspace-wide or instance-wide automatic settings.
- The log, its undo and the D17 settings storage themselves, which are phase 2
  work; this document adds the raise gate and the automatic path on top of
  them.
