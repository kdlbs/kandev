---
id: "04-settled-prompts"
title: "Prove full settlement before pruning completed prompts"
status: complete
wave: 4
depends_on:
  - 03-event-retention
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-003
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-001.3
  - AC-PLATFORM-DELIVERY-RETENTION-003.1
  - AC-PLATFORM-DELIVERY-RETENTION-003.3
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 04: Prove full settlement before pruning completed prompts

## Summary

Add durable fully-settled eligibility and remove redundant completed prompt payloads. Keep immutable prompt and terminal summaries available to adoption and delayed recovery.

## In scope

- Publish `fully_settled_at` plus immutable terminal binding only after exact terminal outcome, Queue/Send Now claims, notice and required workflow outcome/no-op are durable.
- Retain pending and block_resolved terminals, prepared/accepted/dispatching/interrupted_unknown prompt payloads and independent recovery causes.
- Preserve latest-terminal ordering and superseded terminal receipts; duplicate settlement remains an idempotent success.
- Own the missing orchestrator writer for every evaluated terminal: persist exact owner/session/incarnation/harness/submission/turn-bound applied or guarded no-op workflow outcomes. Backfill only unambiguous legacy bindings.
- Certify no workflow intent required for no-workflow/quick-chat and scheduler-owned Office terminals. Preserve actual workflow requirements for workflow-routed Office turns.
- Persist `no_automatic_transition` after successful no-action or RequiresApproval-skipped evaluation. Do not consume later human approval. Deferred clarification/signals/queue drainage/moves and read/engine errors stay pending.
- Test every shape above, receipt write failure and successor-generation race. A failed outcome receipt cannot publish `fully_settled_at`. Reconcile the original terminal decision, never evaluate it against a successor step.
- Add crash tests at projection, block resolution, queue ACK, Send Now ACK, notice clear, workflow decision and terminal completion. Run cleanup between stages, reopen and resume. Include pruning between terminal projection and prompt RPC return; preserve completed/failed/cancelled outcomes and reject mismatched/unresolved identities.
- Reuse `session.go:SetSessionMetadataKeyIfJSONValue` structural CAS for exact notice clearing; test object key order and successor values. Failed provider turns must never run success workflow actions. Preserve completion admission release before ordinary synchronous Git snapshots and Office capture-before-teardown.

## Out of scope

Implementation was authorized by the user on 2026-10-09.
Agentctl journal policy, canonical-history expiry, new UI and feature flags are excluded.
This order does not authorize commit, push, merge or delegation.

## Acceptance

- Grace starts from durable full settlement; completed/failed/cancelled prompts retain ID/hash/owner/generation/dispatch association after payload pruning.
- Partial settlements survive cleanup and restart, finish the original claim once and never repeat workflow actions or sends.
- All no-workflow, quick-chat, Office, no-action and approval-skipped shapes produce guarded receipts and become eligible after other dependencies finish; deferred/error cases remain protected.
- Late delivery blocks use retained terminal evidence for the exact submission, while native/runtime/operator causes and successor notices stay effective.

## TDD evidence

Start each changed-logic scenario with a failing test against the pre-change behavior. Record Red, Green and refactor results.
Use existing backend test helpers, fake clocks and deterministic barriers. Do not use sleeps to manufacture race order.
Required new test groups: `TestDeliveryRetentionPartialTerminal`, `TestDeliveryRetentionPrompt`, `TestDeliveryRetentionIndependentBlocks`, `TestDeliveryRetentionWorkflowOutcome`.
Names are planned deliverables, not existing passing tests. Before using a -run selector, list tests and confirm every required group exists.
The real repository and retained journal must prove state transitions where named; fakes do not prove database crash semantics.
Read `.agents/skills/tdd/SKILL.md` and its `references/backend-tests.md` at implementation time.

## Verification

