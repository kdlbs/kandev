# ADR-2026-10-29-coalesced-delivery-durability-barrier: Gate journal release on a coalesced SQLite durability barrier

**Status:** accepted
**Date:** 2026-10-29
**Area:** backend

## Context

AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1 requires that agentctl may discard
journal data only after the backend's receipt and applied effects survive a host
power loss. PR 4425 met this by opening the shared SQLite writer with WAL
`synchronous=FULL`, which synchronizes the WAL on every commit of every caller.

The SQLite backend has one writer connection. Durable delivery also commits one
receive transaction per streamed event, plus one projection transaction per
batch. `BenchmarkDeliveryWriterContention` shows that with several sessions
streaming, the writer cannot sustain the offered event rate, and ordinary task
writes see p95 latency above 100 ms on the diagnostic host. Users observe a
slow UI when creating, archiving, deleting, or prompting tasks.

Only journal release depends on power-loss durability. Interactive writes ran
with NORMAL before PR 4425 and were never part of that guarantee.

## Decision

- The writer DSN returns to WAL `synchronous=NORMAL`.
- `internal/db.DurabilityBarrier` provides `Await(ctx)`: it returns after a
  write committed on the actual writer connection with `synchronous=FULL` and
  started after the call. A FULL commit synchronizes the whole WAL file, which
  covers every earlier commit. Concurrent callers share one run, with a
  coalescing window of at most 10 ms. A caller can stop waiting when its own
  context ends; the shared run continues in a short-lived goroutine.
- Every backend request that lets agentctl release journal data, namely event
  acknowledgment and submission retirement, goes through one lifecycle releaser
  that awaits the barrier first. A structural test prevents new bypasses.
- Durable delivery commits receipt and projection of a compatible batch in one
  transaction, which reduces commits independently of the barrier.
- PostgreSQL keeps synchronous commit and the barrier is a no-op.

The design is [delivery write cost](../specs/platform/system-design/durable-delivery-write-cost.md).

## Consequences

- fsync frequency scales with barrier runs (bounded by the coalescing window),
  not with commits. Interactive writes stop paying per-commit synchronization.
- ACK latency increases by at most the window plus one fsync. Journal growth
  between ACKs is bounded by the existing ACK cadence and journal limits.
- Writes that no journal release depends on can lose their most recent commits
  on host power loss, as before PR 4425. Process termination still loses
  nothing that committed.
- A new journal-release request type must use the releaser. The structural test
  enforces this for the current client methods.
- The barrier writes one `kandev_meta` key per run, adding a small WAL frame.

## Alternatives considered

- **Keep FULL on every commit.** Correct, but this is the measured cause of
  writer saturation. Rejected.
- **Batch commits only, keep FULL.** Reduces commits roughly threefold. The
  benchmark indicates the writer still cannot keep up with eight active
  sessions. Kept as a complement, not a replacement.
- **`PRAGMA wal_checkpoint(PASSIVE)` as the barrier.** A checkpoint synchronizes
  the WAL only when frames can be backfilled. A reader pinning an old snapshot
  can make it skip synchronization, so it cannot prove durability. Rejected.
- **fsync the `-wal` file through a separate descriptor.** Works on Linux and
  macOS, but it depends on SQLite's file layout and needs a write handle on
  Windows. Rejected.
- **Acknowledge from the inbox only, before projection.** Requires a separate
  projector recovery proof, which stream processing explicitly defers.
  Out of scope.

## References

- [Delivery write cost requirements](../specs/platform/requirements/durable-delivery-write-cost.md), REQ-PLATFORM-DELIVERY-WRITE-COST-001
- [Pruned delivery recovery requirements](../specs/platform/requirements/pruned-delivery-recovery.md), AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1
- [SQLite writer transaction admission](2026-10-05-sqlite-writer-transaction-admission.md)
- [Delivery writer contention plan](../plans/delivery-writer-contention/plan.md)
