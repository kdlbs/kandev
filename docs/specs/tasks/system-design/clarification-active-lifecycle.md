---
status: current
system: tasks
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
  - REQ-TASKS-CLARIFICATION-SCENARIOS-001
---


# Active clarification lifecycle System Design



## Purpose and boundaries



The task system owns clarification bundle authority, turn scoping, response persistence, and detached-resume recovery. The UI and agent systems consume the resulting projections and events.



## Requirement mapping

| Requirement | Design source |
| --- | --- |
| `REQ-TASKS-CLARIFICATION-LIFECYCLE-001` | Extracted from the legacy design sections below. |
| `REQ-TASKS-CLARIFICATION-SCENARIOS-001` | Extracted from the legacy design sections below. |


## Data model

No schema change.

- Clarification questions remain `task_session_messages` rows with
  `type = "clarification_request"`.
- Rows in one bundle share `metadata.pending_id`; terminal status remains in `metadata.status`.
- `task_session_messages.turn_id` associates a question with its turn. The newest authoritative durable
  `task_session_turns` record for the session identifies the current turn; deleting messages does not
  delete that parent turn or move ownership backward.
- A pre-acknowledgement successor carries `metadata.prompt_dispatch_pending=true` plus the source
  clarification turn, pending ID, and exact claimed message IDs. An empty row carrying only the pending
  marker is not turn authority. Immediately before external dispatch,
  `metadata.prompt_dispatch_attempted=true` records the at-most-once boundary and makes the successor
  authoritative if the process stops in the ambiguous window. A message referencing the successor is
  independent durable evidence of ambiguous acceptance.
- `metadata.agent_disconnected=true` records that no in-memory waiter owns an otherwise active bundle.
- A superseded row may retain `metadata.status = "pending"`. Pending metadata is historical evidence,
  not sufficient proof that the request is operational.
- A missing-status row in an older turn is superseded history. Turn ownership takes precedence over
  the legacy rule that missing status means pending.
- `TaskSession.pending_action` and `TaskStatusSummary.pending_action` remain bounded derived fields.
  They are reconstructable and never become independent clarification state.

## API surface

No new route. Task-session responses add `pending_action_revision` beside the existing
`pending_action` projection.

- `GET /api/v1/tasks/:taskId/task-sessions` continues to expose each session's current derived
  `pending_action`; task navigation uses this existing field.
- Semantic `session.message.added`, `session.message.updated`, and `session.message.deleted`
  notifications include the authoritative per-session `pending_action` after mutations that can
  change it. The field is explicit null when clean and omitted when projection fails or the event is
  a replaceable content-only update; clients preserve the prior value when it is omitted. Each
  projection includes a `pending_action_revision`, shared with REST task-session snapshots. Its
  decimal epoch is a database-backed, monotonically allocated backend generation; sequence orders
  reads within that generation. Clients compare both fields and reject any older result, including
  an unseen pre-restart epoch delivered after client state has been rebuilt.
- `GET /api/v1/task-sessions/:sessionId/turns` continues to expose durable turn history;
  unpublished reservations stay hidden until publication or durable message evidence, including while
  an attempt marker makes them internal current-turn authority. Attempted reservations are preserved
  across restart because their dispatch outcome is ambiguous. Visible history is ordered ascending by
  `started_at`, `created_at`, then `id`, matching the reverse ordering used to select the current turn.
- Task list, workflow snapshot, and boot payloads continue to expose task-level `pending_action` in
  the status summary and legacy fallback fields.
- `POST /api/v1/clarification/:pendingId/respond` uses one state-based contract:
  - `active_live`: answer or rejection returns success and is delivered to the same-turn waiter.
  - `active_detached`: an answer returns success only after the orchestrator acknowledges one resume
    dispatch and durably publishes its successor within a bounded wait; it does not wait for the resumed
    turn to complete. Rejection returns success and persists without resuming the agent. If resume
    acceptance fails, an answer returns a server error and the still-current bundle remains answerable
    for retry. If acceptance succeeds but successor publication fails, the endpoint returns a
    non-retryable server error and keeps the bundle terminal.
  - `superseded_history` or `terminal`: answer or rejection returns conflict, performs no write, and
    initiates no agent resume.
  - `delivery_claimed`: a concurrent second response returns conflict, performs no write, and
    initiates no agent resume because the first response already claimed every pending row.
