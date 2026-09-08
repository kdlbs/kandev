---
spec: docs/specs/workspaces/requirements/worktree-branch-templates.md
created: 2026-08-15
status: complete
---

# Implementation Plan: Preserve Worktree Branch Templates

## Overview

Issue [#2611](https://github.com/kdlbs/kandev/issues/2611) occurs because the
template backfill runs on every schema replay. The replay replaces each saved
template with a value derived from the legacy prefix. The repair limits the
backfill to empty values and adds SQLite and Postgres migration tests.

## Confirmed Root Cause

`MigrateLogger.Apply` runs successful data statements during every backend startup.
The `repositories.worktree_branch_template.backfill` statement has no condition, so
each startup updates every repository row.

The column migration also gives legacy rows a non-empty default. A safe repair must
use an empty migration default before it conditionally fills legacy rows.

## Backend

### Repository schema migration

- Update `apps/backend/internal/task/repository/sqlite/base_migrations.go`.
- Add `worktree_branch_template` to a legacy table with an empty default.
- Backfill only rows whose template is null, empty, or whitespace.
- Derive the value from the trimmed legacy prefix.
- Use `feature/` when the legacy prefix is empty.
- Keep the fresh-schema default in `base_schema.go` unchanged.

This sequence preserves old branch naming during the first upgrade. Later schema
replays leave user values unchanged.

## Tests

- **What:** A custom template survives repeated schema migration calls on SQLite.
  **File:** `apps/backend/internal/task/repository/sqlite/worktree_branch_template_migration_test.go`.
  **How:** Save a repository with a custom value, replay `runMigrations`, and read
  the row from the real SQLite store.
- **What:** A legacy SQLite row derives its template once from its prefix.
  **File:** `apps/backend/internal/task/repository/sqlite/worktree_branch_template_migration_test.go`.
  **How:** Remove the template column, seed non-empty and empty prefix cases, run
  the migration twice, and compare both stored values.
- **What:** Postgres has the same legacy upgrade and replay behavior.
  **File:** `apps/backend/internal/task/repository/sqlite/worktree_branch_template_migration_postgres_test.go`.
  **How:** Use `testutil.OpenIsolatedPostgres`, repeat the legacy and custom-value
  cases, and gate the test with `KANDEV_TEST_POSTGRES_DSN`.
- **What:** Existing repository creation and update behavior stays unchanged.
  **File:** `apps/backend/internal/task/service/service_test.go`.
  **How:** Run the existing default-template and update-template service tests.

## Verification Results

- RED: the focused SQLite migration command failed on
  `TestWorktreeBranchTemplateMigrationPreservesCustomValue` because replay
  replaced the custom template with `feature/{title}-{suffix}`.
- GREEN: `cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run 'Test(WorktreeBranchTemplateMigration|PostgresWorktreeBranchTemplateMigration)' -count=1` passed its two SQLite tests; the Postgres test was skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- Service checks: `cd apps/backend && go test -tags fts5 ./internal/task/service -run 'TestService_(CreateRepository_DefaultWorktreeBranchTemplate|UpdateRepository_WorktreeBranchTemplate)' -count=1` passed both tests.
- `git diff --check` passed.
- Fixup: the custom-template regression now replays `runMigrations()` twice;
  the migration and service checks passed again after this test-only change.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-preserve-branch-template](task-01-preserve-branch-template.md)

There are no parallel-safe tasks. The migration and its tests share one schema
boundary.

## Open Questions

None.
