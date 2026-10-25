---
id: "06-workflow-execution-stores"
title: "Port workflow execution stores"
status: done
wave: 6
depends_on:
  - "05-azure-devops-store"
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

# Task 06: Port Workflow Execution Stores

## Summary

Make workflow sync and automation persistence portable. Cover settings, triggers, repositories, runs, and cleanup jobs on PostgreSQL.

## In scope

- Build schema statements with portable timestamp types and boolean defaults.
- Rebind workflow sync and automation queries.
- Replace SQLite-only conflict syntax.
- Add fresh, replay, and representative operation tests on PostgreSQL.

## Out of scope

- Change workflow synchronization behavior.
- Change automation scheduling or task creation.

## Acceptance

- Both stores initialize twice against their PostgreSQL schemas.
- Workflow sync settings and automation lifecycle operations succeed on PostgreSQL.
- Automation read transactions rebind expanded list queries after `sqlx.In`.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/workflowsync ./internal/automation -run 'TestPostgresStore' -v
```

## Files likely touched

- `apps/backend/internal/workflowsync/store.go`
- `apps/backend/internal/workflowsync/store_postgres_test.go`
- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/export_store.go`
- `apps/backend/internal/automation/run_retention.go`
- `apps/backend/internal/automation/store_postgres_test.go`

## Dependencies

- Task 05 completes provider store conversion.

## Risks

- Automation joins core task tables. Tests must use the real repository startup order for prerequisite schemas.

## Parallelism

`sequential`

## Inputs

- Existing workflow sync and automation store tests.
- `apps/backend/internal/backendapp/postgres_boot_test.go`

## Results

- Adapted workflow-sync and automation schema DDL to the active driver,
  including PostgreSQL timestamp and boolean forms.
- Rebound workflow-sync and automation reads, writes, transactions, dynamic
  run projections, and retention queries at the executing database boundary.
- Replaced the automation repository backfill's SQLite-only conflict syntax
  with a portable `ON CONFLICT ... DO NOTHING` clause.
- Preserved task-state joins and isolated-test fallbacks while using
  integer predicates for the shared `task_sessions.is_primary` column.
- Added PostgreSQL fresh/replay coverage for workflow settings, automation
  repositories, triggers, runs, run summaries, retention checks, and cleanup
  jobs.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/workflowsync ./internal/automation -run 'TestPostgresStore'
  -v` passed.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/workflowsync ./internal/automation` passed.
