---
id: "03-gitlab-store"
title: "Port the GitLab store"
status: done
wave: 3
depends_on:
  - "02-github-store"
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

# Task 03: Port the GitLab Store

## Summary

Make GitLab task and merge-request persistence portable. Add PostgreSQL coverage for settings, watches, links, and automation state.

## In scope

- Build GitLab schema statements with portable types and boolean defaults.
- Replace local schema probes with shared helpers.
- Rebind GitLab store queries and transaction queries.
- Add fresh, replay, and representative operation tests on PostgreSQL.

## Out of scope

- Change GitLab provider clients or host selection.
- Change merge-request lifecycle behavior.

## Acceptance

- `gitlab.NewStore` succeeds twice against one PostgreSQL schema.
- PostgreSQL operations cover settings, watches, task links, and automation state.
- GitLab store calls do not send unresolved source placeholders to PostgreSQL.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/gitlab -run 'TestPostgresStore' -v
```

## Files likely touched

- `apps/backend/internal/gitlab/store.go`
- `apps/backend/internal/gitlab/store_config.go`
- `apps/backend/internal/gitlab/store_e2e_reset.go`
- `apps/backend/internal/gitlab/store_mr_automation.go`
- `apps/backend/internal/gitlab/store_task_cleanup.go`
- `apps/backend/internal/gitlab/store_task_mr_link.go`
- `apps/backend/internal/gitlab/store_watch_reset.go`
- `apps/backend/internal/gitlab/store_watches.go`
- `apps/backend/internal/gitlab/store_postgres_test.go`

## Dependencies

- Task 02 establishes the code-host portability pattern.

## Risks

- GitLab stores numeric switches beside boolean columns. Preserve the current model type of each column.

## Parallelism

`sequential`

## Inputs

- `apps/backend/internal/gitlab/store.go`
- Existing GitLab store tests for each listed concept.

## Results

- Adapted GitLab schema DDL and idempotent migrations to the active driver,
  including PostgreSQL timestamp and boolean forms.
- Reused shared schema probes and rebound GitLab settings, task-MR, watch,
  reservation, reset, cleanup, mention-scope, and automation queries.
- Preserved integer-backed `last_ok` and `draft` columns with explicit CASE
  conversion while using boolean literals for PostgreSQL boolean columns.
- Kept the SQLite-only MR-watch table rebuild on SQLite and skipped it on
  PostgreSQL.
- Added PostgreSQL fresh/replay coverage for settings, mention scope, task
  links, automation checkpoints, MR watches, review/issue watches, dedup
  reservations, and action presets.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/gitlab -run 'TestPostgresStore' -v` passed.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/gitlab` passed.
