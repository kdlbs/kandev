---
id: "01-durable-turn-settlement"
title: "Settle one exact stale administrative turn"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001
  - REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002
acceptance_criteria:
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.1
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.2
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.3
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.4
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.5
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.1
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.2
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.3
  - AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.4
system_design:
  - ../../specs/tasks/system-design/administrative-turn-settlement.md
---

# Task 01: Settle one exact stale administrative turn

## Summary

Release one proven-stale workflow-turn identity through a narrow,
evidence-gated settlement path, committing the turn, its completion intent,
and the authorized audit atomically, while the broad stop, session
cancellation, and workflow transitions stay unchanged.

## In scope

- `internal/orchestrator/stale_session_settlement.go` and the completion
  reconciler's settlement commit.
- `internal/mcp/handlers/settle_stale_session.go` with the
  same-task-peer / direct-parent / persisted-supervisor authority matrix.
- Task-mode-only `settle_stale_session_kandev` registration with trusted
  caller identity injection.
- Write-once spawn-supervision provenance backing the supervisor basis.
- Supersession of an intent whose task moved, with once-only on-entry
  delivery for the current transition.
- Durable cross-task delivery receipts with source-turn idempotency and
  the ordinary-dispatch claim cleanup in acknowledgement.
- Regression coverage for the unattributed CREATED-session refusal.

## Out of scope

- Broad parent-child stop semantics, session cancellation or resume,
  frontend recovery surfaces, and settlement of turns without an eligible
  completion intent.

## Acceptance

1. An authorized settlement of an eligible quiet intent commits the turn,
   intent, and audit together; a settled turn never exists without its
   audit event and vice versa.
2. Any active-ownership evidence (reservation, background work, pending
   tool call, cancellation, successor) refuses with `not_stale` and zero
   mutation; a CREATED materialization with no turn rows refuses the same
   way and the refusal records no launch-authority claim.
3. A task that moved before reconciliation gets its old intent superseded,
   the old step is never re-evaluated, and the current transition gets
   exactly one successor prompt.

## Verification

From `apps/backend`:

```bash
go test ./internal/orchestrator/ -run 'SettleStaleSession|HandleStepComplete|MessageTask' -count=1
go test ./internal/mcp/... ./internal/orchestrator/messagequeue/ -count=1
go test ./internal/agent/runtime/lifecycle/ -run 'RecoveredPromptGeneration' -count=1
```

From the repository root:

```bash
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/orchestrator/stale_session_settlement.go`
- `apps/backend/internal/orchestrator/completion_settlement.go`
- `apps/backend/internal/mcp/handlers/settle_stale_session.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_launch.go`
- `apps/backend/internal/orchestrator/messagequeue/delivery_repository.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`

## Dependencies

None.

## Risks

Process materialization must never count as turn evidence; the authority
predicate and `TestSettleStaleSessionRefusesUnattributedCreatedSessionAnomaly`
pin the fail-closed direction. An accepted ordinary dispatch must settle its
recovery claim in the same acknowledgement, or startup reconciliation
re-dispatches an accepted prompt; the recovery-claim tests pin it.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/administrative-turn-settlement.md)
- [System design](../../specs/tasks/system-design/administrative-turn-settlement.md)
- [Plan](plan.md)
- [Legacy workflow spec](../../specs/workflow/administrative-turn-settlement/spec.md)
- [Decision](../../decisions/2026-08-19-durable-administrative-turn-settlement.md)

## Results

Implemented across PR #2909. Manual stale settlement now commits the captured
turn completion, terminal intent transition, and audit event in one
transaction. A failed audit insert leaves the turn open and the intent
settling, and an injected settlement failure leaves the turn open. Refusals
record `not_stale` without mutation; the unattributed CREATED-session anomaly
regression passes; supersession delivers one successor prompt for the current
transition only; and delivery receipts acknowledge their ordinary-dispatch
claim.

Verification on commit `14eb594ecf3aece40c1010155b1d5ef225fbcae1`:

```bash
go test ./internal/orchestrator/ -run 'SettleStaleSession|HandleStepComplete|MessageTask' -count=1
go test ./internal/task/repository/sqlite/ -run 'CompleteTurnAndTransitionCompletionIntent|TransitionCompletionIntentWithControlEvent' -count=1
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

All four commands passed. PR CI was started for this commit; its final gate
remains unresolved because required-check policy lookup failed and the PR
documentation-coverage job hit GitHub code-search rate limits.
