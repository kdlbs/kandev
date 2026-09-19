---
id: "04-real-main-branch"
title: "Create a real main branch"
status: done
wave: 4
depends_on: ["01-backend-initialization", "02-task-create-selector", "03-e2e-and-verification"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/create-local-repository.md"
---

# Task 04: Create a Real Main Branch

The original create-local-repository flow intentionally left Git at an unborn `main` HEAD. This
follow-up supersedes that narrow filesystem contract while preserving the existing path-trust,
cleanup, repository-cache, executor-selection, and responsive picker behavior.

## Acceptance

- A newly initialized repository contains no user files and exactly one empty initial commit
  reachable from a local `main` branch (`refs/heads/main`); the repository DTO and persisted
  `default_branch` remain `main`.
- The existing task-create branch picker receives the new repository's local `main` branch, keeps
  the branch control enabled, and lets the user select `main` as the base branch on desktop and on
  the existing mobile flow.
- Git initialization or initial-commit failure still leaves no persisted repository and performs the
  existing request-owned cleanup; existing conflict, retry, and mobile geometry coverage remains
  green.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers)
(cd apps/web && pnpm e2e:run tests/task/create-task-new-local-repository.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-new-local-repository.spec.ts)
git diff --check
```

The frontend E2E commands use the managed runner so the production Vite build and Go backend are
rebuilt before the branch-selector assertions. If the worktree already has dependencies installed,
the bootstrap command is a no-op.

## Files Likely Touched

- `apps/backend/internal/task/service/local_repository_initialization.go`
- `apps/backend/internal/task/service/local_repository_initialization_test.go`
- `apps/backend/internal/task/gitinit/command_descriptor.go`
- `apps/backend/internal/task/gitinit/command_descriptor_test.go`
- `apps/backend/internal/task/gitinit/command_path.go`
- `apps/backend/internal/task/gitinit/helper.go`
- `apps/web/e2e/tests/task/create-task-new-local-repository.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-new-local-repository.spec.ts`
- `docs/specs/workspaces/requirements/create-local-repository.md`
- `docs/plans/create-local-repository/plan.md`

## Dependencies

The original local-repository initialization and task-create selector implementation are already
landed. No new API route, database migration, frontend component, or responsive surface is needed.

## Inputs

- Spec: updated `What`, `Data Model`, `Failure Modes`, `Persistence Guarantees`, and `Scenarios`.
- Existing Git initialization: `apps/backend/internal/task/gitinit/` and
  `initializeGitRepository` in `local_repository_initialization.go`.
- Existing branch listing: `listGitBranches` in
  `apps/backend/internal/task/service/repository_discovery.go`.
- Existing selector wiring: `useBranches` in
  `apps/web/hooks/domains/workspace/use-repository-branches.ts` and the branch `Pill` in
  `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`.
- Mobile contract: existing Drawer and mobile E2E coverage in
  `mobile-create-task-new-local-repository.spec.ts`; no layout change is planned.

## Implementation Notes

- Keep `git init --initial-branch=main` on the existing descriptor-safe path.
- After initialization, run a classified lifecycle Git commit in the request-owned staging
  directory using `--allow-empty --no-verify`, `commit.gpgsign=false`, and fixed Kandev author/committer
  config. Do not create `.gitkeep`, README, or any other working-tree file.
- Update the service assertions and E2E helper names from “unborn/commitless” to a real local main
  branch and empty baseline commit. The E2E tests should explicitly open the branch picker and assert
  a selectable `main` option rather than relying only on the chip's prefilled text.

## Parallelism

Sequential. The backend behavior and both E2E assertions describe one cross-layer contract, and the
E2E tests share the existing creation flow.

## Output Contract

Report the exact Git command behavior, files changed, targeted backend and desktop/mobile E2E
results, `git diff --check`, cleanup evidence, and any platform-specific blockers. Before marking
this task done, reconcile the actual diff with Files Likely Touched and synchronize this task's
status, the plan checkbox, and `Verification Results` in `plan.md`.

## Results

- Workspace bootstrap: `(cd apps && pnpm install --frozen-lockfile)` passed.
- Backend service and handler tests: `(cd apps/backend && go test ./internal/task/service ./internal/task/handlers)` passed.
- Descriptor helper tests: `(cd apps/backend && go test ./internal/task/gitinit)` passed.
- Desktop Playwright: `(cd apps/web && pnpm e2e:run tests/task/create-task-new-local-repository.spec.ts)` passed, 2 tests.
- Mobile Playwright: `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-new-local-repository.spec.ts)` passed, 1 test.
- Changed E2E files passed Prettier formatting checks.
- `git diff --check` passed.
- Cleanup, persistence, conflict/retry, descriptor replacement, empty-tree, fixed-identity, and no-user-file assertions passed.
