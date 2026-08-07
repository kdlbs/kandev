# ADR-2026-08-02-isolate-replaceable-session-stream-traffic: Isolate Replaceable Session Stream Traffic

**Status:** accepted
**Date:** 2026-08-02
**Area:** backend, frontend, protocol

## Context

The bounded task-status design separated correlated WebSocket responses from
best-effort notifications and removed task-switcher subscriptions to inactive
session details. It did not bound the work produced by one actively subscribed
session.

Incident task `69637afd-c193-4197-aa49-348ac2b0cfb3` exposed that remaining
failure mode. Its ACP execution emitted 28,967 unique `thinking_streaming`
events and 31,314 total stream events over about 41 minutes, peaking at 63
reasoning chunks per second. Each tiny reasoning chunk caused an event-bus
publication, a read/concatenate/full-message database update, a full growing
`session.message.updated` payload, one shared WebSocket notification slot, and
one Zustand/React update. The notification queue overflowed briefly and
dropped five message updates. More importantly, continuous serialization and
browser state churn made the frontend unusable until the backend stopped.

The same capture showed an independent amplifier in the Office task detail:
refetches replaced session objects, causing unchanged multi-session
subscriptions to be torn down and recreated. Some connections issued more than
100 subscribe/unsubscribe operations and snapshots in seconds.

The backend remained running and correlated responses already had a separate
control queue. Therefore this was not a backend crash or response-priority
failure. It was unbounded replaceable work across the ingress, delivery, and
render boundaries.

## Decision

Kandev will treat agent text and reasoning chunks as a high-frequency ingress
format rather than a persistence or render contract.

The lifecycle manager will coalesce adjacent chunks for the same execution,
stream kind, and Kandev message record. The first non-empty chunk creates the record
immediately; later chunks flush no more than once per 100 ms window during
continuous streaming. Semantic boundaries and teardown flush pending content
before they proceed. Coalescing is lossless: final persisted assistant and
reasoning content is byte-for-byte equivalent to the ordered input chunks.

The per-agent ACP adapter/process handoff will apply cancellation-aware
backpressure when its bounded event channel fills. Normalized assistant and
reasoning chunks are not silently discarded before lifecycle coalescing; only a
terminal shutdown may cancel an outstanding send. Because each handoff belongs
to one agent instance, a noisy session slows its own producer rather than
blocking unrelated backend sessions.

The WebSocket gateway will distinguish semantic notifications from
replaceable full-state stream notifications. `session.message.updated` is
replaceable by `(session_id, message_id)` because every payload contains the
complete current public message state. Each client gets bounded per-session
replaceable capacity. A newer update replaces the queued payload in place, and
session queues are drained fairly. Semantic notifications and correlated
control traffic have independent bounded capacity, so a noisy session cannot
occupy their slots. Overload may evict the oldest queued replaceable entry from
the offending session, including an entry that has not yet been superseded.
Because the database remains authoritative, reconnect, snapshot, and
turn-settle reconciliation repair any intermediate replacement that was not
delivered; authoritative persistence is never discarded.

The frontend will coalesce replacement updates once more at the animation-frame
boundary and will key intentional multi-session subscriptions by stable session
identity. Add/delete/turn-settle handling remains ordered so frame batching
cannot resurrect a deleted message or hide a terminal state.

Each layer will expose content-free counters and structured context for
received chunks, coalesced chunks, flushes, queued replacements, replacements,
and per-session evictions. Tests use small injected capacities and fake time;
production constants remain implementation details so they can be tuned
without changing the protocol.

## Consequences

A 63-chunk-per-second agent is reduced to at most ten append publications per
second for a continuously streaming message before gateway replacement and
browser-frame batching. Database writes, JSON size amplification, notification
slots, and React commits are bounded independently. Other sessions and
semantic state continue to move even if the noisy session remains active.

The transcript remains authoritative and complete, but an observer may skip
intermediate visual states and jump directly to a newer full-message
replacement. This is acceptable because streaming updates are progressive
render hints, not individual durable occurrences. Reconnect and existing
session reconciliation repair missed intermediate delivery.

The gateway becomes a small typed scheduler instead of a single notification
FIFO. Ordering tests must cover replacement-in-place, semantic barriers,
fairness, teardown, and zero-value clients. Lifecycle timers add teardown and
race obligations, including execution replacement and backend shutdown.

No new mobile layout or user-facing copy is introduced. Desktop and mobile use
the same store and delivery behavior.

## Alternatives Considered

- **Increase the 256-frame notification queue.** Rejected because it converts
  immediate drops into a larger stale backlog, consumes more memory, and leaves
  database and browser work unbounded.
- **Rate-limit or terminate the ACP process.** Rejected as the primary repair
  because agents may legitimately emit long reasoning streams, and killing the
  producer loses useful work instead of removing redundant intermediate work.
- **Drop normalized events when the per-agent handoff queue is full.** Rejected
  because it makes the supposedly lossless lifecycle coalescer observe an
  incomplete stream. Cancellation-aware backpressure preserves transcript
  content while keeping the pressure scoped to the producing agent instance.
- **Coalesce only in the browser.** Rejected because the event bus, database,
  JSON encoding, and WebSocket queue would still process every chunk.
- **Coalesce only in the lifecycle manager.** Rejected because other producers,
  older backends during rollout, or bursty full-state updates could still
  pressure a client; each shared boundary needs its own containment.
- **Drop all reasoning updates.** Rejected because live reasoning is useful and
  persisted transcripts must remain complete.
- **Give every session a separate WebSocket connection.** Rejected because it
  multiplies connection lifecycle and authorization complexity while bounded
  per-session scheduling provides isolation on the existing transport.
