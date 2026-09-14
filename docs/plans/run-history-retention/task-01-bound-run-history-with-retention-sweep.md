---
id: "01-bound-run-history-with-retention-sweep"
title: "Bound Office run history with a scheduled retention sweep"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-HISTORY-RETENTION-001
  - REQ-OFFICE-RUN-HISTORY-RETENTION-002
  - REQ-OFFICE-RUN-HISTORY-RETENTION-003
  - REQ-OFFICE-RUN-HISTORY-RETENTION-004
  - REQ-OFFICE-RUN-HISTORY-RETENTION-005
acceptance_criteria:
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.1
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.2
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.3
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.4
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.5
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.6
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.7
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.8
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.9
  - AC-OFFICE-RUN-HISTORY-RETENTION-001.10
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.1
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.2
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.3
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.4
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.5
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.6
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.7
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.8
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.9
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.10
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.11
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.12
  - AC-OFFICE-RUN-HISTORY-RETENTION-002.13
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.1
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.2
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.3
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.4
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.5
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.6
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.7
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.8
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.9
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.10
  - AC-OFFICE-RUN-HISTORY-RETENTION-003.11
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.1
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.2
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.3
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.4
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.5
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.6
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.7
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.8
  - AC-OFFICE-RUN-HISTORY-RETENTION-004.9
  - AC-OFFICE-RUN-HISTORY-RETENTION-005.1
  - AC-OFFICE-RUN-HISTORY-RETENTION-005.2
  - AC-OFFICE-RUN-HISTORY-RETENTION-005.3
  - AC-OFFICE-RUN-HISTORY-RETENTION-005.4
  - AC-OFFICE-RUN-HISTORY-RETENTION-005.5
system_design:
  - ../../specs/office/system-design/run-history-retention.md
  - ../../specs/office/system-design/run-history-retention-operations.md
---

# Task 01: Bound Office Run History with a Scheduled Retention Sweep

## Summary

Add `internal/office/retention`: a settings-backed sweep that ages out
`office_routine_runs` and `runs` history (plus their satellites) on its own
interval, a per-owner floor, status-only live/history classification, a preview
pass before first deletion, engine-identical behavior on SQLite and PostgreSQL,
and a `GET`/`PUT /api/v1/system/retention` operator surface with a matching
Settings > System > Data & Logs card.

## In scope

- Eligibility, count, and delete queries shared by preview and the real sweep,
  keyed on `COALESCE(completed_at, created_at)` / `COALESCE(finished_at,
  requested_at)`, oldest-first, chunked across statements.
- A session-scoped PostgreSQL advisory lock (with a SQLite single-process
  equivalent) so only one backend sweeps at a time.
- A per-owner floor (newest N rows retained regardless of age) applied to
  count, preview, and delete identically, re-asserted in the DELETE statement.
- Satellite deletion (`run_events`, `office_run_route_attempts`,
  `office_run_skills`) in the same transaction as their parent run.
- `health.Issue` warnings for approaching the backlog cap and for count/sweep
  failure; a one-time preview per table before any row is deleted.
- `RetentionSettingsCard` on Settings > System > Data & Logs: policy, retained
  counts, preview/backlog state, last sweep, errors.
- Expression indexes serving the sweep's filter/order on both engines.

## Out of scope

- Filesystem/container artifact cleanup (storage maintenance owns that).
- Routine/workspace deletion cascades (already delete runs structurally).
- Any run lifecycle, routing, or task/session/checkout behavior change.

## Acceptance

- History rows (terminal `office_routine_runs`/`runs` statuses) older than the
  configured window are deleted in batches; live-state rows (`received`,
  `task_created`, `queued`, `claimed`) are never age-pruned; an unrecognized
  status fails safe as live state and warns.
- The newest `floor` rows per owner survive regardless of age.
- Each table's first sweep is a preview: it reports would-delete counts and
  deletes nothing; a later sweep performs real deletion.
- Behavior, including the advisory lock and batch chunking, is identical on
  SQLite and PostgreSQL.
- `GET`/`PUT /api/v1/system/retention` read/write policy and report current
  counts, preview state, last sweep outcome, and backlog/error warnings.

## Verification

```bash
cd apps/backend && go test -race -count=1 ./internal/office/retention/... ./internal/office/repository/sqlite/...
cd apps/backend && KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race -count=1 -v ./internal/office/retention/...
cd apps/backend && golangci-lint run ./internal/office/retention/...
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web test -- --run retention-settings-card system-route-copy
cd apps/web && pnpm e2e:run -- --project chromium --grep "System retention settings"
```

## Files likely touched

- `apps/backend/internal/office/retention/*.go`
- `apps/backend/internal/office/repository/sqlite/base_migrations.go`,
  `retention_indexes_test.go`, `retention_indexes_postgres_test.go`
- `apps/backend/internal/backendapp/*` (scheduler/handler wiring)
- `apps/web/src/**/retention-settings-card.tsx` and its tests
- `docs/specs/office/requirements/run-history-retention*.md`,
  `docs/specs/office/system-design/run-history-retention*.md`
- `docs/public/operations.md`

## Dependencies

None.

## Risks

- A dialect-sensitive query bug that only reproduces on PostgreSQL (mitigated
  by an environment-gated Postgres suite covering the advisory lock, the
  EvalPlanQual TOCTOU window, and batch chunking).
- A too-aggressive window or floor deleting rows an operator still needed
  (mitigated by the default 30-day window, the per-owner floor, and the
  preview-before-delete behavior).

## Parallelism

`sequential`

## Inputs

- `REQ-OFFICE-RUN-HISTORY-RETENTION-001` through `-005`.
- [run history retention](../../specs/office/system-design/run-history-retention.md)
  and
  [run history retention operations](../../specs/office/system-design/run-history-retention-operations.md).
- The reference install's 323 consecutive `coalesced` routine-run rows (28
  days, one bricked routine) cited in the requirements' Overview.

## Results

- Implemented `internal/office/retention` (settings store, `Sweeper`,
  `Scheduler`, `CensusTracker`, PostgreSQL advisory lock with SQLite
  equivalent, `Handler` for `GET`/`PUT /api/v1/system/retention`) plus the
  `RetentionSettingsCard` on Settings > System > Data & Logs.
- Full backend gauntlet green: `go build ./...`, `go vet`, `gofmt -l`,
  `go test -race ./internal/office/retention/...` (SQLite), the
  PostgreSQL-gated suite against a real scratch instance (125 tests, 0
  skipped, all PASS, covering the advisory lock, the EvalPlanQual
  concurrent-resurrection TOCTOU regression, and chunked batch deletes),
  `golangci-lint run ./...` (0 issues).
- Frontend: `pnpm run typecheck`, the retention card and System route-copy
  Vitest suites, and a scoped Playwright run (`--grep "System retention
  settings"`, 2 passed).
- Delivered as PR [#3566](https://github.com/kdlbs/kandev/pull/3566)
  ("feat(office): bound run history growth with a scheduled retention
  sweep"), through four Build rounds and four Review rounds; remaining
  non-blocking test-rigor gaps and CI-wiring follow-ups are tracked on the
  linked follow-up card rather than blocking this PR.
