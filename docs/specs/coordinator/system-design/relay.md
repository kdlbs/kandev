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
  "clarification"`) shows **Answer here** to managers while phase 3 is effective.
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

- A permission item shows **Answer here** to managers while phase 3 is effective.
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
| `reply_delivery_claimed_at` | timestamp null | when the latest delivery attempt started, kept for diagnostics and the warn log; it gates nothing and is never rendered. At-most-once rests on the message key, see [Reply delivery](#reply-delivery) |
| `in_reply_to` | text null | a `returned` proposal id of the same coordinator |

`status` gains `returned`, a settled status set only from `pending`. It is
not open, so it does not count toward the 25, does not hold its task's
open-target slot and is not listed by `status=pending`; it is not claimable
and the stale-claim sweep, which selects only `approving`, never reads it. The
client store's never-unsettle rule treats `returned` as settled. The `kind`
and `in_reply_to` semantics, the touch points in the approve switches and the
log row a reply writes are in
[integration](integration.md#proposal-statuses-and-kinds); `kind` is phase 2's
existing column and this design does not add it.

## Reply route

`POST .../proposals/:pid/reply` (`workspace.manage`) with body
`{"text": "..."}`:

1. The coordinator guard refuses a coordinator principal, as for approve and
   reject (`AC-COORDINATOR-RELAY-003.5`).
2. Read the proposal (404 when absent or of another coordinator). When its
   status is not `pending` (a `failed` proposal included), return 409 with the
   current proposal, before validating the text.
3. Trim; empty or over 2,000 characters is 400 naming `text`.
4. In one coordinator-locked transaction, `UPDATE coordinator_proposals SET
   status='returned', reply_text=?, decided_by=?, updated_at=? WHERE id=? AND
   status='pending'` and, when it matched, the `returned` activity row
   ([integration](integration.md#log-rows)). Zero rows: re-read and return 409
   with the current proposal. This is the same single conditional update
   approve's claim and reject use, so a reply racing either leaves one
   winner.
5. Publish `coordinator.updated`, then run [Reply delivery](#reply-delivery)
   and return the proposal with `reply_delivered_at`.

`POST .../proposals/:pid/reply/deliver` (`workspace.manage`) re-runs delivery
for a `returned` proposal whose `reply_delivered_at` is null, returns 409 for
any other proposal, and never changes `status`. It runs delivery steps 1 to 4
whatever `reply_delivery_claimed_at` holds, so after a crash between step 1
and step 4 **Send again** delivers. When step 1 changes no row because a
concurrent delivery has meanwhile set `reply_delivered_at`, it returns 200
with the current proposal and sends nothing; when another delivery is still
running, both reach step 3 and exactly one message is stored.

## Reply delivery

Delivery is exactly one conditional insert keyed by the proposal. The reply's
delivery key is `coordinator-reply:<proposal_id>` (about 55 characters; the
message id column `task_session_messages.id` is a `TEXT` primary key with no
length limit, and the 128-character check in
`internal/task/handlers/message_handlers.go` applies to
the client-supplied `client_message_id`, which this path does not use), used
as both the message id and the queue entry id; the message table's primary key is the uniqueness constraint that decides, so no deadline,
lookup or cancellation is needed for at-most-once.

1. Record the attempt: `UPDATE coordinator_proposals SET
   reply_delivery_claimed_at = ? WHERE id = ? AND status = 'returned' AND
   reply_delivered_at IS NULL`. Zero rows means the reply was delivered or the
   proposal is not `returned`: return the current proposal and send nothing.
   The update has no condition on `reply_delivery_claimed_at`, so a value left
   by a crashed or concurrent attempt never stops this one; overlapping
   deliveries are made safe by step 3.
2. Resolve the coordinator's conversation through the phase 1
   `OpenConversation` (which reuses a live current task or creates one, under
   the caller's identity).
3. Store and enqueue in one transaction, before any dispatch, through the
   durable dispatch receipt the composer already uses for comment-bearing
   messages (`CreateQueuedMessageIdempotent`, which commits the transcript
   message and its `messagequeue.QueuedMessage` together under the caller's
   id). Phase 3 adds a plain variant without plan-comment refs,
   `CreateQueuedMessageOnce(ctx, id, req, queued, maxPerSession) (message,
   created bool, err)`, whose message insert is `INSERT ... ON CONFLICT (id)
   DO NOTHING`: one affected row inserts the queue entry with the same id,
   commits and returns `created = true`; zero affected rows inserts nothing,
   rolls back and returns the stored message with `created = false`. The
   message is authored by the replying manager with
   `metadata.coordinator_reply_proposal_id` set to the proposal id and
   `user_message_recorded`, so its dispatch is an attended turn start
   (`REQ-COORDINATOR-COPILOT-002`). Then delivery calls the orchestrator's
   `NotifyQueuedUserPrompt(taskID, sessionID)`, the same kick the composer
   uses: a promptable session drains the entry at once, and a running
   session drains it when its turn ends, as any queued manager message
   does. The queue removes an entry when it dispatches it, so the entry is
   dispatched once; a second notify finds nothing to drain. The agent-facing
   text depends on the proposal's `kind`. For `create_task`: `Reply to your proposal
   "<title>" (proposal <id>): <reply text>. If you still think the work is
   needed, propose it again with in_reply_to set to <id>.` For `improvement`:
   `Reply to your improvement "<title>" (proposal <id>): <reply text>. If you
   still think a change is needed, propose a new improvement.`
   `propose_improvement_kandev` takes no `in_reply_to`, so an improvement
   card never shows "Revised after your reply".
4. Finalise: when step 3 returned a message, `created` or not, `UPDATE ...
   SET reply_delivered_at = ?, reply_delivery_claimed_at = NULL WHERE id = ?
   AND reply_delivered_at IS NULL`, because the reply is in the conversation.
   On any error before step 3 committed (conversation open, a full queue
   (`ErrQueueFull`), a repository error), log at warn and `UPDATE ... SET
   reply_delivery_claimed_at = NULL WHERE id = ? AND reply_delivered_at IS
   NULL`, leaving `reply_delivered_at` null; the card shows "Reply saved, not
   delivered" with **Send again**, which runs steps 1 to 4 again.
   `NotifyQueuedUserPrompt(ctx, taskID, sessionID)` returns nothing, so no
   error from it reaches delivery. A dispatch that fails after the commit
   (the asynchronous launch of a `CREATED` session or the fast-path drain,
   each of which logs its own failure) does not undo the delivery:
   `reply_delivered_at` stays set, and the entry stays queued and drains at
   the session's next idle point, the queue's existing restart drain, or the
   notify of a later message on that session.

Two deliveries that overlap in any order, including a slow one still running when
a later **Send again** starts, both reach step 3 at most; exactly one insert affects a row,
so one message is stored and one queue entry is dispatched, and the other
returns the stored message and records the delivery. A crash after step 3
commits and before step 4 leaves `reply_delivered_at` null, so the card shows
"Reply saved, not delivered"; **Send again** at once gets `created = false`,
kicks the queue again and records the delivery, storing and dispatching
nothing new. When the conversation was replaced in between, the key still
names the proposal, so the reply stays in the earlier conversation's
transcript and is not sent a second time; the manager can repeat it in the
new conversation by hand. A crash inside step 3's transaction commits
nothing, so Send again stores the reply once.

## Revised proposals

- `propose_task_kandev` accepts optional `in_reply_to`. Validation adds: the
  id names a proposal of the calling coordinator with status `returned` and
  `kind = 'create_task'`, otherwise the call is refused naming `in_reply_to`.
  `propose_improvement_kandev` has no `in_reply_to` argument
  ([improvements](improvements.md#tool)).
- A proposal card with `in_reply_to` shows "Revised after your reply" and the
  quoted `reply_text` of the returned proposal, read through the proposal get
  route.
- A `returned` card shows "Returned with your condition: <reply text>" with no
  actions, plus **Send again** while `reply_delivered_at` is null. While
  this client's reply or deliver request is in flight, the card shows
  "Sending reply" and disables **Send again**; that state is the client's
  pending request only, never `reply_delivery_claimed_at`, so a claim left by
  a crash never hides **Send again**. On the response, the card renders from
  the returned proposal.

## Cards

The **Reply with a condition** control is added to `ProposalCard` for
`pending` proposals (not `failed`), for managers, while phase 3 is effective: a
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