- `POST /api/v1/clarification/:pendingId/cancel` remains the low-level cancellation path for a request
  still owned by the in-memory clarification store. The chat Skip control uses `/respond` with
  `rejected=true`, including for detached requests.

## State machine

One clarification bundle has five operational states:

1. `active_live`: rows are pending in the current turn and an in-memory waiter exists.
2. `active_detached`: rows are pending in the current turn, no waiter exists, and
   `agent_disconnected=true` records deferred-answer behavior.
3. `delivery_claimed`: current-turn rows carry their provisional terminal answer plus a durable
   response-delivery intent, but no handoff boundary has been acknowledged yet.
4. `terminal`: every actionable row is answered, rejected, cancelled, expired, or deleted, with no
   outstanding delivery intent.
5. `superseded_history`: rows still carry pending history, but a newer turn is current.

Transitions:

- Request creation enters `active_live`.
- Wait timeout, disconnect, or turn teardown moves `active_live -> active_detached` once.
- A response first moves either active state to `delivery_claimed`. Successful answer delivery or Skip
  then moves that exact `pending_id` to `terminal`. For a live response, the waiter must durably confirm
  consumption before returning it to the agent. A backend restart before any handoff restores a
  still-current `delivery_claimed` bundle to its prior active state; if a newer turn or terminal session
  already superseded it, recovery retires the intent and preserves terminal history. Cancel, expiry,
  or deletion moves an active state directly to `terminal`. A failed detached resume acceptance returns to
  `active_detached` while the same turn remains current only when dispatch is known not to have been
  accepted and rollback succeeds. A post-attempt crash or post-acceptance publication failure remains
  terminal because the prompt may already be running.
- Acceptance of a newer turn moves any older pending bundle to `superseded_history` operationally;
  no history rewrite is required.
- Neither `terminal` nor `superseded_history` can become active again. Only the provisional
  `delivery_claimed` state is recoverable. A new request creates a new
  bundle identity; message deletion cannot reverse this transition.

## Permissions

No authorization change. A user can see, answer, or dismiss only clarification data for a task and
session they can already access. Session selection does not broaden task visibility.

## Failure modes

- Active-state repository read fails: workflow guarding fails closed, and projections keep the last
  known pending value. A later message event or list/boot read retries convergence.
- Terminal-session expiry persistence fails: the terminal session state still quarantines pending
  history from task/session projections and interactive overlays; a stale response claim remains
  rejected by the terminal-session predicate.
- A forced task-session list cannot project authoritative pending actions: fail the HTTP or WebSocket
  request instead of returning a successful list with empty pending ownership.
- Summary compare-and-set loses a race: reload the newer summary, reapply authoritative pending state,
  and retry within a bounded loop. Exhaustion is an error, so restored-state acknowledgement is withheld
  until the pending action is durably confirmed. Task-list and boot reads explicitly invalidate that
  task's stale summary on repair failure so an unchanged browser cache clears it and exposes the
  authoritative coarse pending action. Ordinary summary omission remains a partial-response no-op, and
  an invalidating response cannot erase a newer live summary received while that read was in flight.
  Never overwrite unrelated newer summary fields.
- Summary repair persists but its WebSocket publication fails: the initiating response carries the
  corrected summary; other clients converge on their next event or read.
- A stale browser submits an older-turn answer: return conflict, do not update runtime ownership, and
  do not dispatch a prompt.
- Detached resume context resolution or orchestrator acceptance fails: use a non-cancelled context with
  a finite deadline for acceptance and persistence, withhold terminal message events, restore the
  still-current bundle to pending, and return a retryable server error instead of reporting false
  success. Attempt to refresh and persist the task summary synchronously from authoritative pending
  rows, and publish the committed restored messages even if that acknowledgement fails. A later event
  or read repairs the summary cache; the response still reports that the durable answer can be retried.
