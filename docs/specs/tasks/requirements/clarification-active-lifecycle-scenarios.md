---
status: active
system: tasks
created: 2026-08-14
owners:
  - kandev
---


# Active clarification lifecycle scenarios Requirements



## Overview



The lifecycle contract covers timeout, supersession, response, cleanup, and recovery scenarios across live and detached sessions.



## Scenarios

- **GIVEN** a clarification wait disconnects and no newer turn exists, **WHEN** the task is reloaded,
  **THEN** the detached question remains visible and answerable, its session and task advertise
  `clarification`, and workflow completion stays blocked.
- **GIVEN** an older turn retains a detached pending row, **WHEN** the session accepts a newer ordinary
  turn, **THEN** the old row remains in history but no overlay, pending projection, detach event, or
  workflow barrier derives from it.
- **GIVEN** a newer turn superseded an older pending question, **WHEN** every message in the newer turn
  is deleted, **THEN** the durable newer turn remains current and the older question stays inert.
- **GIVEN** an attempted successor reservation is authoritative but intentionally hidden from client
  turn history, **WHEN** the session projection explicitly reports no clarification action, **THEN** a
  pending question on the visible predecessor stays inert in chat and pending-input indicators.
- **GIVEN** a browser cached an explicit clean session projection, **WHEN** a new current-turn
  clarification message arrives, **THEN** the same event advances the session projection to
  `clarification` before pending discovery so the question remains visible and answerable.
- **GIVEN** a task-session list request is in flight, **WHEN** a newer semantic message event updates
  the same session before the HTTP response resolves, **THEN** the response's older projection
  revision cannot replace the WebSocket projection (and the reverse delivery order is equally safe).
- **GIVEN** an old detached question and a newer clarification bundle, **WHEN** the user skips the
  newer bundle and reloads, **THEN** neither bundle reappears, the task question icon is absent, and
  later turn completion cannot re-arm the old bundle.
- **GIVEN** a persisted summary says `pending_action=clarification` while current-turn repository state
  has no pending action, **WHEN** a boot or task-list payload is built, **THEN** the summary revision
  advances with no pending action and all unrelated fields remain unchanged.
- **GIVEN** a task's secondary session owns a current clarification while the primary session is
  clean, **WHEN** the user activates the task from the desktop sidebar or phone task drawer, **THEN**
  Kandev bypasses any cached session list, selects the secondary session, closes the drawer when
  applicable, and shows the question.
- **GIVEN** clarification pause observes no active turn, **WHEN** detachment creates the session's first
  turn before cancellation registration completes, **THEN** the pause rejects its stale cancellation
  and does not cancel or complete that new turn.
- **GIVEN** pending-owner loading began for one task-summary revision, **WHEN** a newer summary changes
  the pending action before the session response is applied, **THEN** desktop and phone activation do
  not navigate to the obsolete owner, open the selected task through the task-only fallback, and close
  the phone drawer.
- **GIVEN** two forced session loads overlap for one task, **WHEN** the older response finishes last,
  **THEN** it cannot overwrite the newer session snapshot or revive an obsolete pending owner.
- **GIVEN** a forced session load is pending in the phone task sheet, **WHEN** a responsive-layout
  change unmounts it and mounts the tablet sheet before that load finishes, **THEN** the old
  continuation cannot navigate, change the active selection, or close the replacement sheet.
- **GIVEN** a pending task has no loaded owner, **WHEN** activation falls back to its task route,
  **THEN** the outgoing session layout is released before the active session is cleared.
- **GIVEN** a stale browser still displays a superseded question, **WHEN** it submits an answer,
  **THEN** the server returns conflict and does not resume or otherwise prompt the agent.
- **GIVEN** a detached current-turn answer is not accepted by the orchestrator, **WHEN** the response
  endpoint fails, **THEN** it returns a server error, keeps the question answerable, and a later retry
  delivers exactly one accepted resume request without a failed-dispatch turn superseding the bundle.
- **GIVEN** agentctl accepts a detached answer but successor publication fails, **WHEN** the response
  endpoint completes, **THEN** it returns an error, keeps the answer terminal, and does not make that
  accepted successor eligible for rollback or in-process redispatch.
- **GIVEN** agentctl accepts a detached answer and later transport handling also fails, **WHEN** prompt
  handling returns, **THEN** normal failure cleanup runs and the accepted-answer marker still prevents
  the terminal clarification from being reopened.
- **GIVEN** the backend stops after terminalizing a detached answer and reserving its successor but
  before marking dispatch attempted, **WHEN** it starts again, **THEN** it deletes the empty
  reservation, restores the exact claimed rows to pending, and leaves pre-existing terminal siblings
  unchanged.
- **GIVEN** the backend stops after marking a detached-answer dispatch attempted but before observing
  acknowledgement, **WHEN** it starts again, **THEN** it preserves the successor as current and keeps
  the exact claimed rows terminal rather than risking duplicate answer dispatch.
- **GIVEN** the backend stops after claiming a current-turn response but before either a live waiter or
  detached resumer receives it, **WHEN** it starts again, **THEN** startup restores the exact claimed
  rows to pending and the same answer can be submitted again.
- **GIVEN** a response is enqueued to a live waiter, **WHEN** durable confirmation has not completed,
  **THEN** the waiter does not return the response and restart recovery may safely restore the claim.
- **GIVEN** durable confirmation starts but outlives the responder's bounded wait, **WHEN** the responder
  attempts recovery, **THEN** the immutable claim is race-free and repository serialization lets either
  finalization or restore win without reopening a successfully finalized response.
- **GIVEN** cancellation removes a live clarification after a responder loads it but before response
  delivery is claimed, **WHEN** that responder continues, **THEN** it returns not-found immediately so
  the caller can use detached recovery instead of waiting for an orphaned delivery confirmation. If
  response delivery wins first, later cancellation cannot preempt its durable confirmation.
