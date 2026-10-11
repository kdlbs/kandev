---
id: "01-durable-ack"
title: "Make delivery acknowledgment power-loss durable"
status: in_progress
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.4
system_design:
  - ../../specs/platform/system-design/pruned-delivery-recovery.md
---

# Task 01: Make delivery acknowledgment power-loss durable

## Summary

Close the persistence boundary before journal pruning. Use FULL on actual
SQLite writer connections and preserve projection-before-ACK scheduling.

## In scope

- Red regression for actual writer synchronous setting after open and reconnect.
- FULL writer DSN and corrected connection-local setting documentation.
- Failed receipt/projection and successful real-store ACK ordering regressions.
- Verify PostgreSQL retains its synchronous-commit boundary in owning fixtures.

## Out of scope

- Recovery past an existing gap, reader pool rewrites, new settings, broad
  performance tuning, destructive host crash experiments, or local E2E.

## Acceptance

- `TestSQLiteWriterPowerLossDurability` fails on NORMAL and passes on initial
  and reopened FULL writer connections.
- Failed receipt or projection emits no ACK; ACK starts only after persisted
  receipt and applied effects are observable from an independent reader.
- Existing ACK coalescing, retries, and replacement-owner fences remain valid.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/db -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -run 'Test(SQLiteWriterPowerLossDurability|DeliveryAckDurability|DurableDeliveryAck)' -count=1)
git diff --check
```

Confirm new test names are discovered before interpreting a filtered run as
passing. Run SQL parity in CI using the existing PostgreSQL job fixtures.

## Files likely touched

- `apps/backend/internal/db/sqlite.go`
- `apps/backend/internal/db/sqlite_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_ack.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_ack_durability_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_durability_test.go` (new)

## Dependencies

None.

## Risks

FULL adds commit synchronization cost. Use existing bounded batching; do not
trade away durability to make a timing assertion pass. SQLite's documented
guarantee assumes a functioning storage synchronization implementation.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- Design: acknowledgment durability.
- Accepted ADR and existing ACK worker/transaction test patterns.

## Results

FULL writer connections and reconnect coverage implemented. The real writer durability regression passes. Existing receipt/projection-before-ACK checks pass in focused lifecycle coverage. PostgreSQL parity remains a CI gate. No hardware crash or local E2E test was run.
