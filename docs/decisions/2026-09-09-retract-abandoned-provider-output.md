# ADR-2026-09-09-retract-abandoned-provider-output: Retract Output From Abandoned Provider Attempts

**Status:** accepted
**Date:** 2026-09-09
**Area:** backend, frontend, protocol

## Context

Some providers recover from a response-stream failure inside the active
`session/prompt` call. Codex, for example, emits a structured reconnect update
after partial assistant output and then streams a replacement under a new ACP
message ID. Kandev currently maps each protocol message ID to a separate
durable transcript row. Both rows therefore survive even though the provider
abandoned the first response.

The web client is behaving correctly when it renders both rows. Consecutive
assistant messages are valid, so presentation code cannot infer that similar
text is a duplicate. The ACP adapter understands provider metadata, lifecycle
owns protocol-to-Kandev message correlation, and the task service owns durable
message deletion and its live WebSocket projection.

## Decision

Kandev represents a provider-owned response retry as a provider-neutral,
prompt-generation-scoped `response_attempt_reset` boundary.

The active ACP dialect recognizes exact structured retry evidence. It does not
classify arbitrary prose, and downstream lifecycle, orchestration, and UI code
do not inspect provider names or raw error text. A retry boundary is separate
from a terminal provider error and does not authorize Kandev to replay a
prompt.

Lifecycle records the Kandev IDs of assistant and thinking rows first created
during the current response attempt. A top-level effect boundary commits those
rows and starts a new eligibility window. Before lifecycle publishes a reset,
it flushes pending streaming chunks, detaches the eligible IDs, clears their
protocol correlations, and discards the uncommitted assistant-history segment.
A row that existed before the current eligibility window is never deleted or
truncated, even if the provider later appends to the same protocol message ID.

Orchestration applies the reset under the existing per-session stream guard and
deletes each eligible row through the task service. Task-service deletion is
the durable mutation and publishes the existing `session.message.deleted`
event, so connected desktop and mobile clients remove the same rows that a
reload, search, or another viewer no longer reads.

A reset does not clear prompt-level output or effect evidence. That evidence
continues to fail a later Kandev-owned automatic replay closed. Empty and
repeated resets are idempotent. A deletion failure is logged and does not stop
the provider's replacement attempt.

## Consequences

Provider-owned retry output is removed consistently from storage and live chat
without hiding legitimate multi-message turns. Other providers can add an
adapter dialect recognizer while reusing the same lifecycle and task-service
path.

Lifecycle must retain a small ordered set of attempt-local record IDs and flush
its coalescer before the reset crosses the event bus. The current design does
not roll back an appended suffix on a row committed before the response
attempt; preserving committed content is safer than guessing a truncation
point. Providers that reuse one protocol message ID across an effect boundary
need a future protocol-level segment identity before such suffixes can be
retracted safely.

No message schema or frontend rendering change is required. Storage failures
can still leave an abandoned row, but the failure is observable and does not
interrupt the live provider turn.

## Alternatives Considered

- **Deduplicate similar consecutive messages in React.** Rejected because
  legitimate assistant messages can be similar, and hidden rows would remain
  in storage, search, reloads, and other viewers.
- **Overwrite the previous message whenever a new protocol ID appears.**
  Rejected because a new ID normally means a legitimate new message.
- **Mark abandoned rows as hidden.** Rejected because it introduces a new
  persistence state and leaves stale content available to non-rendering
  consumers when durable deletion already has a live projection.
- **Buffer all assistant output until the prompt completes.** Rejected because
  it removes live streaming and makes long-running turns appear stalled.
- **Delete messages directly from provider-specific adapter code.** Rejected
  because adapters do not own Kandev record IDs, persistence, or client event
  delivery.
