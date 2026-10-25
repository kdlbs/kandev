---
id: "05-azure-devops-store"
title: "Port the Azure DevOps store"
status: done
wave: 5
depends_on:
  - "04-issue-intake-stores"
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

# Task 05: Port the Azure DevOps Store

## Summary

Make Azure DevOps settings, task links, and watcher persistence portable. Add PostgreSQL coverage for each stored surface.

## In scope

- Build schema statements with portable timestamp types and boolean defaults.
- Replace local schema probes with shared helpers.
- Rebind settings, task-link, watcher, reset, and reservation queries.
- Add fresh, replay, and representative operation tests on PostgreSQL.

## Out of scope

- Change Azure DevOps REST behavior.
- Change board or work-item models.

## Acceptance

- `azuredevops.NewStore` succeeds twice against one PostgreSQL schema.
- Settings, task links, watches, resets, and reservations succeed on PostgreSQL.
- Azure DevOps store calls do not send unresolved source placeholders to PostgreSQL.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/azuredevops -run 'TestPostgresStore' -v
```

## Files likely touched

- `apps/backend/internal/azuredevops/store.go`
- `apps/backend/internal/azuredevops/store_task_pr.go`
- `apps/backend/internal/azuredevops/store_task_work_item.go`
- `apps/backend/internal/azuredevops/store_watch_reset.go`
- `apps/backend/internal/azuredevops/watch_store.go`
- `apps/backend/internal/azuredevops/store_postgres_test.go`

## Dependencies

- Task 04 supplies the watcher-store portability pattern.

## Risks

- Watch reset transactions combine dynamic table names and placeholders. Rebind only values, not validated identifiers.

## Parallelism

`sequential`

## Inputs

- `docs/specs/integrations/system-design/azure-devops-integration-01.md`
- Existing Azure DevOps store tests.

## Results

- Adapted Azure DevOps schema DDL and additive migrations to the active
  driver, including PostgreSQL timestamp and boolean forms.
- Reused shared schema probes and rebound Azure DevOps settings, saved-view,
  task-link, watcher, reservation, and reset queries at their database or
  transaction boundary.
- Preserved validated dynamic table identifiers while rebinding only query
  values in watcher reset transactions.
- Added PostgreSQL fresh/replay coverage for configuration health, saved
  views, settings, task PR/work-item links, watcher reservations, assignment,
  reset, workspace deletion, and cleanup operations.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/azuredevops -run 'TestPostgresStore' -v` passed.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/azuredevops` passed.