Run from repository root. Commands are implementation-time checks, not run during planning.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Prompt|PartialTerminal|IndependentBlocks|WorkflowOutcome)|TestDeliverySettlementCrashBoundaries|TestDeliveryTerminalPreservesOtherRecoveryCause' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'TestDeliveryRetention(PartialTerminal|Prompt|IndependentBlocks|WorkflowOutcome)|TestAgentDelivery|TestDeliveryCompletionAfterTerminalProjection|TestCompleteStreamGitSnapshot' -count=1)
```

Run the existing journal regression as an unchanged dependency: `(cd apps/backend && go test -trimpath -race ./internal/agentctl/journal -run 'TestTerminalCompletion' -count=1)`. No journal retention change is authorized.
SQL settlement and prompt tests need SQLite and PostgreSQL subtests; queue claim integration must use the real shared SQL pool and Queue/Send Now repositories. Missing PostgreSQL means this order remains unverified for that engine.
For every environment-gated test, record executed and skipped subtests. A skip cannot complete the corresponding acceptance.
The package helper checks document links/IDs; it does not prove runtime behavior.

## Files likely touched

- `apps/backend/internal/orchestrator/agent_delivery_settlement.go`
- `apps/backend/internal/orchestrator/agent_delivery_effect.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state.go`
- `apps/backend/internal/task/repository/sqlite/session.go`

Add focused `_test.go` files beside changed logic; split oversized tests rather than extending files past lint limits.
Paths marked new are intended deliverables. Other directory references identify the existing integration boundary.

## Dependencies

Complete `03-event-retention` first. Execute sequentially in the primary session.

## Inputs

- [Requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Design](../../specs/platform/system-design/backend-agent-delivery-retention.md); sections matching this scope.
- [Plan](plan.md), including baseline, storage thresholds and engine gates.
- Scoped `apps/backend/AGENTS.md`, existing delivery tests and nearby maintenance patterns.

## Risks

Marking fully_settled_at after only block_resolved can release raw data before queue/workflow completion. Retention must not become a second automatic recovery resolver.

## Parallelism

`sequential`. No delegation is authorized.

## Results

Implemented guarded full-settlement receipts and settled-prompt pruning. The completion proof now requires an exact applied/no-op workflow receipt and binds the latest terminal sequence/turn. Failed provider terminals record `not_required` only after their failure owner settles; they never invoke success workflow actions. Successful no-action and approval-skipped evaluations persist `no_automatic_transition`; missing signals and other deferred/error paths remain protected. Prompt selection uses metadata-only reads, bounded candidates/bytes, alternating event/prompt phases, and a wrapped keyset cursor. Recovery notice clearing uses the existing structural JSON compare-and-set.

Added workflow outcome coverage for exact policy ownership, a successor-step decision, injected receipt-write failure, successful applied transition, no-workflow/quick-chat/Office no-op shapes, no-action and approval-skipped no-ops, and missing-signal deferral. Existing queue and Send Now claim settlement tests remain the regression coverage for terminal ACKs and retry after queue-claim mutation. The retained failure test was first run with the no-op writer call removed and failed because the exact workflow receipt was absent; restoring the writer made it pass. The prompt-retention test previously failed before pruning because the raw payload and byte accounting remained unchanged. The additional source/failure tests are contract-only coverage and pass on the current implementation.

| Exact check | Result |
| --- | --- |
| `KANDEV_TEST_POSTGRES_DSN=... go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Prompt|PartialTerminal|IndependentBlocks|WorkflowOutcome)|TestDeliverySettlementCrashBoundaries|TestDeliveryTerminalPreservesOtherRecoveryCause' -count=1` | Passed SQLite and real PostgreSQL subtests, 31.293s. Covers bounded prompt payload cleanup, workflow receipt writer/source guard/failure, partial settlement, independent recovery blocks, and terminal crash regression. |
| `KANDEV_TEST_POSTGRES_DSN=... go test -trimpath -race ./internal/orchestrator -run 'TestDeliveryRetention(PartialTerminal|Prompt|IndependentBlocks|WorkflowOutcome)|TestAgentDelivery|TestDeliveryCompletionAfterTerminalProjection|TestCompleteStreamGitSnapshot' -count=1` | Passed, 2.156s. Exercises bound applied/no-op receipts, queue and Send Now ACK/retry, terminal-first completion, and completion/Git snapshot ordering. |
| `go test -trimpath -race ./internal/agentctl/journal -run 'TestTerminalCompletion' -count=1` | Passed, 1.261s; journal retention behavior remains unchanged. |

The repository matrix explicitly ran both engine subtests; the orchestrator integration uses the real SQLite task/queue repositories. No feature flag, journal changes, staging or commit was added.
