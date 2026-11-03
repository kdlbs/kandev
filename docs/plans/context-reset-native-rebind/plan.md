---
created: 2026-11-02
status: implemented
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
legacy_specs: []
---

# Fix plan: context reset rebinds the harness generation

## Overview

A workflow context reset replaces the native conversation inside a live execution and
updates the resume token. The committed harness generation, however, keeps the previous
native ID. After a crash, every restart and Resume path compares the two, and the session
cannot recover. This fix rebinds the current generation in the same transaction as the
token. One work order covers it.

## Evidence and root cause

Task "Migrar para 3DF 7 e .NET 10" (session `933bd6fa`), 2026-11-02.

| Record | Value |
|---|---|
| `harness_session_generations` generation 1 | native `f7cdaf37`, committed 2026-10-30 09:50 |
| `executors_running.resume_token` | `83f9cfd3` |
| Session metadata | `context_compaction_count` 51, `acp.session_id` `83f9cfd3` |

1. 10:03:46: after the crash, adoption refused the retained execution with "durable adoption
   owner identity mismatch: adopted native session does not match SQL".
2. 10:08:52: automatic restart recovery prepared a silent-restore attempt. Its checkpoint
   pinned `source_generation.native_session_id = f7cdaf37`.
3. 10:46:52, after a manual SQL repair aligned only the generation row: explicit Resume
   restored `83f9cfd3`. Block resolution still failed with "block changed or recovery
   evidence is incomplete", because `preparedNativeCheckpointMatches`
   (`task/repository/sqlite/native_resume_attempt.go`) compared the pinned `f7cdaf37` with
   the proof's `83f9cfd3`.
4. The same signature occurred on 2026-10-23 for task W102 (session `2f9b2485`).

Root cause. `resetAgentContextWithError` (`orchestrator/event_handlers_workflow.go`)
calls `ResetAgentContext`, clears the token, and stores the new ACP session ID through
`storeResumeToken`. `persistInitialHarnessGeneration` (`orchestrator/event_handlers.go`)
only creates a first generation or an adjacent Start-fresh replacement, so nothing updates
an existing generation. The design states that token fields are projections of the current
committed generation. This path breaks that invariant, and every later comparison fails.

This violates AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.1: the native identifier is not
preserved for native resume. It also violates AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21 and
006.23: restart recovery cannot verify native identity. No requirement changes. The system
design gains an "In-place context reset" rule.

## Scope

### In scope

- A repository method that, in one transaction, stores the token and rebinds the current
  generation's native ID, and appends a history row. It does this only while the generation
  still has the previous native ID, the execution owns `executors_running`, no newer
  generation exists, and no restore attempt is incomplete.
- The workflow context reset uses that method instead of the plain token store.
- If the rebind is refused, the reset fails through the existing reset-failure settlement.

### Out of scope

- Repairing sessions that drifted before this fix, or their pending restore checkpoints.
- Native ID changes that do not go through `ResetAgentContext`.
- Advancing the generation or rotating the delivery stream for in-place resets. This was
  considered and rejected as too large.
- UI changes.

## Technical approach

Persistence (`apps/backend/internal/task/repository/sqlite/`):

- New append-only table `harness_native_session_rebinds`: `id`, `session_id`,
  `incarnation_id`, `generation`, `previous_native_session_id`, `native_session_id`,
  `execution_id`, `rebound_at`. Create it in schema init, with a session index. Register the
  table with its store descriptor (`internal/persistence/requiredstores`) and the conformance
  adapter (`internal/persistence/storeconformance`).
- New `Repository.RebindHarnessGenerationNativeSession(ctx, req)` in
  `harness_native_rebind.go`. In a single transaction it:
  - takes the writer (PostgreSQL: an advisory lock keyed by the session);
  - updates `executors_running` (token and `last_message_uuid`) where
    `agent_execution_id = req.ExecutionID`;
  - updates `harness_session_generations.native_session_id` for the current generation,
    where the native ID still equals `req.PreviousNativeSessionID`, no newer generation
    exists, and no `session_restore_attempts` row has `completed_at IS NULL`;
  - inserts the history row.
  It returns `(bool, error)`. Any zero-row update rolls back and returns `false`.
