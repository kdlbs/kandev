---
id: "08-reply-with-condition"
title: "Reply to a proposal with a condition"
status: pending
wave: 2
depends_on:
  - "01-flag-schema-settings"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-RELAY-003
acceptance_criteria:
  - AC-COORDINATOR-RELAY-003.1
  - AC-COORDINATOR-RELAY-003.2
  - AC-COORDINATOR-RELAY-003.3
  - AC-COORDINATOR-RELAY-003.4
  - AC-COORDINATOR-RELAY-003.5
system_design:
  - ../../specs/coordinator/system-design/relay.md
---

# Task 08: Reply To A Proposal With A Condition (WP-11)

## Summary

Adds the `returned` proposal status, the reply and deliver routes, delivery of
the reply as the manager's own message to the coordinator's conversation,
`in_reply_to` on `propose_task_kandev`, and the **Reply with a condition**
control on both proposal card surfaces.

## In scope

- Backend: [Reply route](../../specs/coordinator/system-design/relay.md#reply-route)
  and deliver route, the single conditional update shared in form with
  approve's claim and reject, [Reply delivery](../../specs/coordinator/system-design/relay.md#reply-delivery)
  through `OpenConversation` and the panel composer's message path as the
  replying manager, with the in-flight marker on `reply_delivery_claimed_at`,
  the delivery key `coordinator-reply:<proposal_id>` as message and queue
  entry id, `metadata.coordinator_reply_proposal_id`, the new
  `CreateQueuedMessageOnce` in `internal/task/service` and its repository
  writer on both dialects (message insert `ON CONFLICT (id) DO NOTHING`, queue
  entry only when one row was inserted, one transaction, before any
  dispatch), the `NotifyQueuedUserPrompt` kick, the finalisation on any
  returned message, and the kind-specific delivery text; the guard's refused list gains both routes;
  `in_reply_to` validation ([Revised proposals](../../specs/coordinator/system-design/relay.md#revised-proposals));
  the client store's never-unsettle rule treats `returned` as settled.
- Web: the control, returned and revised card states and **Send again** in
  the phase 1 `ProposalCard` ([Cards](../../specs/coordinator/system-design/relay.md#cards)).
- Copy in six locales.

## Out of scope

- A reply tool for the coordinator; replies to improvement proposals use the
  same route and the improvement delivery text, and need no `in_reply_to`
  (task 10 renders the control).

## ASCII UI preview

See [plan UI-05](plan.md#ascii-ui-previews).

```text
[Approve] [Edit] [Reject] [Reply with a condition]
  [ Only if it stays under 200 lines ... ]  48/2000   [Send reply] [Cancel]
Returned with your condition: Only if ...   Reply saved, not delivered [Send again]
```

## Acceptance

- A manager's reply of 1 to 2,000 trimmed characters sets `returned` with the
  text and decider, creates no task, emits `coordinator.updated`; empty or
  longer is 400 naming `text`; a settled proposal is 409 before validation; a
  reply racing approve or reject leaves exactly one winner (race test).
- The reply is delivered as the manager's message (opening a conversation when
  none), queued behind a busy turn and dispatched when it ends; a send failure leaves `returned` with
  "Reply saved, not delivered" and **Send again**, which delivers once.
- `CreateQueuedMessageOnce` on SQLite and on PostgreSQL under
  `KANDEV_TEST_POSTGRES_DSN`: the first call returns `created = true` with
  one message row and one queue entry; a second call with the same id
  returns the stored message with `created = false` and adds no message and
  no queue entry; a failure inside the transaction stores neither.
- Two concurrent deliver calls (`-race`), and two deliveries where the first
  is held after step 1 until the second has finished (a slow send outliving
  the other's claim), each store one message, leave one queue entry and
  produce one agent prompt, and both end with `reply_delivered_at` set.
- A crash injected after step 3 commits and before step 4 is followed by a
  Send again that stores and dispatches nothing new and sets
  `reply_delivered_at`; a crash injected inside step 3's transaction is
  followed by a Send again that stores and dispatches the reply once.
- A full queue (`ErrQueueFull`) and a notify error: the first leaves the
  proposal undelivered with **Send again** and nothing stored; the second
  still records the delivery and the entry drains when the session is next
  idle.
- With the conversation replaced after a crash that followed step 3's
  commit, Send again stores nothing in the new conversation and records the
  delivery.
- A reply to an improvement is delivered with the improvement text, which
  names no `in_reply_to`.
- `in_reply_to` naming a `returned` task proposal of the same coordinator is
  accepted; one naming a `returned` improvement is refused; and shown as "Revised after your reply"; any other value is refused
  naming the field; a coordinator principal is refused both routes on every
  transport.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Reply' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'Reply.*Race' -race -count=1
cd apps/backend && go test ./internal/task/service/... ./internal/task/repository/... -run 'QueuedMessageOnce' -race -count=1
cd apps/backend && go test ./internal/mcp/... -run 'Coordinator' -count=1
cd apps/web && pnpm test -- app/coordinator hooks/domains/coordinator
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/reply.spec.ts
```
