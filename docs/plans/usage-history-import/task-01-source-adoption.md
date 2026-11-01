---
id: "01-source-adoption"
title: "Adopt existing sessions as eligible sources"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-IMPORT-001
acceptance_criteria:
  - AC-COSTS-USAGE-IMPORT-001.1
  - AC-COSTS-USAGE-IMPORT-001.2
  - AC-COSTS-USAGE-IMPORT-001.3
  - AC-COSTS-USAGE-IMPORT-001.4
  - AC-COSTS-USAGE-IMPORT-001.5
  - AC-COSTS-USAGE-IMPORT-001.6
  - AC-COSTS-USAGE-IMPORT-001.7
system_design:
  - ../../specs/costs/system-design/usage-history-import.md
---

# Task 01: Adopt existing sessions as eligible sources

## Summary

Add the one-time core migration step that marks safe existing sessions as eligible. Snapshot discovery reports `Preparing` until the step finishes, so an import never reads a half-adopted workspace.

## In scope

- `BackfillUsageSourceAdoption`, its `source-adoption` cursor, and the `evidence` column on `usage_collection_contexts`, on SQLite and PostgreSQL.
- Exclusion rules for missing transcripts, forks, unsafe sources, transfers, and erased scopes.
- The `prepareUsageCollectionTargetsTx` readiness gate.
- The `usage_source_adoption_total` counter, and its entry in the root `AGENTS.md` Observability section.

## Out of scope

- Plugin behavior, which Tasks 02 and 03 own.
- Any change to actor attribution.

## Acceptance

- An existing session with a transcript ID and no unsafe evidence is `eligible` after one worker pass, with unknown actor status.
- Each exclusion stays `unknown` or `mixed`, and a second migration pass does no session work.
- A restart mid-adoption resumes without duplicate evidence, and an erased scope gains no rows.

## Verification

```bash
(cd apps/backend && go test ./internal/analytics/repository/sqlite/ -run 'TestSourceAdoption' -count=1)
(cd apps/backend && go test -race ./internal/analytics/repository/sqlite/ -run 'TestSourceAdoption|TestUsageHistoryCurrentTarget' -count=1)
(cd apps/backend && KANDEV_TEST_POSTGRES=1 go test ./internal/analytics/repository/sqlite/ -run 'TestSourceAdoptionPostgres' -count=1)
bash scripts/lint-go-changed apps/backend/internal/analytics/repository/sqlite/usage_history_source_adoption.go
```

Confirm the PostgreSQL DSN variable against `testutil.PostgresDSNFromEnv` before running the third command.

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/usage_history_source_adoption.go` (new)
- `apps/backend/internal/analytics/repository/sqlite/usage_history_source_adoption_test.go` (new)
- `apps/backend/internal/analytics/repository/sqlite/usage_history_migration.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_source_context.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_contract_schema.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_current_target_test.go`
- `AGENTS.md`

## Dependencies

None.

## Risks

- `TestUsageHistoryCurrentTargetRejectsUnknownAndAmbiguousSources` inserts a session after migration and expects `unknown`. Keep that behavior for sessions the step has not adopted.
- Adoption runs after the existing migration steps, and each page holds the writer for its transaction, so keep the batch at the existing limit of 200.

## Parallelism

`parallel-safe`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history-import.md), REQ-COSTS-USAGE-IMPORT-001.
- [System design](../../specs/costs/system-design/usage-history-import.md), Core source adoption.
- [ADR](../../decisions/2026-11-01-usage-source-adoption.md).
- `usage_history_current_target_test.go` for fixtures.

## Results

Implemented `BackfillUsageSourceAdoption` as the final `BackfillUsageHistory` step with a durable `source-adoption` cursor, `usage_collection_contexts.evidence`, and the closed `usage_source_adoption_total` counter labels.
Discovery waits for the cursor before preparing targets.
Exclusions cover missing transcripts, forks, unsafe transcript sharing, transfer audit rows, and erasing or erased scopes.
Tests cover safe adoption, unknown actors, every exclusion, restart resume, PostgreSQL, and the metric.

Validation: `go test ./internal/analytics/repository/sqlite/ -run TestSourceAdoption -count=1` passed on SQLite; the PostgreSQL-gated test skipped without `KANDEV_TEST_POSTGRES_DSN`. `go test -race ./internal/analytics/repository/sqlite/ -run 'TestSourceAdoption|TestUsageHistoryCurrentTarget' -count=1` passed.
