---
spec: docs/specs/create-local-repository/spec.md
created: 2026-07-21
status: completed
---

# Implementation Plan: Create a Local Repository During Task Creation

## Overview

Add an explicit backend operation that owns directory creation, commitless Git initialization, and
workspace repository persistence. Then add a selector action that opens a shared directory-browser
form, merges the returned repository into workspace state, selects it in the originating task row,
and moves the first task to a compatible direct local executor. Desktop and mobile use the same
mutation/state logic with purpose-built Dialog and Drawer shells.

## Backend

### Initialization service

- Add `InitializeLocalRepositoryRequest` and `Service.InitializeLocalRepository` in
  `apps/backend/internal/task/service/service_requests.go` and a focused new
  `apps/backend/internal/task/service/local_repository_initialization.go`.
- Validate that `Name` is a single non-empty host path segment and `ParentPath` is an existing,
  writable absolute directory. Canonicalize the parent before joining the target, atomically create
  the absent target, and never modify an existing path.
- Run `git init --initial-branch=main` without creating files, configuring an author, or committing.
- Register the canonical target through the existing `CreateRepository` path with `source_type=local`
  and `default_branch=main`, preserving validation and `repository.created` publication.
- On a partial failure, leave no repository row and remove only the target created by this request
  when cleanup is safe; log cleanup failure without masking the primary error.

### HTTP contract

- Register `POST /api/v1/workspaces/:id/repositories/initialize-local` in
  `apps/backend/internal/task/handlers/repository_handlers.go`.
- Bind `{name,parent_path}`, verify workspace ownership/existence before filesystem mutation, map
  validation to `400`, target existence to `409`, and return the existing repository DTO with `201`.
- Add handler coverage in `apps/backend/internal/task/handlers/repository_handlers_test.go`.

## Frontend

### API and shared directory browser

- Add `initializeLocalRepository(workspaceId, { name, parentPath })` to
  `apps/web/lib/api/domains/workspace-api.ts`, returning `Repository`.
- Refactor `apps/web/components/folder-picker.tsx` so its directory-listing, breadcrumb, loading,
  error, navigation, and current-folder selection are exported as a reusable browser body. Keep the
  existing **None** mode trigger behavior unchanged.
- Add focused tests for target-path derivation, validation, error retention, and successful API
  response handling in `apps/web/components/create-local-repository-surface.test.tsx`.

### Repository selector and creation surface

- Extend the cmdk-only `Pill` contract in `apps/web/components/task-create-dialog-pill.tsx` with an
  optional command action rendered as a `CommandItem`; do not insert arbitrary interactive markup
  into `Command`. Cover keyboard and pointer activation in
  `apps/web/components/task-create-dialog-pill.test.tsx`.
- Add `apps/web/components/create-local-repository-surface.tsx`. It owns shared form state and renders
  a desktop `Dialog` or mobile `Drawer` selected with `useResponsiveBreakpoint`.
- Thread a task-create-only **Create new repository** action through
  `apps/web/components/task-create-dialog-repo-chips.tsx` and
  `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`. Expose it only for a single
  repository row. Quick Chat callers of `WorkspaceRepoChips` do not receive the opt-in action.
- Add a repository-created handler in `apps/web/components/task-create-dialog-handlers.ts` and its
  prop/type wiring in `apps/web/components/task-create-dialog-types.ts`,
  `apps/web/components/task-create-dialog-prop-builders.ts`, and
  `apps/web/components/task-create-dialog.tsx`. It directly patches the originating row with the new
  repository ID and `main`, selects a compatible direct local executor/profile, updates task-create
  last-used state, and leaves sibling rows untouched. Surface the automatic executor change in the
  form; block confirmation without filesystem mutation when no direct local profile is available.
- Merge the returned DTO into `repositories.itemsByWorkspaceId[workspaceId]` through the existing
  workspace slice without waiting for an asynchronous refetch; deduplicate by repository ID.
- Extend `apps/web/components/task-create-dialog-repo-chips.test.tsx` and
  `apps/web/components/task-create-dialog-handlers.test.ts` for action visibility, row targeting,
  cache merge, success, conflict, retry, and cancel behavior.

## Mobile Design Contract

- Desktop keeps the repository search popover and opens a compact creation Dialog from its visible
  command action.
