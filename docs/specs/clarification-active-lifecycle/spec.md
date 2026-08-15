---
status: approved
created: 2026-08-14
owner: kandev
---

# Active Clarification Lifecycle

## Why

A clarification can outlive the agent wait that created it. Kandev keeps that detached question
answerable so a timeout or connection loss does not discard required user input. That durability must
not let a question from an older turn remain operational after the session has accepted newer work.

One stale clarification currently can reappear in chat, restore a task-row question icon, and block a
workflow transition after the user has dismissed a newer question. A real question in a secondary
session can also produce a correct task-row icon while task navigation opens the clean primary session,
hiding the action the icon represents.

## What

- A clarification bundle is active only when at least one row in the bundle is pending and the bundle
  belongs to the session's current turn.
- A detached bundle remains active and answerable while its turn remains current. Detachment sets
  `agent_disconnected=true`; it does not by itself resolve the question.
- Acceptance of a newer turn supersedes every pending clarification from an older turn. Superseded
  rows remain transcript history but cannot drive a chat overlay, task/session pending projection,
  workflow guard, turn-completion detach pass, or late agent resume.
- Deleting every message from the newer turn does not move ownership backward or reactivate an older
  clarification.
- All backend consumers derive active clarification state from one repository rule. Event payloads
  trigger projection refreshes; they are not a second source of pending truth.
- Repeated detach/completion processing is a semantic no-op after a bundle is already detached. It
  emits no duplicate `message.updated` occurrence.
- Resolving, rejecting, cancelling, expiring, or deleting one bundle changes only that bundle. It
  cannot clear or re-arm another bundle in the same session.
- The chat's Skip action rejects the exact visible bundle through the existing response endpoint. A
  live waiter receives the rejection in the same turn. A detached current-turn bundle is persisted as
  rejected without resuming the agent.
- An affirmative response to a detached current-turn bundle persists the answer and publishes one
  resume event. A rejection persists terminal status without publishing a resume event.
- Every response atomically claims current-turn ownership before it can reach a live waiter or publish
  a detached resume. Terminal message updates are published only after delivery succeeds. If detached
  resume publication fails, the endpoint returns an error and restores the still-current bundle to
  pending so the same answer can be retried.
- A current-turn bundle remains answerable while any sibling question is pending. Recovery claims only
  those pending rows, preserves siblings already made terminal by an earlier partial write, and restores
  only the claimed rows if detached delivery fails.
- Any response to a superseded or terminal bundle returns conflict, performs no message mutation, and
  publishes no resume event. Current clients close their obsolete local overlay through the existing
  conflict handling.
- Persisted task status summaries reconcile `pending_action` against current-turn repository state on
  source events and task-list/boot reads. Existing summaries are repaired, not only missing rows.
- When a task row advertises a pending action, desktop and phone task activation load the task's
  sessions and select the newest input-capable session whose `pending_action` matches the task action.
  This pending owner outranks remembered-session and primary-session preferences. Normal preference
  order returns when no matching pending owner exists.

## Data model

No schema change.

- Clarification questions remain `task_session_messages` rows with
  `type = "clarification_request"`.
- Rows in one bundle share `metadata.pending_id`; terminal status remains in `metadata.status`.
- `task_session_messages.turn_id` associates a question with its turn. The newest durable
  `task_session_turns` record for the session identifies the current turn; deleting messages does not
  delete that parent turn or move ownership backward.
- `metadata.agent_disconnected=true` records that no in-memory waiter owns an otherwise active bundle.
- A superseded row may retain `metadata.status = "pending"`. Pending metadata is historical evidence,
  not sufficient proof that the request is operational.
- A missing-status row in an older turn is superseded history. Turn ownership takes precedence over
  the legacy rule that missing status means pending.
- `TaskSession.pending_action` and `TaskStatusSummary.pending_action` remain bounded derived fields.
  They are reconstructable and never become independent clarification state.

## API surface

No new route or response field.

- `GET /api/v1/tasks/:taskId/task-sessions` continues to expose each session's current derived
  `pending_action`; task navigation uses this existing field.
- `GET /api/v1/task-sessions/:sessionId/turns` continues to expose durable turn history; active chat
  uses its newest turn identity instead of inferring ownership from surviving messages.
- Task list, workflow snapshot, and boot payloads continue to expose task-level `pending_action` in
  the status summary and legacy fallback fields.
- `POST /api/v1/clarification/:pendingId/respond` uses one state-based contract:
  - `active_live`: answer or rejection returns success and is delivered to the same-turn waiter.
  - `active_detached`: an answer returns success, persists, and publishes one resume event; rejection
    returns success, persists, and publishes no resume event. If resume publication fails, an answer
    returns a server error and the still-current bundle remains answerable for retry.
  - `superseded_history` or `terminal`: answer or rejection returns conflict, performs no write, and
    publishes no resume event.
- `POST /api/v1/clarification/:pendingId/cancel` remains the low-level cancellation path for a request
  still owned by the in-memory clarification store. The chat Skip control uses `/respond` with
  `rejected=true`, including for detached requests.

## State machine

One clarification bundle has four operational states:

