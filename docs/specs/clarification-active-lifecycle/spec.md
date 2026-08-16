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
- Detachment claims pending, non-detached rows from the current durable turn in one database update
  and publishes only the rows that update returns. A concurrent answer or newer turn cannot be
  overwritten by a stale read-modify-write detachment.
- Clarification-pause cancellation snapshots turn authority before detachment. With a wired turn
  service, both a specific turn and the absence of a turn are explicit expectations; a first or
  successor turn created during detachment cannot be cancelled by the stale pause. Installations
  without a turn service retain the legacy unscoped fallback.
- Resolving, rejecting, cancelling, expiring, or deleting one bundle changes only that bundle. It
  cannot clear or re-arm another bundle in the same session.
- The chat's Skip action rejects the exact visible bundle through the existing response endpoint. A
  live waiter receives the rejection in the same turn. A detached current-turn bundle is persisted as
  rejected without resuming the agent.
- An affirmative response to a detached current-turn bundle returns success only after the
  orchestrator accepts one resume dispatch within a bounded wait. The response waits for prompt
  acknowledgement, not agent-turn completion. Before dispatch, the successor is durably reserved but
  marked unpublished so provider frames can reference it without making it current. Immediately before
  the external executor call, Kandev durably marks the reservation attempted. That marker is the
  at-most-once boundary: a crash can no longer prove whether agentctl accepted the prompt, so restart
  preserves the successor and keeps the claimed bundle terminal. Acknowledgement clears the recovery
  metadata and publishes the reservation, and HTTP success requires that durable publication. If the
  predecessor's delayed ready event overlaps this private reservation, ready handling waits for the
  reservation to resolve and then revalidates prompt generation before touching turn or workflow state.
  It cannot complete the reserved successor or run predecessor completion actions against it. When the
  reservation rolls back, a ready event whose predecessor generation still owns the session continues
  through normal completion so the predecessor and its queue are not stranded. A generationless ready
  event that overlapped the reservation is dropped after resolution because ownership cannot be proven.
  If the attempt marker cannot be persisted, dispatch does not occur and a fresh bounded context rolls
  back the reservation and session claim even when the transport context was cancelled, so the answer
  remains retryable.
  If agentctl synchronously rejects the prompt, the reservation is rolled back and the answer can be
  restored. If agentctl accepts the prompt but publication or later transport handling fails, the
  endpoint returns a server error, performs normal prompt-failure cleanup, and keeps the claimed bundle
  terminal because retrying could dispatch the answer twice. Startup
  deletes only an empty, unattempted unpublished reservation and restores the exact clarification rows
  claimed for its dispatch; an attempt marker or message evidence instead proves dispatch ambiguity and
  preserves the successor. Authority and recovery treat boolean `true`, strings `"true"` and `"1"`,
  and numeric `1` as equivalent pending/attempted flags across SQLite and PostgreSQL. If reservation
  reconciliation is unavailable or fails, orchestrator startup
  fails before watcher, scheduler, or prompt admission starts; the next start retries recovery. A
  production turn repository must provide this recovery capability through its compile-time contract.
  A rejection persists terminal status without resuming the agent.
- Every response atomically claims current-turn ownership before it can reach a live waiter or request
  a detached resume. Terminal message updates are published only after delivery succeeds. If detached
  resume acceptance fails, the endpoint returns an error and restores the still-current bundle to
  pending so the same answer can be retried. Restored rows publish after commit even when synchronous
  task-summary acknowledgement fails, preventing clients from retaining the terminal snapshot while the
  endpoint still returns the acknowledgement error. A publication or summary-convergence error after
  the database restore does not make that retry unsafe; durable pending state remains authoritative.
  Once agentctl accepts the prompt, later publication or completion errors cannot roll back the
  successor turn or reopen the answer.
- A current-turn bundle remains answerable while any sibling question is pending. Recovery claims only
  those pending rows, preserves siblings already made terminal by an earlier partial write, and restores
  only the claimed rows if detached delivery fails.
- Any response to a superseded or terminal bundle returns conflict, performs no message mutation, and
  initiates no agent resume. Current clients close their obsolete local overlay through the existing
  conflict handling.
- Persisted task status summaries reconcile `pending_action` against current-turn repository state on
  source events and task-list/boot reads. Existing summaries are repaired, not only missing rows.
- When a task row advertises a pending action, desktop and phone task activation load the task's
  sessions from the server and select the newest input-capable session whose `pending_action` matches
  the task action. Before applying that response, activation revalidates the task-summary revision and
  pending action. If either changes while the task remains present, activation discards the delayed
  session choice and opens the task-only route; phone activation also closes the sheet. Overlapping
  authoritative loads are generation-guarded per task, so an older response cannot replace a newer
  session snapshot. This pending owner outranks remembered-session and primary-session preferences. If
  the task still advertises pending input but no matching input-capable owner exists, activation releases
  the outgoing session layout and fails closed to the task route without guessing a session. Normal
  preference order returns only for a clean task. If the task projection disappears while that
  authoritative load is in flight, desktop and phone leave the selection inert instead of navigating to
  a deleted task, including tasks using the legacy pending-action projection. A forced load aborted by a
  newer load is also inert; it is not treated as a request failure requiring task-only fallback. Each
  mounted phone task sheet owns its selection generation, so simultaneous instances cannot invalidate
  one another's in-flight task choice.

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

No new route or response field.

- `GET /api/v1/tasks/:taskId/task-sessions` continues to expose each session's current derived
  `pending_action`; task navigation uses this existing field.
- `GET /api/v1/task-sessions/:sessionId/turns` continues to expose durable turn history;
  empty unattempted reservations are recovered at startup and excluded from turn authority, while
  attempted reservations are preserved because their dispatch outcome is ambiguous.
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
  `terminal` for that exact `pending_id`. A failed detached resume acceptance returns to
  `active_detached` while the same turn remains current only when dispatch is known not to have been
  accepted and rollback succeeds. A post-attempt crash or post-acceptance publication failure remains
  terminal because the prompt may already be running.
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
- A forced task-session list cannot project authoritative pending actions: fail the HTTP or WebSocket
  request instead of returning a successful list with empty pending ownership.
- Summary compare-and-set loses a race: reload the newer summary, reapply authoritative pending state,
  and retry within a bounded loop. Exhaustion is an error, so restored-state acknowledgement is withheld
  until the pending action is durably confirmed. Never overwrite unrelated newer summary fields.
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
  report whether retry state was recovered, and never restore it after a successor turn is accepted.
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
- **GIVEN** a detached answer reaches synchronous orchestrator resume, **WHEN** the orchestrator does
  not acknowledge it before the bounded deadline, **THEN** Kandev treats acceptance as failed and
  restores the still-current bundle.
- **GIVEN** startup cannot reconcile an unpublished prompt reservation, **WHEN** the orchestrator starts,
  **THEN** startup returns an error before watcher, scheduler, or prompt admission begins.
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

## Out of scope

- Rewriting or deleting historical pending message rows solely to clean old data.
- A dedicated clarification table or schema migration.
- Redesigning the desktop sidebar, phone task drawer, chat cards, or clarification carousel.
- Changing permission-request lifecycle semantics; pending-session navigation reuses the shared
  `pending_action` type for permission parity.
- Changing notification history for a clarification that was valid when its notification fired.
- Automatically choosing a non-primary session when the task has no current pending action.
