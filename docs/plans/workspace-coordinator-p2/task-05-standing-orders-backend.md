---
id: "05-standing-orders-backend"
title: "Standing orders backend"
status: pending
wave: 2
depends_on:
  - "01-shared-interface"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-STANDING-ORDERS-001
  - REQ-COORDINATOR-STANDING-ORDERS-002
  - REQ-COORDINATOR-STANDING-ORDERS-003
acceptance_criteria:
  - AC-COORDINATOR-STANDING-ORDERS-001.1
  - AC-COORDINATOR-STANDING-ORDERS-001.2
  - AC-COORDINATOR-STANDING-ORDERS-001.3
  - AC-COORDINATOR-STANDING-ORDERS-001.7
  - AC-COORDINATOR-STANDING-ORDERS-002.1
  - AC-COORDINATOR-STANDING-ORDERS-002.2
  - AC-COORDINATOR-STANDING-ORDERS-002.3
  - AC-COORDINATOR-STANDING-ORDERS-003.3
system_design:
  - ../../specs/coordinator/system-design/standing-orders.md
---

# Task 05: Standing Orders Backend (WP-7)

## Summary

Serve standing orders: routes with their limits and idempotent retire and
restore, the instruction section a conversation opens with, and the
last-applied read.

## In scope

- Standing orders: `GET`, `POST`, `POST :oid/retire`, `POST :oid/restore`
  with trim and 1 to 500 characters, the limit of 20 active under the
  per-coordinator lock (add, retire and restore all take it),
  idempotent retire and restore (restoring an active
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

## Out of scope

- The goal (task 12).
- The Standing orders section and the reject offer (tasks 11, 09).
- `standing_order_ids` validation on propose tools (task 04).

## Acceptance

- Concurrent adds never leave more than 20 active orders.
- Every change that alters what a conversation is told resets it.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
```

Tests: 25 concurrent adds leave 20 active and 5 `standing_order_limit`
refusals on both dialects; retire twice returns 200 with no second reset;
the instruction builder output with 0, 1 and 20 orders (golden text).

## Likely files

- `apps/backend/internal/coordinator/standing_orders.go`,
  `standing_order_routes.go`, `instructions.go`

## Dependencies

- Task 01 (tables, `resetConversation`).

## Risks

- Task 12 adds its goal section to the same instruction builder; the
  builder takes ordered sections so neither work order edits the other's.
