---
id: "01-reset-rebinds-generation"
title: "Context reset rebinds the current generation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
---

# Task 01: Context reset rebinds the current generation

## Summary

A successful workflow context reset stores the new resume token and rebinds the committed
generation's native ID in one transaction, and records the previous ID in an append-only
history table. A refused rebind fails the reset and keeps the previous identity.

## In scope

- Table `harness_native_session_rebinds`, its schema, store registration, and conformance.
- `Repository.RebindHarnessGenerationNativeSession` with the guards from the plan.
- `resetAgentContextWithError` uses the rebind when a generation is committed.
- Reset-time agent events defer token writes until the guarded rebind settles.
- Partial provider failures and missing live prior IDs use the committed generation identity.
- A refused transition keeps prompt admission blocked until a later reset repairs it.

## Out of scope

- Repairing existing drifted sessions, generation advancement, stream rotation, and UI.

## Acceptance

1. After a reset, the token, the session metadata `acp.session_id`, and the generation
   native ID are all the new ACP session ID, and exactly one history row exists.
2. A wrong previous ID, a foreign execution, a newer generation, or an incomplete restore
   attempt each refuse the rebind with no partial write, and the reset settles as failed.
3. An event before a refused rebind cannot move only the token. The refusal keeps the old
   token and generation, and blocks prompt admission.
4. A provider error after the live ID changes uses the guarded rebind to keep durable identity
   aligned, while the reset still reports the provider error.
5. An unavailable previous live ID uses the committed generation as the guarded prior ID.

## Verification

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 -count=1 ./internal/task/repository/sqlite -run 'RebindHarnessGenerationNativeSession')
(cd apps/backend && go test -trimpath -race -tags fts5 -count=1 ./internal/orchestrator -run 'WorkflowContextReset')
(cd apps/backend && go run ./cmd/sqlguard ./internal && go test -trimpath -race -count=1 ./internal/persistence/storeconformance)
(cd apps/backend && go test -trimpath -race -tags fts5 -count=1 ./internal/task/repository/sqlite ./internal/orchestrator)
(bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go'))
(python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all)
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/harness_native_rebind.go` (new)
- `apps/backend/internal/task/repository/sqlite/harness_native_rebind_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/` schema init for the new table
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/persistence/requiredstores/`, `apps/backend/internal/persistence/storeconformance/`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_reset_rebind.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_reset_rebind_test.go`
- `apps/backend/internal/orchestrator/event_handlers.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`

## Dependencies

None.

## Risks

- Schema replay and PostgreSQL parity for the new table.
- Tests that stub the reset path may need the new repository method on their fakes.

## Parallelism

`sequential`

## Inputs

- `harness-session-continuity.md`, section "In-place context reset".
- `durable-restart-reconciliation.md`, native identity verification.
- Existing tests: `event_handlers_workflow_reset_failure_test.go`, `native_resume_binding_test.go`.

## Results

- Follow-up fix on PR head `8d8d8ee`: the new refusal, partial-reset, and missing-prior-ID
  tests failed before the production changes and passed after them.
- `go test -trimpath -race -tags fts5 -count=1 ./internal/task/repository/sqlite -run 'RebindHarnessGenerationNativeSession'`:
  `ok` (2.781s).
- `go test -trimpath -race -tags fts5 -count=1 ./internal/orchestrator -run 'WorkflowContextReset'`:
  `ok` (1.454s).
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
