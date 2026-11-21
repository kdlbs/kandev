---
created: 2026-11-20
status: done
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/delivery-recovery-block-binding.md
legacy_specs: []
---

# Implementation plan: Delivery recovery block binding

## Overview

This plan fixes [issue #5331](https://github.com/kdlbs/kandev/issues/5331), which is assigned to `carlosflorencio`.
A session stays blocked behind an `unknown_prompt_outcome` recovery block, and no recovery action clears it.
The fix preserves delivery-block identity, settles matching notices, and lets
retry repair stale blocks only after checking the bound submission's durable
state.

## Root cause

Baseline: `de63958fc71`. A temporary test, since deleted, reproduced the issue's database state with the real repository methods:

| Step | Call | Block row |
| --- | --- | --- |
| 1 | `UpsertAgentDeliveryRecovery(A, sA)` | open, `agent_delivery`, A, sA |
| 2 | `ClearAgentDeliveryRecovery(A)` → notice `settled` | **still open**, A, sA |
| 3 | `UpsertSessionRecoveryBlock(interactive, no submission)` | **overwritten**: `interactive`, '', '' |
| 4 | `UpsertSessionRecoveryBlock(agent_delivery, B, '')` | **re-pointed**: B, '', A's ID and `created_at` |

1. The `ON CONFLICT` guard in `upsertSessionRecoveryBlockTx` (`session_continuity.go`) protects a bound block only when both writes are `agent_delivery`. A write from another consumer with the same reason erases the binding. The next failed dispatch then binds the row to B.
2. `ClearAgentDeliveryRecovery` updates only metadata. Terminal settlement resolves only blocks still bound to that submission, so nothing ever resolves A's incident.
3. The notice (A, settled) and the block (B) can disagree in installations with the old bug. Retry reports `independent_recovery_block` without checking whether B is already inactive. Resume's historical-terminal check also needs a resolved source block, so the block stays open.

## Requirement conformance

This is a regression against [REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006](../../specs/platform/requirements/durable-agent-delivery.md) AC .5 (resolve only the matching block), .9 (Resume recovers the incident), and .16 (retry reports actionable recovery and does not resend).
The technical rules are in [delivery recovery block binding](../../specs/platform/system-design/delivery-recovery-block-binding.md).

## Scope

In scope: preserve bound delivery blocks, resolve notice-bound blocks when terminal evidence exists, and let `retry_connection` clear only a mismatched bound block whose canonical submission is no longer in flight.

Out of scope:

- Repairing blocks with missing or ambiguous submission evidence; retry continues to block those cases.
- UI, schema and migrations.

## Work orders

- [x] [01: Preserve bound delivery blocks on upsert](task-01-preserve-bound-delivery-block.md)
- [x] [02: Resolve the matching block when a notice settles](task-02-settle-resolves-block.md)

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/orchestrator -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- Both work orders are implemented. Delivery block upserts preserve an existing nonempty incident binding in both open and resolved writes. Notice settlement matches session, incarnation, submission, and generation; it binds the latest matching terminal effect, including after a reattached stream, and an early notice-only settlement can be repaired by a later Clear. Without terminal evidence, later terminal settlement still resolves the matching block.
- `retry_connection` resolves a mismatched delivery block only when its canonical submission is completed, failed, cancelled, or interrupted_unknown. Prepared, accepted, and dispatching submissions remain blocking; another open recovery cause remains blocking. Session rows are updated before bound block rows to preserve write lock order.
- Focused race tests, package builds, SQL guard, documentation validation, spec lint, and changed-file Go lint passed. An earlier `lint-go-changed` attempt timed out after five minutes while reporting zero issues; its retry passed, and the same packages also passed with a 15-minute timeout. PostgreSQL regression tests are registered and skip when `KANDEV_TEST_POSTGRES_DSN` is unset.
- The full race-enabled SQLite package timed out after 10 minutes in unrelated database-initialization tests, including with `-parallel 1`. The orchestrator package passed. Focused recovery tests and the isolated migration test passed.
