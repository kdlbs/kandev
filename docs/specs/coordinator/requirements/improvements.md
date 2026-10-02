---
id: coordinator-improvements
title: Improvement proposals
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Improvement proposals Requirements

## Overview

Once a coordinator runs unattended turns, it can see where its own
instructions cost it time or money. Phase 3 lets it propose a change to its
own context text, linked to the unattended turns that motivated it. A manager
approves the proposal as a reviewable change; nothing is applied until a
manager applies that change in the coordinator's settings. A coordinator can
never change its own configuration or permissions.

## Terminology

- **Improvement proposal:** a proposal of kind `improvement` that carries a
  replacement for the coordinator's context text, a rationale and evidence.
  It shares the proposal store, statuses, open limit and cards with task
  proposals.
- **Evidence:** one to ten references, each an unattended turn of this
  coordinator (a **run**) or a task in its workspace. At least one reference
  is a run.
- **Pending change:** the stored result of approving an improvement: the
  proposed context text and the context it was written against (its base),
  waiting for a manager to apply or discard it.
- Other terms are defined in [proposals](proposals.md#terminology) and
  [wake](wake.md#terminology).

## Mockup

The source mockup (`mockup-v2.1`, outside this repository) shows an
improvement card with a why metric, "Runs behind it", a "Changes Policy" pill,
"Show the policy diff", "Approve as a reviewable change" disabled until the
diff is opened, and the toast "Approved as a reviewable change. Nothing
self-applied." Phase 3's improvement changes the context, not the policy; the
plan's ASCII preview UI-07 is the reference.

## Requirements

### REQ-COORDINATOR-IMPROVEMENTS-001: Proposing an improvement

**Intent:** The coordinator can suggest a change to its own instructions,
with the runs that show why.

#### Acceptance criteria

- **AC-COORDINATOR-IMPROVEMENTS-001.1:** A conversation opened while phase 3
  is effective ([integration](integration.md#terminology)) shall have
  `propose_improvement_kandev`; one opened earlier shall not until it is
  reopened (`AC-COORDINATOR-INTEGRATION-003.3`). When the tool is called with
  a title of 1 to 60 characters after trimming, a rationale of 1 to 10,000
  characters after trimming, a non-empty replacement context that passes the
  coordinator context validation, and valid evidence, the system shall store
  one `pending` improvement proposal holding the current context beside the
  replacement, write its `proposed` activity row (class `improvement`) in the
  same transaction, emit `coordinator.updated` and change nothing else. When
  the tool is called while phase 3 is not effective, including from a
  conversation opened while it was, the system shall answer with the phase 1
  unknown-action error, store nothing and write no activity row.
- **AC-COORDINATOR-IMPROVEMENTS-001.2:** The system shall refuse the call,
  storing nothing and naming the field, when any field above is invalid, when
  the replacement context is empty or equals the current context, or when the
  evidence has fewer than one or more than ten entries, names the same run or
  the same task twice, has an entry that is not exactly one of `run_id` or
  `task_id`, has no run, names a run that is not this coordinator's, or names a
  task outside its workspace.
- **AC-COORDINATOR-IMPROVEMENTS-001.3:** Improvement proposals shall count
  toward the coordinator's limit of 25 open proposals.
- **AC-COORDINATOR-IMPROVEMENTS-001.4:** While phase 3 is not effective,
  improvement proposals shall be hidden from every read and decision route: not
  listed, not counted toward the limit, and not decidable (a decision on one
  answers as an absent proposal), except that a claim left `approving` on one
  shall still be settled by the stale-claim sweep or by a manager's Approve on
  it, and an Approve already past its claim shall still complete, exactly as
  for phase 2 kinds. They shall appear again, with
  their stored status, when phase 3 is effective again.

### REQ-COORDINATOR-IMPROVEMENTS-002: The improvement card

**Intent:** The manager sees the change and its evidence before deciding.

Mockup:

- Plan UI-07: the improvement card.

#### Acceptance criteria

- **AC-COORDINATOR-IMPROVEMENTS-002.1:** An improvement card shall show
  "Improvement", the title, the rationale, the pill "Changes coordinator
  context", and "Runs behind it" listing each run's start time, outcome and
  cost and each task's identifier and title; a run whose record has been
  pruned shall read "Run record expired".
- **AC-COORDINATOR-IMPROVEMENTS-002.2:** **Show the change** shall reveal the
  context as it was when proposed and the proposed context as a line diff. For
  managers on a `pending` or `failed` improvement, **Approve as a reviewable
  change** shall stay disabled until the change has been shown in this card,
  with **Reject** available on both, **Reply with a condition** available on a
  `pending` one only, and no **Edit**. The diff shall mark removed lines with `-`
  and added lines with `+`, not by colour alone, and a `failed` improvement shall
  read "Approving did not finish. Nothing was applied. You can approve again."
  while a successful Approve shall toast "Approved as a reviewable change.
  Nothing was applied."
- **AC-COORDINATOR-IMPROVEMENTS-002.3:** An improvement proposal shall never be
  approved automatically, whatever the D17 settings say.

### REQ-COORDINATOR-IMPROVEMENTS-003: Approve, then apply

**Intent:** Approval records a change; only a manager's separate apply
changes the coordinator.

#### Acceptance criteria

- **AC-COORDINATOR-IMPROVEMENTS-003.1:** When a manager approves an improvement
  proposal, the system shall set it `approved`, store one pending change, leave
  the coordinator's configuration unchanged, and the card shall say "Approved
  as a reviewable change. Nothing was applied." while the change is pending;
  once the change is applied or discarded the card shall say so instead, and
  the wording is in [the design](../system-design/improvements.md#card).
- **AC-COORDINATOR-IMPROVEMENTS-003.2:** The coordinator's settings shall list
  pending changes with the diff, **Apply** and **Discard**. Apply shall write
  the context exactly as a manager's context edit does, including replacing
  the conversation and the same refusals (a stored replacement that no longer
  passes context validation, or an agent or executor profile that no longer resolves,
  or the autonomy interlock, refuses Apply and leaves the change pending). When the current context
  differs from the change's base, Apply shall be refused with 409 and the
  change shall stay pending, for the manager to discard. A manager's context
  edit and an Apply that race shall never lose either write: the one that
  commits second either sees the first (Apply then answers 409) or applies on
  top of it (the edit). The settings page shall not start an Apply while its own
  save is in flight, nor a save while an Apply is in flight, so a finishing save
  never restores the context Apply replaced.
- **AC-COORDINATOR-IMPROVEMENTS-003.3:** Apply and Discard shall be refused to
  readers and to a coordinator principal on any transport, and each shall
  settle the pending change exactly once under concurrent calls; a call that
  finds the change already settled shall answer 409 with the change and
  change nothing.
- **AC-COORDINATOR-IMPROVEMENTS-003.4:** A pending change shall be listed,
  applied and discarded only within its own coordinator and workspace and only
  while the proposal that produced it is `approved`; otherwise the request
  shall answer as an absent change (404) and change nothing. The routes shall
  exist only while phase 3 is effective.

## Out of scope

- Improvements to anything other than the coordinator's context text: its
  permissions, phase 2's Standing orders, Watches or May do, workflows, and
  other agents' profiles. A later change may add targets; none may be D17
  permissions.
- Applying a change automatically, and editing an improvement proposal.
- Any metric the coordinator computes itself; the card shows stored run
  records only.
