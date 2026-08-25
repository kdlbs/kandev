---
id: "02-reject-stale-fallbacks"
title: "Reject stale worktree fallbacks"
status: done
wave: 2
depends_on:
  - "01-route-required-refresh"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.4
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.5
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 02: Reject Stale Worktree Fallbacks

## Summary

Make required worktree sync return errors. Select a base only when it is proven
to contain current fetched remote state and any preserved local commits.

## In scope

- Add failing worktree tests for authentication failure, network failure,
  timeout, missing remote base, diverged refs, and uncertain ancestry.
- Change `pullBaseBranch` and its required-sync helpers to return errors.
- Propagate sync errors through `resolveBaseRefWithFallback`, `Create`, and
  recreation callers.
- Replace stale-ref success events with failed sync progress events.
- Select the remote ref when it contains the local ref.
- Select the local ref when it contains the fetched remote ref.
- Preserve local-only behavior when `PullBeforeWorktree` is false.
- Keep Git commands non-interactive and bounded by the existing admission and
  timeout contracts.

## Out of scope

- Credential-route selection from Task 01.
- Reset, merge, rebase, or automatic divergence recovery.
- Branch recovery for existing reusable worktrees.
- Task error rendering.

## Acceptance

- Required fetch failure returns an error and creates no worktree from a local
  fallback ref.
- Comparable local and remote refs select the containing ref without losing
  commits. Diverged or unverified refs return an error without mutation.
- Pull-before-worktree disabled retains current offline local behavior.

## Verification

Change one current fallback test to expect an error and confirm that it fails
before the production change. Then run:

```bash
# From apps/backend:
rtk go test ./internal/worktree -run 'PullBaseBranch|RequiredRefresh|BaseRef|Fallback' -race
rtk go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/... -race
```

## Files likely touched

- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/manager_pull_fallback_test.go`
- `apps/backend/internal/worktree/manager_fetch_retry_fallback_test.go`
- `apps/backend/internal/worktree/manager_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree_test.go`

## Dependencies

- Task 01 defines when `RemoteSyncHandled` is valid and when the manager owns
  the required fetch.

## Risks

- Returning errors changes several internal method signatures. Update all
  fallback and live-default call sites before broad tests.
- A boolean ancestry helper cannot distinguish a negative result from a failed
  probe. Return a typed result or error so uncertain ancestry fails safely.
- Fallback base-branch recovery must not bypass a required refresh.

## Parallelism

`sequential`

## Inputs

- `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1` and
  `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3` through `.5`.
- The base-ref selection table in the system design.
- Existing unpushed-commit preservation tests.

## Results

- Required worktree refresh and checkout/recreate fetch failures now propagate
  instead of selecting stale local fallbacks.
- Base-ref selection accepts only verified containing refs and rejects diverged
  or uncertain ancestry without mutation.
- Disabled pull-before-worktree behavior remains available for offline local
  workflows.
- Verified with:
  - `rtk go test ./internal/worktree -run 'PullBaseBranch|RequiredRefresh|BaseRef|Fallback' -race`
  - `rtk go test ./internal/worktree/... ./internal/agent/runtime/lifecycle/... -race`
