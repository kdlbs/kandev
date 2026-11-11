---
id: "01-postgres-recovery-query"
title: "Use dialect-safe recovery query expressions"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUN-SCHEDULING-001
acceptance_criteria:
  - AC-TASKS-RUN-SCHEDULING-001.9
system_design:
  - ../../specs/tasks/system-design/run-scheduling.md
---

# Task 01: Use dialect-safe recovery query expressions

## Summary

Make the Office recovery query select unstarted tasks on SQLite and PostgreSQL
with the same UTC lookback and duplicate-run behavior.

## Acceptance

- PostgreSQL queries use portable date and JSON expressions, and a non-UTC
  session time zone does not change the `created_at` lookback.
- Queued, claimed, and finished runs prevent duplicate recovery. Failed and
  cancelled runs do not.
- SQLite keeps its existing behavior and SQL guard exemptions describe only
  expressions that still require an exemption.

## Verification

```bash
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -v -run '^TestPostgresListUnstartedTasks$' ./internal/office/repository/sqlite)
```

The PostgreSQL integration test requires `KANDEV_TEST_POSTGRES_DSN`.

## Files

- `apps/backend/internal/office/repository/sqlite/tasks.go`
- `apps/backend/internal/office/repository/sqlite/list_unstarted_tasks_postgres_test.go`
- `apps/backend/internal/db/sqlguard/exemptions.json`

## Results

The PostgreSQL query already uses dialect helpers for JSON extraction,
lookback arithmetic, and naive UTC timestamps. Removed its stale SQLite-only
date-function exemption from the SQL guard list.

- `cd apps/backend && go run ./cmd/sqlguard ./internal`: passed.
- `cd apps/backend && go test -trimpath -v -run '^TestPostgresListUnstartedTasks$' ./internal/office/repository/sqlite`: package compiled; the integration test skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.
- `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: passed with the changed work order and its linked requirement and design.
- `python3 scripts/list-docs.py validate`: passed (403 decisions and 1,732 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
