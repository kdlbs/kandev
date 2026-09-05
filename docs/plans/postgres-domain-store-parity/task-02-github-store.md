---
id: "02-github-store"
title: "Port the GitHub store"
status: done
wave: 2
depends_on:
  - "01-portable-schema-helpers"
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

# Task 02: Port the GitHub Store

## Summary

Make GitHub schema initialization and store operations portable. Add PostgreSQL coverage for authentication, settings, watches, and task pull requests.

## In scope

- Build GitHub schema statements with portable types and boolean defaults.
- Replace local schema probes with shared helpers.
- Rebind GitHub store queries and transaction queries.
- Replace SQLite-only conflict syntax.
- Add fresh, replay, and representative operation tests on PostgreSQL.

## Out of scope

- Change GitHub API clients or authentication behavior.
- Change table names or stored models.

## Acceptance

- `github.NewStore` succeeds twice against one PostgreSQL schema.
- PostgreSQL operations cover workspace connections, watches, task pull requests, and app registrations.
- GitHub store calls do not send unresolved source placeholders to PostgreSQL.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/github -run 'TestPostgresStore' -v
```

## Files likely touched

- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_connections.go`
- `apps/backend/internal/github/deployment_app_store.go`
- `apps/backend/internal/github/store_task_cleanup.go`
- `apps/backend/internal/github/store_task_pr_ownership.go`
- `apps/backend/internal/github/webhook_service.go`
- `apps/backend/internal/github/store_postgres_test.go`

## Dependencies

- Task 01 supplies schema helpers.

## Risks

- GitHub has SQLite table rebuilds for historical schemas. Their SQLite behavior must remain unchanged.

## Parallelism

`sequential`

## Inputs

- `apps/backend/internal/github/store.go`
- Existing GitHub store tests for each listed concept.

## Results

- Adapted GitHub schema DDL and migrations to the active driver, including
  PostgreSQL timestamp and boolean forms plus replayable registration triggers.
- Reused shared schema probes and rebound GitHub store, transaction, settings,
  watch, task PR, authentication, webhook, CI, and statistics queries.
- Replaced SQLite conflict statements with portable `ON CONFLICT` clauses and
  preserved the personal/workspace GitHub App registration invariants on both
  SQLite and PostgreSQL.
- Added PostgreSQL fresh/replay coverage for connections, personal auth,
  settings, watches, task PRs, CI automation, auth flows, statistics, and
  trigger enforcement.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/github -run '^TestPostgresStore' -v` passed.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/github` passed.
