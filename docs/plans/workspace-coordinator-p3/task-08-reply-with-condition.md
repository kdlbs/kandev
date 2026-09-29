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
  - REQ-COORDINATOR-INTEGRATION-004
  - REQ-COORDINATOR-INTEGRATION-006
acceptance_criteria:
  - AC-COORDINATOR-RELAY-003.1
  - AC-COORDINATOR-RELAY-003.2
  - AC-COORDINATOR-RELAY-003.3
  - AC-COORDINATOR-RELAY-003.4
  - AC-COORDINATOR-RELAY-003.5
  - AC-COORDINATOR-INTEGRATION-004.2
  - AC-COORDINATOR-INTEGRATION-006.1
  - AC-COORDINATOR-INTEGRATION-006.2
  - AC-COORDINATOR-INTEGRATION-006.3
system_design:
  - ../../specs/coordinator/system-design/relay.md
  - ../../specs/coordinator/system-design/integration.md
---

# Task 08: Reply To A Proposal With A Condition (WP-11)

## Summary

Adds the `returned` proposal status (set only from `pending`), the reply and deliver routes, delivery of
the reply as the manager's own message to the coordinator's conversation,
`in_reply_to` on `propose_task_kandev`, and the **Reply with a condition**
control on both proposal card surfaces.

## In scope

