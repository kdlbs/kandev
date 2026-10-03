---
id: "04-journal-batching"
title: "Commit bounded journal batches before publication"
status: done
wave: 4
depends_on: ["03-projection-and-acks"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 04: Commit bounded journal batches before publication

## Summary

Commit bounded journal batches before publication. Preserve the existing durable identity and admission boundaries.

## In scope

Introduce a single ordered journal writer shared by all normalized event producers, including direct errors and process exit.
Commit batches of at most 64 KiB with a maximum 20 ms batching wait. Keep the aggregate producer queue at most 4 MiB.
Keep durable synchronization enabled. Split large text with stable identity; permit one supported larger indivisible event in a bounded singleton transaction.
Preserve the 1 MiB event limit. Reject oversized unsupported payloads with typed errors.
Assign sequences and publish only after the whole transaction commits. Commit terminal event and terminal submission evidence atomically.
Flush earlier events before terminal completion. Keep Stop and permission responses independent of storage locks.
Detach preserves the writer; actual shutdown drains or explicitly fails pending work and closes ownership safely.

## Out of scope

Other work orders, automatic prompt resend, native conversation replacement, and unrelated cleanup.

## Acceptance

- A forced transaction failure publishes none of its batch; restart replays committed events in order.
- A deterministic 2000-event small-chunk burst uses fewer than 200 journal commit transactions, with exact sequence and terminal evidence.
- Timer, byte limits, cancellation, detach, shutdown, and direct producers have deterministic coverage.

## Regression evidence

Add journal/batch_test.go and process/delivery_batch_test.go: TestJournalBatchAtomicity, TestDeliveryBatchByteAndTimeLimits, TestDeliveryBatchTerminalBarrier, TestDeliveryBatchShutdown. Add BenchmarkJournalDeliveryBurst; compare durable transaction counts and report storage environment.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
(cd apps/backend && go test ./internal/agentctl/journal -run '^$' -bench BenchmarkJournalDeliveryBurst -benchmem -count=3)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/journal/journal.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/submission_delivery.go`

Add named regression files beside the relevant production package.

## Dependencies

Task 03: Batch canonical projection and retry cumulative acknowledgments.

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

Implemented ordered journal `AppendBatch` and the process delivery writer with
bounded byte and time batching before publication.

Validation passed:

- `(cd apps/backend && go test -race ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)`
- `(cd apps/backend && go test ./internal/agentctl/journal -run '^$' -bench BenchmarkJournalDeliveryBurst -benchmem -count=3)`
- `make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
