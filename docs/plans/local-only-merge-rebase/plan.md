---
spec: docs/specs/workspaces/requirements/local-repositories.md
created: 2026-08-22
status: implemented
---

# Implementation Plan: Local-only Merge and Rebase

## Overview

Issue [#2923](https://github.com/kdlbs/kandev/issues/2923) reports that Merge and Rebase fail in a local-only repository. Both operations always fetch `origin` before they use the base branch. The repair selects the target from repository state before it starts either history operation. It keeps the current remote behavior when `origin` exists.

**Confirmed root cause.** `GitOperator.Merge` and `GitOperator.Rebase` always run `git fetch origin <base>`. They then use `origin/<base>`. A repository without `origin` returns exit status 128 during the fetch. The operation returns before Git can use the valid local base branch.

---

## Backend

### Base-target preparation

Files:

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_base_target.go`

Add this shared method:

```go
func (g *GitOperator) prepareBaseBranchTarget(
    ctx context.Context,
    baseBranch string,
) (output string, target string, err error)
```

The method lists the configured Git remotes. If `origin` exists, it runs the current `git fetch origin <base>` command and returns `origin/<base>`. A fetch error keeps the current `failed to fetch base branch` message and does not fall back.

If `origin` does not exist, the method validates `refs/heads/<base>` with Git. It returns that fully qualified local ref when the branch exists. It returns `base branch "<base>" does not exist locally` when the branch is missing.

Update `GitOperator.Merge` and `GitOperator.Rebase` to use the returned target. Keep the current locking, conflict detection, rebase abort, and merge conflict behavior.

### Agentctl route coverage

Files:

- `apps/backend/internal/agentctl/server/process/git_base_target_test.go`
- `apps/backend/internal/agentctl/server/api/git_local_base_operations_test.go`

Add focused tests for target selection and the real HTTP route. Use temporary Git repositories and real Git commands.

---

## Frontend

No frontend source or wire contract changes. Desktop and mobile already send an unqualified base branch to the same agentctl operations.

### Mobile design contract

- Desktop entry: the existing Pull split-button menu.
- Mobile entry: the existing Git actions menu in the task top bar.
- Mobile exemplar: the existing Radix menu treatment in `apps/web/app/globals.css`.
- Shared logic: both entries call the existing Merge and Rebase handlers.
- Presentation: no layout, touch, scroll, or safe-area behavior changes.
- Parity proof: one desktop Merge scenario and one mobile Rebase scenario use a repository without `origin`.

---

## Tests

- **What:** target selection uses `origin/<base>` only when `origin` exists.
  **File:** `apps/backend/internal/agentctl/server/process/git_base_target_test.go`.
  **How:** use real temporary repositories for remote, local-only, and missing-local-branch cases.
- **What:** Merge and Rebase succeed through their HTTP routes with a local base branch and no `origin`.
  **File:** `apps/backend/internal/agentctl/server/api/git_local_base_operations_test.go`.
  **How:** advance local `main`, invoke each route from a feature branch, and prove that the base commit is reachable from `HEAD`.
- **What:** a missing local base branch returns a clear error and does not move `HEAD`.
  **File:** `apps/backend/internal/agentctl/server/api/git_local_base_operations_test.go`.
  **How:** invoke both routes with a missing branch and compare the result and commit before and after the request.
- **What:** remote behavior stays unchanged.
  **File:** `apps/backend/internal/agentctl/server/api/git_handlers_test.go`.
  **How:** retain `TestHandleGitRebase_ReplaysOntoBase` and `TestHandleGitMerge_BringsBaseCommitIn` as remote-path regression tests.

Targeted command:

```shell
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'TestPrepareBaseBranchTarget|TestHandleGit(Rebase|Merge)_(LocalOnly|MissingLocalBase|ReplaysOntoBase|BringsBaseCommitIn)' -count=1
```

---

## E2E Tests

- **Desktop Merge:** a task repository without `origin` has a newer local `main`. The user chooses Merge from the existing Pull menu. The success toast appears, and the local base commit becomes reachable from the task branch.
  **File:** `apps/web/e2e/tests/git/local-base-operations.spec.ts`.
- **Mobile Rebase:** the same repository shape uses the existing mobile Git actions menu. The user taps Rebase. The success toast appears, and the task branch moves onto local `main`.
  **File:** `apps/web/e2e/tests/git/mobile-local-base-operations.spec.ts`.
- Shared Git setup belongs in `apps/web/e2e/tests/git/local-base-operations-helpers.ts`. Cleanup restores the seeded repository remote and branch state.

Targeted commands:

```shell
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/git/local-base-operations.spec.ts -- --grep "merges a local base without origin"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-local-base-operations.spec.ts -- --grep "rebases a local base without origin"
```

---

## Public Documentation

Update the reference table and troubleshooting section in `docs/public/git-operations.md`. Document the local target behavior, the remote precedence rule, and the missing-local-branch error.

Targeted commands:

```shell
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

---

## Verification Results

- The new API regressions were run before the production change and failed as
  expected because the old implementation attempted to fetch `origin` in a
  local-only repository.
- Targeted backend verification passed: `Go test: 11 passed in 2 packages`.
- `pnpm install --frozen-lockfile` completed successfully from `apps`.
- Desktop Merge E2E passed in the managed Docker runner: 1 passed.
- Mobile Rebase E2E passed in the managed Docker runner: 1 passed.
- Public documentation validation passed: 61 tests and 41 published pages.
- The E2E cleanup restored the seeded repository remote and `main` branch after
  each scenario.
- PR fixup at head `c472ea8599f1e727b821a93d31fae2dbfad3d041` completed with 46
  checks passed, 0 failed, and 0 pending. The review findings corrected the
  missing-base scenario wording, preserved the underlying local-ref error, and
  aligned desktop task navigation with the mobile test.
- Fixup verification passed: the focused backend suite passed 11 tests, the
  desktop local-base E2E passed 1 test, and public docs validation passed 61
  tests with 41 published pages.

---

## Implementation Waves And Parallel Candidates

The default execution order is sequential in the primary conversation.

```text
Wave 1:
- [x] [task-01-resolve-local-base-targets](task-01-resolve-local-base-targets.md)

Wave 2:
- [x] [task-02-document-local-git-operations](task-02-document-local-git-operations.md)
```

Task 01 owns the regression tests, backend repair, and responsive workflow proof. Task 02 documents the final behavior. No task is marked parallel-safe.

## Risks And Out-of-scope Work

- Remote fetch errors must not trigger a local fallback. Such a fallback can merge stale local history.
- The repair does not change Pull, Push, or change-request creation.
- The repair does not change remote names or add an upstream-selection feature.
- The repair does not change the existing merge-conflict or rebase-abort policies.