- Persisting a successor turn or dispatch-attempt marker fails: use a fresh bounded context to roll back
  the session claim and any reserved successor before making an external executor call, restore the
  still-current bundle, and return a retryable server error.
- Reserved-successor rollback fails or has an ambiguous durable outcome: keep the live reservation
  unresolved so ready handling waits and later prompt admission remains blocked until restart recovery
  reconciles the row.
- Clarification detachment, expiry persistence, and terminal bundle publication ignore request
  cancellation but always use a fresh bounded context, so a database lock or synchronous summary
  refresh cannot hold the per-session pause or HTTP response indefinitely.
- Agentctl accepts a detached resume but durable successor publication fails: return a non-retryable
  server error, keep the claimed bundle terminal, and make later rollback fail closed so the accepted
  answer cannot be dispatched again in-process.
- Live waiter delivery fails unexpectedly: restore the durable claim only if its turn remains current,
  report whether retry state was recovered, and never restore it after a successor turn is accepted. A
  started confirmation may finish after the responder's bounded wait, but it cannot mutate the
  responder's claim snapshot and its durable finalization serializes against that restore.
- Historical partial terminalization leaves pending and terminal siblings in one current-turn bundle:
  complete the pending siblings without rewriting terminal history or returning a permanent conflict.
- A malformed persisted pending row has no matching durable turn: drain any live in-memory waiter, but
  keep the row inert. If such pre-turn history is encountered, repair it through explicit data cleanup
  rather than treating it as current input authority.
- Session loading fails during task activation: retain existing navigation fallback instead of
  stranding the user in the task drawer or on an unchanged URL.
- A newer task-summary revision arrives while pending-owner loading is in flight: discard the delayed
  owner-session result, open the requested task through the task-only fallback, and let the current
  projection render the authoritative pending owner.
- Backend stops after reserving a detached-answer successor but before the attempt marker: startup
  restores only the clarification rows claimed for that dispatch and removes the empty reservation.
- Backend stops after the attempt marker but before dispatch acknowledgement: startup fails closed,
  preserves the successor as authoritative, and keeps its exact clarification claim terminal. Output
  referencing the reservation provides the same conservative authority even without the marker.
- A delayed predecessor ready event arrives between the attempt marker and agentctl acknowledgement:
  wait for the live reservation outcome, then reject the event if its prompt generation was superseded;
  never complete the reserved successor or evaluate predecessor workflow completion against it. If the
  reservation rolls back and the predecessor generation still owns the session, process the ready event
  normally instead of stranding the predecessor turn. Drop a generationless event after the wait because
  it cannot be correlated safely with the predecessor.
- Unpublished-reservation reconciliation fails during startup: fail startup before event processing or
  prompt admission begins so no new turn can supersede an unrecovered clarification claim.
- Response-delivery intent reconciliation fails during startup: fail startup before event processing or
  prompt admission begins. Never leave an unhanded terminal claim permanently unanswerable.
- Unpublished-reservation recovery metadata contains a non-string or empty claimed-message ID: fail the
  recovery transaction and startup, preserving the reservation and claimed rows for diagnosis and retry.

## Persistence guarantees

- Message history remains durable and is not destructively rewritten merely because a newer turn
  exists.
- Active clarification state is reconstructable after restart from message status plus the newest
  authoritative durable turn.
- Current-turn ownership is reconstructable from durable turn rows even when a turn has no remaining
  messages.
- Unpublished detached-answer reservations carry enough recovery identity to restore only their own
  claimed rows. A durable attempt marker separates safe rollback from ambiguous dispatch, and startup
  reconciliation runs even when no executor record remains.
- Provisional terminal response claims carry a durable delivery intent. Startup restores that exact
  current-turn claim when no live or detached handoff became authoritative, while prompt-reservation
  recovery owns any detached dispatch that reached its durable reservation boundary.
- Task summaries are caches. Boot and task-list reads correct a stale persisted `pending_action` with
  a monotonic revision while preserving all unrelated summary fields.
- No one-off mutation or backfill of an existing installation database is required. Deploying the
  corrected derivation makes historical older-turn pending rows inert and repairs their summaries on
  normal reads.
