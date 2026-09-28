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

- **AC-COORDINATOR-IMPROVEMENTS-001.1:** While `features.coordinatorPhase3` is
  on, a coordinator session shall have `propose_improvement_kandev`. When it
  is called with a title of 1 to 60 characters after trimming, a rationale of
  at most 10,000 characters, a replacement context that passes the
  coordinator context validation, and valid evidence, the system shall store
  one `pending` improvement proposal holding the current context beside the
  replacement, emit `coordinator.updated`
  and change nothing else.
- **AC-COORDINATOR-IMPROVEMENTS-001.2:** The system shall refuse the call,
  storing nothing and naming the field, when any field above is invalid, when
  the evidence has fewer than one or more than ten references, has no run,
  names a run that is not this coordinator's, or names a task outside its
  workspace, or when the replacement equals the current context.
- **AC-COORDINATOR-IMPROVEMENTS-001.3:** Improvement proposals shall count
  toward the coordinator's limit of 25 open proposals.

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
  current and proposed context as a line diff. For managers,
  **Approve as a reviewable change** shall stay disabled until the change has
  been shown in this card, with **Reject** and **Reply with a condition**
  always available. There shall be no **Edit**.
- **AC-COORDINATOR-IMPROVEMENTS-002.3:** An improvement proposal shall never be
  approved automatically, whatever the D17 settings say.

### REQ-COORDINATOR-IMPROVEMENTS-003: Approve, then apply

**Intent:** Approval records a change; only a manager's separate apply
changes the coordinator.

#### Acceptance criteria

- **AC-COORDINATOR-IMPROVEMENTS-003.1:** When a manager approves an improvement
  proposal, the system shall set it `approved`, store one pending change, leave
  the coordinator's configuration unchanged, and the card shall say "Approved
  as a reviewable change. Nothing was applied."
- **AC-COORDINATOR-IMPROVEMENTS-003.2:** The coordinator's settings shall list
  pending changes with the diff, **Apply** and **Discard**. Apply shall write
  the context exactly as a manager's context edit does, including replacing
  the conversation; when the current context differs from the change's base,
  Apply shall be refused with 409 and the change shall stay pending, for the
  manager to discard.
- **AC-COORDINATOR-IMPROVEMENTS-003.3:** Apply and Discard shall be refused to
  readers and to a coordinator principal on any transport, and each shall
  settle the pending change exactly once under concurrent calls.

## Out of scope

- Improvements to anything other than the coordinator's context text: its
  permissions, phase 2's Standing orders, Watches or May do, workflows, and
  other agents' profiles. A later change may add targets; none may be D17
  permissions.
- Applying a change automatically, and editing an improvement proposal.
- Any metric the coordinator computes itself; the card shows stored run
  records only.
