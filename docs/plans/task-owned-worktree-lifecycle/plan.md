---
spec: docs/specs/tasks/requirements/session-delete-resource-cleanup.md
decision: docs/decisions/2026-08-08-task-owned-worktree-lifetime.md
created: 2026-08-08
status: implemented
---

# Implementation Plan: Preserve Task-Owned Worktrees Across Session Deletion

## Overview

Redesign the closed PR #2421 around the actual ownership boundary: a task owns
its materialized worktrees, while sessions only reference them. Deleting a
session must remove its conversation and associations without invoking any
filesystem, Git worktree, or branch cleanup. Archive, delete, cascade, workspace
delete, quick-chat expiry, and explicit environment reset remain the only
physical cleanup owners and continue through the durable cleanup worker.

The persistence repair finishes the existing task-environment model instead of
adding another table. `task_environment_repos` becomes the sole physical
worktree record, `task_sessions.task_environment_id` is the only session link,
and `task_session_worktrees` plus deprecated flat environment worktree columns
are removed after a validated SQLite/PostgreSQL backfill. A durable task-lifecycle
preparation barrier closes the race between inventory capture and new
session/worktree creation without holding database locks while Git or filesystem
locks are held.

Implementation starts from `main`; do not carry forward the PR's
`session_delete` cleanup trigger/job, `resource_cleanup_session_delete.go`, or
`ReclaimSessionWorktree` physical cleanup path.

## Persistence and Worktree Store

Likely files:

- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`
- `apps/backend/internal/task/repository/sqlite/schema_replay_test.go`
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/worktree/manager.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/task/models/models.go`
- `apps/web/lib/api/domains/task-environment-api.ts`
- task-environment API consumers that still read deprecated flat fields

Extend `task_environment_repos` with the lifecycle fields needed by worktree
cleanup, then backfill it from existing environment-repository rows, deprecated
flat `task_environments` metadata, and `task_session_worktrees`. Sessions with a
valid environment retain it; otherwise normalize each connected legacy
worktree/session group under exactly one task-owned environment. Fail closed on
incompatible task, environment, repository, identity, path, or branch data.

In the same upgrade, validate the final ownership graph, rebuild/drop
`task_session_worktrees`, and rebuild/drop the deprecated
`task_environments.repository_id`, `worktree_id`, `worktree_path`, and
`worktree_branch` columns. SQLite and PostgreSQL must roll back the entire
normalization if validation fails. Fresh schema contains only the final tables.
There is no runtime dual-read/dual-write window.

Implement this as a dedicated migration that returns every error; do not use the
best-effort `MigrateLogger.Apply` path. Acquire the SQLite writer lock or a
PostgreSQL migration advisory lock, populate shadow tables, and compare the
complete old/new worktree identity, path, and branch sets before any schema swap.
Validate session-to-environment resolution, canonical owner uniqueness, row
counts, constraints, foreign keys, and the exact final column/index set inside
the transaction. Injected failures at every copy/validation/drop/rename boundary
must prove rollback restores the complete pre-upgrade schema and data.

SQLite's existing pre-upgrade `VACUUM INTO` snapshot is mandatory and remains
fatal on failure. PostgreSQL relies on transactional DDL plus the repository's
existing operator-managed backup policy. Its transaction takes a migration
advisory lock plus exclusive locks on the affected ownership tables, and aborts
on lock timeout. Deployment documentation must require mixed-version PostgreSQL
writers to stop before cutover and explain that downgrade restores the backup;
the clean final schema is not readable by an older binary. The migration never
touches a workspace directory or invokes Git, and repository initialization must
complete before the cleanup worker starts.

Defensively remove any preview-build cleanup jobs with trigger
`session_delete` before the worker starts, then remove that trigger identity and
all session-specific cleanup implementation. Preserve every task-lifecycle job.

Persist the environment, repository worktrees, and session environment link in
the owning transaction after physical materialization. If persistence is
rejected, remove only the just-created physical worktree as compensation. Move
task/session inventory, storage inventory, branch updates, API projections, and
live-path queries to the normalized environment-repository graph. Remove the
`TaskSessionWorktree` model/CRUD surface and the worktree store's duplicate schema
initializer.

## Task Lifecycle Barrier and Session Deletion

Likely files:

- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/repository/sqlite/resource_cleanup.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/task_environment.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_reuse.go`

Reserve a prepared `task_resource_cleanup_jobs` row before task resource
inventory is captured. Session creation and canonical environment-repository
persistence must serialize against the task row and reject creation while that
lifecycle barrier is active. PostgreSQL uses row locking; SQLite uses its
serialized writer transaction. Commit the barrier before acquiring target-path,
repository, or Git locks so the existing lock order is not inverted.

Keep `DeleteSession` limited to runtime quiescence, session-row deletion,
in-memory state cleanup, and primary-session promotion. Add a regression seam or
spy that proves the method cannot enqueue resource cleanup or call a physical
worktree cleaner, including when it deletes the final session.

## Durable Task Cleanup

Likely files:

- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/service_cleanup_inventory_test.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/backendapp/storage_inventory.go`
- `apps/backend/internal/automation/run_retention.go`
- `apps/backend/internal/office/infra/gc.go`

