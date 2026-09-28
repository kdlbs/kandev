---
id: coordinator-relay-design
title: Answering in place and replying with a condition design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-RELAY-001
  - REQ-COORDINATOR-RELAY-002
  - REQ-COORDINATOR-RELAY-003
---

# Answering in place and replying with a condition System Design

## Purpose and boundaries

Answering in place is a client feature over one new read route: the
coordinator reads the pending bundle or permission for one task and the card
resolves it through the existing clarification resolver and permission
response path, unchanged. Replying with a condition adds one proposal status,
two columns, one decision route and one tool argument to the proposal design
of [proposals](proposals.md). Nothing in this design touches the Inbox's rows,
count or tabs (D14), or the clarification and permission contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-RELAY-001` | [Relay read](#relay-read), [Question card](#question-card) |
| `REQ-COORDINATOR-RELAY-002` | [Relay read](#relay-read), [Permission card](#permission-card) |
| `REQ-COORDINATOR-RELAY-003` | [Reply store](#reply-store), [Reply route](#reply-route), [Reply delivery](#reply-delivery), [Revised proposals](#revised-proposals) |

## Relay read

`GET /api/v1/workspaces/:id/coordinators/:cid/relay/:taskId`
(`workspace.read`, registered only while phase 3 is effective) returns:

```json
{
  "task_id": "...",
  "session_id": "...",
  "clarification": {"pending_id": "...", "messages": []},
  "permission": {"message": {}}
}
```

- The task must be in the coordinator's workspace (404 otherwise); the route
  reads the task's primary session.
- `clarification` is the answerable bundle of that session, read with the
  same repository query the Inbox list uses
  (`task/repository/sqlite/clarification_bundle_query.go`), so "answerable"
  means exactly what the Inbox means by it; `messages` has the shape the
  Inbox's `ClarificationInboxBundle.messages` has. `null` when there is none.
- `permission` is the session's newest `permission_request` message whose
  status is pending, returned as the task chat receives it (including its
  `request_id`, `pending_id`, `title`, `options` and action details in
  metadata). `null` when there is none.
- A read error is 500; the card treats it as "no answer available"
  (`AC-COORDINATOR-RELAY-001.4`).

The route works whether or not the Needs-you Inbox flag is on: it calls the
repository query directly, not the flag-gated Inbox handler.

## Question card

`apps/web/app/coordinator/components/question-answer.tsx`:

- A question item (the `question or permission` group of
  [needs-you](needs-you.md#classification) with `pending_action ==
  "clarification"`) shows **Answer here** to managers while phase 3 is on.
  Readers and phase-3-off clients keep the phase 1 text and **Open task**.
- **Answer here** fetches the relay read and renders
  `ClarificationPanelSection` with `pending`, the bundle `messages`,
  `maxHeightVh={50}` and an `onOutcome` handler, as
  `needs-you-inbox-row.tsx` does. Submission therefore goes through
  `use-clarification-group.ts` to `POST /api/v1/clarification/:id/respond`
  and the shared `Resolver.ResolveBundle`.
- Outcomes, from the component's `onOutcome`:

  | Outcome | Card |
  | --- | --- |
  | recorded | collapse; the item leaves when `task.status_summary.updated` clears `pending_action` |
  | lost to another caller | collapse; toast "Already answered: <winning outcome>" |
  | no longer active | collapse; toast "This question is no longer waiting" |
  | failed | stay expanded with the answer kept; **Try again** |

- A relay read with `clarification: null`, or a failed read, shows the phase
  1 text and **Open task**.

## Permission card

`apps/web/app/coordinator/components/permission-answer.tsx`:

- A permission item shows **Answer here** to managers while phase 3 is on.
- It renders the permission message's title, action details and one button
  per option, and resolves through the same `permission.respond` WebSocket
  request `use-permission-handlers.ts` sends, with the message's `task_id`,
  `session_id`, `request_id` and `pending_id` and the chosen `option_id`.
  The backend's existing handler records the `PermissionResolutionAudit` with
  source `web` and the browser identity.
- A stale-response error (the same `isStalePermissionResponse` test the chat
  uses) collapses with the toast "This permission is no longer waiting"; any
  other error keeps the card expanded with **Try again**.
- The request builder is extracted from `use-permission-handlers.ts` into
  `apps/web/lib/permissions/respond.ts` so both callers share it; the chat's
  behaviour is unchanged and pinned by its existing tests.

## Reply store

`coordinator_proposals` gains, by additive `ALTER`:

| Column | Type | Notes |
| --- | --- | --- |
| `reply_text` | text null | trimmed, 1 to 2,000 characters, set with `returned` |
| `reply_delivered_at` | timestamp null | set when the reply message is stored |
| `reply_delivery_claimed_at` | timestamp null | the delivery claim; see [Reply delivery](#reply-delivery) |
| `in_reply_to` | text null | a `returned` proposal id of the same coordinator |

`status` gains `returned`, a settled status. It is not open, so it does not
count toward the 25 and is not listed by `status=pending`. The client store's
never-unsettle rule treats `returned` as settled.

## Reply route

`POST .../proposals/:pid/reply` (`workspace.manage`) with body
`{"text": "..."}`:

1. The coordinator guard refuses a coordinator principal, as for approve and
   reject (`AC-COORDINATOR-RELAY-003.5`).
2. Read the proposal (404 when absent or of another coordinator). When its
   status is not `pending` or `failed`, return 409 with the current proposal,
   before validating the text.
3. Trim; empty or over 2,000 characters is 400 naming `text`.
4. `UPDATE coordinator_proposals SET status='returned', reply_text=?,
   decided_by=?, updated_at=? WHERE id=? AND status IN ('pending','failed')`.
   Zero rows: re-read and return 409 with the current proposal. This is the
   same single conditional update approve's claim and reject use, so a reply
   racing either leaves one winner.
5. Publish `coordinator.updated`, then run [Reply delivery](#reply-delivery)
   and return the proposal with `reply_delivered_at`.

`POST .../proposals/:pid/reply/deliver` (`workspace.manage`) re-runs delivery
for a `returned` proposal whose `reply_delivered_at` is null, returns 409 for
any other proposal, and never changes `status`. When another delivery holds
the claim it returns 200 with the current proposal and sends nothing.

## Reply delivery

1. Claim: `UPDATE coordinator_proposals SET reply_delivery_claimed_at = ?
   WHERE id = ? AND status = 'returned' AND reply_delivered_at IS NULL AND
   (reply_delivery_claimed_at IS NULL OR reply_delivery_claimed_at < ?)`,
   the last value being now minus two minutes. Zero rows means the reply was
   delivered or another delivery is in flight: return the current proposal
   and send nothing. Two concurrent deliveries therefore send at most once.
2. Resolve the coordinator's conversation through the phase 1
   `OpenConversation` (which reuses a live current task or creates one, under
   the caller's identity).
3. Send the message through the same conversation message path the panel's
   composer uses, as the replying manager, so it is an attended turn start
   (`REQ-COORDINATOR-COPILOT-002`), with `metadata.coordinator_reply_proposal_id`
   set to the proposal id. Before sending, delivery looks in the
   conversation's messages created at or after the proposal's `updated_at`,
   and in its primary session's queued message (the orchestrator queue read
   [wake admission](wake.md#admission) uses), for one carrying that key; when it finds one, it skips the send and goes to
   step 4, because the reply already reached the conversation. The
   agent-facing text depends on the proposal's `kind`. For `task`:
   `Reply to your proposal "<title>" (proposal <id>): <reply text>. If you
   still think the work is needed, propose it again with in_reply_to set to
   <id>.` For `improvement`: `Reply to your improvement "<title>" (proposal
   <id>): <reply text>. If you still think a change is needed, propose a new
   improvement.` `propose_improvement_kandev` takes no `in_reply_to`, so an
   improvement card never shows "Revised after your reply".
4. On success set `reply_delivered_at` and clear the claim. On any error, log
   at warn, clear the claim and leave `reply_delivered_at` null; the card
   shows "Reply saved, not delivered" with **Send again**.

A crash after the send and before step 4 leaves the claim set and
`reply_delivered_at` null, so the card shows "Reply saved, not delivered".
**Send again** sends nothing until the claim is two minutes old; after that
it finds the stored or queued message by its metadata and records the
delivery without sending again. When the
conversation was replaced in between, the new conversation holds no such
message and the reply is sent to it once.

A delivered reply that lands while the conversation is busy queues behind the
running turn through the existing message queue, as any manager message does.

## Revised proposals

- `propose_task_kandev` accepts optional `in_reply_to`. Validation adds: the
  id names a proposal of the calling coordinator with status `returned` and
  `kind = 'task'`, otherwise the call is refused naming `in_reply_to`.
  `propose_improvement_kandev` has no `in_reply_to` argument
  ([improvements](improvements.md#tool)).
- A proposal card with `in_reply_to` shows "Revised after your reply" and the
  quoted `reply_text` of the returned proposal, read through the proposal get
  route.
- A `returned` card shows "Returned with your condition: <reply text>" with no
  actions, plus **Send again** while `reply_delivered_at` is null.

## Cards

The **Reply with a condition** control is added to `ProposalCard` for
`pending` and `failed` proposals, for managers, while phase 3 is on: a
textarea (2,000 characters, counter), **Send reply** disabled until the
trimmed text is non-empty, and **Cancel** returning focus to the control. On
the chat card it opens in place, unlike Edit, because it has one field. A 409
shows the current state as approve does.

## Security

- The reply route and the deliver route are on the coordinator's refused
  list; the coordinator surface has no reply tool.
- The relay read exposes a pending bundle and permission only for tasks in
  the coordinator's workspace, under `workspace.read`, the scope Needs you
  already needs.
- Answering reuses the existing respond endpoints, whose own task-access
  checks apply unchanged.

## Observability

`coordinator_reply_total{delivered}` and
`coordinator_relay_read_failed_total` counters, plus a warn log for each
undelivered reply with the proposal id.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
