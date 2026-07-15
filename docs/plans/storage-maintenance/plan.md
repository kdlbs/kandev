---
spec: docs/specs/system-page/storage-maintenance.md
created: 2026-07-14
status: done
---

# Implementation Plan: Storage Maintenance

## Overview

Build one install-wide storage service under `internal/system/storage`, backed by durable settings,
run history, quarantine records, and task cleanup intents. Land persistence and activity admission
first, then implement ownership-specific providers, expose the System API, and finish with a
responsive Storage settings page and desktop/mobile/container E2E coverage. The existing
`office/infra` sweep is retired only after the replacement workspace reconciler is wired.

The implementation preserves PR #1687's unarchive contract: archived filesystem payload may be
reclaimed, historical worktree rows and branch metadata remain, and pending archive cleanup is
cancelled when a task becomes active again.

---

## Backend

### Durable task lifecycle cleanup

- Add `task_resource_cleanup_jobs` creation and replay-safe indexes in
  `apps/backend/internal/task/repository/sqlite/base_schema.go` and
  `base_migrations.go`, plus repository operations in a new
  `resource_cleanup.go`.
- Define the serialized cleanup snapshot and job state in
  `apps/backend/internal/task/models/resource_cleanup.go`.
- Replace `Service.runAsyncTaskCleanup` in
  `apps/backend/internal/task/service/service_tasks.go` with enqueue/claim/retry logic in new
  `resource_cleanup_jobs.go` and `resource_cleanup_worker.go` files. Capture cleanup handles before
  archive/delete/cascade mutations and keep the worker independent of the optional maintenance
  scheduler.
- Check archived state before each archive-job destructive step. Wire unarchive in
  `handoff_cascade.go` to cancel non-terminal archive jobs before publishing the restored task.
- Preserve `task_session_worktrees` history for archived tasks. Delete snapshots carry paths and
  runtime handles that would otherwise disappear through foreign-key cascades.

### Persistent storage contracts

- Create `apps/backend/internal/system/storage/` with `types.go`, `settings.go`, `store.go`, and
  `store_migrations.go`.
- Store `StorageMaintenanceSettings` as JSON under the existing `settings` key
  `storage_maintenance`, following `internal/system/metrics/store.go`.
- Create `storage_maintenance_runs` and `storage_quarantine_entries` with replay-safe SQLite and
  Postgres DDL. Normalize settings with the ranges and cross-field dedicated-Docker validation in
  the spec.
- Persist every analysis/cleanup run and retain the newest 20 terminal rows plus all non-terminal
  rows.

### Resource activity coordinator

- Add a neutral package at `apps/backend/internal/agent/runtime/activity/` with a process-wide
  `Coordinator`, cancellable maintenance lease, active-resource snapshot, and quiet-period clock.
- Instrument lifecycle launch/prepare/stop and agent turn execution in
  `internal/agent/runtime/lifecycle/`; instrument task shell/process requests in the agentctl client
  or API boundary; instrument worktree setup/cleanup scripts and Docker image builds.
- A task-resource lease cancels a maintenance lease and waits for provider cancellation before
  admission. Maintenance acquisition rechecks active leases after locking to close the idle-check
  race.
- Scheduled acquisition enforces the configured quiet period. Manual Run now skips that elapsed-idle
  check while retaining the same current-activity and mutual-exclusion checks.
- Expose the same coordinator to the storage scheduler through backend composition.

### Workspace inventory and quarantine

- Implement `internal/system/storage/workspaces/` with an authoritative `Inventory` interface,
  analyzer, candidate classifier, quarantine mover, restore operation, permanent deletion, and
  startup reconciliation.
- Extend task/worktree repository queries to return protected paths from active worktrees,
  `TaskEnvironment.WorkspacePath`, active execution metadata, and repository-less task paths.
  Normalize paths and protect every ancestor beneath `<KANDEV_HOME_DIR>/tasks`.
