---
id: "03-project-durable-counts"
title: "Project deduplicated native counts atomically"
status: blocked
wave: 3
depends_on:
  - "02-normalize-native-events"
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-002
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-004
  - REQ-UI-CONTEXT-COMPACTION-COUNT-001
acceptance_criteria:
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.1
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.2
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.3
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.4
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.5
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.6
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.7
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-002.8
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.2
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.3
  - AC-AGENTS-COPILOT-NATIVE-COMPACTION-004.7
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.5
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.12
  - AC-UI-CONTEXT-COMPACTION-COUNT-001.13
system_design:
  - ../../specs/agents/system-design/copilot-native-compaction.md
  - ../../specs/ui/system-design/context-compaction-count.md
---

# Task 03: Project deduplicated native counts atomically

## Summary

Commit accepted native identity and the lifetime increment inside canonical durable projection.
Retain deduplication through replay, native rebind, receipt retirement, and task-session lifetime, with equivalent SQLite/PostgreSQL behavior.

## In scope

- Add `task_session_compactions` and schema adoption/replay coverage, with the unique session/provider/native/attempt key.
- Retain first delivery provenance, preserve legacy totals, and remove rows only with owning session deletion.
- Add one transaction helper shared by canonical single/batch/combined paths before projection is marked complete.
- Verify authenticated provider and native generation/rebind identity under existing lock order.
- Use row locking for aggregate/reporting metadata and bound parameters in both dialects.
- Prove rollback after receipt insertion/count mutation and no lost updates with concurrent ordinary context-window writes.
- Add reporting source, current availability, sticky lifetime partial coverage, and an atomic reporting revision without clobbering other metadata.
- Preserve ledger rows through task/workspace transfer and include session-owned receipts in transfer-preservation checks.
- Return committed metadata for post-commit live publication and normal boot/recovery hydration.
- Wire backend opt-in only when the optional agentctl capability and durable projector are present.
- Preserve adoption of older retained agentctl; report native collection unavailable without increment fallback.
- Apply accepted historical journal evidence during recovery with lifecycle callbacks suppressed, and support valid bootstrap events outside active submissions.
- Separate late raw events from already-journaled evidence, including native changes within a harness generation.
- Keep new observations protected retention barriers. Do not expand generic receipt pruning here.

## Out of scope

- Counter increments in callbacks, browser arithmetic, historical backfill, receipt TTL, private native history, or a second count authority.
- Making the new capability globally required for survival or automatically replacing retained processes.

## Acceptance

1. One accepted attempt adds one durable increment across same/different streams and restarts. Same attempt IDs in different native conversations remain distinct.
2. Receipt, count/reporting state, and projection cursor commit together; rollback and concurrent-writer tests pass on SQLite and real PostgreSQL.
3. Projection-only recovery and rebind-history tests preserve original ownership and lifetime totals without mutating replacement gauge state or relying on callbacks.

## Verification

Run from the repository root. Real PostgreSQL evidence is required before this work order becomes done.
Use the isolated test database mechanism; never substitute a developer's main database.

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Provide an isolated PostgreSQL test DSN}"
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestSessionCompactions|^TestPostgresSessionCompactions' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestSessionCompactionsConcurrent|^TestPostgresSessionCompactionsConcurrent' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestCopilotCompaction|^TestDurable.*Replay|^TestPublishContextWindow' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestContextWindow|^TestCopilotCompaction' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestUpdateSessionContextWindow|^TestPostgresUpdateSessionContextWindow|^TestRebindHarnessGenerationNativeSession' -count=1)
git diff --check
```

Run the new atomic projection regression for RED before adding the transaction helper.
Run `bash scripts/lint-go-changed` with every changed Go path and record the full command.
Record a missing PostgreSQL DSN as incomplete evidence, not a passing parity check.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/session_compactions_schema.go`, `session_compactions.go`, `session_compactions_test.go`, `session_compactions_postgres_test.go` (new).
- `apps/backend/internal/task/repository/sqlite/base_schema.go`, `agent_delivery_projection.go`, `agent_delivery_combined_projection.go`, and session-deletion and transfer-preservation ownership paths.
- `apps/backend/internal/task/models/agent_delivery.go` and `context_compaction.go` (new metadata/carrier support as needed).
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`, its canonical projection consumers, `events.go`, and `copilot_compaction_recovery_test.go` (new).
- `apps/backend/internal/orchestrator/context_window.go`, `event_handlers_git.go`, and focused `copilot_compaction_test.go` (new).
- Narrow repository interfaces and dependency wiring that own canonical projection/reporting metadata.

## Dependencies

Task 02: normalized completion/reporting carrier, root usage, and optional peer capability/configuration.

## Risks

The existing projector commits generic receipts before callbacks; a callback-only increment can be permanently lost.
Native rebind can change identity within one generation, so checking only the current resume token is insufficient.
Deleting receipts with timeline/transport history would re-enable duplicate increments.

## Parallelism

`sequential`

## Inputs

- [Provider design](../../specs/agents/system-design/copilot-native-compaction.md), ledger through reporting sections.
- [Shared count design](../../specs/ui/system-design/context-compaction-count.md), baseline/reset and metadata publication.
- Listed requirement/acceptance IDs, the ADR, existing canonical SQL projection and native rebind tests.
- `apps/backend/AGENTS.md`, especially SQL dialect, lock-order, and schema-adoption rules.

## Results

Blocked on PostgreSQL parity evidence.

- `task_session_compactions` is created after session/continuity schema with the unique key `(session_id, provider, native_session_id, compaction_attempt_id)` and cascades with the owning session.
- One transaction helper is shared by single, batch, and combined canonical projection. It validates the authenticated harness generation or rebind history, locks the session row on PostgreSQL, inserts the receipt with conflict-do-nothing, and increments only on insertion.
- Count, reporting metadata, receipt, effect, and cursor commit together. Reporting revisions increase atomically; partial coverage is sticky.
- SQLite projection tests cover duplicates across streams, distinct native conversations, stale native identity, concurrent context-window writes, partial coverage, rollback after receipt insertion, and schema reopen.
- `TestPostgresSessionCompactionsAtomicProjection` and `TestPostgresSessionCompactionsConcurrent` are present and skipped without an isolated DSN.
- `TestCopilotCompactionProjectionOnlyRecovery` passes and proves journaled observations reach canonical projection with callbacks suppressed.
- Committed count/reporting metadata is attached to the live notification; recovery without callbacks needs no callback.

Review remediation added: journaled predecessor completions project once after an in-place native rebind; historical-generation replay can add its retained count without changing current availability; a current-runtime unavailable verdict can downgrade a previously available reporting revision while preserving the lifetime total. Focused rebind, historical-availability, and downgrade regressions pass.

Limitations: no PostgreSQL execution was available here, so dialect parity is not claimed as executed evidence. The work order stays blocked until both PostgreSQL tests run against `KANDEV_TEST_POSTGRES_DSN`. Receipts are intentionally retained until session deletion.