- Add the method to the repository interface (`task/repository/interface.go`).

Orchestrator (`apps/backend/internal/orchestrator/`):

- In `resetAgentContextWithError`, after `ResetAgentContext` succeeds, call a new
  `rebindResetNativeSession` instead of `clearResumeToken` followed by `storeResumeToken`.
  It passes `previousACPSessionID`, the current ACP session ID, the execution, and the
  current generation. A `false` result settles through `settleFailure` with
  `workflow_context_reset_failed`. Sessions without a committed generation (legacy or
  non-durable) keep the existing clear-and-store path.
- `persistACPSessionID` still mirrors the token into session metadata on success.

## Tests

| Criterion | Evidence |
|---|---|
| AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.1 | `orchestrator/event_handlers_workflow_reset_rebind_test.go`: `TestWorkflowContextResetRebindsCommittedGeneration`. The reset leaves the token and the generation native ID equal to the new ACP session ID, and adds one history row. Fails on main: the generation keeps the old ID. |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21, 006.23 | `task/repository/sqlite/harness_native_rebind_test.go`: `TestRebindHarnessGenerationNativeSession`. Covers acceptance, then refusal with no partial writes for a wrong previous ID, a foreign execution, a newer generation, and an incomplete restore attempt. `TestPostgresRebindHarnessGenerationNativeSession` repeats it when `KANDEV_TEST_POSTGRES_DSN` is set. |
| Fail-closed reset | `TestWorkflowContextResetRefusedRebindFailsReset`: a refused rebind settles the reset failure and keeps the previous token and generation. |

## E2E tests

None. A crash between a context reset and restart recovery cannot be driven from the
browser deterministically. The UI is unchanged.

## Work orders

- [x] [Task 01: Context reset rebinds the current generation](task-01-reset-rebinds-generation.md)

## Verification results

- `go test -trimpath -race -count=1 ./internal/task/repository/sqlite -run 'RebindHarnessGenerationNativeSession'`:
  failed against the stub (`Should be true`, `no such table`), then `ok`. The PostgreSQL variant
  skips without `KANDEV_TEST_POSTGRES_DSN` (not run locally).
- `go test -trimpath -race -count=1 ./internal/orchestrator -run 'WorkflowContextReset'`:
  `TestWorkflowContextResetRebindsCommittedGeneration` failed on main
  (`expected "native-after-reset"`, `actual "native-before-reset"`). The refusal test failed with
  `Should be false`. Both are `ok` after the change.
- `go run ./cmd/sqlguard ./internal`: exit 0.
- `go test -trimpath -race -count=1 ./internal/persistence/storeconformance ./internal/persistence/requiredstores`: `ok`.
  This needed `harness_native_session_rebinds` in the v0.93.0 upgrade manifest's
  `known_missing_required_tables`.
- `go test -trimpath -race -count=1 ./internal/task/repository/sqlite ./internal/orchestrator`: `ok` (387s, 183s).
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues, after the validation moved into `completeNativeRebind` (cyclop).
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`: passed.

Deviation from the plan: the orchestrator reaches the new method through a local
`harnessNativeRebindStore` interface, which follows the existing `sessionContinuityStore`
pattern. `task/repository/interface.go` is unchanged.

## Risks

- The new table needs SQLite fresh, replay, and upgrade coverage, plus PostgreSQL behavior
  evidence (`go run ./cmd/sqlguard ./internal`, storeconformance race test).
- A reset during an incomplete restore attempt now fails visibly instead of silently drifting.
- Sessions that have already drifted stay broken until they are repaired manually (align the
  generation and the pending restore checkpoint), or until a later repair package.
