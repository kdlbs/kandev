---
id: "02-qualified-materialization"
title: "Materialize the qualified PR base"
status: done
wave: 2
depends_on:
  - "01-pr-base-identity"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.15
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.16
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.17
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 02: Materialize the qualified PR base

## Summary

Carry qualified base identity through preparation and materialize its exact ref.
Preserve checkout-head selection, origin, push routing, and valid worktree reuse.

## In scope

- Extract a shared Git materialization primitive into `internal/common/gitbase`.
  Reuse it from agentctl comparison setup and host worktree preparation.
- Forward the typed contract through lifecycle request copies, per-repository
  requests, worktree creation/recreation, and remote materialization DTOs.
- Select qualified bases before branch-only fallback. Verify the fetched commit
  and optional provider OID. Reject target failure without using origin/local refs.
- Preserve exact PR-head fetching from its owning namespace. Keep the PR head
  and target base separate when both repositories use the same branch name.
- Test both same-name and non-default targets, OID drift, collisions, auth errors,
  cancellation, linked-worktree common directories, and valid reuse.

## Out of scope

Recovery actions, changed comparison scheduling, new credentials, and new UI.

## Acceptance

- Two bare-repository fixtures prove preparation uses upstream's OID even when
  fork `main` exists. The checkout still starts at the requested PR head.
- Any required qualified fetch/identity failure blocks preparation. No fallback
  rewrites origin, upstream, user branches, or push configuration.
- Remote client round trips preserve the target. Existing asynchronous
  comparison, ordinary worktree, and same-repository fallback tests pass.

## Verification

```bash
(cd apps/backend && go test ./internal/common/gitbase ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(MaterializeQualifiedBase|CreateWorktree_QualifiedPRBase|MaterializeRepository_QualifiedPRBase|QualifiedPRBase|MaterializeComparisonTarget|ResolveRemoteDefaultBranch|ResolveBaseRefWithFallback|CreateWorktree_MissingRemoteBase)' -count=1)
(cd apps/backend && go test ./internal/common/gitbase ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process -count=1)
git diff --check
```

Start with failing same-name fork/upstream tests. Cover every changed test file
in the complete package command. Record actual results after implementation.

## Files likely touched

- `apps/backend/internal/common/gitbase/materialize.go` and `materialize_test.go` (new)
- `apps/backend/internal/worktree/worktree.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/manager_pr_base.go` and `manager_pr_base_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/workspace_materialization.go`
- `apps/backend/internal/agent/runtime/lifecycle/qualified_pr_base_test.go` (new)
- `apps/backend/internal/agent/runtime/agentctl/client_workspace_sources.go`
- `apps/backend/internal/agent/runtime/agentctl/client_workspace_sources_test.go`
- `apps/backend/internal/agentctl/server/api/workspace_materialize.go`
- `apps/backend/internal/agentctl/server/api/workspace_materialize_pr_base_test.go` (new)
- `apps/backend/internal/agentctl/server/process/comparison_target.go`
- `apps/backend/internal/agentctl/server/process/comparison_target_test.go`

## Dependencies

Task 01 supplies validated target identity and provider observations.
Read scoped agentctl and API guidance before changing their code.

## Risks

Preparation and background comparison have different failure policies. Share
only the Git primitive. Preserve context deadlines and task credential scopes.
Do not add the same target fetch to both preparation and immediate reuse.

## Parallelism

`sequential`

## Inputs

- Worktree base refresh design: Preparation and materialization.
- Existing `materializeComparisonTarget` implementation and its tests.
- Existing live-default, stacked-PR, and remote-contribution worktree tests.

## Results

- Added `internal/common/gitbase.Materialize` for deterministic qualified
  remote setup, exact branch fetch, commit resolution, and optional provider
  OID verification. Agentctl comparison materialization now uses the shared
  primitive and keeps its existing asynchronous status/error policy.
- Host worktree creation and recreation carry the qualified target, bypass
  branch/default fallback, preserve cancellation, and fetch the PR snapshot
  from the validated base repository separately from the target branch.
- Qualified remote checkout reuse now fetches and verifies the pinned base OID
  and PR head before accepting an existing checkout. PR-head fetches return the
  observed commit OID, and branch restoration uses that immutable object ID.
- When a qualified base and contribution binding coexist, both materializers
  verify the PR number, branches, base attachment, and source repository. A
  same-number, same-branch contribution from another fork is rejected.
- Lifecycle single-repository, multi-repository, workspace recovery, and remote
  materialization requests preserve the typed PR base. Remote agentctl checks
  request identity, materializes the target, and checks out the exact PR head.
- Regression coverage includes linked worktrees, same-named fork/upstream
  branches, non-default target branches, PR-head preservation, stale OIDs,
  collisions, authentication errors, cancellation, and DTO propagation.
- Focused verification: `go test ./internal/common/gitbase ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process -run 'Test(MaterializeQualifiedBase|CreateWorktree_QualifiedPRBase|MaterializeRepository_QualifiedPRBase|QualifiedPRBase|MaterializeComparisonTarget|ResolveRemoteDefaultBranch|ResolveBaseRefWithFallback|CreateWorktree_MissingRemoteBase)' -count=1`: passed.
- Full package verification: `go test ./internal/common/gitbase ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process -count=1`: passed.
- Task 01 regression check: `go test ./internal/github ./internal/backendapp ./internal/orchestrator/executor -run 'Test(PRBase|ResolveTaskRepoInfo|GithubPRBase)' -count=1`: passed.
- `git diff --check`: passed.
