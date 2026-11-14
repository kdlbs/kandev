---
id: "02-combined-receipt-projection"
title: "Commit batched receipt and projection together"
status: done
wave: 2
depends_on:
  - "01-contention-benchmark"
plan: plan.md
requirements:
  - REQ-PLATFORM-DELIVERY-WRITE-COST-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-WRITE-COST-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
system_design:
  - ../../specs/platform/system-design/durable-delivery-write-cost.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 02: Commit batched receipt and projection together

## Summary

Replace the per-event receive commit for batched output with one writer
transaction per batch that receives, projects, and advances both cursors.

## In scope

- `Repository.ReceiveAndProjectCanonicalAgentDeliveryEvents` in
  `agent_delivery_projection.go` (or a new sibling file): one transaction, the
  union lock set, `receiveAgentDeliveryEventTx` per event in sequence order, a
  cursor reload, `projectCanonicalDeliveryBatchTx`, and one commit. It returns
  per-event results that mark events already projected before the call. The
  PostgreSQL path uses the existing advisory-lock order.
- Lifecycle: preparation for batchable events defers receipt (reader-pool
  checks only). `flushBatch` calls the combined method through a new optional
  interface on the delivery repository. Duplicates schedule ACK without
  notification.
- Fallback: a typed admission error from the combined call replays the batch's
  events through the existing two-phase path, so `ErrAgentDeliveryReplayExpired`
  reconciliation and typed recovery are unchanged.
- Extend the benchmark with a `combined` variant.

## Out of scope

- Unbatched events (tool call, shell output, terminal, permission, owner
  change).
- Synchronous level, barrier, or ACK transport changes (WO-03).

## Acceptance

- `TestReceiveAndProjectCanonicalBatchAtomic`: on SQLite and, gated by
  `KANDEV_TEST_POSTGRES_DSN`, on PostgreSQL, a failure injected after receipt
  leaves no inbox row, no cursor change, and no message. A retry yields the same
  canonical message IDs as an uninterrupted run. Duplicate and
  received-but-unprojected inputs return the correct per-event result.
- `TestStreamProcessorBatchUsesOneWriterCommit`: a batch of compatible chunks
  produces exactly one writer transaction, counted by a repository fake, and ACK
  is scheduled only after that commit.
- `TestCombinedBatchAdmissionFailureFallsBack`: replay expiry, owner mismatch,
  and a sequence gap in the combined call reach the same outcomes as the
  current two-phase path.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'Test(ReceiveAndProjectCanonicalBatchAtomic|AgentDelivery|CanonicalDelivery)' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(StreamProcessorBatchUsesOneWriterCommit|CombinedBatchAdmissionFailureFallsBack|DurableDelivery|StreamEventProcessor|CanonicalAgent)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -run '^$' -bench 'BenchmarkDeliveryWriterContention' -benchtime=100x -count=3 ./internal/task/repository/sqlite)
bash scripts/lint-go-changed <changed-go-files>
```

Confirm that the new test names are discovered before you treat a filtered
run as passing. PostgreSQL parity runs in CI.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_combined_projection.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_combined_projection_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/stream_event_processor.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/stream_event_processor_combined_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_writer_contention_bench_test.go`

## Dependencies

WO-01.

## Risks

Deferred receipt moves the stale-owner and sequence checks inside the batch
transaction. Keep the existing validation helpers rather than re-implementing
them, and keep the fallback path covered.

## Results

Done.

- `Repository.ReceiveAndProjectCanonicalAgentDeliveryEvents` admits the batch
  receipts, reloads the cursor, projects the canonical messages, and advances
  both cursors in one writer transaction. Results carry `AlreadyProjected`, and
  duplicate events still return their committed message identity.
- `prepareAgentEventWithReceipt` defers receipt for batchable events after the
  reader-pool stale-owner and effect checks. `streamEventProcessor.flushBatch`
  commits the batch through the optional receiver interface, and combined
  duplicate results schedule the cumulative ACK without a notification.
- A typed admission error replays the batch through the existing two-phase path
  one event at a time, preserving expiry reconciliation and typed recovery.
- Duplicate-notification suppression is scoped to the combined path:
  `commitPreparedAgentEvents` marks `AlreadyProjected` results as
  `skipCallback`, so the single-event and projection-only paths retain their
  existing notification behavior. `TestCombinedBatchDuplicateDoesNotNotify`
  covers the combined path and `TestAlreadyProjectedSingleEventStillNotifies`
  pins the unchanged single-event path.
- `TestReceiveAndProjectCanonicalBatchAtomic` (SQLite and gated PostgreSQL),
  `TestReceiveAndProjectReceivedButUnprojectedEvent`,
  `TestStreamProcessorBatchUsesOneWriterCommit`,
  `TestStreamProcessorCombinedBatchWithSQLiteRepository`,
  `TestCombinedBatchAdmissionFailureFallsBack`,
  `TestCombinedBatchDuplicateDoesNotNotify`, and
  `TestAlreadyProjectedSingleEventStillNotifies` pass, including under `-race`.
- `go run ./cmd/sqlguard ./internal` passes.

