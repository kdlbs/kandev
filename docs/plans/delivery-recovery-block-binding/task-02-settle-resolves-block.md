---
id: "02-settle-resolves-block"
title: "Resolve the matching block when a notice settles"
status: done
wave: 2
depends_on: ["01-preserve-bound-delivery-block"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
system_design:
  - ../../specs/platform/system-design/delivery-recovery-block-binding.md
---

# Task 02: Resolve the matching block when a notice settles

## Summary

`ClearAgentDeliveryRecovery` settles the notice by its session, incarnation, submission, and generation. If a matching terminal effect is already `block_resolved` or `completed`, Clear also resolves the submission's open block using the effect's terminal binding. Without a terminal effect, Clear settles only the notice and later terminal settlement resolves the block.

## In scope

- Write `TestClearAgentDeliveryRecoveryResolvesMatchingBlock` first. Settle submission A before Clear, while its terminal effect is `block_resolved` but not fully completed. Give the notice and its open block a stream that differs from the terminal stream.
  - After Clear, the block is resolved and carries the terminal binding (stream, sequence, turn, outcome, `durable_delivery_<outcome>`).
  - Complete the terminal settlement after Clear, then assert `loadHistoricalTerminalSettlementTx` accepts the block and settled notice.
  - Control: an open block for another submission stays open.
- Test that Clear without a terminal effect settles the notice and leaves its block open for later terminal settlement.
- Turn `ClearAgentDeliveryRecovery` into one transaction, using the existing locks:
  1. Match the existing notice by its session, incarnation, submission, generation, and unsettled phase.
  2. Compare-and-set the metadata to settled.
  3. If the latest matching terminal effect is `block_resolved` or `completed`, bind matching open delivery blocks to that effect, including replacing a mismatched stream.
  4. Advance the recovery session revision.
- Reuse the terminal-binding UPDATE from `agent_delivery_settlement.go` instead of writing a second one.
- Widen the resolved-upsert guard so a resolved write from another consumer cannot overwrite an existing bound delivery block.

## Acceptance

1. The test fails on `main` and passes after the fix.
2. After Clear and terminal-effect completion, `loadHistoricalTerminalSettlementTx` accepts the resolved block. Resume can then recover a later interrupted prompt.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/orchestrator -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state.go apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state_test.go`
- `apps/backend/internal/task/repository/sqlite/session_recovery_block_resolved_upsert.go`
- `apps/backend/internal/task/repository/sqlite/session_recovery_block_binding_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_clear_test.go` (new)

## Results

- `ClearAgentDeliveryRecovery` checks the notice's exact prompt identity and unsettled phase. It settles the notice even before a terminal effect exists; when the latest matching effect is `block_resolved` or `completed`, it also binds open blocks in the same transaction.
- Shared the terminal-binding update with terminal settlement. A reattached terminal stream replaces the notice's older stream on both the block and settled notice; a block for another submission stays open.
- The regressions exercise the production order, prove that `loadHistoricalTerminalSettlementTx` accepts the resolved terminal binding after completion, and cover notice-only settlement before an effect exists.
- Resolved upserts now preserve a delivery binding against a conflicting consumer.
- Focused SQLite terminal/recovery tests and targeted orchestrator settlement tests passed with `-race`. The affected repository and orchestrator packages build; SQL guard, changed-file Go lint, docs validation, spec lint, and `git diff --check` pass. The full SQLite race suite timed out in unrelated database-initialization tests during the previous verification; see the plan results.
