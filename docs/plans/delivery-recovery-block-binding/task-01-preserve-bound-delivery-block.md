---
id: "01-preserve-bound-delivery-block"
title: "Preserve bound delivery blocks on upsert"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
system_design:
  - ../../specs/platform/system-design/delivery-recovery-block-binding.md
---

# Task 01: Preserve bound delivery blocks on upsert

## Summary

After an open block is bound to a submission, no later write can change that binding.

## In scope

- Write `TestUpsertPreservesBoundDeliveryBlock` in a new `session_recovery_block_binding_test.go`. The test:
  1. Opens a bound A block.
  2. Writes an `interactive` block with the same reason.
  3. Writes a delivery block for B with an empty stream.
  4. Asserts that the row still names `agent_delivery`, A and A's stream.
  - Control: an open unbound block can still gain a binding.
- Widen the existing CASE guard in `upsertSessionRecoveryBlockTx`. The protection condition becomes: the stored row is an `agent_delivery` row with a non-empty submission, and either the incoming consumer or the incoming submission differs.

## Acceptance

1. The test fails on `main` at step 2 and passes after the fix.
2. The existing tests in the package still pass.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/session_continuity.go apps/backend/internal/task/repository/sqlite/session_recovery_block_binding_test.go
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/session_continuity.go`
- `apps/backend/internal/task/repository/sqlite/session_recovery_block_resolved_upsert.go`
- `apps/backend/internal/task/repository/sqlite/session_recovery_block_binding_test.go` (new)

## Results

- Added a regression for an interactive write followed by a conflicting delivery write, plus an unbound-to-bound control.
- Confirmed the regression failed before the upsert guard changed and passed after.
- Verification: `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestUpsertPreservesBoundDeliveryBlock$' -count=1)` and the final `lint-go-changed` retry passed. An earlier lint attempt timed out at five minutes after reporting zero issues; package-scoped `golangci-lint` with a 15-minute timeout also passed. The full race-enabled SQLite package suite timed out after 10 minutes in unrelated database-initialization tests, including with `-parallel 1`; the focused regression passed.
