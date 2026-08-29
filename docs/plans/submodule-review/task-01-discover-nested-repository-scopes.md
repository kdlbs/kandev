---
id: "01-discover-nested-repository-scopes"
title: "Discover nested repository scopes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/submodule-review.md"
---

# Task 01: Discover nested repository scopes

## Acceptance

- Agentctl discovers every initialized direct and nested submodule as a tracker whose repository name is its task-root-relative path, while retaining a valid Git root tracker.
- Each submodule tracker compares from the gitlink SHA in its parent's comparison tree, including after manager reconstruction; invalid, uninitialized, or inaccessible children are skipped without disabling the parent.
- Construction, rescan, rebind, subscriptions, polling mode, search, and teardown preserve both existing bare multi-repository behavior and the new root-plus-children graph.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process -run 'TestManager_.*Submodule|TestManager_.*RootAndChildren|TestManager_.*MultiRepo'
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_rescan.go`
- `apps/backend/internal/agentctl/server/process/manager_submodules.go` (new)
- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_content_search.go`
- `apps/backend/internal/agentctl/server/process/manager_submodule_test.go` (new)
- Existing manager multi-repository/rescan tests when expectations require amendment

## Dependencies

None.

## Parallelism

`sequential` — this establishes the repository graph and manager contract used by every later task.

## Inputs

- Spec sections **What**, **API surface**, and **Failure modes**.
- ADR `2026-08-05-nested-submodules-as-repository-scopes.md`.
- Existing immediate-sibling discovery in `manager.go`, graph reconciliation in `manager_rescan.go`, and hardened submodule setup in `internal/worktree/submodule.go`.

## Output contract

Report the discovered graph shape, base-ref derivation, invalid-child behavior, files changed, exact tests and counts, blockers/risks, and synchronized task/plan status.

## Results

Implemented recursive initialized-submodule discovery with slash-delimited task-root-relative scopes, parent gitlink comparison anchors, root-plus-child tracker retention, safe-path checks, rescan/reconcile preservation, and subscription/poll-mode propagation. Uninitialized or invalid children are skipped without losing the parent.

Verification:

- `go test ./internal/agentctl/server/process -run 'Test(ReconcileRepositories_PrunesRemovedTrackerAndPreservesSubscription|Manager_DiscoversNestedSubmoduleScopesWithStableAnchors|Manager_RescanDiscoversNewNestedSubmoduleAndRetainsRoot|Manager_SkipsUninitializedSubmoduleWithoutLosingRoot)' -count=1` — 4 passed.
- `go test ./internal/agentctl/server/process -count=1` — 584 passed, including the post-rebase task-root rescan regression.
- `make -C apps/backend test` — passed; `make -C apps/backend lint` — 0 issues.
