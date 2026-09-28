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
  replying manager, with the delivery claim on `reply_delivery_claimed_at`,
  `metadata.coordinator_reply_proposal_id` and the stored-or-queued message
  lookup before each send, and the kind-specific delivery text; the guard's refused list gains both routes;
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
  none), queued behind a busy turn; a send failure leaves `returned` with
  "Reply saved, not delivered" and **Send again**, which delivers once.
- Two concurrent deliver calls send one message; a crash injected after the
  send and before `reply_delivered_at` is followed, once the claim is two
  minutes old, by a Send again that finds the message by its metadata and
  sends nothing (`synctest`).
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
cd apps/backend && go test ./internal/mcp/... -run 'Coordinator' -count=1
cd apps/web && pnpm test -- app/coordinator hooks/domains/coordinator
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/reply.spec.ts
```
