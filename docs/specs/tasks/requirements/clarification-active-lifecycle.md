---
status: active
system: tasks
created: 2026-08-14
owners:
  - kandev
---


# Active clarification lifecycle Requirements



## Overview



Clarification bundles remain answerable only for the session turn that owns them, while detached and recovery paths preserve the response contract.



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
- After a session successfully enters a terminal state, its current-turn pending clarification bundles
  expire so no answerable overlay survives a completed, failed, or cancelled session.
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
  preserves the successor and keeps the claimed bundle terminal. Acknowledgement publishes a
  recovery-clean `turn.started` payload while the recovery metadata remains durable, then clears that
  metadata after the event bus accepts the event. HTTP success requires both operations. A rejected
  event publication therefore remains discoverable by startup reconciliation. Recovery atomically
  replaces an accepted or ambiguous reservation with a durable start-event outbox marker. Before
  admitting work, startup replays `turn.started`, followed by `turn.completed` when the turn is already
  terminal, and clears the marker only after the event bus accepts every required event. Failed replay
  fails startup and leaves the marker for the next attempt. Every public turn event, including a
  completion racing live cleanup or emitted during recovery, strips the private prompt-dispatch fields.
  If the predecessor's delayed ready event overlaps this private reservation, ready handling waits for the
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
- Every response atomically claims current-turn ownership and persists a response-delivery recovery
  intent before it can reach a live waiter or request a detached resume. A live waiter runs durable
  delivery confirmation before returning the response to the agent; enqueue alone does not retire the
  intent. Once confirmation starts, it owns its durable operation through completion even if the
  responder's bounded wait expires. Its input claim remains immutable, its result remains local to the
  callback, and any compensating restore serializes against finalization so only one durable outcome
  wins. The detached path retires the intent only at its durable resume boundary. Startup first
  reconciles prompt reservations, then restores an unhanded current-turn claim to pending; a terminal
  session or newer authoritative turn instead retires the stale intent without reactivating history.
  Terminal message updates are published only after delivery succeeds. If detached
  resume acceptance fails, the endpoint returns an error and restores the still-current bundle to
  pending so the same answer can be retried. Restored rows publish after commit even when synchronous
  task-summary acknowledgement fails, preventing clients from retaining the terminal snapshot while the
  endpoint still returns the acknowledgement error. A publication or summary-convergence error after
  the database restore does not make that retry unsafe; durable pending state remains authoritative.
  Once agentctl accepts the prompt, later publication or completion errors cannot roll back the
  successor turn or reopen the answer. A primary-answer watchdog is observable before a confirmed live
  waiter returns the response to the agent, so acknowledgement activity cannot occur before the
  watchdog can observe it. The resolver receives a synchronous local notifier at construction time for
  this ordering boundary; event-bus fan-out, including NATS publication, is not an acknowledgement. The
  watchdog carries the clarification turn ID and revalidates that ID both
  before fallback and inside serialized prompt admission, so it cannot dispatch a stale answer into a
  successor turn. Its fallback keeps the watchdog cancellation context through authority reads and
  prompt admission so independent session activity or service shutdown interrupts in-flight recovery
  work. Activity emitted by the fallback's own silent cancellation remains part of that recovery and
  cannot cancel its context before the answer reaches the replacement handoff. Recovery may exempt only
  a frame with the captured execution and prompt-generation identity and a cancellation-acknowledgement
  type. Message, thinking, and tool frames remain authoritative activity even when their identity
  matches the cancelled prompt.
- A current-turn bundle remains answerable while any sibling question is pending. Recovery claims only
  those pending rows, preserves siblings already made terminal by an earlier partial write, and restores
  only the claimed rows if detached delivery fails. Primary delivery events and detached recovery derive
  one turn identity from the same bundle rule: legacy empty turn IDs do not mask a consistent non-empty
  identity, while conflicting non-empty IDs invalidate the identity.
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

## Requirements



### REQ-TASKS-CLARIFICATION-LIFECYCLE-001: Active clarification lifecycle



**Intent:** Clarification bundles remain answerable only for the session turn that owns them, while detached and recovery paths preserve the response contract.



#### Acceptance criteria



- **AC-TASKS-CLARIFICATION-LIFECYCLE-001.1:** When a clarification is pending, the system shall expose and process only the bundle owned by the session's current turn.
- **AC-TASKS-CLARIFICATION-LIFECYCLE-001.2:** When a newer turn or terminal session state supersedes a bundle, the system shall keep its transcript history without leaving it answerable or able to block workflow progress.
- **AC-TASKS-CLARIFICATION-LIFECYCLE-001.3:** When detached clarification recovery succeeds or fails, the system shall return the bounded response and durable recovery outcome defined by the lifecycle contract.



## Out of scope



Clarification presentation details belong to the UI system; agent identity and provider runtime behavior belong to the agent system.
