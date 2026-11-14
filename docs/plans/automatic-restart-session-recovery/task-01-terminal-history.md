---
id: "01-terminal-history"
title: "Separate obsolete terminal recovery ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.6
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.9
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 01: Separate obsolete terminal recovery ownership

## Summary

An earlier proven terminal incident cannot own recovery of a later interrupted
prompt. Add one guarded historical-source classification and use it in terminal
settlement and native recovery. Preserve independent and allocated recovery work.

## In scope

- Permanent regression `TestHistoricalTerminalRecovery` from the disposable
  reproduction recorded in the plan. Require RED before the correction.
- Exact terminal evidence, source identity, resolved-block and settlement checks.
- The existing `blocked` outcome and terminal checkpoint for obsolete prepared
  work. Add `SilentRestoreOutcomeBlocked` and reason `source_terminal_settled`,
  preserving an existing reason as `completeSupersededContextAttemptTx` does.
- Transactional handling of old settled notices and old pending attempts.
- Repair existing records on recovery. No startup bulk rewrite.
- SQLite and PostgreSQL parity, rollback, and concurrent settlement evidence.

## Out of scope

- Automatic worker selection, browser changes, or new recovery permissions.
- Changing source payloads, outcomes, terminal bindings, or active candidates.

## Acceptance

1. The clean control and stale-notice, stale-attempt, and combined cases resolve
   the current authenticated incident. Historical terminal data stays unchanged.
2. Missing terminal proof, a settled notice for the current unknown prompt,
   foreign evidence, independent live work, and allocated candidates remain blocked.
3. Crash/retry and concurrent terminal arrival preserve atomic, idempotent
   recovery under both supported database dialects.

## Verification

Run from the repository root. PostgreSQL coverage is required for the changed
transaction. Record the test environment and actual non-skipped results.

```bash
test -n "$KANDEV_TEST_POSTGRES_DSN"
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/repository/sqlite -run '^Test(Postgres)?(HistoricalTerminalRecovery|NativeResumeAuthenticatedMissingBinding|NativeResumeRestoresRecoveredNoticeAfterExecutionLoss|NativeResumeAfterSilentRestore)$' -count=1 -v)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths before
delivery. Do not run full backend lint as a substitute.

The three existing suites each define SQLite and PostgreSQL entry points.
Add both `TestHistoricalTerminalRecovery` and `TestPostgresHistoricalTerminalRecovery`
in the new file. The run must show all eight selected top-level tests passing,
including all four PostgreSQL names. A missing variant, skipped PostgreSQL suite,
or a `no tests to run` result leaves this work order incomplete.

## Files likely touched

- `apps/backend/internal/task/models/session_continuity.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_attempt.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_binding.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_historical_terminal_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/native_resume_recovered_notice_test.go`
- `apps/backend/internal/orchestrator/agent_delivery_settlement.go`, only if the
  existing terminal-settlement transaction needs its orchestration call adjusted.

## Dependencies

None.

## Risks

The classification must apply to every relevant record. One settled source
cannot hide another live or unverifiable owner. Reuse `blocked` consistently.
An obsolete attempt must not become a restored conversation or an active guard.

## Parallelism

`sequential`

## Inputs

- Convergence design: Historical terminal incidents.
- Plan: Confirmed defect and disposable reproduction.
- Existing `native_resume_binding_test.go` and `silent_restore_test.go` fixtures.
- Backend scoped guidance and TDD backend-test reference.

## Results

- RED: the SQLite `TestHistoricalTerminalRecovery` regression failed for stale
  settled notices, stale prepared attempts, and their combination. The clean
  control and fail-closed cases passed before the correction.
- GREEN: the exact SQLite/PostgreSQL matrix passed, including all eight selected
  top-level suites. `TestHistoricalTerminalRecovery` and
  `TestPostgresHistoricalTerminalRecovery` passed every subcase, including the
  PostgreSQL rollback trigger and terminal-completion retry. Existing
  SQLite-specific rollback subtests skipped only in their PostgreSQL variants.
- `(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)`: passed.
- `(cd apps/backend && KANDEV_TEST_POSTGRES_DSN=<disposable PostgreSQL 16> go test -trimpath -race ./internal/persistence/storeconformance -count=1)`: passed.
- `bash scripts/lint-go-changed` with the seven changed Go paths: passed, 0 issues.
- `git diff --check`: passed.
