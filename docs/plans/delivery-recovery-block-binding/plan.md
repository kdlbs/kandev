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
The fix is two small repository changes, applied in order.

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
3. The notice (A, settled) and the block (B) now disagree. Retry reports `independent_recovery_block`. Resume's historical-terminal check needs a resolved A block, which no longer exists, so the block stays open.

## Requirement conformance

This is a regression against [REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006](../../specs/platform/requirements/durable-agent-delivery.md) AC .5 (resolve only the matching block) and .9 (Resume recovers the incident).
No requirement changes. The technical rule is in [delivery recovery block binding](../../specs/platform/system-design/delivery-recovery-block-binding.md).

## Scope

In scope: the two rules in that design.

Out of scope:

- Repairing sessions that are already stuck. Use the issue's one-row SQL workaround.
- Retry-connection changes. Resume is the recovery action for an unknown outcome.
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

- Both work orders are implemented. Delivery block upserts preserve an existing nonempty incident binding. Notice settlement matches the session, incarnation, submission, and generation; it binds an open block when a matching terminal effect is already `block_resolved` or `completed`, and otherwise settles only the notice for later terminal settlement.
- The production-order regression covers a reattached terminal stream, Clear-before-Complete, and historical-terminal acceptance after Complete. Resolved upserts also preserve a delivery binding against a conflicting consumer.
- Focused race tests, package builds, changed-file Go lint, SQL guard, documentation validation, and spec lint passed. PostgreSQL regression tests are registered and skip when `KANDEV_TEST_POSTGRES_DSN` is unset.
- The full race-enabled SQLite package timed out after 10 minutes in unrelated database-initialization tests, including with `-parallel 1`. The orchestrator package passed. Focused recovery tests and the isolated migration test passed.
