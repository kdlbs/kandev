---
created: 2026-10-29
status: in_progress
requirements:
  - REQ-PLATFORM-DELIVERY-WRITE-COST-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/durable-delivery-write-cost.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/pruned-delivery-recovery.md
legacy_specs: []
---

# Implementation plan: Delivery writer contention

## Overview

Stop durable agent delivery from saturating the single SQLite writer. Commit
each compatible output batch's receipt and projection together, return the
writer to WAL `synchronous=NORMAL`, and gate every journal release on a
coalesced durability barrier so the power-loss guarantee is kept. Three
sequential work orders run in the original session. No delegation or Kandev
task/session is authorized.

## Evidence and root cause

User report: with several tasks running, creating, switching, archiving,
deleting, and prompting tasks feels slow.

Root cause, confirmed by code and benchmark:

1. `internal/db.OpenSQLite` uses one writer connection. PR 4425
   (`179fc69cc68`) changed it from WAL `synchronous=NORMAL` to `FULL`, so every
   commit by every caller synchronizes the WAL (about 4.5 ms per commit on the
   diagnostic host).
2. `StreamManager.prepareAgentEvent` commits each durable event through
   `Repository.ReceiveAgentDeliveryEvent` before the stream worker commits the
   batched projection. The result is about 1.5 synchronized commits per streamed
   event.
3. Durable delivery is negotiated for every launch, including local
   executors. `features.agentSurvival` controls only process survival, so no
   flag disables this cost.

