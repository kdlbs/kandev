---
created: 2026-09-14
status: complete
requirements:
  - REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001
  - REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002
system_design:
  - ../../specs/tasks/system-design/administrative-turn-settlement.md
legacy_specs:
  - ../../specs/workflow/administrative-turn-settlement/spec.md
---

# Implementation plan: Prevent stale administrative turns from blocking workflow progress

## Overview

Deliver one vertical recovery capability: settle one proven-stale
administrative turn exactly once, atomically with its audit, while keeping
the session, worktree, history, queues, and sibling sessions intact, and
while keeping successor workflow delivery correct across a superseded
intent.

Work orders:

- [Task 01: Settle one exact stale administrative turn](task-01-durable-turn-settlement.md)

The task system owns the package because it owns the durable turn,
completion-intent, queue-admission, and audit records that prove the exact
finished identity and its successor.

## Scope

In scope: the settlement evidence evaluation and atomic commit, the narrow
same-workspace authority matrix backed by write-once spawn-supervision
provenance, provider-completion and reconciliation paths that never close a
successor or re-run a transition, durable cross-task delivery receipts with
exact-turn idempotency, and the regression coverage pinning the
unattributed-anomaly boundary.

Out of scope: session cancellation or restart, broad parent stop semantics,
general long-running turn settlement without an eligible intent, and any
frontend recovery panel.

## Technical approach

Persist the accepted completion signal as a `session_completion_intents`
row keyed to (session, turn, workflow step). Settle it either from the
provider's terminal event or, when the quiet grace passes with no
conflicting activity, through the narrow `settle_stale_session_kandev` tool
whose authority bases are same-task peer, direct parent, or the write-once
persisted spawn supervisor. Commit the terminal transition, the intent
state, and the audit event in one transaction; mark a moved task's old
intent superseded and deliver only the current transition's on-entry once.

Cross-task reporting rides durable `message_deliveries` receipts with
source-turn-fingerprint idempotency, bounded worker retry, capacity
promotion, and the ordinary-dispatch claim cleanup that keeps an accepted
prompt from being re-dispatched at startup.

## Tests

| Criteria | Evidence |
| --- | --- |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.1 | `internal/mcp/server/settle_stale_session_tool_test.go` (task-mode only, trusted identity injection) |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.2 | `internal/mcp/handlers/settle_stale_session_test.go` and `staleSettlementAuthority` authority matrix coverage |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.3 | `internal/orchestrator/stale_session_settlement_test.go` active-ownership refusal matrix |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.4 | `TestSettleStaleSessionRefusesUnattributedCreatedSessionAnomaly` |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.5 | `TestSettleStaleSessionRecordsAuditEventAtomicallyWithSettlement` |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.1 | `TestSettleStaleSessionRefusesActiveAdministrativeOwnership` rearm cases |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.2 | `internal/orchestrator/event_handlers_step_completion_test.go` moved-task supersession |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.3 | `internal/orchestrator` duplicate on-entry and transition-keyed delivery tests |
| AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.4 | settlement preservation assertions in the orchestrator suite |

## Risks

A settlement that treats process materialization as turn evidence would
settle a session that never ran a turn; the authority predicate and the
regression test pin the fail-closed direction. An accepted ordinary
dispatch whose claim cleanup misses would re-dispatch on startup; the
recovery-claim tests pin the claim-settling acknowledgement.

## Parallelism

`sequential`

## Results

Implemented across backend commits at PR #2909 heads through f30b3bd67:
completion-intent persistence and atomic settlement with audit,
`settle_stale_session_kandev` task-mode tool with the three-basis
authority matrix, write-once spawn supervision provenance, superseded-intent
handling with once-only on-entry delivery, durable cross-task delivery
receipts with source-turn idempotency and bounded retry, ordinary-dispatch
claim cleanup in the durable acknowledgement, and the unattributed
CREATED-session refusal regression. Focused orchestrator, MCP, messagequeue,
and lifecycle suites pass; spec lint and plan/work-order lint pass.

PR #2909 commit `14eb594ecf3aece40c1010155b1d5ef225fbcae1` closes the remaining
manual-settlement atomicity gap: turn completion, terminal intent transition,
and authorization audit now commit together. The focused failure-path tests and
spec validation pass. Exact-head CI remains blocked by unavailable required-
check policy and a GitHub code-search rate limit in PR documentation coverage.
