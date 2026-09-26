---
spec: docs/specs/integrations/requirements/pr-outcome-attribution.md
created: 2026-08-13
status: implemented
---

# Implementation Plan: Pull request outcome attribution

## Overview

Persist five facts that GitHub already provides during the existing pull
request syncs. The change is backend-first: add nullable storage and activation
metadata, source the fields from each client, reconcile them in the existing
sync writer, and expose the values in the shared frontend type. No core UI or
mutation endpoint is part of this feature.

## Backend

### Schema and activation

`apps/backend/internal/persistence/meta.go` provides exported read and
write-once helpers for `kandev_meta`.

`apps/backend/internal/github/store.go`:

- Declares `is_draft`, `changed_files`, `merged_by_login`,
  `closed_by_login`, and `auto_merge_observed_at` as nullable columns.
- Adds missing columns with the fail-loud migration path.
- Records `github_task_pr_outcome_activated_at` once after all columns exist.
- Includes the columns in every explicit projection and row writer.
- Carries the columns through the legacy multi-repository table rebuild.

`apps/backend/internal/github/models.go` keeps nullable pointers on `TaskPR`
and presence flags on the upstream `PR` and `PRStatus` models.

### Upstream field sourcing

- `graphql.go` requests the shared fields and the latest closed-event actor.
- `gh_client.go` requests the fields available from the gh CLI.
- `pat_client.go` decodes the fields available from the REST pull-request API.
- `client_helpers.go` marks full single-PR reads as populated.
- No-op and partial reads leave populated flags false.

### Sync writer

`service_pr_watch.go` applies populated observations, preserves stored values
for unpopulated reads, and compares nullable values before publishing an event.
The auto-merge timestamp is latched with a SQL `COALESCE` guard.

`ReplaceTaskPR` and `RestoreTaskPR` resolve outcome fields inside their write
transactions. The sync path re-reads the committed row before publishing.
Unwatched terminal reconciliation uses the same rules.

## Frontend

`apps/web/lib/types/github.ts` exposes the five nullable fields on `TaskPR`.
No component, store slice, API mutation, or translation is added because the
core product has no screen for these observations.

## Tests

- Migration replay and activation: `store_task_pr_outcome_migration_test.go`.
- Column-list and rebuild coverage: `store_taskpr_schema_drift_test.go` and
  `store_task_pr_detach_test.go`.
- GraphQL, gh, REST, and presence flags: the corresponding client tests.
- Sync, latch, unwatched reconciliation, and writer health:
  `service_pr_outcome_sync_test.go`, `service_pr_unwatched_test.go`,
  `service_pr_writer_health_test.go`, and detach tests.
- Meta helper semantics: `internal/persistence/meta_test.go`.

The implementation is complete. CI provides backend and frontend verification;
there is no browser or mobile surface to exercise.
