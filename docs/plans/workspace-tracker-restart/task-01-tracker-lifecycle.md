---
id: "01-tracker-lifecycle"
title: "Restore tracker lifecycle after agent restart"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.5
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.19
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.43
system_design:
  - ../../specs/platform/system-design/workspace-tracker-restart.md
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 01: Restore tracker lifecycle after agent restart

## Summary

Replace stopped workspace trackers before the process manager starts a replacement agent.
Restore current reads and existing subscriber delivery while retaining full Stop cleanup.

## In scope

- Add the failing Stop/Start regression before production edits.
- Cover persistent and one-shot starts through the common start boundary.
- Add a focused manager helper for graph renewal.
- Exclude stopped trackers from graph reuse and invalidate stopped lazy entries.
- Preserve root classification, attached repositories, submodules, current configuration, poll mode, and process-output routing.
- Preserve subscriber channels, source ordering, waiter cancellation, and terminal teardown.
- Add API regressions for both Git-status routes after manager restart.

## Out of scope

- Changing managed-runtime recovery classification, delays, versions, or attempts.
- Resetting a stopped tracker in place or removing Stop cleanup.
- Frontend state, layout, copy, retries, and database changes.

## Acceptance

1. After Stop/Start, fresh membership and settled patch reads succeed for every valid selected scope, including lazy paths and initialized submodules.
   The original regression fails with `context canceled` before the fix.
2. Existing subscribers receive replacement-tracker snapshots without resubscription.
   Healthy unchanged trackers retain identity, and each replaced tracker receives a new source identity.
3. Repeated cycles, concurrent reads/unsubscribe, old-worker completion, canceled callers, and final teardown pass under the race detector.
   Cleanup still drains trackers and removes owned temporary indexes.

## Implementation notes

Read `startWithGeneration`, `stopWorkspaceTrackers`, and the root/one-shot startup paths before editing.
Keep new lifecycle logic in `manager_workspace_restart.go` to avoid expanding the large manager file.
Reuse `buildWorkspaceTrackerGraph`, `configureTracker`, and existing graph publication paths.
Extend the root and named reuse predicates to require a live tracker.
Use current root classification when choosing the graph builder's bare-root policy.
Clear stopped lazy entries under their existing lock. Recreate them through the existing authorized lazy-read path.
Keep live lazy entries and avoid duplicate root or named trackers for a cached path.

Use existing lifecycle admission and `rescanMu` for renewal.
Document the lock order next to the helper if its invariant is not obvious.
Attach only current subscribers, handle unsubscribe safely, and update the process runner reference.
Use the manager's current effective configuration rather than copying the old status cache.

Planned tests in `manager_workspace_restart_test.go`:

- `TestManagerWorkspaceRestartFreshStatus`, including persistent, one-shot, and Stop-before-first-start subtests.
- `TestManagerWorkspaceRestartExistingSubscriber`.
- `TestManagerWorkspaceRestartLiveGraph` and `TestManagerWorkspaceRestartMixedGraph`.
- `TestManagerWorkspaceRestartRetiresOldPublication`.
- `TestManagerWorkspaceRestartScopes`, `TestManagerWorkspaceRestartLazyTracker`, and `TestManagerWorkspaceRestartConfiguration`.
- `TestManagerWorkspaceRestartCleanup`.

