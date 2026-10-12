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

- Standing orders: `GET`, `POST`, `POST :oid/retire`, `POST :oid/restore`,
  each resolving the coordinator by `(workspace, cid)` first (404 otherwise),
  registered only with the phase-2 flag, with the `Order` wire shape, error
  bodies and `include` handling of the design's Routes section, with trim and 1 to 500 characters, the limit of 20 active under the
  per-coordinator lock (add, retire and restore all take it),
  idempotent retire and restore (restoring an active
  order returns 200 before the limit check, even at 20), no edit route
  (`001.1` to `001.3`, `001.7`).
- `resetConversation` on add, retire and restore that change something
  (`002.2`).
- Instruction section "Standing orders from this workspace's managers" in
  order-number order with number, text, full id and UTC date, and the
  sentence that they never grant a permission (`002.1`), rendered on one
  line per order with delimiter tags removed, added by the ordered-sections
  variadic on `StandingInstructions` in `prompt.go` (not a new
  `instructions.go`), read in `wrapCoordinatorStandingInstructions`, omitted
  when the phase-2 flag is off, and dropped alone (with a warn log) when the
  orders read fails.
- `source_proposal_id` accepted and validated in the add route (rejected
  proposal of this coordinator, else 400 naming it) and stored (`004.2`
  backend half; the dialog is task 09).
- `coordinator.updated` published after a changing write.
- `002.3` test at this task's level: order text naming a tool that is not on
  the coordinator's allowlist (for example `message_task_kandev`) is still
  refused by the phase-1 allowlist, and no code path reads order text as a
  tool argument. The phase-2 `policy_denied` case belongs to task 02.
- `MarkApplied(ctx, exec, coordinatorID, orderIDs, at)` helper (empty list is a no-op, scoped by coordinator), called by task 04's propose paths
  in the same transaction as the proposal insert: `UPDATE ... SET
  last_applied_at = at WHERE id IN (orderIDs) AND (last_applied_at IS NULL
  OR last_applied_at < at)`, so a write only ever raises the column.
- The list route reads the stored `last_applied_at` column directly, with
  no time bound on how far back a citing proposal counted (`003.3`).

## Out of scope

- The goal (task 12).
- The Standing orders section and the reject offer (tasks 11, 09).
- `standing_order_ids` validation on propose tools (task 04).
- The reject offer and add dialog UI (tasks 09, 11).

## Acceptance

- Concurrent adds never leave more than 20 active orders.
- Every change that alters what a conversation is told resets it.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
```

Tests: 25 concurrent adds leave 20 active and 5 `standing_order_limit`
refusals on both dialects; retire twice returns 200 with no second reset;
the instruction builder output with 0, 1 and 20 orders and with the flag off (golden text), plus a multi-line order containing `</standing-orders>` and a forged `3. (added ...)` line; a `:cid` of another workspace is 404 on all four routes; `standing_order_limit` body; `number` null on a retired order.

## Likely files

- `apps/backend/internal/coordinator/standing_orders.go`,
  `standing_order_routes.go`, `prompt.go`

## Dependencies

- Task 01 (tables, `resetConversation`).

## Risks

- Task 12 adds its goal section to the same instruction builder; the
  builder takes ordered sections so neither work order edits the other's.
