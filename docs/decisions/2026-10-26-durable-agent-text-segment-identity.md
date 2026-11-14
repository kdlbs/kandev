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
event's reconstructable lifecycle metadata. The dedicated
`agent_delivery_inbox.canonical_message_id` column is the authoritative
committed projection ID. The payload copy is ancillary; it is only a fallback
for older projected rows whose new column is empty while their payload remains
available. Projection returns the exact committed column value for duplicates.
The ID is projection-only metadata: transport identity and payload-hash
comparison exclude it, and duplicate admission must not overwrite it. Chunks
within a segment share the ID. Native protocol message IDs and reasoning
identities keep their current rules.

Payload compaction preserves `canonical_message_id` while replacing the payload
with its compact form. If an older projected row has an empty column and its
payload has already been pruned, duplicate projection returns an empty ID. It
does not infer one from incoming metadata or the current boundary, rewrite a
transcript row, or notify a missing row. Later whole-row receipt retirement
deletes the stored ID with the receipt and continues to reject replay below the
retired cursor with the existing expired-replay error.

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
- Cursor updates, message effects, resolved event identity, and the authoritative
  inbox ID must commit atomically. A failed projection advances none of them and
  exposes no tentative ID through caller-owned event data.
- Payload pruning retains the authoritative inbox ID. Whole-row receipt
  retirement removes it and preserves the existing expired-replay behavior.
- Projection-only IDs do not change immutable transport identity or hashes;
  duplicate admission preserves the committed inbox value.
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
