---
id: "01-plugin-remote-turn-change-checkouts"
title: "Project plugin remote turn-change checkout subpaths"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-TASKS-TURN-CHANGES-002
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-002.4
  - AC-TASKS-TURN-CHANGES-002.1
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 01: Project plugin remote turn-change checkout subpaths

## Summary

Pass the resolved executor type into `turnChangeCheckoutsFromWorkspaceRepositories` from both lifecycle call sites so `plugin_remote` turn-change checkouts carry the materialized `<repo>-<branch>` workspace subpath instead of the bare repository name. Keep every other executor type byte-identical.

## In scope

- Update `turnChangeCheckoutsFromWorkspaceRepositories(repositories []WorkspaceRepositorySpec, executorType string) []TurnChangeCheckout` in `apps/backend/internal/agent/runtime/lifecycle/turn_changes.go`:
  - When `executorType == string(models.ExecutorTypePluginRemote)`, compute the workspace key via `launchRepositoryProjectionKey(&LaunchRequest{ExecutorType: executorType}, RepoLaunchSpec{RepoName: repository.RepoName, BaseBranch: repository.BaseBranch, CheckoutBranch: repository.CheckoutBranch, BranchSlug: repository.BranchSlug}, index)`.
  - When `err == nil`, set `RepositorySubpath = key` and `RepositorySubpathKnown = true`.
  - When `err != nil`, keep `repository.RepositorySubpath` and `repository.RepositorySubpathKnown`.
  - For every other executor type, keep `repository.RepositorySubpath` and `repository.RepositorySubpathKnown` unchanged.
- Update both call sites:
  - `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go` (`initializeCreatedExecution`): pass `info.ExecutorType`, falling back to `execution.ExecutorType` when empty.
  - `apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go` (`populateLaunchTurnChangeCheckouts`): pass `info.ExecutorType`, falling back to `execution.ExecutorType` when empty.
- Add focused regression tests in `apps/backend/internal/agent/runtime/lifecycle/turn_changes_plugin_remote_test.go`.

## Out of scope

- Changing `turnChangeCheckoutsFromWorkspaceRepositories` behavior for non-plugin remote executors (`k8s`, `local_docker`, `remote_docker`, `sprites`, `ssh`).
- Changing `internal/task/changes/coordinator.go` scope matching or `service_turns.go` `WorkspaceRepositorySpec` fields.
- Changing base-commit capture or `agentctl.Client.GetGitStatusWithDetails` (owned by Task 02).

## Acceptance

1. For `plugin_remote` single-repository and multi-repository workspaces (both `BaseBranch`-only and `CheckoutBranch`-overridden entries, including sanitized repo names), `turnChangeCheckoutsFromWorkspaceRepositories` produces `RepositorySubpath` values identical to `remoteWorkspaceProjectionFromLaunch(req, true)` destinations and `RepositorySubpathKnown == true`.
2. When `plugin_remote` has no safe branch-scoped directory (for example, both `BaseBranch` and `CheckoutBranch` are empty), `turnChangeCheckoutsFromWorkspaceRepositories` preserves the spec's recorded `RepositorySubpath` and `RepositorySubpathKnown`.
3. Every non-plugin executor type (`""`, `local`, `worktree`, `k8s`, `local_docker`, `remote_docker`, `sprites`, `ssh`) produces byte-identical `TurnChangeCheckout` slices, and both `initializeCreatedExecution` and `populateLaunchTurnChangeCheckouts` populate projected subpaths for `plugin_remote` sessions.

## Verification

Write the regression tests first and confirm they fail before the fix (producing the bare repository name instead of `<repo>-<branch>`). After implementing the fix, run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestTurnChangeCheckoutsPluginRemote|TestPopulateLaunchTurnChangeCheckouts|TestLaunchTurnChangeCheckouts|TestPromptAdmissionRefreshesTurnChangeCheckouts' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/lifecycle/turn_changes.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_execution.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go \
  apps/backend/internal/agent/runtime/lifecycle/turn_changes_plugin_remote_test.go
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/turn_changes.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go`
- `apps/backend/internal/agent/runtime/lifecycle/turn_changes_plugin_remote_test.go` (new)

## Dependencies

None.

## Risks

- A `WorkspaceInfo` returned by a test fake or partial provider may omit `info.ExecutorType` while `execution.ExecutorType` is set; falling back to `execution.ExecutorType` prevents silently reverting to bare subpaths.
- Keep new tests in a dedicated file rather than appending to `manager_turn_changes_test.go` to avoid file-length lint limits.

## Parallelism

`sequential`

## Inputs

- [Fix plan](plan.md), Defect 1 root cause and technical approach.
- [Remote executor plugins system design](../../specs/executors/system-design/remote-executor-plugins.md).
- [Turn changed files system design](../../specs/tasks/system-design/turn-changed-files.md), Admission and terminal fences / Start.
- Existing projection pattern in `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` (`comparisonTargetsFromWorkspaceRepositories`) and `workspace_materialization.go` (`launchRepositoryProjectionKey`, `remoteWorkspaceProjectionFromLaunch`).

## Results

Implemented plugin-remote checkout scope projection through `launchRepositoryProjectionKey` at both lifecycle call sites. Recorded subpaths remain the fallback when no safe branch-scoped key exists; other executor types keep their recorded subpaths.

- Red: the focused race test failed because `owner-service` did not match the materialized `owner-service-main` scope.
- Green: `(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'TestTurnChangeCheckoutsPluginRemote|TestPopulateLaunchTurnChangeCheckouts|TestLaunchTurnChangeCheckouts|TestPromptAdmissionRefreshesTurnChangeCheckouts' -count=1)` passed.
- Package: `(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -count=1)` passed in 303.7 seconds.
- Lint: `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/turn_changes.go apps/backend/internal/agent/runtime/lifecycle/manager_execution.go apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes.go apps/backend/internal/agent/runtime/lifecycle/turn_changes_plugin_remote_test.go` passed with 0 issues.