- Backend: [Reply route](../../specs/coordinator/system-design/relay.md#reply-route)
  and deliver route, the single conditional update shared in form with
  approve's claim and reject, [Reply delivery](../../specs/coordinator/system-design/relay.md#reply-delivery)
  through `OpenConversation` and the panel composer's message path as the
  replying manager, with the diagnostic attempt time on `reply_delivery_claimed_at` (never a gate),
  the delivery key `coordinator-reply:<proposal_id>` as message and queue
  entry id, `metadata.coordinator_reply_proposal_id`, the new
  `CreateQueuedMessageOnce` in `internal/task/service` and its repository
  writer on both dialects (message insert `ON CONFLICT (id) DO NOTHING`, queue
  entry only when one row was inserted, one transaction, before any
  dispatch), the `NotifyQueuedUserPrompt` kick, the finalisation on any
  returned message, and the kind-specific delivery text; the reserved MCP action names `coordinator.reply_proposal` and `coordinator.deliver_reply` join `DecisionActions` (no handler registered), so the guard refuses both for a coordinator or unresolved principal; both routes are registered only while phase 3 is effective and the proposal DTO carries the three reply fields only then;
  `in_reply_to` validation ([Revised proposals](../../specs/coordinator/system-design/relay.md#revised-proposals));
  the client store's never-unsettle rule treats `returned` as settled.
- The `returned` status against phase 2's proposal store
  ([Proposal statuses](../../specs/coordinator/system-design/integration.md#proposal-statuses-and-kinds)):
  the reply's conditional update is `WHERE id = ? AND status = 'pending'`;
  `ApproveProposal`, `approveKind` and reject answer a `returned` proposal with
  409 and the current proposal; stale-claim recovery and the open-proposal
  count and open-target index need no change; the `returned` activity row
  (`ActivityOutcome` `returned`, class `kindAction(kind)`, authorization
  `requires_approval`, actor the replying manager, detail the reply text)
  written in the reply's locked transaction, and its copy in six locales
  ([Log rows](../../specs/coordinator/system-design/integration.md#log-rows)).
  The unattended mark (`AC-COORDINATOR-INTEGRATION-004.1`, rendering only;
  task 05 stamps `unattended_turn_id` and the activity DTO carries it) is
  rendered and its copy (`activityUnattended`) added here with the what-it-did
  text table: a row with a turn id shows "During an unattended turn" as a second
  muted line on the Action cell and the phone card. A `failed` card keeps its phase 2 controls (Approve,
  Edit and Reject; an improvement card Approve and Reject) and shows no
  **Reply with a condition**; there is no **Try again** on a proposal card
  (`AC-COORDINATOR-INTEGRATION-006.1`).
- Web: the control (shown only for kinds with registered delivery text:
  `create_task`, `message`, `move`, `resume`), returned and revised card states
  and **Send again** in the phase 1 `ProposalCard`, on the Needs you item and
  the copilot chat card (no `status=all` list is added) ([Cards](../../specs/coordinator/system-design/relay.md#cards)).
- Copy in six locales.

## Out of scope

- A reply tool for the coordinator.
- The `improvement` kind, which task 10 adds after this task: its delivery
  text, its `returned` activity class (`improvement`), its reply control and
  the refusal of `in_reply_to` naming a returned improvement are task 10's
  ([task 10](task-10-improvements.md)). This task's delivery selects text by
  kind and treats a kind with no registered text as not replyable, so task 10
  adds one entry.

## ASCII UI preview

See [plan UI-05](plan.md#ascii-ui-previews).

```text
[Approve] [Edit] [Reject] [Reply with a condition]
  [ Only if it stays under 200 lines ... ]  48/2000   [Send reply] [Cancel]
Returned with your condition: Only if ...   Reply saved, not delivered [Send again]
```

## Acceptance

- A manager's reply of 1 to 2,000 trimmed characters to a `pending` proposal
  sets `returned` with the text and decider, creates no task, writes the
  `returned` activity row in the same transaction, emits
  `coordinator.updated`; empty or longer is 400 naming `text`; a proposal in
  any other status, including `failed`, is 409 with the current proposal
  before validation; a reply racing approve or reject leaves exactly one
  winner (race test). Approve and reject of a `returned` proposal are 409; a
  `returned` proposal is not counted toward the 25 open, does not hold its
  task's open-target slot, and is ignored by the stale-claim pass
  (`AC-COORDINATOR-INTEGRATION-004.2`, `006.1`, `006.2`).
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
- A full queue (`ErrQueueFull`) leaves the proposal undelivered with **Send
  again** and nothing stored. A dispatch failure injected after the commit
  (the coordinator's notifier interface is faked with a
  `NotifyQueuedUserPrompt` that dispatches nothing, standing in for a failed
  asynchronous launch or drain, since the real method returns nothing) still
  records the delivery and keeps the one queue entry, and a later drain of
  that session dispatches it once.
- **Send again** after a crash injected between step 1 and step 4, with
  `reply_delivery_claimed_at` still set, runs delivery and stores the reply
  once; the card shows **Send again** throughout and "Sending reply" only
  while its own request is pending.
- With the conversation replaced after a crash that followed step 3's
  commit, Send again stores nothing in the new conversation and records the
  delivery.
- A reply to a `message`, `move` or `resume` proposal is delivered with the
  kind text of the design, which names no `in_reply_to`; a reply to a
  `create_task` proposal names it.
- A revised proposal is a new `pending` row carrying `in_reply_to`; the
  returned row is never reopened or edited (`AC-COORDINATOR-INTEGRATION-006.3`).
- `in_reply_to` naming a `returned` `create_task` proposal of the same
  coordinator is accepted and the new card shows "Revised after your reply";
  several proposals may name one returned row; any other value (another
  status, another coordinator, a `returned` `message`, `move` or `resume`
  proposal, or any value while phase 3 is off) is refused naming
  `in_reply_to`; a coordinator principal is refused both routes on every
  transport.
- A row with `unattended_turn_id` renders the "During an unattended turn"
  mark on the Action cell and the phone card, and a row without one renders no
  mark (component test).
- A reply to a proposal whose kind has no registered delivery text is 400
  naming `kind` and writes nothing; delivery text titles are `spec_json.title`
  for `create_task` and `Resume|Message|Move task <task_id>` for the others;
  the queue entry, not the message, carries `user_message_recorded`; delivery
  runs under a 30-second deadline detached from the client; a reply sent from a
  Needs you item stays rendered from the response with **Send again** on a
  failed delivery.
- A failed delivery answers the reply and deliver routes with 200 and the
  proposal with `reply_delivered_at` null, never a 5xx; with phase 3 off both
  routes are 404 and the DTO omits `reply_text`, `reply_delivered_at` and
  `in_reply_to`; `coordinator.updated` is published after the reply and again
  when delivery is recorded.

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
