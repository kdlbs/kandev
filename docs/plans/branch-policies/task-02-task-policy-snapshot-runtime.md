---
id: "02-task-policy-snapshot-runtime"
title: "Snapshot and apply task branch policies"
status: complete
wave: 2
depends_on:
  - "01-branch-policy-persistence-api"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-004
  - REQ-WORKSPACES-BRANCH-POLICIES-005
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-POLICIES-004.1
  - AC-WORKSPACES-BRANCH-POLICIES-004.2
  - AC-WORKSPACES-BRANCH-POLICIES-004.3
  - AC-WORKSPACES-BRANCH-POLICIES-004.4
  - AC-WORKSPACES-BRANCH-POLICIES-004.5
  - AC-WORKSPACES-BRANCH-POLICIES-004.6
  - AC-WORKSPACES-BRANCH-POLICIES-005.4
system_design: "../../specs/workspaces/system-design/branch-policies.md"
---

# Task 02: Snapshot and apply task branch policies

## Objective

Resolve a policy during task creation, persist its immutable task-repository
snapshot, and use that snapshot for every branch-producing runtime path and
pull-request default.

## TDD contract

Start with failing service and runtime tests for successful resolution, stale
and cross-repository IDs, atomic failure, policy edits after task creation,
legacy fallback, local fresh branch naming, title rename, and PR target choice.

## Implementation scope

- Add optional `branch_policy_id` to task-repository create inputs.
- Add replayable `task_repositories` snapshot columns for policy ID, name,
  worktree template, and pull-request target.
- Resolve and authorize the policy inside task creation; copy the policy base
  and snapshot fields without trusting browser-derived configuration.
- Centralize effective task worktree-template resolution and update orchestrator,
  lifecycle, local fresh-branch, and title-generated rename paths to use it.
- Project the pull-request target to task/session clients and use it as the
  desktop/mobile default before falling back to base branch.
- Inject the snapshotted target into first-launch and context-reset agent
  instructions. Give passthrough agents the same instruction as plain text.
- Preserve raw-selection and legacy task behavior.

## Implementation acceptance

- Task creation either persists a complete authorized snapshot or persists no
  task.
- Every named runtime branch consumer uses snapshot-first, fallback-second
  resolution.
- Existing task inputs and rows continue to produce their current branches and
  pull-request bases.
- Policy-backed agents receive the immutable target. Raw-branch agents receive
  no policy-target instruction.

## Exclusions

- Repository policy CRUD and Gitflow seeding.
- Task-create selector presentation.
- Automated merge or backport behavior.

## Files likely touched

- `apps/backend/internal/task/models.go`
- `apps/backend/internal/task/dto/`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/task/repository/postgres/`
- `apps/backend/internal/task/service/`
- `apps/backend/internal/orchestrator/`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/worktree/`

## Verification

- `cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service -run 'Test.*BranchPolic(y|ies)|Test.*PolicySnapshot'`
- `cd apps/backend && go test ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/... -run 'Test.*BranchPolic(y|ies)|Test.*TitleBranch|Test.*PullRequestTarget'`

## Dependencies and parallelism

Depends on Task 01. Sequential by default because it consumes and may refine
the policy DTO and repository interface introduced there.

## Output contract

Report the RED tests, snapshot precedence, every updated runtime consumer,
migration compatibility, exact test results, changed files, and remaining
risks. Mark this task and the plan checkbox complete together.

## Results

Implemented policy-ID resolution at task creation, repository ownership checks,
immutable task-repository snapshots, snapshot-first branch-template resolution,
local fresh-branch and title-rename consumption, executor resume behavior, and
desktop/mobile pull-request target projection with raw-branch fallback.

Verification:

- Focused task service, handler, and SQLite tests passed: 61 tests.
- `go test ./internal/backendapp ./internal/orchestrator ./internal/orchestrator/executor -run 'TestTaskRepositoryBranchTemplate|TestRenderTitleBranchName|TestBranchMaterializer|TestBuildRepoSpecs|TestResolveTaskRepoInfo' -count=1` passed.
- The full backend validation listed in Task 01 passed.
- First-launch, stored-message, context-reset, Office, and passthrough prompt
  paths now use the snapshotted pull-request target.
- RED tests failed before the prompt wiring existed. The affected orchestrator,
  message-handler, and system-prompt packages passed 2,813 tests after the fix.

Review remediation verification:

- Remote contribution policy/base mismatches are rejected before task-row
  insertion, and the regression test confirms no orphan task remains.
- Fresh-branch repository replacement preserves the generated effective branch
  while retaining the immutable policy snapshot.
- Task-created and task-updated event regression tests include all policy
  snapshot fields, timestamps, and metadata.
- `go test ./internal/task/service ./internal/task/handlers` passed: 1,877
  tests across both packages.
- `make -C apps/backend test` passed with `KANDEV_INTERNAL_CONFIG_FILE` and
  `KANDEV_HOME_DIR` cleared so the host user's external config cannot alter
  test defaults. Backend lint also passed with zero issues.