Reproduction: `BenchmarkDeliveryWriterContention` (draft, uncommitted, in
`apps/backend/internal/task/repository/sqlite/agent_delivery_writer_contention_bench_test.go`).
With FULL, four sessions offering 400 events/s sustained 57 to 111 events/s,
and `CreateTask` p95 was 58 to 134 ms against 9 ms idle. With NORMAL, the same
commit pattern sustained 450 to 500 events/s with p95 5 to 13 ms. The full
table is in the [design](../../specs/platform/system-design/durable-delivery-write-cost.md#measured-baseline).

Classification: the intended behavior was missing from requirements. This
package adds REQ-PLATFORM-DELIVERY-WRITE-COST-001 and keeps
AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1 unchanged. The design change is
recorded in [ADR 2026-10-29](../../decisions/2026-10-29-coalesced-delivery-durability-barrier.md).

## Scope

### In scope

- The contention benchmark, with a `baseline` variant that preserves the
  pre-change pattern for AC-PLATFORM-DELIVERY-WRITE-COST-001.1 comparisons.
- The combined receipt-and-projection transaction for batched output events.
- `internal/db.DurabilityBarrier`, the NORMAL writer DSN, the lifecycle journal
  releaser, its structural guard, and barrier metrics.
- Replacing `TestSQLiteWriterPowerLossDurability`, which currently asserts
  FULL, with barrier-based durability tests.

### Out of scope

- Combining receipt for tool-call, shell-output, terminal, or other unbatched
  events.
- Disabling the journal for local executors, or tying it to
  `features.agentSurvival`.
- Throttling `refreshAgentDeliveryLag`.
- Multiple writer connections or write prioritization.
- Inbox-only acknowledgment.
- Host power-cut experiments and local E2E runs.

## Technical approach

Follow the [delivery write cost design](../../specs/platform/system-design/durable-delivery-write-cost.md).
The order matters: the NORMAL DSN must not land before every release site
awaits the barrier, so WO-03 changes both together.

## Tests

| Test | Work order | Fails before the fix because |
| --- | --- | --- |
| `BenchmarkDeliveryWriterContention` | 01 | Evidence only; records baseline |
| `TestReceiveAndProjectCanonicalBatchAtomic` | 02 | Method does not exist; receipt commits separately |
| `TestStreamProcessorBatchUsesOneWriterCommit` | 02 | Processor commits receipt per event |
| `TestCombinedBatchAdmissionFailureFallsBack` | 02 | No fallback path exists |
| `TestCombinedBatchDuplicateDoesNotNotify` | 02 | Combined duplicates notified |
| `TestAlreadyProjectedSingleEventStillNotifies` | 02 | The global check suppressed the existing path |
| `TestDurabilityBarrierCommitsWithFull` | 03 | Barrier does not exist |
| `TestDurabilityBarrierCoalescesWaiters` | 03 | Barrier does not exist |
| `TestDurabilityBarrierFailureDiscardsConnection` | 03 | Barrier does not exist |
| `TestSQLiteWriterUsesNormalWithBarrier` | 03 | Replaces the FULL assertion |
| `TestJournalReleaseAwaitsDurability` | 03 | ACK and retire calls do not await a barrier |
| `TestJournalReleaseBarrierFailureRetainsProgress` | 03 | No barrier failure path |
| `TestNoDirectJournalReleaseCalls` | 03 | Five direct call sites exist |

Existing ACK ordering, coalescing, owner-replacement, replay expiry, segment
identity, and PostgreSQL parity suites must stay green.

## E2E tests

None added. This change has no new UI. AC-PLATFORM-DELIVERY-WRITE-COST-001.1 is evaluated by the
benchmark; its latency bound remains unverified (see Verification results).
Existing CI E2E suites cover streaming and recovery behavior.

## Work orders

| ID | Work order | Depends on | Status |
| --- | --- | --- | --- |
| 01 | [Contention benchmark](task-01-contention-benchmark.md) | none | done |
| 02 | [Combined receipt and projection](task-02-combined-receipt-projection.md) | 01 | done |
| 03 | [Durability barrier and release gate](task-03-durability-barrier.md) | 02 | in_progress |

Execution is sequential. A wave does not authorize delegation.

## Verification results

Implementation is complete; verification is partially complete. All commands
below ran on the shared diagnostic host with external load average between 12
and 30, which makes absolute benchmark numbers noisy.

AC-PLATFORM-DELIVERY-WRITE-COST-001.1 is **not yet satisfied**: the recorded run
meets its rate bound but not its p95 bound. WO-03 remains in progress until a
quiet-host run demonstrates both bounds in one run.

- `go build -trimpath ./...` passes.
- `go test -trimpath -tags fts5 ./internal/db` passes.
- `go test -trimpath -tags fts5 -timeout 30m ./internal/task/repository/sqlite`
  passes (735.8 s).
- `go test -trimpath -tags fts5 -timeout 30m ./internal/agent/runtime/lifecycle`
  passes (181.4 s).
- Race runs for the new barrier, combined-batch, and releaser tests pass.
- `go run ./cmd/sqlguard ./internal`, `go test -race
  ./internal/persistence/storeconformance`, `bash scripts/lint-go-changed`, and
  the documentation validators pass.

Benchmark evidence from one `-benchtime=100x` run that contained the baseline
and final variants. `delivered/s` counts projected events during the timed
window; `offered/s` counts producer ticks.

| Variant | Sessions | p50 | p95 | p99 | delivered/s (offered/s) | delivery |
| --- | --- | --- | --- | --- | --- | --- |
| baseline | 0 | 2.40 ms | 4.27 ms | 5.82 ms | n/a | n/a |
| baseline | 4 | 11.49 ms | 47.29 ms | 74.84 ms | 173.9 (400.0) | 43.5% |
| baseline | 8 | 23.82 ms | 81.25 ms | 99.26 ms | 166.5 (799.2) | 20.8% |
| final | 0 | 0.41 ms | 0.64 ms | 2.49 ms | n/a | n/a |
| final | 4 | 1.69 ms | 12.19 ms | 14.90 ms | 399.2 (399.2) | 100.0% |
| final | 8 | 12.13 ms | 58.68 ms | 95.82 ms | 759.8 (800.1) | 95.0% |

The final variant met the rate bound in that run (759.8 of 800.1 offered
events/s). Its p95 was 58.68 ms against the baseline's 81.25 ms, about 72% of
the baseline against a 25% bound, so the latency bound is **unverified**. The
earlier low-load run used the synchronous producer and is not the same-run
comparison the AC requires; it is supporting evidence only. Re-run this
benchmark version on a quiet host to evaluate both bounds in one run, then
update this section and WO-03.

`strace -f -c -e trace=fsync,fdatasync` over the final variant recorded 172
fsync calls (combined, without the barrier, recorded 93). Both are far below
one fsync per commit; the extra calls accompany barrier runs and SQLite
checkpoints.

## Risks

- **SQLite synchronization semantics.** The barrier relies on a FULL WAL commit
  synchronizing the whole WAL file. WO-03 verifies this with an fsync count
  under strace on Linux and documents the SQLite version used.
- **Missed release path.** A future journal-release request could bypass the
  barrier. The structural test covers the current client methods; adding a
  method requires extending it.
- **Batch fallback correctness.** Typed errors in the combined transaction must
  replay through the existing path with no partial state. WO-02 covers expiry,
  owner mismatch, and sequence gaps.
- **Interactive durability.** Recent non-delivery writes can be lost on power
  loss, as before PR 4425. This is the stated compatibility effect of AC-PLATFORM-DELIVERY-WRITE-COST-001.4.
- **Noisy host.** Benchmark absolute numbers vary with host load. AC-PLATFORM-DELIVERY-WRITE-COST-001.1 is
  relative to a baseline from the same run.
