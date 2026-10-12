# ADR-2026-10-10-delivery-event-retention-barriers: Scope delivery barriers to immutable event streams

**Status:** accepted
**Date:** 2026-10-10
**Area:** backend, protocol

## Context

Retention and foreground delivery currently take an exclusive PostgreSQL session barrier, including operations on different streams.
The measured active-cleanup latency exceeds the reviewed target. Smaller leases and fewer lock commands have not met that target.
Event compaction changes an immutable stream receipt. Submission, terminal, workflow and recovery evidence can affect several streams.
Canonical projection also accepts supplied effects and can derive terminal settlement receipts. Its event stream alone cannot bound every mutation.

## Decision

Use a shared delivery-session barrier only for nonterminal event receive and strictly scoped generic canonical projection.
Canonical projection qualifies only when every supplied effect is completed, timestamped and matches its event's deterministic generic key and stream/sequence.
Any supplied ownership fields must be empty or match that event. Terminal events and every other effect shape retain exclusive locking.
Keep the existing exclusive turn admission and exclusive stream barriers in both paths.

Event-only retention takes shared session barriers and exclusive stream barriers. Every selected prompt's session takes an exclusive barrier.
Combine mixed session modes in global session order before sorted stream locks. Never upgrade a shared barrier within that transaction.
Owner, generation, submission, terminal, workflow, recovery, deletion, retirement and recreation mutations keep exclusive session barriers.
Future mutations that change a retention proof must participate in that exclusive barrier.

Lock commands read no authority. Following statements reload current proofs using fresh READ COMMITTED snapshots after all locks return.
SQLite retains its existing immediate writer transaction and separate WAL readers. Hashes, checkpoints, budgets and dispatch authority remain unchanged.
Require actual PostgreSQL barriers for concurrency, retained exclusions, unsafe-effect fallback, mixed modes, cancellation and fresh owner reads.
Those checks passed with race detection. This decision supplies no passing latency claim.

## Consequences

Different immutable streams can make progress together while proof-changing writers remain excluded.
Same-stream replay still waits for compaction and then observes its committed hash or retirement checkpoint.
Flexible projection arguments retain their existing exclusive path. New effect types cannot silently gain shared access.
The lock policy adds an explicit classification that future delivery writers must preserve.
A compatible older binary's exclusive barriers remain safe with the new shared barriers.

## Alternatives Considered

- Keep exclusive session barriers everywhere: simple, but serializes independent streams and leaves the measured latency gate unmet.
- Share every canonical projection: unsafe because supplied effects and derived terminal receipts can change another stream's dependencies.
- Share prompt or authority mutations: invalidates proofs used by concurrent retention.
- Cache authority across transactions: can admit an obsolete owner and breaks authenticated replay.
- Read authority in the waiting lock command: uses the command's older snapshot after an owner replacement.

## Related records

- [Retention requirements](../specs/platform/requirements/backend-agent-delivery-retention.md), especially REQ-PLATFORM-DELIVERY-RETENTION-003 and REQ-PLATFORM-DELIVERY-RETENTION-005.
- [Retention design](../specs/platform/system-design/backend-agent-delivery-retention.md).
- [Checkpoint decision](2026-10-05-backend-delivery-retirement-checkpoints.md).
- [Integration work order](../plans/backend-agent-delivery-retention/task-08-integration-and-storage.md).
