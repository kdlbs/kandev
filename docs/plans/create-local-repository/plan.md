---
spec: docs/specs/workspaces/requirements/create-local-repository.md
created: 2026-07-21
updated: 2026-08-05
status: implemented
---

# Implementation Plan: Create a Local Repository During Task Creation

## Overview

The original implementation owns directory creation, Git initialization, and workspace repository
persistence, then merges the returned repository into the task-create selector on desktop and
mobile. This follow-up changes the initialization contract so the new repository has one empty root
commit on `main`; the existing branch endpoint can therefore return a real local branch and the
selector can offer it as a base without a frontend production-code change.

## Backend

### Initialization service

- Add `InitializeLocalRepositoryRequest` and `Service.InitializeLocalRepository` in
  `apps/backend/internal/task/service/service_requests.go` and a focused new
  `apps/backend/internal/task/service/local_repository_initialization.go`.
- Validate that `Name` is a single non-empty host path segment and `ParentPath` is an existing,
  writable absolute directory. Canonicalize the parent before joining the target, atomically create
  the absent target, and never modify an existing path.
- Run `git init --initial-branch=main`, then create one empty, unsigned initial commit on `main`
  without adding working-tree files. Use the existing classified Git subprocess seam and fixed
  Kandev commit identity so the operation does not depend on the host user's Git identity; both
  commands remain bound to the opened staging directory on descriptor-safe platforms.
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

### Branch selector follow-up

- Keep the existing repository and branch picker wiring unchanged. Once the backend creates
  `refs/heads/main`, `useBranches` and `Pill` will expose `main` as a selectable local option for
  the newly returned repository.
- Update the existing desktop and mobile creation E2E assertions to open the branch picker, verify
  that `main` is enabled and listed, and select it as the task's base branch.

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
  real Git to assert canonical path, a real `refs/heads/main` pointing at one empty initial commit,
  no user files in the working tree, the persisted workspace record, and the repository-created event.
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

- Desktop: update `apps/web/e2e/tests/task/create-task-new-local-repository.spec.ts`. Open **New
  Task**, create a repository under the isolated backend home, assert it is selected with `main`,
  open the branch picker and select the listed local `main`, submit the task, and verify the task is
  bound to the persisted repository, uses the existing direct local executor policy, and the
  filesystem has one empty commit with no user files. Keep the target-conflict case unchanged.
- Mobile: update `apps/web/e2e/tests/task/mobile-create-task-new-local-repository.spec.ts`. Enter
  through `MobileKanbanPage.mobileFab`, complete the same create-and-select outcome, open the branch
  picker and select `main`, and retain the Drawer containment, internal scroll, 44px action rows,
  safe-area footer, focus/dismiss, and zero-horizontal-overflow assertions.

## Implementation Waves

Wave 1:

- [x] [task-01-backend-initialization](task-01-backend-initialization.md) - done

Wave 2 (after Wave 1):

- [x] [task-02-task-create-selector](task-02-task-create-selector.md) - done

Wave 3 (after integrated backend and frontend):

- [x] [task-03-e2e-and-verification](task-03-e2e-and-verification.md) - done

Wave 4 (follow-up):

- [x] [task-04-real-main-branch](task-04-real-main-branch.md) - done

The frontend task is sequential because the shared picker, task-create handlers, and state wiring are
one behavior and edit overlapping files. E2E follows the integrated product path.

## Verification Results

Task 04 completed with the requested change-aware checks:

- `(cd apps && pnpm install --frozen-lockfile)` passed.
- `(cd apps/backend && go test ./internal/task/service ./internal/task/handlers)` passed.
- `(cd apps/backend && go test ./internal/task/gitinit)` passed.
- `(cd apps/web && pnpm e2e:run tests/task/create-task-new-local-repository.spec.ts)` passed (2 tests).
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-new-local-repository.spec.ts)` passed (1 test).
- Changed E2E files passed Prettier checks and `git diff --check` passed.

## Risks

- Filesystem and database persistence cannot share a transaction. The service must narrowly own and
  test partial-failure cleanup without ever deleting a pre-existing target.
- The initial commit changes the repository history contract. The commit must remain empty and use a
  deterministic, non-interactive identity so branch availability does not introduce user files,
  signing prompts, or a dependency on host Git configuration.
- The repository list may already be hydrated while the event arrives asynchronously. Selection must
  use the returned DTO directly and cache insertion must be idempotent.
- Nested overlays inside the full-height mobile task dialog can break scroll and focus ownership. The
  directory browser must render inside the creation Drawer rather than opening another phone popover.