- Update task-root creation in lifecycle/worktree preparation to write an atomic ownership marker.
  Support both semantic multi-repo roots and `<workspace-id>/<task-id>` scratch roots.
- Move candidates using same-filesystem rename into `<KANDEV_HOME_DIR>/trash/tasks`, persist the
  quarantine record, and reconcile marker/DB disagreement after restart. Never follow symlinks or
  operate outside the configured roots.
- Add a `WorkspaceRecovery` hook to the merged PR #1687 unarchive path in
  `task_http_handlers.go`: restore a task-matched quarantine entry before
  `RecoverTaskBranches`, report `restored|not_found|failed`, and leave branch recovery unchanged.
- Remove `internal/office/infra/gc.go` startup ownership after parity tests prove the System
  provider protects all live layouts. Do not leave both periodic sweepers enabled.

### Managed Go build cache

- Implement `internal/system/storage/gocache/` analysis and cleanup for
  `<KANDEV_HOME_DIR>/cache/go-build`, including ownership marker, containment validation,
  quarantine rotation, recreation, and optional confirmed adoption of an external absolute path.
- Inject the managed `GOCACHE` into host-local executor, setup/cleanup script, shell, and agent process
  environments when `go_cache.enabled=true`. Centralize this in lifecycle environment construction
  so profile values cannot create inconsistent paths within one execution.
- Treat the configured byte maximum as an idle-time cleanup trigger, not a quota. Never discover
  and delete `$HOME/.cache/go-build` implicitly.
- Keep disabled-cache cleanup out of scheduled and global manual runs. Dispatch the provider's
  explicit cleanup path only for a non-empty manual resource selection containing `go_cache`.

### Docker storage provider

- Extend `internal/agent/docker/client.go` with typed usage and prune methods backed by the Docker
  SDK: Kandev-labeled container inventory/removal, build-cache usage/prune filters, and unused-image
  usage/removal filters.
- Implement `internal/system/storage/dockerstore/` as an optional provider. Kandev container cleanup
  requires label and positive task/runtime inventory evidence; it never removes running containers.
- Include exactly labeled managed-container count and writable-layer bytes in read-only analysis,
  independently degrading Docker usage errors.
- Revalidate `dedicated_daemon_acknowledged` immediately before build-cache or image deletion.
  Do not expose daemon-wide volume/network prune operations.
- Report unavailable Docker capability without failing other storage providers.

### Scheduler, service, and API

- Add `service.go`, `scheduler.go`, `runner.go`, and `handler.go` under
  `internal/system/storage/`. The runner executes selected providers independently, aggregates
  bytes/counts/warnings, and mirrors persistent run state into the existing System jobs tracker.
- Start the scheduler from `internal/system/system.go:StartBackground` only when persisted settings
  enable it. The first run waits a full interval; busy intervals create `skipped_busy` history.
- Register the spec routes under `/api/v1/system/storage`, including confirmed Go-cache adoption,
  settings, analyze, manual run, run history, quarantine restore, and confirmed permanent delete.
- Wire settings store, task cleanup worker, lifecycle activity coordinator, worktree/task inventory,
  Docker client, and unarchive recovery in `internal/backendapp` composition.
- Add health issues for invalid persisted settings, retry-exhausted cleanup jobs, failed quarantine
  entries, and unavailable explicitly-enabled providers.

---

## Frontend

### Route and navigation

- Add `apps/web/app/settings/system/storage/page.tsx` and a **Storage** leaf to
  `components/app-sidebar/sections/settings/system-group.tsx`.
- Reuse `SystemPageShell`; hydrate initial storage state through the existing Go-served SPA/API
  conventions rather than fetching directly from components.

### API, types, state, and hooks

- Add storage contracts to `apps/web/lib/types/system.ts` and API calls to
  `apps/web/lib/api/domains/system-api.ts`.
- Extend the system Zustand slice and WS job handling for persisted storage runs, capabilities,
  settings, summary, and quarantine entries.
