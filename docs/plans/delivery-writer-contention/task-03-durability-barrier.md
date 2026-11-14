---
id: "03-durability-barrier"
title: "Gate journal release on a coalesced durability barrier"
status: in_progress
wave: 3
depends_on:
  - "02-combined-receipt-projection"
plan: plan.md
requirements:
  - REQ-PLATFORM-DELIVERY-WRITE-COST-001
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-WRITE-COST-001.1
  - AC-PLATFORM-DELIVERY-WRITE-COST-001.3
  - AC-PLATFORM-DELIVERY-WRITE-COST-001.4
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.4
system_design:
  - ../../specs/platform/system-design/durable-delivery-write-cost.md
  - ../../specs/platform/system-design/pruned-delivery-recovery.md
---

# Task 03: Gate journal release on a coalesced durability barrier

## Summary

Return the writer to WAL `synchronous=NORMAL` and move power-loss durability to
a coalesced barrier awaited before every journal release. Land both in this
work order so no build has NORMAL without the gate.

## In scope

- `internal/db.DurabilityBarrier`: `Await(ctx)`, the writer `DB.Conn`
  sequence (FULL, `kandev_meta` upsert, NORMAL), a coalescing window of at most
  10 ms, discard on failure, and a no-op for PostgreSQL.
- `OpenSQLite` DSN back to `_synchronous=NORMAL`, with its comment updated.
  Replace `TestSQLiteWriterPowerLossDurability`.
- `Repository.AwaitAgentDeliveryDurability`, constructed over the shared writer.
- Lifecycle `deliveryJournalReleaser`, wired at the five call sites listed in
  the design. ACK worker barrier failure uses a new `durability_barrier` error
  category and keeps pending progress.
- Structural test `TestNoDirectJournalReleaseCalls`.
- Metrics `agent_delivery_durability_barrier_total{result}` and
  `agent_delivery_durability_barrier_seconds`. Add a line to the root
  `AGENTS.md` Observability section.
- Extend the benchmark with the final variant (combined, NORMAL, simulated
  20 ms ACK barrier per stream).

## Out of scope

- Changes to journal internals or the agentctl protocol.
- Power-cut testing on real hardware.

## Acceptance

- `TestDurabilityBarrierCommitsWithFull` observes `PRAGMA synchronous` = 2
  inside the run and 1 afterwards, plus a changed `kandev_meta` generation.
  `TestDurabilityBarrierCoalescesWaiters` shows concurrent waiters sharing one
  run, and a waiter that arrives mid-run waits for the next one.
  `TestDurabilityBarrierFailureDiscardsConnection` shows the reopened
  connection at NORMAL.
- `TestJournalReleaseAwaitsDurability` and
  `TestJournalReleaseBarrierFailureRetainsProgress` cover ACK, duplicate ACK,
  retention ACK, and both retire paths. `TestNoDirectJournalReleaseCalls` fails
  if any direct client call is reintroduced.
- One benchmark run meets AC-PLATFORM-DELIVERY-WRITE-COST-001.1 (final variant against `baseline`, eight
  sessions). On Linux,
  `strace -f -c -e trace=fsync,fdatasync` over the final variant shows fsync
  calls scaling with barrier runs rather than commits. Record both in Results.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/db -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(JournalRelease|NoDirectJournalReleaseCalls|DurableDeliveryAck|DeliveryAck)' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'Test(AgentDelivery|AwaitAgentDeliveryDurability)' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go test -trimpath -run '^$' -bench 'BenchmarkDeliveryWriterContention' -benchtime=100x -count=3 ./internal/task/repository/sqlite)
bash scripts/lint-go-changed <changed-go-files>
```

## Files likely touched

- `apps/backend/internal/db/sqlite.go`
- `apps/backend/internal/db/durability_barrier.go` (new)
- `apps/backend/internal/db/durability_barrier_test.go` (new)
- `apps/backend/internal/db/sqlite_test.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_durability.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/delivery_journal_releaser.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/delivery_journal_releaser_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_ack.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/agent_delivery_retention.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_delivery.go`
- `apps/backend/internal/agent/runtime/lifecycle/interrupted_submission_retirement.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_writer_contention_bench_test.go`
- `AGENTS.md`

## Dependencies

WO-02.

## Risks

The barrier briefly holds the only writer connection for one synchronized
commit. Keep the window short and never hold it across other I/O. If the strace
check does not show WAL synchronization on the FULL commit, stop and revisit the
design.

## Results

Status: in progress. The barrier and release-gate implementation is complete,
but this work order's acceptance condition is not met. The recorded run's final
p95 was 58.68 ms against the baseline's 81.25 ms, about 72% against a 25% bound,
so AC-PLATFORM-DELIVERY-WRITE-COST-001.1's latency bound is **unverified**. The
rate bound (759.8 of 800.1 offered events/s, 95.0%) is recorded in
[the plan](plan.md#verification-results). A quiet-host rerun of this benchmark
version must demonstrate both bounds in one run before this work order can be
marked done.

- `internal/db.DurabilityBarrier` commits one FULL `kandev_meta` generation on
  the actual writer connection, coalesces callers for at most 10 ms, discards
  the connection on failure, and runs each coalesced commit in a short-lived
  goroutine so a canceled caller stops waiting while the shared run continues.
- `internal/db.OpenSQLite` uses `_synchronous=NORMAL`.
  `TestSQLiteWriterUsesNormalWithBarrier` replaces the old FULL assertion;
  `TestDurabilityBarrierCommitsWithFull`,
  `TestDurabilityBarrierCoalescesWaiters`, and
  `TestDurabilityBarrierFailureDiscardsConnection` pass under `-race`.
- `Repository.AwaitAgentDeliveryDurability` exposes the barrier and publishes
  `agent_delivery_durability_barrier_total{result}` and
  `agent_delivery_durability_barrier_seconds`.
- `deliveryJournalReleaser` awaits the barrier before the ACK worker, duplicate
  ACK, retention ACK, and both retire paths. A barrier failure keeps ACK
  progress pending and reports the new `durability_barrier` category.
  `TestNoDirectJournalReleaseCalls` rejects a reintroduced direct client call.
- The final benchmark variant delivered 759.8 of 800.1 offered events/s (95.0%),
  meeting the rate bound only. The `strace` run recorded 172 fsync calls
  against 93 for the combined variant that has no barrier. Both are far below
  one fsync per commit. The strace and benchmark host used SQLite 3.51.1
  (go-sqlite3 v1.14.33). The full table and the unverified p95 bound are in
  [the plan](plan.md#verification-results).
- `go test -race ./internal/persistence/storeconformance` passes.