Use disposable repositories, existing process fixtures, and bounded channels.
Pair one ready named repository with one failed current repository to preserve truthful partial failure.
Use a blocked old observation to prove stale publication rejection across renewal.
Record the expected failure before adding the production correction.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process -run '^TestManagerWorkspaceRestartFreshStatus$' -count=1 -v)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(BuildWorkspaceCallbacks|WorkspaceStreamPromotion|ConnectWorkspaceStream|RetryManagedRuntimeStartupLifecycle)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/process/manager.go apps/backend/internal/agentctl/server/process/manager_workspace_restart.go apps/backend/internal/agentctl/server/process/manager_workspace_restart_test.go apps/backend/internal/agentctl/server/process/manager_submodules.go apps/backend/internal/agentctl/server/process/workspace_tracker.go apps/backend/internal/agentctl/server/api/git_status_restart_test.go
git diff --check
```

The first command is the RED gate before production edits and must pass afterward.
If implementation changes `manager_rescan.go`, add that existing path to the changed-file lint command.
The API test `TestGitStatusAfterManagerRestart` must verify complete membership and actual patch content, not only HTTP 200.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_workspace_restart.go` (new)
- `apps/backend/internal/agentctl/server/process/manager_workspace_restart_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager_submodules.go`
- `apps/backend/internal/agentctl/server/process/workspace_stream.go`
- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/agentctl/server/api/git_status_restart_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager_rescan.go`, only if the existing graph boundary requires it

## Dependencies

None.

## Risks

- A root-only fix misses named or cached lazy trackers.
- A graph rebuild can change scope classification, comparison anchors, credential environment, or polling mode.
- Subscriber attachment can race with channel closure.
- A broad lifecycle-lock change can deadlock process teardown.

## Parallelism

`sequential`

## Inputs

- [Plan and reproduction evidence](plan.md#confirmed-diagnosis).
- [Tracker restart design](../../specs/platform/system-design/workspace-tracker-restart.md).
- [Workspace Git requirement](../../specs/platform/requirements/workspace-git-status.md).
- `manager_stop_test.go`, `manager_multi_repo_stream_test.go`, `manager_git_subpath_test.go`, and `manager_poll_mode_inherit_test.go`.
- `manager_submodule_test.go`, `manager_rescan_test.go`, and `git_status_fresh_concurrency_test.go`.
- `apps/backend/AGENTS.md`, `apps/backend/internal/agentctl/AGENTS.md`, and the TDD backend-test reference.

## Results

Completed.

- The RED gate failed before the production edit with `get fresh status for ...: context canceled`; `TestManagerWorkspaceRestartFreshStatus/persistent` now passes.
- `manager_workspace_restart.go` renews a stopped graph at the common `startWithGeneration` boundary. Live root, repository, and lazy trackers keep their identity; stopped lazy entries are pruned.
- Graph reconciliation reuses only live trackers, publishes before attaching subscribers under the subscriber lock, retires old trackers outside graph locks, preserves the effective poll mode, and updates the process runner reference.
- Subscriber handoff is atomic: graph publication, attachment, and retirement share `streamSubscribersMu`; subscribe and unsubscribe hold it through attachment or channel closure; retired trackers clear their actual subscription sets. `TestManagerWorkspaceRestartSubscriberHandoffRace` deterministically covers publication, concurrent unsubscribe, retirement, delayed old publication, and delayed subscribe for both reconcile and rebind.
- Cached root and repository Git operators are rebound to the replacement trackers while keeping their operation locks. Replacement comparison targets are prepared before polling starts. Lazy tracker creation is admitted instance-owned work, so permanent teardown rejects it instead of starting a fresh observation.
- The one-shot regression now exercises the public `Stop` and `Start` lifecycle through an adapter-construction seam. Restart assertions require the patch marker in the returned diff instead of accepting an empty patch.
- `TestManagerWorkspaceRestart*` covers persistent, one-shot, stop-before-first-start, existing subscribers, live and mixed graphs, retired publication, named scopes with one unavailable repository, initialized submodules with preserved anchors, plain task roots that stay outside Git, configuration, comparison-target preparation, cached Git operators, lazy reads and lazy teardown rejection, repeated cycles, canceled waiters, unsubscribe, and final teardown.
- `TestGitStatusAfterManagerRestart` covers single and multi routes with fresh membership, changed tracker identity, complete files, ready details, and required patch markers after a real restart.
- Process tests, the full process race suite (486.214s), the full API race suite (209.218s), the focused restart suite (7.028s), and the lifecycle callback subset (2.217s) passed. Changed Go lint reported `0 issues`.
- Implementation files remain unstaged and uncommitted.