- **GIVEN** a primary-answer watchdog survives until another turn supersedes its clarification turn,
  **WHEN** its fallback timer expires, **THEN** both the preflight and serialized prompt-admission checks
  reject the stale answer without prompting or cancelling the successor.
- **GIVEN** a primary-answer watchdog has entered fallback authority or prompt work, **WHEN** session
  activity or service shutdown cancels the watchdog, **THEN** that cancellation reaches the in-flight
  repository and prompt calls.
- **GIVEN** a reserved successor is marked dispatch-attempted but remains unpublished, **WHEN** a client
  loads turn history before agentctl accepts or rejects it, **THEN** the successor is omitted until
  publication or durable message evidence prevents rollback from leaving stale client-only history.
- **GIVEN** a predecessor ready event arrives while a detached-answer successor is reserved, **WHEN**
  the successor dispatch resolves, **THEN** the handler revalidates prompt generation and the stale
  predecessor cannot complete the successor or run `on_turn_complete` against it; if reservation
  rollback leaves that predecessor generation authoritative, its ready event completes normally.
- **GIVEN** reserved-successor deletion returns an error, **WHEN** rollback handling completes, **THEN**
  the reservation waiter remains unresolved and another prompt cannot enter that session before
  restart recovery.
- **GIVEN** an unpublished reservation stores an attempted marker as boolean `true`, string `"true"` or
  `"1"`, or numeric `1`, **WHEN** startup recovery runs, **THEN** it preserves the ambiguous successor
  and clears recovery metadata instead of restoring the answer and deleting the turn.
- **GIVEN** a generationless ready event waits on a live reservation, **WHEN** the reservation rolls
  back, **THEN** Kandev drops the uncorrelatable event without changing session or workflow state.
- **GIVEN** a cancelled request triggers clarification detach or expiry, **WHEN** repository work
  begins, **THEN** it uses a non-cancelled context with a finite deadline.
- **GIVEN** a session transition to completed, failed, or cancelled is durably accepted, **WHEN** the
  transition publishes, **THEN** current-turn pending clarification bundles expire before a terminal
  session can leave an interactive overlay behind.
- **GIVEN** a detached answer reaches synchronous orchestrator resume, **WHEN** the orchestrator does
  not acknowledge it before the bounded deadline, **THEN** Kandev treats acceptance as failed and
  restores the still-current bundle.
- **GIVEN** startup cannot reconcile an unpublished prompt reservation, **WHEN** the orchestrator starts,
  **THEN** startup returns an error before watcher, scheduler, or prompt admission begins.
- **GIVEN** restart recovery preserves an accepted or ambiguous prompt reservation, **WHEN** its public
  start event may have been missed, **THEN** startup durably retains and retries an ordered event replay
  until `turn.started` and any already-required `turn.completed` event are accepted before clearing the
  recovery marker.
- **GIVEN** an unpublished prompt reservation contains a malformed claimed-message ID list, **WHEN**
  startup recovery decodes it, **THEN** recovery fails closed without deleting the reservation or
  reopening only a subset of its claimed rows.
- **GIVEN** pending ownership changes while desktop or mobile task selection loads sessions, **WHEN** the
  delayed load settles, **THEN** Kandev ignores its stale owner choice but still opens the selected task
  through the task-only fallback, and the mobile sheet closes.
- **GIVEN** a task-list refresh observes a detached answer's temporary terminal claim, **WHEN** resume
  rejection restores that claim, **THEN** Kandev acknowledges durable summary convergence before
  reporting retryability and publishes the restored pending rows so other clients converge.
- **GIVEN** live publication or synchronous summary acknowledgement fails after restored clarification
  rows commit, **WHEN** the detached resume returns, **THEN** Kandev reports the convergence error while
  still identifying the durably restored answer as safe to retry.
- **GIVEN** the selected task projection disappears while desktop or mobile session loading is in
  flight, **WHEN** the delayed load settles, **THEN** Kandev leaves task and session selection unchanged.
- **GIVEN** a newer desktop or mobile forced session load aborts an older load for the same task,
  **WHEN** the older continuation handles `AbortError`, **THEN** it leaves the winning task and session
  selection unchanged.
- **GIVEN** two phone task sheets are mounted, **WHEN** both have an in-flight task selection, **THEN**
  closing or selecting within one sheet does not invalidate the other sheet's selection generation.
- **GIVEN** authoritative pending-action projection fails while listing a task's sessions, **WHEN** the
  HTTP or WebSocket list request completes, **THEN** it returns an internal error rather than a false
  clean-session response.
- **GIVEN** two request identities produce terminal and pending events close together, **WHEN** the
  projector refreshes, **THEN** its result matches current repository state rather than event order.

## Requirements



### REQ-TASKS-CLARIFICATION-SCENARIOS-001: Active clarification lifecycle scenarios



**Intent:** The lifecycle contract covers timeout, supersession, response, cleanup, and recovery scenarios across live and detached sessions.



#### Acceptance criteria



- **AC-TASKS-CLARIFICATION-SCENARIOS-001.1:** When any listed lifecycle scenario occurs, the system shall produce the observable state and response outcome described by that scenario.



## Out of scope

- Rewriting or deleting historical pending message rows solely to clean old data.
- A dedicated clarification table or schema migration.
- Redesigning the desktop sidebar, phone task drawer, chat cards, or clarification carousel.
- Changing permission-request lifecycle semantics; pending-session navigation reuses the shared
  `pending_action` type for permission parity.
- Changing notification history for a clarification that was valid when its notification fired.
- Automatically choosing a non-primary session when the task has no current pending action.