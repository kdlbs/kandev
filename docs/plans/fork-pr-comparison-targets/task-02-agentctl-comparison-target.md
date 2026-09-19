---
id: "02-agentctl-comparison-target"
title: "Make agentctl comparison refs authoritative"
status: done
wave: 2
depends_on: ["01-comparison-target-domain"]
plan: "plan.md"
spec: "../../specs/platform/requirements/workspace-git-status.md"
---

# Task 02: Agentctl comparison remote and authoritative ref

Teach agentctl to materialize and own repository-qualified targets. All comparison-derived Git paths must
use the same exact ref or return an explicit unavailable state.

## Red tests first

Create a temporary current upstream and stale fork with a same-named `main`. The feature branch should be
one commit over upstream but many commits over fork `origin/main`. Add failing route/process tests proving:

- the explicit target yields one commit and the upstream-sized diff for status, commits, cumulative diff,
  and Review;
- caller `target_branch=main` cannot redirect an active explicit target to fork `origin/main`;
- a deterministic comparison remote is added/fetched idempotently with an exact branch refspec;
- an existing remote with another URL is not overwritten;
- fetch, validation, collision, or merge-base failure produces a bounded error and no origin/local fallback;
- porcelain working-tree files remain available when comparison is unavailable; and
- multi-repository target maps update only the named tracker and refresh active/lazy trackers safely.

## Implementation

- Extend agentctl instance/config/create payloads with `ComparisonTargets map[string]models.ComparisonTarget`
  alongside `BaseBranches`. Desired targets must be present before tracker polling begins.
- Add an authenticated internal `POST /api/v1/workspace/comparison-targets` route and runtime client method.
  Validate every binding and repository key at the HTTP boundary.
- Add process-manager target state guarded independently from base branches. For each target, ensure the
  deterministic remote URL, disable Kandev push operations through it, and fetch only
  `refs/heads/<target>:refs/remotes/<compare-remote>/<target>` with tags disabled.
- Publish the target map only after per-repository materialization resolves; retain typed unavailable state
  for failures so the resolver cannot fall back. Do not make one repository failure erase ready siblings.
- Extend `WorkspaceTracker`, `GitStatusUpdate`, status HTTP results, and lifecycle aliases with optional
  target display/status/error code. Keep raw provider/Git error text out of bounded stream payloads.
- Centralize effective comparison-ref resolution and use it from branch statistics, ahead/behind, Git log,
  cumulative diff, and Review expansion. Preserve legacy branch-only behavior when no explicit target exists.
- Ensure updates trigger the same detached refresh/cache publication pattern as base-branch changes and are
  race-safe with rescan/lazy subpath creation.

## Acceptance

- Every local comparison surface agrees with the provider target for the stale-fork fixture.
- Explicit-target failure is observable and cannot publish fork-origin totals as authoritative.
- Existing ordinary, stacked-base, submodule, and multi-repository behavior remains unchanged without a
  target.

## Likely files

- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/workspace_comparison_targets.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_comparison_targets_test.go` (new)
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_comparison_target_test.go` (new)
- `apps/backend/internal/agentctl/types/workspace.go`
- `apps/backend/internal/agent/runtime/agentctl/workspace_comparison_targets.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/*_test.go`

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agent/runtime/agentctl
```

## Parallelism

`sequential`. Depends on Task 01's validated model; Task 03 consumes this control API.

## Output contract

Record red/green commands, endpoint payload, error-code set, remote/ref naming, and proof that all comparison
surfaces share one resolver. Update task/plan status only after focused tests pass.

## Completion record

- Green: `go test ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agent/runtime/agentctl`.
- The control payload is a repository-keyed `ComparisonTargets` map. Each target uses a deterministic
  `compare-<12 hex>` remote and exact `refs/remotes/<remote>/<branch>` ref.
- Status, log, cumulative diff, and Review all use the explicit resolver; materialization failures return
  bounded error codes and never fall back to `origin` or a same-named local branch.
