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
(`workspace.read`, registered only while phase 3 is effective, so 404
otherwise) returns:

```json
{
  "task_id": "...",
  "session_id": "...",
  "clarification": {"pending_id": "...", "context": "...", "messages": []},
  "permission": {"message": {}}
}
```

- The coordinator must belong to workspace `:id` and the task to that
  workspace, else 404 (the same guard the other coordinator routes use). The
  route reads the task's primary session only. A task with no primary session
  returns 200 with `session_id` empty and both fields `null`. A task whose
  primary session is completed, failed or cancelled returns 200 with both
  `null`, because both reads below exclude those states
  (`nonTerminalSessionPredicate`). No archive filter is added: an archived
  session that is not in one of those states is read like any other.
- `clarification` is the one answerable bundle of that session. The query is
  `ListUnresolvedClarificationBundles` (`task/repository/sqlite/
  clarification_bundle_query.go`), which owns "answerable" for the Inbox; it
  gains one optional `ListClarificationBundlesOptions.SessionID` predicate,
  applied inside the bundle query and leaving the query byte-for-byte unchanged
  when empty, as the `Sidecar` option does. The relay calls it with
  `Unscoped: true`, `SessionID` set, `Sidecar` nil (no per-user dismiss or
  snooze applies to a relay read) and `Limit: 1`. Its existing order,
  `created_at ASC, pending_id ASC`, decides which bundle wins when a session
  has several: the oldest. The bundle query itself applies the session's
  current-turn authority and the non-terminal predicate, so an unresolved
  clarification from an earlier turn is not answerable and not returned, the
  same scope the permission read has. The bundle's `messages` and `context` are hydrated
  by the Inbox's own hydration. That hydration is a method on the unexported
  `*Handlers`, so it is extracted, not wrapped: the message lookup
  (`FindMessagesByPendingIDs`), `orderInboxMessages`, `inboxBundleContext` and
  `renderInboxMessages` move into one exported package-level function in
  `internal/clarification` taking the bundle store and one summary, which the
  Inbox method then calls. The coordinator's relay handler takes only that
  store, never a second `Handlers`. The shape is exactly
  `ClarificationInboxBundle.messages` and `context`; the task title and
  session state enrichment stay in the Inbox method; the Inbox's routes,
  responses and tests are unchanged.
  `null` when there is no bundle or hydration yields no messages.
- `permission` is read with `ListPendingInteractions` (task repository) for
  `SessionIDs: [primary]` and `Kinds: [permission]`. That read applies the
  authority the `pending_action` projection uses: the session's current turn,
  non-terminal sessions only, the newest permission of the turn ordered by
  `created_at DESC` then message row order `DESC`, and pending when its
  `metadata.status` is absent or `pending`. It therefore returns at most one
  row, the newest, which is the row the chat picks. The read then returns
  `null`, never an older permission, when that row cannot be answered the way
  the chat can: its `metadata.request_id` is absent or empty (the chat's
  `parsePermission` renders such a row as expired), its `metadata.pending_id`
  is absent or empty, or its `options` list is empty. The message is returned in the chat's
  message shape (including `request_id`, `pending_id`, `title`, `options` and
  action details in metadata). `null` when there is none.
- A task whose `pending_action` comes from a non-primary session shows
  `clarification: null` and `permission: null`, so its item keeps the phase 1
  text and **Open task**; answering another session's request from here is not
  offered.
- A read error is 500, counted in `coordinator_relay_read_failed_total`; a
  404 is not counted. The card treats every non-200 as "no answer available"
  (`AC-COORDINATOR-RELAY-001.4`).

The route works whether or not the Needs-you Inbox flag is on: it calls the
repository query directly, not the flag-gated Inbox handler.

## Question card

`apps/web/app/coordinator/components/question-answer.tsx`:

