---
id: "05-orders-goals-backend"
title: "Standing orders and goals backend"
status: pending
wave: 3
depends_on:
  - "01-shared-interface"
  - "03-activity-log-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-STANDING-ORDERS-001
  - REQ-COORDINATOR-STANDING-ORDERS-002
  - REQ-COORDINATOR-STANDING-ORDERS-003
  - REQ-COORDINATOR-GOALS-001
  - REQ-COORDINATOR-GOALS-003
acceptance_criteria:
  - AC-COORDINATOR-STANDING-ORDERS-001.1
  - AC-COORDINATOR-STANDING-ORDERS-001.2
  - AC-COORDINATOR-STANDING-ORDERS-001.3
  - AC-COORDINATOR-STANDING-ORDERS-001.7
  - AC-COORDINATOR-STANDING-ORDERS-002.1
  - AC-COORDINATOR-STANDING-ORDERS-002.2
  - AC-COORDINATOR-STANDING-ORDERS-002.3
  - AC-COORDINATOR-STANDING-ORDERS-003.3
  - AC-COORDINATOR-GOALS-001.1
  - AC-COORDINATOR-GOALS-001.2
  - AC-COORDINATOR-GOALS-001.3
  - AC-COORDINATOR-GOALS-001.4
  - AC-COORDINATOR-GOALS-001.5
  - AC-COORDINATOR-GOALS-001.6
  - AC-COORDINATOR-GOALS-001.7
  - AC-COORDINATOR-GOALS-001.8
  - AC-COORDINATOR-GOALS-003.1
  - AC-COORDINATOR-GOALS-003.2
  - AC-COORDINATOR-GOALS-003.3
  - AC-COORDINATOR-GOALS-003.4
system_design:
  - ../../specs/coordinator/system-design/standing-orders.md
  - ../../specs/coordinator/system-design/goals.md
---

# Task 05: Standing Orders and Goals Backend (WP-7, WP-10)

## Summary

Serve standing orders and the goal: routes with their limits and idempotent
retire and restore, the instruction sections a conversation opens with, the
last-applied read, the baseline recorded in the goal's transaction and the
measures computed at read time.

## In scope

- Standing orders: `GET`, `POST`, `POST :oid/retire`, `POST :oid/restore`
  with trim and 1 to 500 characters, the limit of 20 active under the
  per-coordinator lock, idempotent retire and restore (restoring an active
  order returns 200 before the limit check, even at 20), no edit route
  (`001.1` to `001.3`, `001.7`).
- `resetConversation` on add, retire and restore that change something
  (`002.2`).
- Instruction section "Standing orders from this workspace's managers" in
  order-number order with number, text and date, and the sentence that they
  never grant a permission (`002.1`). A test that an order text asking for a
  denied action is still refused by the guard (`002.3`).
- `last_applied_at` per active order from every proposal that cites it, with
  no time bound (`003.3`).
- Goal: `GET goal`, `PUT goal` (create or update in place, criteria keep
  their done state by id; an unknown or repeated id is 400 naming
  `criteria[i].id`; an omitted criterion is removed), `POST
  goal/criteria/:cid` done toggle under the per-coordinator lock,
  `POST goal/met` idempotent, 403 for readers (`GOALS-001.1` to `001.5`,
  `001.8`).
- Goal instruction section with done states, or "No goal is set"
  (`001.6`); `resetConversation` on name, due, criteria, met and new goal,
  not on a done toggle (`001.7`).
- Baselines in the activation transaction: Open tasks over watched tasks,
  Approved and Rejected over 7 days, none when the coordinator is younger
  than 7 days (`003.1`, `003.2`); current values and direction with the
  threshold of 2 at read time (`003.3`, `003.4`).

## Out of scope

- Standing orders and Goal sections, the reject offer and the goal note
  (tasks 06, 09, 11).
- `standing_order_ids` validation on propose tools (task 04).

## Acceptance

- Concurrent adds never leave more than 20 active orders.
- A goal's baseline is written once, with the goal, and never recomputed.
- Every change that alters what a conversation is told resets it; a done
  toggle does not.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
```

Tests: 25 concurrent adds leave 20 active and 5 `standing_order_limit`
refusals on both dialects; retire twice returns 200 with no second reset;
the instruction builder output with 0, 1 and 20 orders and with a met,
active or absent goal (golden text); baseline with a coordinator 6 and 8
days old; direction at deltas 1, 2 and -2; the active-goal unique index
refuses a second concurrent create and the loser returns the winner.

## Likely files

- `apps/backend/internal/coordinator/standing_orders.go`,
  `standing_order_routes.go`, `goals.go`, `goal_routes.go`,
  `measures.go`, `instructions.go`

## Dependencies

- Task 01 (tables, `resetConversation`); task 03 (the summary read shares
  the proposal counts used by measures).

## Risks

- Measures read many tasks; they use the Watches-filtered count query of
  phase 1, not a per-task loop.
