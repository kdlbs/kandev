---
id: "02-lifecycle-migration"
title: "Preserve history through deletion and migration"
status: pending
wave: 2
depends_on: ["01-retained-admission"]
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-001
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-001.1
  - AC-COSTS-USAGE-HISTORY-001.2
  - AC-COSTS-USAGE-HISTORY-001.3
  - AC-COSTS-USAGE-HISTORY-001.4
  - AC-COSTS-USAGE-HISTORY-001.5
  - AC-COSTS-USAGE-HISTORY-001.6
  - AC-COSTS-USAGE-HISTORY-002.5
  - AC-COSTS-USAGE-HISTORY-002.6
  - AC-COSTS-USAGE-HISTORY-002.7
  - AC-COSTS-USAGE-HISTORY-007.1
  - AC-COSTS-USAGE-HISTORY-007.2
  - AC-COSTS-USAGE-HISTORY-007.3
  - AC-COSTS-USAGE-HISTORY-007.6
  - AC-COSTS-USAGE-HISTORY-007.7
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 02: Preserve history through deletion and migration

## Summary

Backfill existing native and external usage through restart-safe batches.
Integrate retained accounting with archive, delete, pruning, erasure, and concurrent admission.

## In scope

- Add durable high-water marks and immutable keyset cursors. Resume after every migration crash point without duplication.
- Preserve old measurements before task cascade, using durable deletion admission and bounded preservation batches.
- Fence session pruning before it clears usage links or cascades the turn evidence required for historical attribution.
- Serialize preservation completion/final deletion with native and external writes in SQLite and PostgreSQL.
- Detach live references while preserving original subject incarnation, scope, dates, models, and amounts.
- Keep task ledger cascade and task-session APIs intact. Late retained observations require previously validated execution bindings.
- Register every new table/checkpoint with existing workspace/org erasure, suspension policy, and E2E reset.
- Add explicit retained-scope erasure by captured org ID, independent of live workspaces. Fence concurrent writers and every rebuild generation.
- Preserve historical ownership across reassignment and flag changes. Leave unsupported historical identities unknown.
- Document safe read rollback and unsafe old-binary deletion paths. Do not rebuild all history on each startup.

## Out of scope

- New erasure controls, generic orphan admission, exact recovery of absent data, and statistics UI changes.

## Acceptance

- Archive, task deletion, session pruning, ID reuse, and uninstall leave retained amounts intact.
  Explicit erasure removes captured org history, including transferred-workspace scopes, without worker resurrection.
- A concurrent delete cannot erase a source row before its retained copy commits. Failures leave source data and retryable cleanup.
- Backfill interruption, repeated startup, flag changes, and non-UTC PostgreSQL sessions do not duplicate or reassign usage.

## Verification

```bash
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/task/repository/sqlite ./internal/task/service -run 'TestUsageHistory|TestManagedDeletion|TestArchiveTask' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/org/... ./internal/backendapp -run 'TestUsageHistory|TestOrgDelete|TestOrgSuspend' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... ./internal/task/repository/sqlite ./internal/org/... -run 'TestUsageHistoryPostgres' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/usage_history_migration*.go` and lifecycle tests (new)
- `apps/backend/internal/task/repository/sqlite/{task.go,resource_cleanup.go,usage_events.go}`
- `apps/backend/internal/task/service/{managed_deletion.go,resource_cleanup_preparation.go}`
- `apps/backend/internal/org`, `internal/backendapp/orgs.go`, and `task/service/service_resources.go` (`DeleteOrganizationWorkspaces`)
- `apps/backend/internal/backendapp` startup/reset wiring and database migration metadata

## Dependencies

Task 01. Retained schema and native admission must exist before destructive lifecycle cutover.

## Risks

A process-local lock cannot protect cross-process PostgreSQL deletion races.
Preservation must not require an unavailable transcript tool or keep a volatile-only migration cursor.
Restore and binary downgrade need separate treatment from reverting the read path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), REQ-001/002/007 and listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), Retained identity, Native admission, Migration and recovery.
- Existing task deletion admission, org/workspace erasure services, and [native ledger contract](../../specs/task-cost-ledger/spec.md).

## Results

Pending.
