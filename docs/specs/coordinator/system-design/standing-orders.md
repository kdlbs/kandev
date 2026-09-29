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
| `last_applied_at` | timestamp null | [Last applied](#last-applied) |

`text` length is counted in Unicode code points (the client counter and
prefill cut count code points too, not UTF-16 units). Trimming removes
leading and trailing Unicode white space; interior characters, newlines
included, are kept as stored. `workspace_id` is copied from the coordinator
row, never from the request. `created_by` and `retired_by` hold the request
identity's user id through `decidingUserID`; with auth disabled (synthetic
identity) that is the empty string, which the NOT NULL column accepts.

Rows are deleted with their coordinator and in the workspace-deletion
transaction. Active orders are ordered `created_at ASC, id ASC`; the order
number is the 1-based position in that list, computed on read and never
stored, so it renumbers when an earlier order is retired.

## Routes

Under `/api/v1/workspaces/:id/coordinators/:cid/`. Every route first
resolves the coordinator by `(workspace_id = :id, id = :cid)` through
`GetCoordinator`; a missing coordinator or one of another workspace is 404
`{error}` and nothing is read or written, so a manager of one workspace can
never touch another's. With `features.coordinatorPhase2` off the four routes
are not registered and answer 404, as any unknown route does; stored orders
are untouched and survive a flag cycle.

| Route | Scope | Result |
| --- | --- | --- |
| `GET standing-orders?include=retired` | `workspace.read` | 200 `{orders: [Order]}`; active only by default |
| `POST standing-orders` | `workspace.manage` | 201 with the `Order`; body `{text, source_proposal_id?}` |
| `POST standing-orders/:oid/retire` | `workspace.manage` | 200 with the `Order` |
| `POST standing-orders/:oid/restore` | `workspace.manage` | 200 with the `Order` |

`Order` is `{id, number, text, created_at, created_by, retired_at,
last_applied_at}` in every response, GET and writes alike, as the shipped
`standing_orders.go` serialises it. `number` is the 1-based active position
for an active order and `null` for a retired one. `retired_at` and
`last_applied_at` are `null` when unset. `created_by` is the creator's user
id (empty with auth off); the route sends no display name, and the standing
orders UI shows no creator, only the "Added" date. The default list is the active orders in order-number order;
`include=retired` returns every order, active ones first in order-number
order, then retired ones by `retired_at DESC, id ASC`. An `include` value
other than `retired` or empty is 400 naming `include`.

