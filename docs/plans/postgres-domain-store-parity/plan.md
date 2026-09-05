---
created: 2026-09-03
status: done
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
legacy_specs: []
---

# Implementation Plan: PostgreSQL Domain Store Parity

## Overview

This work restores PostgreSQL support for eight built-in domain stores. Shared dialect helpers land first, followed by focused provider conversions and a boot regression.

The order keeps each store reviewable. Each store gets real PostgreSQL schema replay and operation coverage before the final boot test changes.

## Scope

### In scope

- Make schema types and schema introspection portable.
- Rebind all affected store queries before execution.
- Replace SQLite-only conflict and boolean syntax in shared paths.
- Add PostgreSQL tests for schema replay and representative store operations.
- Make the PostgreSQL boot test detect unavailable integration and automation stores.

### Out of scope

- Provider API behavior and authentication changes.
- Active-active PostgreSQL support.
- A new migration framework.
- Frontend behavior changes.

## Confirmed root cause

The eight stores use SQLite schema types, catalog queries, conflict syntax, or unresolved `?` placeholders. PostgreSQL rejects these statements.

Provider construction errors are nonfatal. Therefore, the existing boot test returns success while services or stores remain unavailable.

## Technical approach

### Shared database helpers

Add portable timestamp and schema-introspection helpers under `internal/db`. Reuse the active `sqlx` driver name and current PostgreSQL schema.

### Provider stores

Build the schema for each store with its active driver. Replace local `PRAGMA` and `sqlite_master` reads with the shared helpers.

Pass every query with source `?` placeholders through the `Rebind` method of the executor. Rebind after `sqlx.In` expands a query.

Use portable `ON CONFLICT` clauses and boolean literals. Keep existing data models, table names, and provider behavior.

### Startup regression

Extend the PostgreSQL boot test beyond the outer `provideServices` error. Assert every affected service or direct store boundary is available.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.1` | Package `TestPostgresStoreSchemaReplay` tests and `TestPostgresBootInitializesRepositories` |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.2` | Each package initializes the same isolated PostgreSQL schema twice and reads preserved rows |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3` | Representative PostgreSQL create, read, update, list, and remove operations in each store package |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.4` | Existing provider initialization error tests and startup log paths |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.5` | Full PostgreSQL service-graph test plus provider store operation tests |

## E2E tests

No browser test is required. The defect occurs before provider HTTP behavior, and the PostgreSQL boot test covers the complete backend construction boundary.

## Work orders

- [x] [Task 01: Add portable schema helpers](task-01-portable-schema-helpers.md)
- [x] [Task 02: Port the GitHub store](task-02-github-store.md)
- [x] [Task 03: Port the GitLab store](task-03-gitlab-store.md)
- [x] [Task 04: Port issue intake stores](task-04-issue-intake-stores.md)
- [x] [Task 05: Port the Azure DevOps store](task-05-azure-devops-store.md)
- [x] [Task 06: Port workflow execution stores](task-06-workflow-execution-stores.md)
- [x] [Task 07: Harden PostgreSQL boot coverage](task-07-postgres-boot-coverage.md)

## Verification results

- Task 01: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/db/... -run 'Test.*(TimestampType|TableExists|TableColumns)' -v`
  passed.
- Task 02: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/github -run '^TestPostgresStore' -v` passed.
- Task 02: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/github` passed.
- Task 03: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/gitlab -run '^TestPostgresStore' -v` passed.
- Task 03 review regression: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go
  test -race ./internal/gitlab -run
  'TestPostgresStore|TestMigrateMRWatchUniqueKey_PostgresLegacy' -v` passed.
- Task 03: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/gitlab` passed.
- Task 04: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/jira ./internal/linear ./internal/sentry -run '^TestPostgresStore'
  -v` passed.
- Task 04: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/jira ./internal/linear ./internal/sentry` passed.
- Task 05: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/azuredevops -run '^TestPostgresStore' -v` passed.
- Task 05: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/azuredevops` passed.
- Task 06: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/workflowsync ./internal/automation -run '^TestPostgresStore'
  -v` passed.
- Task 06: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/workflowsync ./internal/automation` passed.
- Review regression: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test
  -race ./internal/automation ./internal/github ./internal/gitlab -run
  'TestPostgresStore|TestMigrateMRWatchUniqueKey_PostgresLegacy' -v` passed.
- Final affected-package race run across `internal/db/...`, GitHub, GitLab,
  Jira, Linear, Sentry, Azure DevOps, workflow sync, automation, and
  `backendapp` passed after the review fixes.

## Risks

- GitHub and GitLab contain many store methods. A missed query can pass schema tests and fail during later provider activity.
- Boolean columns currently mix boolean and integer expressions. PostgreSQL rejects comparisons between these types.
- SQLite table rebuilds can lose constraints if a portability change alters their statement order.
