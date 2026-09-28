---
id: coordinator-standing-orders
title: Standing orders
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Standing orders Requirements

## Overview

A standing order is a dated rule a manager gives a coordinator, such as
"Prefer small cards" or "Never propose work on the release board on
Fridays". Orders are given to the coordinator when a conversation starts.
Changing them starts the next conversation fresh. Orders are guidance only:
they grant nothing, and the May do settings still decide what can run. A
rejection reason can become an order in one step.

## Terminology

- **Active order:** a standing order that is not retired.
- **Order number:** the 1-based position of an active order when the active
  orders are sorted by `created_at` ascending, then `id` ascending.
- Other terms are defined in the [system README](../README.md#terms) and
  [permissions](permissions.md#terminology).

## Mockup

- [`docs/plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png`](../../../plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png): the Sections row with Standing orders and its help text.

## Requirements

### REQ-COORDINATOR-STANDING-ORDERS-001: Adding and retiring orders

**Intent:** A manager keeps a short list of rules the coordinator follows.

**User story:** As a workspace manager, I want to give my coordinator
standing rules, so that I do not repeat them in every conversation.

Mockup:

- [`docs/plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png`](../../../plans/workspace-coordinator-p2/assets/p2-02-settings-coordinator-sections.png): the Standing orders tab.

#### Acceptance criteria

- **AC-COORDINATOR-STANDING-ORDERS-001.1:** When a manager adds a standing
  order whose text, after trimming, is 1 to 500 characters, the system shall
  store it with a new id, the trimmed text, `created_at` and the manager's
  user id, and return it. Other text shall be refused with 400 naming
  `text`.
- **AC-COORDINATOR-STANDING-ORDERS-001.2:** When a coordinator already has 20
  active orders, the system shall refuse an add, or a restore of a retired
  order, with 400 and the code `standing_order_limit`; restoring an order
  that is already active shall return it as `AC-COORDINATOR-STANDING-ORDERS-001.3`
  says, whatever the count. Concurrent adds shall never leave more than 20
  active.
- **AC-COORDINATOR-STANDING-ORDERS-001.3:** When a manager retires an active
  order, the system shall set its `retired_at` and `retired_by` and keep the
  row. When a manager restores a retired order, the system shall clear both.
  Retiring a retired order or restoring an active one shall return the order
  unchanged with 200.
- **AC-COORDINATOR-STANDING-ORDERS-001.4:** The Standing orders section shall
  list the active orders in order-number order, each showing "Standing order
  N", its text, "Added <date>", its last-applied text
  (`AC-COORDINATOR-STANDING-ORDERS-003.3`) and **Retire this order**, with
  **Add standing order** below the list. With no active order it shall say
  that there are no standing orders yet.
- **AC-COORDINATOR-STANDING-ORDERS-001.5:** When a manager retires an order, a
  toast shall say it was retired and offer **Undo** for 10 seconds; Undo shall
  restore it.
- **AC-COORDINATOR-STANDING-ORDERS-001.6:** A reader shall see the list without
  Add, Retire or Undo; a reader's add, retire or restore request shall be
  refused with 403 and change nothing.
- **AC-COORDINATOR-STANDING-ORDERS-001.7:** An order's text cannot be edited;
  to change a rule, a manager retires it and adds a new one.

### REQ-COORDINATOR-STANDING-ORDERS-002: Given when a conversation starts

**Intent:** Orders reach the coordinator without depending on it to look.

#### Acceptance criteria

- **AC-COORDINATOR-STANDING-ORDERS-002.1:** When a coordinator conversation
  opens, its standing instructions shall include every active order in
  order-number order, each with its number, its text and its added date,
  introduced as rules from the workspace's managers that never grant a
  permission. With no active order, the instructions shall have no
  standing-orders section.
- **AC-COORDINATOR-STANDING-ORDERS-002.2:** When a manager adds, retires or
  restores an order, the system shall archive the coordinator's current
  conversation task and clear its reference, so the next conversation starts
  with the new list. A retire or restore that changes nothing shall keep the
  conversation.
- **AC-COORDINATOR-STANDING-ORDERS-002.3:** An order shall grant no permission:
  an action the coordinator attempts because of an order is decided by its
  May do settings and the guard alone.

### REQ-COORDINATOR-STANDING-ORDERS-003: Shaped by, and last applied

**Intent:** A manager sees which rule shaped a proposal.

#### Acceptance criteria

- **AC-COORDINATOR-STANDING-ORDERS-003.1:** Every propose tool shall accept an
  optional `standing_order_ids` list of at most 5 ids. When any id is not an
  active order of the calling coordinator, or the list is longer than 5 or
  repeats an id, the system shall refuse the call naming
  `standing_order_ids`. Accepted ids shall be stored on the proposal.
- **AC-COORDINATOR-STANDING-ORDERS-003.2:** A proposal card with cited orders
  shall show "Shaped by: Standing order N" for each order that is still
  active, and "Shaped by: a retired standing order" for each that is not; the
  order's text shall be available from the label.
- **AC-COORDINATOR-STANDING-ORDERS-003.3:** Each active order shall show "Last
  applied <relative time>", from the most recent `created_at` of the
  coordinator's proposals that cite it, or "Never applied" when none does.

### REQ-COORDINATOR-STANDING-ORDERS-004: Make it a standing order

**Intent:** A manager turns a reason for rejecting into a lasting rule.

#### Acceptance criteria

- **AC-COORDINATOR-STANDING-ORDERS-004.1:** When a manager rejects a proposal
  with a non-empty reason while the phase-2 flag is on, a toast shall say
  "Rejected. Keep the reason as a standing order?" with **Make it a standing
  order** for 10 seconds. A rejection without a reason shall show no offer.
- **AC-COORDINATOR-STANDING-ORDERS-004.2:** When the manager chooses **Make it
  a standing order**, an add dialog shall open with the reason as editable
  text; saving shall add the order as in
  `AC-COORDINATOR-STANDING-ORDERS-001.1` with `source_proposal_id` set to the
  rejected proposal, and cancelling shall add nothing.

## Out of scope

- Editing an order's text in place.
- Orders shared by several coordinators or set for a whole workspace.
- The coordinator suggesting its own orders (phase 3 improvement proposals).
- Checking whether the coordinator followed an order; last applied counts
  only the orders its proposals cite.
