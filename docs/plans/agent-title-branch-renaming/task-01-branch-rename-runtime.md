---
id: "01-branch-rename-runtime"
title: "Branch rename runtime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/agent-generated-titles.md"
---

# Task 01: Branch rename runtime

## Acceptance

- The orchestrator identifies only the title-owner session's Kandev-generated repository branches;
  explicit `checkout_branch` bindings and all Local-executor branches produce preservation outcomes
  without invoking Git.
- Eligible names reuse the repository's configured branch template and the final task-title context,
  and Git renames are scoped to the correct repository subpath in multi-repository workspaces.
- Each successful rename synchronously updates the repository-scoped session worktree and applicable
  environment/running/live snapshots. Individual failures leave their old snapshots intact and do not
  stop other eligible repositories.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite ./internal/worktree -run 'Test.*(TaskTitleBranch|Rename.*Branch|BranchName)'
```

## Files likely touched

- `apps/backend/internal/orchestrator/task_title_branch.go`
- `apps/backend/internal/orchestrator/task_title_branch_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_branch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_branch_test.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- focused SQLite repository tests
- `apps/backend/internal/worktree/config.go` and tests, only if a shared final-title renderer helper is
  needed

## Dependencies

None. Reuse the shipped title-owner metadata, branch-template renderer, agentctl Git rename, and
repository-scoped worktree models.

## Parallelism

Sequential in the primary conversation. The runtime contract and persistence behavior are inputs to
the MCP integration.

## Inputs

- Spec sections: **What**, **Data model**, **Failure modes**, **Persistence guarantees**, and branch
  scenarios.
- ADR 0032 branch-template contract and the existing Changes-panel branch rename implementation.
- `TaskRepository.CheckoutBranch`, `TaskSessionWorktree.RepositoryID`, and task-environment repository
  rows.

## Risks

- Direct remote checkouts may use a suffixed local branch when the head branch is already checked out;
  eligibility must use persisted `checkout_branch`, not compare current branch strings.
- Partial multi-repository success cannot be transactional with Git. Persist each successful rename
  before continuing and report failures without guessing rollback behavior.
- Legacy primary snapshots must not be overwritten for a non-primary repository.

## Results

- Implemented the lifecycle `GitRenameBranch` seam, primary running-row/metadata update, and
  repository/worktree-scoped SQLite snapshot writers.
- Implemented orchestrator branch ownership matching, template rendering, deterministic executor-specific
  suffixes, manual-branch/remote-checkout/Local preservation, mixed multi-repository outcomes, and
  surfaced snapshot failures. Branch-switch events now scope snapshot updates to the tagged
  repository/worktree.
- Exact verification: `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/repository/sqlite ./internal/worktree -run 'Test.*(TaskTitleBranch|Rename.*Branch|BranchName)'` — 31 passed in 4 packages.
- Affected-package verification: `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp ./internal/task/repository/sqlite` — 3469 passed in 6 packages.
- Focused race verification: `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/mcp/handlers -run 'Test.*(TaskTitleBranch|RenameBranchForSession|SetTaskTitle|HandleBranchSwitched)'` — 11 passed in 3 packages.
- `make -C apps/backend lint` — golangci-lint reported 0 issues.
- Git operations remain local to the owning execution; no remote branch or task directory is renamed.