- Add `hooks/domains/system/use-storage-maintenance.ts` to own loading, save, analyze, run, restore,
  delete, polling fallback, and feedback state.

### Responsive Storage page

- Add components beneath `apps/web/components/settings/system/storage/`:
  `storage-summary-card.tsx`, `maintenance-settings-card.tsx`,
  `storage-resource-card.tsx`, `maintenance-history-card.tsx`,
  `quarantine-card.tsx`, and confirmation dialogs.
- Desktop uses a two-column summary/settings grid followed by full-width resource, quarantine, and
  history cards. Mobile stacks every card, wraps action bars, renders resource/history rows as
  vertical key/value blocks, and keeps all primary actions at least 44 px high.
- The dedicated-Docker acknowledgment and external Go-cache adoption use explicit warning dialogs.
  Permanent quarantine deletion requires typing `DELETE`; restore remains a normal confirm action.
- No required action is hover-only. Long paths wrap, byte counts remain aligned, and the page does
  not introduce horizontal document scrolling at Pixel 5 width.
- Show quarantine count/bytes and managed-container count/writable bytes in the analysis accordion,
  and expose a responsive, threshold/ownership-gated **Clean Go cache** action on the Go-cache row.

---

## Tests

- **What:** task archive/delete persists cleanup before mutation, retries across restart, captures
  handles before FK cascades, and cancels on unarchive.
  **File:** `apps/backend/internal/task/service/resource_cleanup_worker_test.go` and
  `internal/task/repository/sqlite/resource_cleanup_test.go`.
  **How:** table-driven service tests plus real SQLite replay tests; include a backend-restart worker
  reconstruction test.
- **What:** settings defaults, range validation, dedicated-Docker cross-field validation, run
  persistence/retention, and quarantine state transitions.
  **File:** `apps/backend/internal/system/storage/settings_test.go` and `store_test.go`.
  **How:** table-driven normalization plus SQLite and env-gated Postgres replay tests.
- **What:** activity admission closes the idle race, task work cancels maintenance, scheduled runs
  enforce the quiet period, and manual runs gate only on current activity or another maintenance run.
  **File:** `apps/backend/internal/agent/runtime/activity/coordinator_test.go`.
  **How:** `testing/synctest` tests with explicit lease channels; no sleeps.
- **What:** live semantic roots, live scratch roots, multi-repo ancestors, inventory errors,
  symlinks, grace periods, quarantine reconciliation, restore conflicts, and permanent deletion.
  **File:** `apps/backend/internal/system/storage/workspaces/provider_test.go`.
  **How:** table-driven filesystem tests under `t.TempDir()` with fake complete/error inventories.
- **What:** ownership-marker creation for every local task layout.
  **File:** lifecycle and worktree preparation tests beside the existing layout tests.
  **How:** real temporary worktree and repository-less launch fixtures.
- **What:** unarchive cancels pending cleanup, restores task-matched quarantine first, preserves
  historical worktree records, and reports restore failures without blocking unarchive.
  **File:** `apps/backend/internal/task/service/handoff_unarchive_test.go`,
  `task/handlers/task_http_handlers_test.go`, and storage workspace integration tests.
  **How:** service integration tests aligned with PR #1687 branch-recovery cases.
- **What:** managed Go path injection is consistent across processes; cleanup only rotates marked
  owned paths above threshold; unadopted `/root/.cache/go-build` is untouched.
  **File:** `apps/backend/internal/system/storage/gocache/provider_test.go` and lifecycle env tests.
  **How:** temporary filesystem tests and environment-construction unit tests.
- **What:** Docker provider scopes Kandev containers by label, keeps running/unrelated containers,
  rejects global cleanup without acknowledgment, and applies age/storage filters.
  **File:** `apps/backend/internal/system/storage/dockerstore/provider_test.go` and
  `internal/agent/docker/client_test.go`.
  **How:** mocked SDK unit tests plus an env-gated real-Docker integration test.
