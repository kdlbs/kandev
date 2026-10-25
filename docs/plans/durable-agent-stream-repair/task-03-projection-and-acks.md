---
id: "03-projection-and-acks"
title: "Batch canonical projection and retry cumulative acknowledgments"
status: done
wave: 3
depends_on: ["02-complete-replay"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 03: Batch canonical projection and retry cumulative acknowledgments

## Summary

Batch canonical projection and retry cumulative acknowledgments. Preserve the existing durable identity and admission boundaries.

## In scope

Separate projection success from ACK transport success in processAgentEvent. A failed ACK must not suppress a committed canonical notification.
Use one ordered projection worker per stream with bounded batches. Merge compatible contiguous text or reasoning updates by canonical message identity.
Record every original inbox sequence and effect identity. Flush before tool, permission, terminal, turn, generation, or ownership boundaries.
Apply each batch and advance its projected cursor atomically. Preserve metadata.thinking and initial-versus-append notification semantics.
Schedule cumulative ACKs after projection, with one in-flight request per stream. Coalesce to the highest contiguous committed projected cursor.
Retry lost ACKs independently; never resend prompts. Cancel workers on owner replacement or shutdown without losing durable pending work.
On projection failure, stop cursor advancement and trigger typed lifecycle recovery. Do not log and keep consuming an irreparable gap.
Persist before notifying. Use existing browser subscription recovery for a crash after SQL commit but before notification.

## Out of scope

Other work orders, automatic prompt resend, native conversation replacement, and unrelated cleanup.

## Acceptance

- ACK failure after SQL commit does not hide output, repeat effects, or stop future cumulative ACK progress.
- Batched projection preserves exact text, thinking metadata, and terminal ordering across crash/replay and owner changes.
- A deterministic 2000-chunk burst performs fewer than 200 canonical row updates and ACK requests; slow input flushes within the configured timer bound.

## Regression evidence

Add TestCanonicalDeliveryBatchCrashBoundaries, TestCanonicalDeliveryAckFailureStillNotifies, and TestCanonicalDeliveryBatchBoundaries beside existing projection/lifecycle tests. Instrument transaction, update, and ACK counts. Add BenchmarkCanonicalDeliveryBurst for 2000 and 20000 chunks; report allocations and elapsed time without claiming a preset speedup.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite ./internal/orchestrator -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^$' -bench BenchmarkCanonicalDeliveryBurst -benchmem -count=3)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Persistence gate: cover fresh boot, close/reopen, previous-format upgrade, interrupted migration, and unsupported newer formats. Run PostgreSQL conformance when SQL changes; record missing infrastructure as a release evidence gap. Never report skipped database cases as passed.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/agentctl/client_delivery.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`
- `apps/backend/internal/task/repository/interface.go`

Add named regression files beside the relevant production package.

## Dependencies

Task 02: Replay every retained page before joining live delivery.

## Risks

Partial failure must preserve ownership, original submission identity, and replay evidence. Do not clear uncertainty solely because transport reconnects.

## Parallelism

`sequential`

## Inputs

- [Plan and source evidence](plan.md).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- Existing tests beside the listed files.

## Results

Implemented canonical projection batching, projection-before-notification, and
cumulative ACK retries that retain the highest projected cursor. Added SQLite
batch projection and idempotency coverage.

Validation passed:

- `(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite ./internal/orchestrator -count=1)`
- `(cd apps/backend && go run ./cmd/sqlguard ./internal)`
- `(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)`
- `(cd apps/backend && go test ./internal/task/repository/sqlite -run '^$' -bench BenchmarkCanonicalDeliveryBurst -benchmem -count=3)`
- `make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
