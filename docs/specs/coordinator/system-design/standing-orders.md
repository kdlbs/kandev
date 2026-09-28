---
id: coordinator-standing-orders-design
title: Standing orders design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-STANDING-ORDERS-001
  - REQ-COORDINATOR-STANDING-ORDERS-002
  - REQ-COORDINATOR-STANDING-ORDERS-003
  - REQ-COORDINATOR-STANDING-ORDERS-004
---

# Standing orders System Design

## Purpose and boundaries

This design stores standing orders per coordinator, gives them to the
coordinator in its standing instructions (ADR D24), lets propose tools cite
them, and adds the Standing orders section and the Make it a standing order
offer. Orders are text; they never reach the guard or the policy
([permissions](permissions.md)).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-STANDING-ORDERS-001` | [Store](#store), [Routes](#routes), [Standing orders UI](#standing-orders-ui) |
| `REQ-COORDINATOR-STANDING-ORDERS-002` | [Instructions](#instructions), [Conversation reset](#conversation-reset) |
| `REQ-COORDINATOR-STANDING-ORDERS-003` | [Citations](#citations), [Last applied](#last-applied), [Shaped by UI](#shaped-by-ui) |
| `REQ-COORDINATOR-STANDING-ORDERS-004` | [Make it a standing order](#make-it-a-standing-order) |

## Store

`coordinator_standing_orders` in the coordinator store, both dialects:

| Column | Type | Notes |
| --- | --- | --- |
| `id` | text primary key | UUID |
| `coordinator_id` | text not null | indexed with `retired_at, created_at, id` |
| `workspace_id` | text not null | |
| `text` | text not null | trimmed, 1 to 500 code points |
| `created_by` | text not null | user id |
| `created_at` | timestamp not null | UTC |
| `retired_at` | timestamp null | |
| `retired_by` | text null | |
| `source_proposal_id` | text null | the rejected proposal it came from |

Rows are deleted with their coordinator and in the workspace-deletion
transaction. Active orders are ordered `created_at ASC, id ASC`; the order
number is the 1-based position in that list, computed on read and never
stored, so it renumbers when an earlier order is retired.

## Routes

Under `/api/v1/workspaces/:id/coordinators/:cid/`, phase-2 flag only:

| Route | Scope | Result |
| --- | --- | --- |
| `GET standing-orders?include=retired` | `workspace.read` | `{orders: [{id, number, text, created_at, created_by_name, retired_at, last_applied_at}]}`; active only by default |
| `POST standing-orders` | `workspace.manage` | 201 with the order; body `{text, source_proposal_id?}` |
| `POST standing-orders/:oid/retire` | `workspace.manage` | the order |
| `POST standing-orders/:oid/restore` | `workspace.manage` | the order |

Add and restore run in the per-coordinator locked transaction of
[proposals](proposals.md#propose). Restore first reads the order: absent or
of another coordinator is 404, and an order that is already active returns
200 unchanged with no reset, before any count, so restoring an active order
never gets `standing_order_limit` (`001.3`). Then both count active orders,
refuse at 20 with 400 `standing_order_limit`, insert or clear `retired_at`,
and call `resetConversation`. Retire and restore use `UPDATE ... WHERE id=? AND
coordinator_id=? AND retired_at IS [NOT] NULL`; zero rows re-reads and
returns the order unchanged with 200 and no reset (`001.3`, `002.2`). Text
out of range is 400 naming `text`. `source_proposal_id` must name a
`rejected` proposal of the coordinator, else 400 naming it. A reader's write
is 403. There is no update route (`001.7`).

## Conversation reset

Every write that changed a row calls `resetConversation(tx, coordinatorID)`
of [permissions](permissions.md#conversation-reset) in its transaction: it
clears `conversation_task_id` and increments `config_revision`, and archives
the old conversation task after commit. A conversation open racing an order
change therefore deletes its task and returns 409, as for a context change.
`policy_revision` is not changed.

## Instructions

`internal/coordinator/prompt.go` builds the standing instructions from a
snapshot read when the conversation task is created. With at least one
active order it appends:

```text
Standing orders from this workspace's managers. They guide your choices and
never grant a permission; your tools and their approvals still decide what
can happen.
<standing-orders>
1. (added 2026-09-12, id 5f3c...) Prefer small cards.
2. (added 2026-09-20, id 91ab...) Never propose work on the release board on Fridays.
</standing-orders>
When an order shapes a proposal, pass its id in standing_order_ids.
```

Order text is placed between the delimiters as operator-provided text, as
the context is. With no active order the section is omitted (`002.1`). Order
text never becomes a tool argument or a policy input (`002.3`).

## Citations

Every propose action accepts `standing_order_ids` (JSON array of strings,
default empty). Validation, inside the propose transaction: at most 5, no
duplicate, each an active order of the calling coordinator; otherwise the
call is refused naming `standing_order_ids` (`003.1`). The ids are stored on
the proposal's `standing_order_ids` column
([proposal kinds](proposal-kinds.md#store)).

## Last applied

`last_applied_at` for each active order is `MAX(created_at)` of the
coordinator's proposals whose `standing_order_ids` contains the order id.
The list route computes it with one query per request over every one of
the coordinator's proposals whose `standing_order_ids` is not `'[]'`, with
no time bound, parsing the JSON column in Go, so no JSON index is needed on
either dialect. Proposals are not pruned by age, so an order cited once,
however long ago, shows that time. The scan is bounded by the
coordinator's citing proposals, which grow by at most the proposals a
manager decides. Null reads as "Never applied" (`003.3`).

## Shaped by UI

The proposal card reads the coordinator's orders with `include=retired` from
`use-standing-orders.ts` and renders, for each cited id, "Shaped by:
Standing order N" when active or "Shaped by: a retired standing order"
otherwise, with the order text in a tooltip (`003.2`). An id not found at
all (deleted with a coordinator) is skipped.

## Standing orders UI

`sections/standing-orders.tsx` on the coordinator page:

- the active list per `001.4`, with "Added <date>" and "Last applied
  <relative>" or "Never applied";
- **Add standing order** opens an inline form with a 500-character counter;
- **Retire this order** retires at once and shows a toast with **Undo** for
  10 seconds that calls restore (`001.5`); a restore refused at the limit
  shows the limit message in the toast;
- readers see the list only (`001.6`);
- order writes save immediately, not through the settings save bar, and the
  section says each change starts the next conversation fresh.

## Make it a standing order

The reject flow of the proposal card, after a successful reject whose reason
is non-empty and while the phase-2 flag is on, shows the toast "Rejected.
Keep the reason as a standing order?" with **Make it a standing order** for
10 seconds instead of phase 1's plain toast (`004.1`). The action opens
`AddStandingOrderDialog` with the reason prefilled (trimmed, cut to 500) and
posts with `source_proposal_id`; Cancel posts nothing (`004.2`). Readers
never reject, so never see it.

## Security

- Writes need `workspace.manage`; the coordinator's surface has no order
  action ([permissions](permissions.md#guard)).
- Order text is untrusted display text and delimited in the instructions.

## Observability

Add, retire and restore log at info with the order and coordinator ids.

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