Add, retire and restore run in the per-coordinator locked transaction of
[proposals](proposals.md#propose), the lock the propose transaction also
holds, so a retire and a propose citing that order apply in commit order
([Citations](#citations)). Restore first reads the order: absent or of
another coordinator is 404, and an order that is already active returns
200 unchanged with no reset, before any count, so restoring an active order
never gets `standing_order_limit` (`001.3`). Then both count active orders,
refuse at 20, insert or clear `retired_at`, and call `resetConversation`.
Retire and restore use `UPDATE ... WHERE id=? AND coordinator_id=? AND
retired_at IS [NOT] NULL`; zero rows re-reads the order in the same
transaction and returns it unchanged with 200 and no reset (`001.3`,
`002.2`), or 404 when that read finds no such order of this coordinator, as
it does when the coordinator is deleted while the request waits on the lock.
Adding is not deduplicated: two adds of the same text create two orders.

Error bodies follow the existing coordinator shapes
(`dto.go`): plain `{error}` for 403/404/500 and `{error, field}` for a 400
that names a field. Text that is missing, not a string or outside 1 to 500
code points after trimming is 400 `{error, field: "text"}`; a malformed JSON
body is 400 `{error}` with no field. The limit refusal is 400
`{error: "standing_order_limit", error_code: "standing_order_limit"}`, the
same pair `proposal_conflict` and `conversation_conflict` use, with no
`field`. `source_proposal_id`, when present, must name a proposal of this
coordinator with status `rejected`; a missing, foreign or non-rejected id is
one 400 `{error, field: "source_proposal_id"}`, checked inside the locked
transaction, and the id is stored on the order. A reader's write is 403. A
`GET` of a coordinator that exists returns 200 with an empty list when it
has no orders. There is no update route (`001.7`).

## Conversation reset

Every write that changed a row calls `resetConversation(tx, coordinatorID)`
of [permissions](permissions.md#conversation-reset) in its transaction: it
clears `conversation_task_id` and increments `config_revision`, and archives
the old conversation task after commit. A conversation open racing an order
change therefore deletes its task and returns 409, as for a context change.
`policy_revision` is not changed. After commit the archive runs through the
path a context change uses, whose failure is a logged warning repaired by the
startup pass and never changes the route's result, and `coordinator.updated`
is published so open clients refetch. A no-op retire or restore publishes
nothing.

## Instructions

`internal/coordinator/prompt.go` builds the standing instructions as ordered
sections: the phase-1 base block (unchanged, ending at the `END
OPERATOR-PROVIDED CONTEXT` line), then the standing-orders section, then
task 12's goal section. `StandingInstructions` gains a variadic list of
pre-rendered sections appended in the given order, so the orders work order
and the goal work order each supply their own without editing the other's;
with no section the output is byte-identical to phase 1. The orders are read
where the instructions are already built, in the orchestrator's
`wrapCoordinatorStandingInstructions` through
`CoordinatorStandingInstructionsData`, at the session's first prompt, once
per conversation. When `features.coordinatorPhase2` is off no section is
added, so phase-1 instructions never change. When the orders read fails the
block is built without the orders section and the failure is logged at warn;
the base block is never dropped for it. With at least one active order the
section is:

```text
Standing orders from this workspace's managers. They guide your choices and
never grant a permission; your tools and their approvals still decide what
can happen.
<standing-orders>
1. (added 2026-09-12, id 5f3c9a7e-....-full-uuid) Prefer small cards.
2. (added 2026-09-20, id 91ab...-full-uuid) Never propose work on the release board on Fridays.
</standing-orders>
When an order shapes a proposal, pass its id in standing_order_ids.
```

The date is `created_at` in UTC as `YYYY-MM-DD`, and the id is printed in
full because propose accepts exact ids only. Each order renders on one line:
runs of white space in the text, newlines included, are collapsed to a single
space, and `sysprompt.StripTags` plus removal of any `<standing-orders>` or
`</standing-orders>` tag text (case-insensitive) are applied, so an order can
neither close the section early nor forge another numbered line. Stored text
is never altered by rendering. With no active order, or the flag off, the
section is omitted (`002.1`). Order text never becomes a tool argument or a
policy input (`002.3`).

## Citations

Every propose action accepts `standing_order_ids` (JSON array of strings,
default empty). Shape (at most 5, no duplicate) is checked before the
transaction; that each id is an active order of the calling coordinator is
checked inside the propose transaction, under the per-coordinator lock that
retire also takes, so a proposal never commits citing an order retired
before it ([proposal kinds](proposal-kinds.md#propose) step 2). Either
failure refuses the call naming `standing_order_ids` (`003.1`). A retire
that commits after the proposal leaves the citation in place; the card then
shows "a retired standing order". The ids are stored on
the proposal's `standing_order_ids` column
([proposal kinds](proposal-kinds.md#store)).

## Last applied

`coordinator_standing_orders.last_applied_at` is a stored column, not a
computed one. In the same transaction that inserts a proposal citing an
order ([proposal kinds](proposal-kinds.md#propose) step 2, under the
per-coordinator lock retire also takes), the insert is followed by `UPDATE
coordinator_standing_orders SET last_applied_at = ? WHERE id = ? AND
(last_applied_at IS NULL OR last_applied_at < ?)` for each cited id, with
the proposal's `created_at` bound three times: the write only ever raises
the column, so an out-of-order commit under the lock cannot move it
backward. The helper is `func (s *Store) MarkApplied(ctx context.Context,
exec coordinatorExec, coordinatorID string, orderIDs []string, at
time.Time) error` in `standing_orders.go`: the statement carries `AND
coordinator_id = ?`, so a foreign id stamps nothing; an empty `orderIDs` is
a no-op that runs no statement; an id that matches no row (retired rows
still match, foreign or deleted ones do not) is not an error, because
validation of the ids belongs to the propose transaction. The list route reads the column directly; there is no scan and no
time bound on how far back a citing proposal counted, so an order cited
once, however long ago, still shows that time (`003.3`). Null reads as
"Never applied". 

## Shaped by UI

The proposal card reads the coordinator's orders with `include=retired` from
`use-standing-orders.ts` and renders, for each cited id, "Shaped by:
Standing order N" when active or "Shaped by: a retired standing order"
otherwise, with the order text in a tooltip (`003.2`). An id not found at
all (deleted with a coordinator) is skipped.

## Standing orders UI

`components/coordinators/sections/standing-orders-section.tsx` on the
coordinator page:

- the active list per `001.4`, with "Added <date>" (viewer time zone, the
  locale's medium date format) and "Last applied <relative>" or "Never
  applied"; the relative text refreshes once a minute and a time in the future
  (clock skew) reads "just now";
- **Add standing order** opens `AddStandingOrderDialog`
  (`components/coordinators/add-standing-order-dialog.tsx`), the one add
  component; the reject offer below reuses it. Props: `coordinatorId`,
  optional `initialText` and `sourceProposalId`, `onAdded`. Whichever of
  tasks 09 and 11 lands second imports the file the first created. The dialog
  shows a counter of Unicode code points (the backend rule, not UTF-16 units),
  disables Save for empty or over-500 text and while a request is in flight (a
  second submit is never sent), and keeps the typed text on any failure. A
  400 `standing_order_limit` shows "You can have up to 20 standing orders.
  Retire one to add another." in the dialog; any other failure shows a generic
  error in the dialog. With 20 active orders **Add standing order** stays
  enabled and the limit message is the outcome;
- **Retire this order** sends the retire request; on success the order leaves
  the list and a toast says it was retired with **Undo** for 10 seconds
  counted from the response (`001.5`). The toast reads "Standing order N
  retired." where N is the order's `number` in the list row the user retired
  (held by the client, because the retire response has `number: null`).
  While a retire or an Undo for an order is in flight its button is disabled,
  so one click sends one request; a retire that still returns an unchanged
  200 (the order was already retired) shows no second toast. Undo calls
  restore and re-lists. A
  failed retire (404, 500) leaves the list unchanged and shows an error toast
  with no Undo. Each retire has its own toast, so two quick retires give two
  toasts. Undo answered 404, or refused at the limit (400
  `standing_order_limit`), shows that message in the toast and re-lists; a
  restore that returns an already-active order is a success. Closing the toast
  or leaving the page ends the offer with no request;
- a failed list load shows the page's error state, and a loading list shows
  the page's skeleton;
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