- **What:** handler-to-service-to-store contracts for every Storage endpoint, including `409` busy,
  `400` validation, async job IDs, and `DELETE` confirmation.
  **File:** `apps/backend/internal/system/storage/handler_test.go`.
  **How:** Gin integration tests using a real SQLite store and fake providers.
- **What:** frontend API shapes, settings state, capability warnings, action feedback, responsive row
  rendering, and typed confirmation state.
  **File:** colocated `*.test.ts(x)` files under `lib/api/domains`, `lib/state/slices/system`, and
  `components/settings/system/storage`.
  **How:** Vitest API mocks and Testing Library interaction tests.

Verification for backend tasks starts with focused package tests, then runs:

```bash
make -C apps/backend fmt
make -C apps/backend test
make -C apps/backend lint
```

Frontend verification runs:

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps && pnpm --filter @kandev/web test
```

---

## E2E Tests

- **Scenario:** scheduling defaults off; analyze reports an orphan task directory with
  `node_modules`; manual run quarantines it; restore returns it to the original path.
  **File:** `apps/web/e2e/tests/system/storage-maintenance.spec.ts`.
  **What to verify:** settings persist after reload, job progress/result renders, quarantine row
  appears, byte counts change, and restore succeeds through the UI.
- **Scenario:** a busy task blocks manual maintenance and scheduled maintenance records
  `skipped_busy` without moving its workspace.
  **File:** `apps/web/e2e/tests/system/storage-maintenance.spec.ts`.
  **What to verify:** visible busy feedback and unchanged task files.
- **Scenario:** an archived task with a quarantined workspace is unarchived.
  **File:** `apps/web/e2e/tests/task/unarchive-storage-recovery.spec.ts`.
  **What to verify:** workspace recovery is reported before branch recovery and the task can launch
  from the restored path.
- **Scenario:** the complete Storage workflow works through the mobile settings sheet.
  **File:** `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`.
  **What to verify:** navigate from the mobile sheet, analyze, expand resource details, save safe
  settings, and restore a quarantine entry without horizontal page scrolling.
- **Scenario:** real Docker analysis and Kandev-labeled stopped-container cleanup do not remove an
  unrelated container; build-cache cleanup remains gated by acknowledgment.
  **File:** `apps/web/e2e/tests/docker/storage-maintenance.spec.ts` using `docker-test-base`.
  **What to verify:** labeled orphan removed, unrelated/running containers retained, global action
  disabled until acknowledgment.

Run focused UI tests through the managed production-build runner:

```bash
cd apps/web && pnpm e2e:run tests/system/storage-maintenance.spec.ts
cd apps/web && pnpm e2e:run tests/system/mobile-storage-maintenance.spec.ts -- --project=mobile-chrome
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers tests/docker/storage-maintenance.spec.ts
```

---

## Implementation Waves

Wave 1 (parallel foundations):

- [x] [Task 01 — Durable task cleanup](task-01-durable-task-cleanup.md)
- [x] [Task 02 — Storage persistence contracts](task-02-storage-persistence.md)

Wave 2 (parallel providers after Task 02; activity also depends on Task 01):

- [x] [Task 03 — Activity gate and scheduler core](task-03-activity-gate-scheduler.md)
- [x] [Task 04 — Workspace quarantine and unarchive](task-04-workspace-quarantine.md)
- [x] [Task 05 — Managed Go build cache](task-05-managed-go-cache.md)
- [x] [Task 06 — Docker storage provider](task-06-docker-storage.md)

Wave 3:

- [x] [Task 07 — System storage API and composition](task-07-system-storage-api.md)

Wave 4:

- [x] [Task 08 — Responsive Storage settings UI](task-08-storage-settings-ui.md)

Wave 5:

- [x] [Task 09 — E2E, QA, and final verification](task-09-e2e-qa-verification.md)
