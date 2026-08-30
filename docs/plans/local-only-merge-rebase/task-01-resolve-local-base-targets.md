---
id: "01-resolve-local-base-targets"
title: "Resolve local base targets"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/local-repositories.md"
---

# Task 01: Resolve local base targets

## Intent

Make Merge and Rebase use a verified local base branch when the repository has no `origin` remote. Preserve the current remote path when `origin` exists.

## Inputs

- Spec: `docs/specs/workspaces/requirements/local-repositories.md`, especially What, Failure Modes, and Scenarios.
- Plan: `docs/plans/local-only-merge-rebase/plan.md`, Backend and Tests.
- Root cause: `GitOperator.Merge` and `GitOperator.Rebase` always fetch `origin` before they select the base target.
- Existing remote integration tests: `TestHandleGitRebase_ReplaysOntoBase` and `TestHandleGitMerge_BringsBaseCommitIn`.

## Acceptance

- Merge and Rebase use `refs/heads/<base>` when `origin` is not configured and that local branch exists.
- Repositories with `origin` keep the current remote behavior. Missing local branches return a clear error and leave `HEAD` unchanged.
- Desktop completes Merge from the Pull menu. Mobile completes Rebase from the Git actions menu.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_base_target.go`
- `apps/backend/internal/agentctl/server/process/git_base_target_test.go`
- `apps/backend/internal/agentctl/server/api/git_local_base_operations_test.go`
- `apps/web/e2e/tests/git/local-base-operations-helpers.ts`
- `apps/web/e2e/tests/git/local-base-operations.spec.ts`
- `apps/web/e2e/tests/git/mobile-local-base-operations.spec.ts`

## Verification

Follow TDD. Run the new regression tests before the production change and record the expected failures. Then run:

```shell
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'TestPrepareBaseBranchTarget|TestHandleGit(Rebase|Merge)_(LocalOnly|MissingLocalBase|ReplaysOntoBase|BringsBaseCommitIn)' -count=1
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/git/local-base-operations.spec.ts -- --grep "merges a local base without origin"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-local-base-operations.spec.ts -- --grep "rebases a local base without origin"
```

## Dependencies

None.

## Parallelism

`sequential`. This task owns the shared Git target-selection behavior.

## Risks

- Detect remote presence before a fetch. Do not classify every fetch error as a missing remote.
- Keep branch-name validation before target construction.
- Keep the existing operation lock and conflict cleanup policies.
- Restore the seeded E2E repository remote and branch state after each browser scenario.
- Use causal waits for the operation response. Do not use an arbitrary delay.

## Mobile design contract

- Entry: use the current task-top-bar Git actions menu.
- Exemplar: use the current responsive Radix menu treatment.
- Hierarchy: keep Merge and Rebase as existing menu actions.
- Surface: no new drawer, route, or responsive branch.
- Scroll and safe area: unchanged because the menu composition does not change.
- Shared logic: desktop and mobile use the same agentctl request contract.

## Output contract

Report the selected remote/local decision boundary, changed files, test results, cleanup evidence, and remaining risks. Update this task and `plan.md` in the same conversation.

## Results

Implemented the shared base-target decision boundary.

- Added remote-aware target preparation. Repositories with `origin` retain the
  fetch and `origin/<base>` path. Local-only repositories validate and use
  `refs/heads/<base>`.
- A missing local base returns `base branch "<base>" does not exist locally`
  before history changes. Remote fetch failures do not fall back to a local
  branch.
- Added process and HTTP route regressions for remote, local-only, missing-base,
  Merge, and Rebase behavior.
- Added managed desktop Merge and mobile Rebase E2E coverage with repository
  cleanup.
- Red: the local-only route regressions failed against the old unconditional
  `origin` fetch. Green: the targeted backend suite passed 11 tests in 2
  packages.
- Desktop and mobile focused E2E scenarios each passed with 1 test.

PR fixup:

- The missing-local-base error now keeps the clear public prefix and wraps the
  underlying Git verification error for diagnostics.
- The desktop E2E opens the created task by ID, matching the mobile helper and
  avoiding duplicate-title ambiguity.
- Fixup red: the new underlying-error assertions failed 4 tests before the
  production change. Fixup green: the focused backend suite passed 11 tests and
  the desktop E2E passed 1 test.
