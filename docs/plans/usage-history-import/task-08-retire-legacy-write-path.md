---
id: "08-retire-legacy-write-path"
title: "Retire the repository legacy write path"
status: done
wave: 3
depends_on:
  - "07-remove-host-write-api"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-004
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-004.1
  - AC-COSTS-USAGE-HISTORY-004.3
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 08: Retire the repository legacy write path

## Summary

With the Host API gone, `UsageRepository.UpsertSessionUsage` has no production caller. Remove it and its helpers, which also removes a 200-line function with lint suppressions and the `file-length-limit` suppression if the file then fits.

## In scope

- Remove `UpsertSessionUsage`, `normalizeUsage`, the `normalizeLegacyUsage*` helpers, `classifyUsageWrite`, `validateUsageOwnership`, and `admitLegacyUsageSourceTx` when unused.
- Keep the storage tables and the shared helpers `Report` admission uses.
- Move tests to `Report` admission, or to a `_test.go` fixture helper when a test seeds rows for another subject.
- Remove source-owned `ListSessionUsageMeasurements` if no production caller remains.

## Out of scope

- The native adoption path and the `measurements`/`buckets` backfill, unless they are proven unreachable on installs. Record what was found in Results.

## Acceptance

- `rg 'UpsertSessionUsage|normalizeLegacyUsage' apps/backend --glob '!*_test.go'` finds nothing.
- Every existing behavior assertion in the affected tests is preserved, or its removal is justified in Results.
- `usage.go` carries no `nolint` or `revive:disable` suppression for the removed function.

## Verification

```bash
(cd apps/backend && go test ./internal/analytics/... ./internal/task/repository/sqlite/ -count=1)
(cd apps/backend && go test -race ./internal/analytics/repository/sqlite/ -count=1)
bash scripts/lint-go-changed apps/backend/internal/analytics/repository/sqlite/usage.go
```

## Files likely touched

- `apps/backend/internal/analytics/repository/usage.go`
- `apps/backend/internal/analytics/repository/sqlite/usage.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_source_context.go`
- `apps/backend/internal/analytics/service/service.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_test.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_admission_test.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_contract_test.go`
- `apps/backend/internal/task/repository/sqlite/usage_history_lifecycle_test.go`

About 20 test files call this path today. Run `rg -l UpsertSessionUsage apps/backend` for the full list.

## Dependencies

Task 07.

## Risks

- Tests can quietly lose meaning during a fixture migration. Migrate in small batches and compare assertions.
- Rows written through `Report` carry contract metadata that fixture rows lack. Tests of `Metadata == ""` repair paths may need a direct-insert helper.

## Parallelism

`sequential`

## Inputs

- `apps/backend/internal/analytics/repository/sqlite/usage_history_mutations.go`.
- `apps/backend/internal/analytics/repository/sqlite/usage_history_contract_regression_helpers_test.go`.

## Results

Removed `UsageRepository.UpsertSessionUsage`, the legacy normalization and classification helpers, `admitLegacyUsageSourceTx`, and the obsolete service methods from production.
The retired upsert behavior now exists only in a `_test.go` fixture helper, and cross-package task lifecycle tests seed retained rows through `Report`.
`usage.go` no longer carries the file-length suppression.

Validation: `go test ./internal/analytics/... ./internal/task/repository/sqlite/ -count=1` passed, as did `go test -race ./internal/analytics/repository/sqlite/ -count=1`. `rg 'UpsertSessionUsage|normalizeLegacyUsage' apps/backend --glob '!*_test.go'` has no matches and `usage.go` has no `file-length-limit` suppression.