- Mobile keeps the repository selector entry point but opens an inset bottom Drawer modeled on
  `apps/web/components/task/mobile/mobile-picker-sheet.tsx`.
- The drawer has fixed name/target context, one internally scrolling directory list, and a fixed
  safe-area-aware primary footer. Directory and action rows are at least 44px high.
- Shared form state, validation, listing requests, creation request, cache merge, row selection, and
  direct-local executor selection are viewport-independent. Dismissal creates nothing and returns
  focus to the originating selector.
- The mobile E2E proves creation and selection, internal scrolling/viewport containment, touch target
  size, safe-area footer visibility, and absence of document horizontal overflow.

## Tests

- **Initialization success:**
  `apps/backend/internal/task/service/local_repository_initialization_test.go` uses `t.TempDir()` and
  real Git to assert canonical path, unborn `main`, the absence of `HEAD` commits, persisted workspace
  record, and repository-created event.
- **Input and conflict safety:** the same service test table covers invalid names, relative/missing/
  non-directory parents, existing empty/non-empty targets, and no mutation or persistence.
- **Partial failure cleanup:** service tests inject or induce Git/persistence failures and assert no
  repository row plus request-owned target cleanup behavior.
- **HTTP mapping:** `apps/backend/internal/task/handlers/repository_handlers_test.go` covers `201`,
  `400`, `404`, and `409`, including no filesystem mutation for an unknown workspace.
- **Frontend form:** `apps/web/components/create-local-repository-surface.test.tsx` covers validation,
  target preview, in-flight disablement, error retention/retry, success callback, and responsive shell.
- **Selector wiring:** existing pill/repo-chip/handler tests prove the action is task-create-only, a
  returned repository updates only the originating row and active-workspace cache, a worktree
  selection changes to a direct local profile, missing local profiles prevent the mutation, and the
  action is absent from multi-repository tasks.

## E2E Tests

- Desktop: add `apps/web/e2e/tests/task/create-task-new-local-repository.spec.ts`. Open **New Task**,
  create a repository under the isolated backend home, assert it is selected with `main`, submit the
  task, and verify the task is bound to the persisted repository, uses a direct local executor, and
  the filesystem has no commit. Add a target-conflict case that proves the existing directory remains
  unchanged.
- Mobile: add `apps/web/e2e/tests/task/mobile-create-task-new-local-repository.spec.ts`. Enter through
  `MobileKanbanPage.mobileFab`, complete the same create-and-submit outcome, and assert Drawer
  containment, a scrollable internal list, 44px action rows, visible safe-area footer, focus/dismiss
  behavior, and zero document horizontal overflow.

## Implementation Waves

Wave 1:

- [x] [task-01-backend-initialization](task-01-backend-initialization.md) - done

Wave 2 (after Wave 1):

- [x] [task-02-task-create-selector](task-02-task-create-selector.md) - done

Wave 3 (after integrated backend and frontend):

- [x] [task-03-e2e-and-verification](task-03-e2e-and-verification.md) - done

The frontend task is sequential because the shared picker, task-create handlers, and state wiring are
one behavior and edit overlapping files. E2E follows the integrated product path.

## Verification

```bash
make fmt
cd apps/backend && go test ./internal/task/service ./internal/task/handlers
cd apps && pnpm --filter @kandev/web test -- --run \
  components/create-local-repository-surface.test.tsx \
  components/task-create-dialog-pill.test.tsx \
  components/task-create-dialog-repo-chips.test.tsx \
  components/task-create-dialog-handlers.test.ts
cd apps/web && pnpm e2e:run tests/task/create-task-new-local-repository.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome \
  tests/task/mobile-create-task-new-local-repository.spec.ts
make typecheck test lint
```

## Risks

- Filesystem and database persistence cannot share a transaction. The service must narrowly own and
  test partial-failure cleanup without ever deleting a pre-existing target.
- An unborn Git repository cannot support the existing branch/worktree flow. The form must switch
  the first task to a direct local executor and must not silently fall back to worktree, container, or
  remote execution.
- The repository list may already be hydrated while the event arrives asynchronously. Selection must
  use the returned DTO directly and cache insertion must be idempotent.
- Nested overlays inside the full-height mobile task dialog can break scroll and focus ownership. The
  directory browser must render inside the creation Drawer rather than opening another phone popover.
