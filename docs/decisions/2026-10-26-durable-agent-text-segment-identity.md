# ADR-2026-10-26-durable-agent-text-segment-identity: Persist durable text segment boundaries

**Status:** accepted
**Date:** 2026-10-26
**Area:** backend

## Context

The durable projector gives ID-less assistant text chunks the same canonical
message ID while they share a turn. The legacy streaming path starts a new
message after a top-level tool call, but durable projection keeps appending to
the old row. Flushing a batch at the tool call does not change the canonical ID.

The segment boundary must survive duplicate delivery, replay, batching, partial
projection, reconnect, and either process restarting. Existing projected
messages and retained inbox events can outlive the backend process. The backend
SQL cursor owns canonical projection checkpoints; agentctl owns transport
sequence and replay state.

## Decision

The backend SQL delivery cursor owns the latest committed top-level tool-call
sequence for each stream. Projecting a top-level tool call updates this
checkpoint in the same transaction as its effect and the projected cursor.
Nested tool calls and tool updates do not advance it.
Every route that projects a top-level tool start, including normal event
projection, retained replay, and canonical shell-start projection, updates the
checkpoint. Mixed batches apply boundaries in event sequence order.

For ID-less assistant text, canonical projection derives a stable message ID
from a stable base identity and the committed boundary sequence. The base and
resolved IDs remain distinct: retry and reconnect derive only from the base,
and an already-resolved ID is never hashed again. Projection does not mutate
caller-owned event payloads before commit. It persists the resolved ID with the
event's reconstructable lifecycle metadata and returns the exact committed ID
for notification, including on duplicate projection. Chunks within a segment
share the ID. Native protocol message IDs and reasoning identities keep their
current rules. Persist the resolved ID in a dedicated inbox column as well as
the reconstructable event payload so retention pruning cannot erase the value
needed by a duplicate projection result.

An additive nullable SQL column holds the boundary for existing streams; new
streams start at zero. On first projection after upgrade, a NULL value is
initialized once from the then-current projected sequence, or from the current
event sequence when that event is a top-level tool call. This conservative
baseline prevents new output from appending to an older projected row when
prior tool boundaries are no longer retained. Reopen and owner/generation/cursor
updates preserve a non-NULL boundary. This can start a fresh segment after
upgrade, but does not rewrite existing transcript rows. Retained, unprojected
events continue through ordered projection and keep their resolved identity
once projection commits.

## Consequences

- Segment identity has one durable recovery authority and does not depend on an
  in-memory counter or the currently connected agentctl process.
- Cursor updates, message effects, resolved event identity, and the dedicated
  inbox ID must commit atomically. A failed projection advances none of them.
- The cursor schema changes additively, and both SQLite and PostgreSQL must
  preserve the same migration and projection semantics.
- The first new text after upgrade can begin a new row even when no retained
  evidence identifies the historical segment. Existing rows remain unchanged.

## Alternatives Considered

### Process-local segment counter

A counter is easy to update at each tool call, but restart can lose it and
replay can assign a different message ID. It does not satisfy durable identity.

### Agentctl journal owns the text segment

This would store transcript projection state beside transport events. It would
duplicate backend conversation authority and require a journal format and
recovery contract change. The backend cursor already owns the projected
sequence and canonical transcript effects.

### Create one message per text chunk

This avoids merging across tool calls but changes normal chunk coalescing and
creates excessive transcript rows. It does not preserve existing rendering
semantics.

### Rewrite historical transcript rows

Splitting existing rows requires reconstructing boundaries that may have been
pruned and mutating conversation history. New projection boundaries solve the
regression without changing stored transcripts.
