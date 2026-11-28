---
created: 2026-11-27
status: complete
requirements:
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/platform/system-design/workspace-git-capture-recovery.md
legacy_specs: []
---

# Fix Plan: Plugin remote turn-change checkouts and base-commit capture

## Overview

Fix [issue #5554](https://github.com/kdlbs/kandev/issues/5554), where `plugin_remote` sessions always settle turn-change capture as `unavailable` / `checkout_unavailable` with 0 files, never persist a session `base_commit_sha`, and return `{"cumulative_diff": null}` on `session.cumulative_diff`.

Issue #5554 is assigned to `carlosflorencio`.

Deliver the two independent backend fixes sequentially in two small work orders:

1. Project `plugin_remote` turn-change checkout subpaths through `launchRepositoryProjectionKey` so checkout scopes match the `<repo>-<branch>` directories materialized by `agentctl`.
2. Add a single-non-root-scope fallback in `agentctl.Client.GetGitStatusWithDetails` when the root tracker returns `repository_unavailable`, restoring base-commit capture and the `session.cumulative_diff` fallback without extra requests on root-repository layouts.

Both work orders touch disjoint packages (`internal/agent/runtime/lifecycle` vs `internal/agent/runtime/agentctl` + `internal/agent/handlers`) and have no ordering dependency between them. Execute 01 then 02 sequentially by default.

## Evidence and root cause

Inspected directly in the workspace against the current `main` code paths cited in issue #5554:

### Defect 1: Turn-change checkouts carry bare repo names while `plugin_remote` materializes `<repo>-<branch>`

1. `reconcileLaunchWorkspace` (`apps/backend/internal/agent/runtime/lifecycle/retained_launch_workspace.go:10-29`) sets `pluginRemote := req.ExecutorType == string(models.ExecutorTypePluginRemote)` and passes `includePrimary = true` to `remoteWorkspaceProjectionFromLaunch` (`workspace_materialization.go:36-65`). Every `plugin_remote` repository, including index `0`, is cloned into `remoteWorkspaceRepositoryDestination(spec)` = `<sanitized-repo>-<sanitized-branch>` (`workspace_materialization.go:67-84`).
2. Because the workspace root (`/workspace`) is not a Git repository, `agentctl`'s `Manager.RepositoryScopes()` (`apps/backend/internal/agentctl/server/process/manager.go:1785-1802`) omits `""` and returns only `["<sanitized-repo>-<sanitized-branch>", ...]`.
3. `turnChangeCheckoutsFromWorkspaceRepositories` (`apps/backend/internal/agent/runtime/lifecycle/turn_changes.go:61-75`) does not accept `executorType` and copies `repository.RepositorySubpath` verbatim from `WorkspaceRepositorySpec`, which `Service.workspaceRepositorySpec` (`apps/backend/internal/task/service/service_turns.go:1359`) sets to the bare sanitized repository name `projection.repoName`.
4. Both callers — `Manager.initializeCreatedExecution` (`manager_execution.go:1401`) and `Manager.populateLaunchTurnChangeCheckouts` (`manager_turn_changes.go:74`) — pass only `info.WorkspaceRepositories`.
5. During start capture (`internal/task/changes/coordinator_start_capture.go:58-67`), `checkoutScope` (`internal/task/changes/coordinator.go:723-742`) short-circuits to `""` only when `""` is present in `scopeSet`. Without a root scope, it checks `scopeSet[checkout.RepositorySubpath]`, which misses (`"<repo>"` != `"<repo>-<branch>"`), so every repository row settles as `unavailable` / `checkout_unavailable` with `checkout "<id>" is absent from registered repository scopes`.
6. Other launch projection helpers (`comparisonTargetsFromWorkspaceRepositories` in `manager_launch.go:498-527`, `collectBaseBranches` in `manager_launch.go:623-657`, `collectRemoteContributions`, `collectContributionDestinations`, and `shipRemoteCopyfilesForLaunch`) already project `plugin_remote` specs through `launchRepositoryProjectionKey`.

Smallest reproduction: build a `WorkspaceInfo` with `ExecutorType: string(models.ExecutorTypePluginRemote)` and one repository `{RepoName: "owner-repo", BaseBranch: "main", RepositorySubpath: "owner-repo", RepositorySubpathKnown: true}`, run `populateLaunchTurnChangeCheckouts`, and observe `TurnChangeCheckouts[0].RepositorySubpath == "owner-repo"` instead of `"owner-repo-main"` (causing `changes.Coordinator.Admit` against `scopes = ["owner-repo-main"]` to mark the row `checkout_unavailable`).

### Defect 2: Base-commit capture and cumulative-diff fallback read only the bare workspace root

1. `Executor.captureBaseCommit` (`apps/backend/internal/orchestrator/executor/executor_execute.go:3723`) calls `lifecycleAdapter.GetGitStatusWithDetails` (`apps/backend/internal/backendapp/adapters.go:1321-1332`), which calls `agentClient.GetGitStatusWithDetails(ctx)` (`apps/backend/internal/agent/runtime/agentctl/git.go:753-766`).
2. `GitHandlers.computeCumulativeDiff` (`apps/backend/internal/agent/handlers/git_handlers.go:1016`), `GitHandlers.computeGitCommits` (`git_handlers.go:933`), `ReviewChangeSource` (`review_change_source.go:53,90`), and archive/snapshot helpers (`orchestrator/task_operations.go:6522,6679,6776`) call the same `Client.GetGitStatusWithDetails(ctx)`.
3. `Client.GetGitStatusWithDetails` requests `GET /api/v1/git/status?fresh=true&details=wait` with no `repo` parameter.
4. On `plugin_remote`, the bare root tracker returns HTTP 200 with `success: false, status_state: "unavailable", detail_state: "unavailable", error_code: "repository_unavailable"` (`workspace_git_status.go:208-210`, `server/api/git.go:1745`).
5. `Client.GetGitStatusWithDetails` returns `git status details unavailable: repository_unavailable`. `captureBaseCommit` logs a warning and never persists `base_commit_sha`, and `computeCumulativeDiff` hits its `baseCommit == ""` fallback, gets the same error, and returns `{"cumulative_diff": null}` — even though `agentctl`'s `handleGitCumulativeDiff` (`server/api/git.go:1139-1144`) already fans out over `RepositoryScopes()` via `shouldFanOutRepositoryScopes` once a base commit is present.

Smallest reproduction: point `agentctl.Client` at an HTTP test server where `GET /api/v1/git/status?fresh=true&details=wait` (no `repo`) returns `{"success":false,"status_state":"unavailable","detail_state":"unavailable","error_code":"repository_unavailable"}`, `GET /api/v1/git/turn-changes/scopes` returns `{"scopes":["owner-repo-main"]}`, and `GET /api/v1/git/status?fresh=true&details=wait&repo=owner-repo-main` returns ready status with `base_commit: "abc123"`. Calling `GetGitStatusWithDetails` or `GitHandlers.wsCumulativeDiff` currently fails / returns `cumulative_diff: null`.

## Requirement and design reconciliation

| Area | Classification | Active criteria | System-design update |
| --- | --- | --- | --- |
| `plugin_remote` turn-change checkouts | 1: implementation violates active criteria | `AC-EXECUTORS-PLUGIN-002.4` (normal Git flows on plugin remote sessions), `AC-TASKS-TURN-CHANGES-002.1` (fresh start/end endpoints for each eligible attached Git checkout) | [Turn changed files](../../specs/tasks/system-design/turn-changed-files.md): note `launchRepositoryProjectionKey` projection for `plugin_remote` checkout subpaths |
| Single non-root scope detailed Git status fallback | 1: implementation violates active criteria | `AC-EXECUTORS-PLUGIN-002.4`, `AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5` (enriched launch baseline capture retains observed comparison baseline and rejects unavailable enrichment) | [Workspace Git capture recovery](../../specs/platform/system-design/workspace-git-capture-recovery.md): document the single-non-root-scope fallback on `repository_unavailable` in `Client.GetGitStatusWithDetails` |

No requirement text, ADR, SQL schema, wire protocol, runtime flag, UI, or public documentation change is needed. `docs/specs/executors/system-design/remote-executor-plugins.md` already declares that core owns `agentctl.Client` and Git materialization and requires no edit.

## Scope

### In scope

- Pass `executorType` into `turnChangeCheckoutsFromWorkspaceRepositories` from both lifecycle call sites (`initializeCreatedExecution` and `populateLaunchTurnChangeCheckouts`, falling back to `execution.ExecutorType` when `info.ExecutorType` is empty).
- For `plugin_remote`, resolve each checkout's `RepositorySubpath` via `launchRepositoryProjectionKey` (matching `remoteWorkspaceProjectionFromLaunch(req, true)`), preserving recorded `RepositorySubpath`/`RepositorySubpathKnown` on other executor types or when no safe branch-scoped directory exists.
- In `agentctl.Client.GetGitStatusWithDetails`, when the root `fresh=true&details=wait` read returns `error_code == "repository_unavailable"`, query `/api/v1/git/turn-changes/scopes` and, only when agentctl registers exactly one non-empty scope (`len(scopes) == 1 && scopes[0] != ""`), re-read `/api/v1/git/status?fresh=true&details=wait&repo=<scope>`.
- Unit and handler-boundary regression tests covering both fixes across single-repo, multi-repo, root-repo, repo-less, and error layouts.

### Out of scope

- Multi-repository turn-change subpath projection for non-plugin remote executors (`k8s`, `local_docker`, `remote_docker`, `sprites`, `ssh`) — mentioned as unverified in issue #5554 and kept out of scope so non-plugin executors stay byte-identical in this fix.
- Multi-repository `base_commit_sha` / `cumulative_diff` fallback when more than one scope is registered (keeps today's behavior).
- Changes to `agentctl` server routes, wire DTOs, frontend UI, or Playwright E2E suites.
- Commits, pushes, PR creation, or delegated implementation.

## Technical approach

### 1. Turn-change checkout projection (`internal/agent/runtime/lifecycle`)

- Update `turnChangeCheckoutsFromWorkspaceRepositories(repositories []WorkspaceRepositorySpec, executorType string) []TurnChangeCheckout` in `apps/backend/internal/agent/runtime/lifecycle/turn_changes.go`.
- When `executorType == string(models.ExecutorTypePluginRemote)`, compute the scope via `launchRepositoryProjectionKey(&LaunchRequest{ExecutorType: executorType}, RepoLaunchSpec{RepoName: repository.RepoName, BaseBranch: repository.BaseBranch, CheckoutBranch: repository.CheckoutBranch, BranchSlug: repository.BranchSlug}, index)`. When `err == nil`, set `RepositorySubpath = key` and `RepositorySubpathKnown = true`; when `err != nil`, keep the recorded `repository.RepositorySubpath` and `repository.RepositorySubpathKnown`.
- In `manager_execution.go:1401` and `manager_turn_changes.go:74`, resolve `executorType := info.ExecutorType; if executorType == "" { executorType = execution.ExecutorType }` and pass it to `turnChangeCheckoutsFromWorkspaceRepositories`.

Why this is the simplest sufficient solution: it reuses the exact `launchRepositoryProjectionKey` helper already used by `comparisonTargetsFromWorkspaceRepositories` and `remoteWorkspaceProjectionFromLaunch`, introducing zero new projection logic or divergence.

### 2. Single non-root scope fallback in `agentctl.Client.GetGitStatusWithDetails` (`internal/agent/runtime/agentctl`)

- In `apps/backend/internal/agent/runtime/agentctl/git.go`:
  - Define `gitStatusErrorNoRepository = "repository_unavailable"`.
  - Extract `getGitStatusWithDetails(ctx context.Context, repo string) (*GitStatusResult, error)` sharing the exact request, HTTP error, and readiness checks (`/api/v1/git/status?fresh=true&details=wait`, appending `&repo=` + `url.QueryEscape(repo)` when `repo != ""`).
  - Extract `soleNonRootRepositoryScope(ctx context.Context) (string, bool)` using `c.fetchJSONResult(ctx, "/api/v1/git/turn-changes/scopes", &resp)` (reusing `c.httpClient` timeout and lease transport rather than the 5-minute `longRunningHTTPClient`). Return `(resp.Scopes[0], true)` only when `err == nil`, `status >= 200 && status < 300`, `len(resp.Scopes) == 1`, and `resp.Scopes[0] != ""`.
  - In `GetGitStatusWithDetails(ctx)`: call `c.getGitStatusWithDetails(ctx, "")`; return immediately on `err == nil`. Only when `result != nil && result.ErrorCode == gitStatusErrorNoRepository`, check `c.soleNonRootRepositoryScope(ctx)` and, if found, return `c.getGitStatusWithDetails(ctx, scope)`. Otherwise return the original `(result, err)`.

Why this is the simplest sufficient solution: every backend consumer of `Client.GetGitStatusWithDetails` (`Executor.captureBaseCommit` via `lifecycleAdapter`, `GitHandlers.computeCumulativeDiff`, `GitHandlers.computeGitCommits`, `ReviewChangeSource`, and archive/snapshot capture) wants the session's single repository detailed status when no root repository exists. Placing the bounded fallback in `Client.GetGitStatusWithDetails` avoids duplicating a second method across adapters and handlers while preserving identical single-request behavior for all root-repository layouts and identical error behavior for repo-less and multi-repo layouts.

### Compatibility matrix

| Layout / executor | Root status response | Scopes response | `GetGitStatusWithDetails` requests & result | Turn-change checkout subpath |
| --- | --- | --- | --- | --- |
| `plugin_remote` single-repo | `repository_unavailable` | `["<repo>-<branch>"]` | 3 requests (root -> scopes -> `?repo=<scope>`); returns scoped enriched status | `<repo>-<branch>` via `launchRepositoryProjectionKey` |
| `plugin_remote` multi-repo | `repository_unavailable` | `["<r1>-<b1>", "<r2>-<b2>"]` | 2 requests (root -> scopes); preserves root `repository_unavailable` | `<r1>-<b1>`, `<r2>-<b2>` via `launchRepositoryProjectionKey` |
| Local / Worktree / k8s / Docker / Sprites / SSH single-repo | `ready` (`Success: true`) | Not called | 1 request; unchanged | Recorded `RepositorySubpath` (unchanged; coordinator resolves `""` when root scope exists) |
| Repo-less task | `repository_unavailable` | `[]` | 2 requests (root -> scopes); preserves root `repository_unavailable` | Empty manifest |
| Real repo with `details_unavailable` or HTTP 5xx | Non-`repository_unavailable` error | Not called | 1 request; preserves original error | Unchanged |
| Older/mock agentctl without scopes route (404) | `repository_unavailable` | HTTP 404 | 2 requests; preserves root `repository_unavailable` | Unchanged |

## Tests

Backend-only fix; no rendered UI or browser E2E changes are required.

| Criteria | Regression test (fails before fix) and location |
| --- | --- |
| `AC-EXECUTORS-PLUGIN-002.4`, `AC-TASKS-TURN-CHANGES-002.1` | `TestTurnChangeCheckoutsPluginRemoteMatchMaterializedDestinations` in `apps/backend/internal/agent/runtime/lifecycle/turn_changes_plugin_remote_test.go` — verifies single-repo, multi-repo (`BaseBranch` and `CheckoutBranch`), and sanitized repo names equal `remoteWorkspaceProjectionFromLaunch(req, true)` destinations, fallback when branch is missing, and byte-identical output for all non-plugin executor types |
| `AC-EXECUTORS-PLUGIN-002.4`, `AC-TASKS-TURN-CHANGES-002.1` | `TestPopulateLaunchTurnChangeCheckoutsResolvesPluginRemoteScope` in `apps/backend/internal/agent/runtime/lifecycle/turn_changes_plugin_remote_test.go` — exercises `populateLaunchTurnChangeCheckouts` (and `initializeCreatedExecution` / `admitTurnChangeCapture`) and verifies checkout subpaths match registered `<repo>-<branch>` scopes |
| `AC-EXECUTORS-PLUGIN-002.4`, `AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5` | `TestGetGitStatusWithDetailsSingleNonRootScopeFallback` in `apps/backend/internal/agent/runtime/agentctl/git_status_single_scope_test.go` — covers root-repo (1 request), bare-root single non-root scope (fallback succeeds with `BaseCommit` and `RepositoryName`), scoped `details_unavailable` (not masked), 0 scopes, 2+ scopes, scopes HTTP 404, and non-`repository_unavailable` errors |
| `AC-EXECUTORS-PLUGIN-002.4`, `AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5` | `TestWsCumulativeDiffSingleNonRootScopeFallback` in `apps/backend/internal/agent/handlers/git_handlers_single_scope_test.go` — fake agentctl HTTP server verifying `wsCumulativeDiff` resolves `base_commit` and returns non-null `cumulative_diff` in both root-repo and bare-root single-scope layouts, and keeps `cumulative_diff: null` for multi-scope and zero-scope bare roots |

## Work orders

- [x] [Task 01: Project plugin remote turn-change checkout subpaths](task-01-plugin-remote-turn-change-checkouts.md)
- [x] [Task 02: Fall back to sole non-root repository scope for detailed Git status](task-02-single-scope-git-status-fallback.md)

## Verification results

Task 01 completed on 2026-11-28:

- Focused lifecycle race tests passed after the expected pre-fix failure.
- Full `internal/agent/runtime/lifecycle` test package passed in 303.7 seconds.
- Changed-file Go lint passed with 0 issues.

Task 02 completed on 2026-11-28:

- Agentctl, handler, and executor focused race tests passed after expected pre-fix failures for the client and handler regressions.
- Changed-file Go lint passed with 0 issues.
- Specification validation, specification lint, and `git diff --check` passed.
- `make -C apps/backend build` passed.

Design-phase validation on 2026-11-27:

- `python3 scripts/list-docs.py validate`: passed (468 decisions and 2133 specifications validated).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` (simulated with `turn_changes.go` and `git.go`): `status: "covered"`, `ok: true`, 0 errors.
- `git diff --check` and `git status --short`: clean; design files remain unstaged/uncommitted for review.
- GitHub issue #5554 assignment verified via `gh issue view 5554`: assigned to `carlosflorencio`.

## Risks

- **Branchless or unsafe plugin specs:** If a `WorkspaceRepositorySpec` lacks both `CheckoutBranch` and `BaseBranch`, `launchRepositoryProjectionKey` returns an error; keeping the recorded `RepositorySubpath` avoids overwriting a known subpath with an empty string.
- **Extra HTTP request on bare-root multi-repo / repo-less sessions:** When `GetGitStatusWithDetails` hits a bare root, it performs one lightweight in-memory `GET /api/v1/git/turn-changes/scopes` check (no Git subprocess). Root-repository sessions remain at 1 request.
- **Non-plugin multi-repo remote executors:** Multi-repository tasks on `k8s`, Docker, Sprites, or SSH materialize secondary repositories in `<repo>-<branch>` directories and may exhibit a related `checkout_unavailable` symptom; this package intentionally scopes Task 01 to `plugin_remote` to keep built-in executors untouched and verifiable in a separate issue if confirmed.
- **Large existing test files:** `manager_turn_changes_test.go`, `git_test.go`, and `git_handlers_test.go` are already large; add focused new `*_test.go` files to stay well within `golangci-lint` file-length limits.
