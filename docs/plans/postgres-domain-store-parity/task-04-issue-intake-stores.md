---
id: "04-issue-intake-stores"
title: "Port issue intake stores"
status: done
wave: 4
depends_on:
  - "03-gitlab-store"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 04: Port Issue Intake Stores

## Summary

Make Jira, Linear, and Sentry stores portable. These stores share settings, health, watch, and reservation patterns.

## In scope

- Use portable schema types and shared schema probes.
- Rebind store and transaction queries.
- Replace SQLite-only reservation syntax.
- Add fresh, replay, settings, watch, and reservation tests on PostgreSQL.

## Out of scope

- Change provider polling or external issue filters.
- Change workspace ownership rules.

## Acceptance

- Each `NewStore` succeeds twice against its PostgreSQL schema.
- Settings, health, watch, and reservation operations succeed on PostgreSQL.
- SQLite legacy migrations keep their current data-preservation behavior.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/jira ./internal/linear ./internal/sentry -run 'TestPostgresStore' -v
```

## Files likely touched

- `apps/backend/internal/jira/store.go`
- `apps/backend/internal/jira/store_postgres_test.go`
- `apps/backend/internal/linear/store.go`
- `apps/backend/internal/linear/store_issue_watch.go`
- `apps/backend/internal/linear/store_postgres_test.go`
- `apps/backend/internal/sentry/store.go`
- `apps/backend/internal/sentry/store_issue_watch.go`
- `apps/backend/internal/sentry/store_postgres_test.go`

## Dependencies

- Task 03 completes the code-host store conversions.

## Risks

- Sentry uses guarded table rebuilds for multi-instance settings. PostgreSQL replay must not enter SQLite rebuild paths.

## Parallelism

`sequential`

## Inputs

- Jira, Linear, and Sentry store tests.
- `apps/backend/internal/integrations/AGENTS.md`

## Results

- Adapted Jira, Linear, and Sentry schema DDL and additive migrations to the
  active driver, including PostgreSQL timestamp and boolean forms.
- Reused shared schema probes and rebound store and transaction queries across
  configuration, health, watches, resets, and reservations.
- Replaced SQLite-only `INSERT OR IGNORE` reservation statements with
  portable `ON CONFLICT ... DO NOTHING` clauses.
- Kept the Jira/Linear singleton migrations and Sentry instance/watch rebuilds
  on SQLite, so PostgreSQL startup does not enter SQLite catalog or PRAGMA
  paths.
- Preserved integer-backed health flags with explicit CASE conversion and
  retained Sentry duplicate/FK error mapping on PostgreSQL.
- Added PostgreSQL fresh/replay coverage for settings, health, watches,
  instance binding, FK protection, and reservation operations in all three
  packages.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/jira ./internal/linear ./internal/sentry -run 'TestPostgresStore'
  -v` passed.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/jira ./internal/linear ./internal/sentry` passed.