- A question item (the `question or permission` group of
  [needs-you](needs-you.md#classification)) is a question when its
  `pending_action` is `clarification`, and a permission when it is
  `permission`; the item's `pending_action`, which already gives permission
  priority over clarification for a task, picks the card.
- While phase 3 is effective and the viewer is a manager, the item performs
  one relay read per item when it first renders, deduplicated by task id. Until
  it resolves with a bundle (`clarification` non-null for a question item,
  `permission` non-null for a permission item) the item shows the phase 1 text
  and **Open task**; when it does, **Answer here** replaces the text. A
  non-200 or a null field leaves the phase 1 text. The read is repeated when
  `task.status_summary.updated` arrives for the task and each time **Answer
  here** is clicked to expand. Only the newest request issued for a task
  applies: a response for an older request is dropped. Expanding renders the
  last resolved bundle at once and fires the expand read. The expand read is
  the only read that can collapse the card, and only if it resolves before
  the manager has typed or chosen anything in the panel; from the first
  keystroke or choice on, the card is "engaged" and every read, including an
  expand read still in flight, is ignored for the rest of that expansion.
  While the card is expanded and not engaged, an expand read returning a null
  field or a non-200 collapses the card to the phase 1 text, and one returning
  a different `pending_id` replaces the bundle. Engaged or not, a read
  returning the same `pending_id` never replaces the rendered messages or
  discards entered input (the rendered snapshot is kept). A read fired by
  `task.status_summary.updated` while the card is expanded changes nothing
  except that it never collapses the card. While engaged, the
  next submit reaches the resolver's own lost or no-longer-active outcome. A
  collapsed card whose read failed makes no other retry; the next event or
  expand repeats the read.
- `ClarificationPanelSection` keeps its own collapse to a header bar (Escape
  or its labelled toggle, state per `pending_id`). That collapse is a nested
  state, not the item's: the item stays expanded, the entered answer is kept,
  reads are handled exactly as above, and the item collapses only through
  **Answer here** or an outcome in the table below. The card adds no control
  for it and does not disable it.
  Readers and phase-3-off clients make no relay read and keep the phase 1 text
  and **Open task**.
- **Answer here** renders `ClarificationPanelSection` with `pending`, the
  bundle `messages`, `maxHeightVh={50}` and an `onOutcome` handler, as
  `needs-you-inbox-row.tsx` does. Submission therefore goes through
  `use-clarification-group.ts` to `POST /api/v1/clarification/:id/respond`
  and the shared `Resolver.ResolveBundle`. The component's own submit control
  is disabled while a submission is in flight, so one click sends one request;
  Try again resubmits the retained answer for the same `pending_id` without
  rereading the bundle.
- Outcomes, from the component's `onOutcome`:

  | Outcome | Card |
  | --- | --- |
  | recorded | collapse; when answering reveals another pending action on the task the item follows the next `task.status_summary.updated` or list refresh and is a new item state, never a card switch inside the same expansion; the item leaves when `task.status_summary.updated` clears `pending_action`; if that event is missed the item leaves at the next Needs you list refresh |
  | lost to another caller | collapse; toast with the Inbox row's existing copy: `needsYouInbox:anotherCallerRejected` when the winner's status is `rejected`, else `needsYouInbox:anotherCallerResolved` (including when the status is absent), with the bundle's first question text as `{question}`, or `needsYouInbox:questionFromAgent` when that text is empty (the Inbox row's fallback); no new copy |
  | no longer active | collapse; toast `needsYouInbox:bundleNoLongerActive` with the same `{question}` and fallback; no new copy |
  | late message admitted | not reachable on this card: the component reports it only when `onLateAnswer` is passed and the card passes none; no handler branch is required |
  | failed | stay expanded with the answer kept; **Try again** |

- A relay read with `clarification: null`, or a failed read, shows the phase
  1 text and **Open task**.

## Permission card

`apps/web/app/coordinator/components/permission-answer.tsx`:

- A permission item (`pending_action` `permission`) shows **Answer here** to
  managers while phase 3 is effective, once the relay read of the previous
  section has resolved with `permission` non-null; otherwise the phase 1 text
  and **Open task**.
- A permission whose relay `options` list is empty is never returned by the
  relay read, so its item keeps the phase 1 text and **Open task**
  (`AC-COORDINATOR-RELAY-002.5`).
- It renders the permission message's title, action details and the decision
  buttons of the bullet below, and resolves through the same `permission.respond` WebSocket
  request `use-permission-handlers.ts` sends, with the message's `task_id`,
  `session_id`, `request_id` and `pending_id`. The backend's existing handler
  records the `PermissionResolutionAudit` with source `web` and the browser
  identity.
- The extraction `apps/web/lib/permissions/respond.ts` carries three things
  from `use-permission-handlers.ts`, exported for both callers: the request
  builder, `isStalePermissionResponse`, and the option-to-request mapping.
  The mapping is the chat's: an option of kind `reject_once` or
  `reject_always` sends `rejected: true`; the Codex cancel decision sends
  `cancelled: true` with no `option_id`; any other option sends its `option_id` with both flags false.
  The buttons are the chat's row, not one per option. For a non-Codex
  request: **Deny** sends the first option, in list order, whose kind is
  `reject_once` or `reject_always` (the chat's `handleReject`), else the
  cancel decision; **Approve** sends the first `allow_once` option, else the
  first `allow_always` option, and is shown disabled, sending nothing, when
  neither exists; **Always allow** appears only when an `allow_always` option
  exists and sends it. So two reject options never produce two buttons. A
  request is a Codex request when at least one option carries
  `metadata.codex_app_server === true` (the chat's `offeredChoices` filter);
  the card then renders the chat's offered choices: one button per such
  option, ignoring options without the flag, labelled by the chat's
  `codexDecisionLabel` (a Codex `decline` reads **Deny**), and a
  `codex_decision` of `cancel` sends `cancelled: true`. Labels and the
  builders are exported from the same module and reuse the chat's `task:`
  and `common:` keys, so the card adds no new copy for them.
- A missing `request_id` on the message, or no WebSocket client, keeps the
  card expanded and shows the failed-response toast (`task:permissionResponseFailed`)
  with **Try again** available; unlike the chat, the card never returns
  silently. The relay read never returns a message without `request_id` or
  `pending_id`, so the first case is reachable only if the message changes
  between the read and the click.
- While a response is in flight every option button is disabled. **Try
  again** resends the option last chosen; the card does not reread the
  permission before retrying.
- A stale-response error (the `isStalePermissionResponse` test) collapses
  with the toast `task:permissionRequestNoLongerAvailable` (the chat's
  existing copy); any other error keeps the card expanded with the toast
  `task:permissionResponseFailed` and **Try again**. A timeout is a non-stale
  error: if the timed-out send had in fact been recorded, Try again returns a
  stale-response error and the card collapses with the no-longer-available
  toast, which is accepted (the answer was recorded once, and the resend
  changes nothing). A success collapses the card; the item
  leaves when `task.status_summary.updated` clears `pending_action`, or at the
  next list refresh.
- The chat's behaviour is unchanged and pinned by its existing tests.

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
