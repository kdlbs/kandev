# ADR-2026-09-01-guard-agent-queue-disposition: Guard Agent Queue Disposition With Exact Snapshot Claims

**Status:** accepted
**Date:** 2026-09-01
**Area:** backend, protocol, security, workflow

## Context

A long-running Coordinator can become blocked by repeated scheduled wakes or
stale pending messages. Existing user queue controls authorize an interactive
session owner, but agents had no narrow way to inventory or dispose of exact
entries. Operational workarounds based on SQL, unrestricted message reads, or
broad cancellation would bypass the queue's session locks and authorization.

An entry ID alone is insufficient for safe delayed disposal because
coalesce-replace keeps the ID and FIFO position while replacing the stored
snapshot. A caller that saw the old version must not remove the replacement.

## Decision

Expose two task MCP tools bound to the calling task, session, and workspace:

- a content-free FIFO census that returns immutable IDs and opaque complete
  snapshot claims;
- exact disposition that accepts only returned `(id, claim)` pairs and reports
  `removed`, `changed`, or `not_found` with atomic before and after counts.

The backend principal, not tool arguments, supplies the target scope. Exact
comparison and deletion occur under the same per-session repository lock and
transaction used by delivery and queue mutation. Reserved in-flight lifecycle
rows remain outside the disposable set.

Scheduled automation messages may coalesce only when a trusted automation
principal, stable automation and trigger identity, target session, and complete
expanded payload are identical. The retained row keeps its entry ID and FIFO
position. Every other message remains distinct.

## Consequences

- Coordinators can recover from stale pending work without body disclosure,
  clear-all authority, or database access.
- Snapshot claims prevent an old census from deleting a coalesced replacement.
- Concurrent and repeated disposition is idempotent in effect and exposes the
  winner through per-entry outcomes.
- Identical scheduled wakes do not consume capacity repeatedly, including
  after restart, while peer, human, event, and different scheduled messages
  retain lossless FIFO behavior.
- The contract adds no schema migration because queue metadata and immutable
  IDs are already persisted.

## Alternatives Considered

1. **Authorize removal by entry ID alone.** Rejected because coalescing can
   replace the row snapshot while retaining its ID.
2. **Expose queued message bodies to help an agent choose.** Rejected because
   it creates a broad data-read surface unnecessary for exact disposal.
3. **Provide clear-all, sender filters, or text matching.** Rejected because
   concurrent new or distinct messages could be over-removed.
4. **Use operator SQL or a Support relay.** Rejected because those paths bypass
   product authorization, queue locks, and auditable outcomes.
5. **Coalesce every identical message.** Rejected because peers or humans can
   intentionally send repeated text; only trusted scheduled automation carries
   the required routine identity.