1. `active_live`: rows are pending in the current turn and an in-memory waiter exists.
2. `active_detached`: rows are pending in the current turn, no waiter exists, and
   `agent_disconnected=true` records deferred-answer behavior.
3. `terminal`: every actionable row is answered, rejected, cancelled, expired, or deleted.
4. `superseded_history`: rows still carry pending history, but a newer turn is current.

Transitions:

- Request creation enters `active_live`.
- Wait timeout, disconnect, or turn teardown moves `active_live -> active_detached` once.
- Successful answer delivery, Skip, cancel, expiry, or deletion moves either active state to
  `terminal` for that exact `pending_id`. A failed detached answer publication returns to
  `active_detached` while the same turn remains current.
- Acceptance of a newer turn moves any older pending bundle to `superseded_history` operationally;
  no history rewrite is required.
- Neither `terminal` nor `superseded_history` can become active again. A new request creates a new
  bundle identity; message deletion cannot reverse this transition.

## Permissions

No authorization change. A user can see, answer, or dismiss only clarification data for a task and
session they can already access. Session selection does not broaden task visibility.

## Failure modes

- Active-state repository read fails: workflow guarding fails closed, and projections keep the last
  known pending value. A later message event or list/boot read retries convergence.
- Summary compare-and-set loses a race: reload the newer summary, reapply authoritative pending state,
  and retry within a bounded loop. Never overwrite unrelated newer summary fields.
- Summary repair persists but its WebSocket publication fails: the initiating response carries the
  corrected summary; other clients converge on their next event or read.
- A stale browser submits an older-turn answer: return conflict, do not update runtime ownership, and
  do not dispatch a prompt.
- Detached resume context resolution or event publication fails: use a non-cancelled write context,
  withhold terminal message events, restore the still-current bundle to pending, and return a retryable
  server error instead of reporting false success.
- Historical partial terminalization leaves pending and terminal siblings in one current-turn bundle:
  complete the pending siblings without rewriting terminal history or returning a permanent conflict.
- A malformed persisted pending row has no matching durable turn: drain any live in-memory waiter, but
  keep the row inert. If such pre-turn history is encountered, repair it through explicit data cleanup
  rather than treating it as current input authority.
- Session loading fails during task activation: retain existing navigation fallback instead of
  stranding the user in the task drawer or on an unchanged URL.

## Persistence guarantees

- Message history remains durable and is not destructively rewritten merely because a newer turn
  exists.
- Active clarification state is reconstructable after restart from message status plus the newest
  durable turn.
- Current-turn ownership is reconstructable from durable turn rows even when a turn has no remaining
  messages.
- Task summaries are caches. Boot and task-list reads correct a stale persisted `pending_action` with
  a monotonic revision while preserving all unrelated summary fields.
- No one-off mutation or backfill of an existing installation database is required. Deploying the
  corrected derivation makes historical older-turn pending rows inert and repairs their summaries on
  normal reads.

## Scenarios

- **GIVEN** a clarification wait disconnects and no newer turn exists, **WHEN** the task is reloaded,
  **THEN** the detached question remains visible and answerable, its session and task advertise
  `clarification`, and workflow completion stays blocked.
- **GIVEN** an older turn retains a detached pending row, **WHEN** the session accepts a newer ordinary
  turn, **THEN** the old row remains in history but no overlay, pending projection, detach event, or
  workflow barrier derives from it.
- **GIVEN** a newer turn superseded an older pending question, **WHEN** every message in the newer turn
  is deleted, **THEN** the durable newer turn remains current and the older question stays inert.
- **GIVEN** an old detached question and a newer clarification bundle, **WHEN** the user skips the
  newer bundle and reloads, **THEN** neither bundle reappears, the task question icon is absent, and
  later turn completion cannot re-arm the old bundle.
- **GIVEN** a persisted summary says `pending_action=clarification` while current-turn repository state
  has no pending action, **WHEN** a boot or task-list payload is built, **THEN** the summary revision
  advances with no pending action and all unrelated fields remain unchanged.
- **GIVEN** a task's secondary session owns a current clarification while the primary session is
  clean, **WHEN** the user activates the task from the desktop sidebar or phone task drawer, **THEN**
  Kandev selects the secondary session, closes the drawer when applicable, and shows the question.
- **GIVEN** a stale browser still displays a superseded question, **WHEN** it submits an answer,
  **THEN** the server returns conflict and does not resume or otherwise prompt the agent.
- **GIVEN** a detached current-turn answer cannot publish its resume event, **WHEN** the response
  endpoint fails, **THEN** it returns a server error, keeps the question answerable, and a later retry
  publishes exactly one successful resume event.
- **GIVEN** two request identities produce terminal and pending events close together, **WHEN** the
  projector refreshes, **THEN** its result matches current repository state rather than event order.

## Out of scope

- Rewriting or deleting historical pending message rows solely to clean old data.
- A dedicated clarification table or schema migration.
- Redesigning the desktop sidebar, phone task drawer, chat cards, or clarification carousel.
- Changing permission-request lifecycle semantics; pending-session navigation reuses the shared
  `pending_action` type for permission parity.
- Changing notification history for a clarification that was valid when its notification fired.
- Automatically choosing a non-primary session when the task has no current pending action.
