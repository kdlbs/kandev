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

# Task 02: Resolve delivery blocks during settlement and retry

## Summary

`ClearAgentDeliveryRecovery` settles the notice by its session, incarnation, submission, and generation. If a matching terminal effect is already `block_resolved` or `completed`, Clear also resolves the submission's open block using the effect's terminal binding. Without a terminal effect, Clear settles only the notice and later terminal settlement resolves the block. A repeated Clear can repair the same settled notice after terminal evidence arrives. `retry_connection` can also clear an open mismatched delivery block after it verifies that the bound submission is no longer in flight.

## In scope

- Write `TestClearAgentDeliveryRecoveryResolvesMatchingBlock` first. Settle submission A before Clear, while its terminal effect is `block_resolved` but not fully completed. Give the notice and its open block a stream that differs from the terminal stream.
  - After Clear, the block is resolved and carries the terminal binding (stream, sequence, turn, outcome, `durable_delivery_<outcome>`).
  - Complete the terminal settlement after Clear, then assert `loadHistoricalTerminalSettlementTx` accepts the block and settled notice.
  - Control: an open block for another submission stays open.
- Test that Clear without a terminal effect settles the notice and leaves its block open for later terminal settlement.
- Test that a settled notice can bind a later terminal effect on a reattached stream, and that historical settlement accepts the binding after completion.
- Test that retry resolves a mismatched block for a completed or `interrupted_unknown` submission, but keeps it open for a `dispatching` submission.
- Resolve a mismatched retry block in a repository transaction that validates the block and canonical submission together. Update the session row before the recovery block row.
- Turn `ClearAgentDeliveryRecovery` into one transaction, using the existing locks:
  1. Match the existing notice by its session, incarnation, submission, and generation. An already settled notice is eligible only to bind terminal evidence that arrived later.
  2. Compare-and-set the metadata to settled.
  3. If the latest matching terminal effect is `block_resolved` or `completed`, bind matching open delivery blocks to that effect, including replacing a mismatched stream. Do not require a terminal effect to settle the notice.
  4. Advance the recovery session revision before updating block rows.
- Reuse the terminal-binding UPDATE from `agent_delivery_settlement.go` instead of writing a second one.
- Widen the resolved-upsert guard so a resolved write from another consumer cannot overwrite an existing bound delivery block.

## Acceptance

1. The test fails on `main` and passes after the fix.
2. After Clear and terminal-effect completion, `loadHistoricalTerminalSettlementTx` accepts the resolved block. Resume can then recover a later interrupted prompt.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^(TestClearAgentDeliveryRecovery.*|TestUpsertPreservesBoundDeliveryBlock|TestSQLiteDeliveryTerminalSettlesMatchingBlock|TestDeliveryTerminalBeforeRecoveryBlockResolvesExactSubmission|TestUpsertProjectedTerminalResolvesExistingOpenBlock|TestAgentDeliveryRecoveryRevisionFencesPhaseOwnerAndSubmission)$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run '^(TestRetrySessionDeliveryResolvesStaleInactiveSubmissionBlock|TestRetrySessionDeliveryKeepsStaleInFlightSubmissionBlock|TestRetrySessionDeliveryKeepsOtherOpenBlockAfterResolvingStaleSubmission|TestRetrySessionDeliverySettlesProjectedTerminalWithoutOpenBlock)$' -count=1)
(cd apps/backend && go build -trimpath ./internal/task/repository/sqlite ./internal/orchestrator)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/session_delivery_recovery.go apps/backend/internal/orchestrator/session_delivery_terminal_retry_test.go apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_clear_test.go apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state.go apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go apps/backend/internal/task/repository/sqlite/session_recovery_block_inactive_delivery.go
(cd apps/backend && GOMEMLIMIT=3GiB GOMAXPROCS=2 GOFLAGS=-p=2 golangci-lint run ./internal/orchestrator ./internal/task/repository/sqlite --new-from-rev=de63958fc71a036e41021faded983035ec025189 --timeout=15m --concurrency=2)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state_test.go`
- `apps/backend/internal/task/repository/sqlite/session_recovery_block_inactive_delivery.go`
- `apps/backend/internal/task/repository/sqlite/session_recovery_block_resolved_upsert.go`
- `apps/backend/internal/task/repository/sqlite/session_recovery_block_binding_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_clear_test.go` (new)
- `apps/backend/internal/orchestrator/session_delivery_recovery.go`
- `apps/backend/internal/orchestrator/session_delivery_terminal_retry_test.go`

## Results

- `ClearAgentDeliveryRecovery` checks the notice's session, incarnation, submission, and generation. It settles the notice even before a terminal effect exists; when the latest matching effect is `block_resolved` or `completed`, it also binds open blocks in the same transaction. A repeated Clear repairs the same settled identity if terminal evidence arrived on a reattached stream.
- Shared the terminal-binding update with terminal settlement. A reattached terminal stream replaces the notice's older stream on both the block and settled notice; a block for another submission stays open.
- `retry_connection` verifies stale block identity and submission state transactionally. It settles `completed`, `failed`, `cancelled`, and `interrupted_unknown` submissions; a `dispatching` submission remains blocking. Terminal effects provide the final block binding when available, and another open recovery cause remains blocking.
- Terminal settlement advances the session row before updating recovery blocks, preserving the shared lock order.
- The regressions exercise production settlement order, delayed terminal evidence on a different stream, historical-terminal acceptance after completion, notice-only settlement, stale retry settlement, the in-flight guard, and preservation of another open cause.
- Resolved upserts now preserve a delivery binding against a conflicting consumer.
- Focused SQLite terminal/recovery tests and targeted orchestrator retry tests passed with `-race`. The affected repository and orchestrator packages build; SQL guard, docs validation, spec lint, and `git diff --check` pass. The final `lint-go-changed` retry passed; an earlier five-minute attempt timed out after reporting zero issues, and package-scoped Go lint with a 15-minute timeout also passed with zero issues. The full SQLite race suite timed out in unrelated database-initialization tests during the previous verification; see task 01 Results.