Change `gatherWorktreesForDelete` and related inventory to read
`task_environment_repos` through `task_environments.task_id`, so cleanup remains
possible after every session row is gone and after a backend restart. Snapshot
those handles into the durable job before archive/delete/cascade mutation.
Preserve the existing shared-environment and active-session checks, including
ownership transfer when another task borrows the environment. A transfer changes
the environment owner once; its repository worktrees follow automatically.

Continue to use `CleanupWorktrees`/`removeWorktree` only from task lifecycle
cleanup. Filesystem or Git failures remain retryable through the existing worker;
restart must reclaim pending jobs from their snapshots.

## Frontend, Mobile, and Documentation

Likely files:

- `apps/web/components/task/session-tab-menu.tsx`
- `apps/web/components/task/session-tab-menu.test.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.test.tsx`
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn}/task.json`
- `apps/web/e2e/tests/session/multi-session-ux.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-deletion.spec.ts`
- `docs/public/sessions-and-review.md`
- `docs/public/operations.md`
- `docs/public/k8s.md`

Keep the existing desktop AlertDialog and native mobile session-actions dialog.
Their destructive action deletes conversation history, so the confirmation copy
must explicitly state that the task workspace and files are retained. Do not add
uncommitted/unpushed-work fetches or warnings to either surface; those warnings
remain on task archive/delete cleanup dialogs.

Desktop and mobile retain the same action availability and confirmation
semantics. This is a copy/contract change, not a compressed desktop layout:
mobile keeps its existing actions-sheet entry point, viewport-safe AlertDialog,
and touch targets. Rendered component tests cover both surfaces and the existing
mobile Playwright flow verifies the native entry point.

Update public session documentation to distinguish ordinary session deletion
from task archive/delete. Do not change the separate Quick Chat documentation:
closing Quick Chat deletes its backing task and is therefore a task-lifecycle
cleanup operation. Update the operations and Kubernetes upgrade/rollback guides
with the clean-cutover backup, stopped-writer, single-initializer, and downgrade
restore requirements.

## Implementation Waves

Wave 1:

- [x] [task-01-task-worktree-persistence](task-01-task-worktree-persistence.md)

Wave 2:

- [x] [task-02-lifecycle-creation-barrier](task-02-lifecycle-creation-barrier.md)

Wave 3:

- [x] [task-03-task-owned-durable-cleanup](task-03-task-owned-durable-cleanup.md)

Wave 4:

- [x] [task-04-session-delete-contract-and-e2e](task-04-session-delete-contract-and-e2e.md)

## Verification

Backend focused tests:

```bash
cd apps/backend && go test ./internal/task/repository/... ./internal/worktree/...
cd apps/backend && go test ./internal/task/service/... ./internal/orchestrator/...
cd apps/backend && go test ./internal/backendapp/... ./internal/automation/... ./internal/office/infra/...
```

Migration safety tests must cover fresh, replay, legacy single-repository,
multi-repository, shared-environment, zero-session, preview-PR, ambiguous-data,
and injected-failure fixtures on both SQLite and PostgreSQL. PostgreSQL coverage
must run with a real isolated database; a skipped run is not release evidence.

PostgreSQL migration tests must run with the repository's configured PostgreSQL
test DSN in addition to SQLite. A skipped PostgreSQL run is not final evidence.

Frontend bootstrap and focused tests:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- components/task/session-tab-menu.test.tsx components/task/mobile/mobile-sessions-section.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm run i18n:ratchet && pnpm run i18n:check
```

Managed desktop and mobile E2E:

```bash
cd apps/web && pnpm e2e:run tests/session/multi-session-ux.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-deletion.spec.ts
```

Public documentation validation:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Risks

- Backfill ambiguity must stop the migration with diagnostic detail; silently
  choosing a path can leak or delete the wrong worktree later.
- The normalization must not use a helper that swallows unexpected migration
  errors. Every failure must abort repository initialization and roll back.
- PostgreSQL multi-instance startup must serialize through a database advisory
  and affected-table locks; process-local locking is insufficient.
- Canonical ownership must be persisted at every materialization path, including
  initial creation, resume/recreate, multi-repository additions, and mid-session
  branch materialization.
- The lifecycle barrier must cover session and worktree creation in PostgreSQL,
  not merely an in-process mutex or SQLite's single writer.
- Cleanup must not delete a worktree transferred to or actively borrowed by
  another task between preparation and execution.
- Physical compensation after a rejected environment-repository transaction must
  target only the just-created worktree and preserve pre-existing
  directories/registrations.
- Every legacy reader and writer must be removed in the normalization task;
  storage inventory, automation retention, branch updates, API projections, and
  orphan GC cannot ship with a fallback to the deleted table or columns.
- CI must assert that legacy table/type/trigger names occur only in the one-way
  migration and its fixtures, never in production runtime packages.
- Because obsolete schema is removed, rollback means restoring the pre-upgrade
  database backup; an old binary must never be started against the normalized
  database.

## Documentation Impact

The session-delete preservation spec, runtime-cleanup spec, and ownership ADR
define the durable contract. Public session documentation and localized
confirmation copy must state that deleting a session retains the task workspace,
while existing archive/delete warnings continue to describe physical cleanup.
