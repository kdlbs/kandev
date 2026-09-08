---
id: "01-restore-local-base-fallback"
title: "Restore Local Base Fallback"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.4
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.5
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.9
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Restore Local Base Fallback

## Summary

Restore best-effort refresh for host worktrees with a valid local base. Keep
strict remote materialization when Kandev has no usable local ref.

## In scope

- Add the failing `TestCreateWorktree_PullEnabledUsesLocalOnlyBase` regression.
- Add fetch-error, missing-ref, divergence, and uncertain-ancestry fallback
  coverage.
- Preserve successful remote-ahead and local-ahead selection.
- Preserve strict failure when both local and remote bases are unavailable.
- Preserve caller cancellation as a terminal preparation result.
- Emit bounded warning progress without raw Git credential output.
- Keep empty-remote and multi-repository behavior compatible.

## Out of scope

- E2E changes.
- Public documentation changes.
- Repository setting or API changes.
- Automatic remote publication.

## Acceptance

- A valid local base creates a worktree after a non-cancellation refresh error.
- A missing local base still needs successful remote materialization.
- Caller cancellation never creates a fallback worktree.
- Local fallback never changes refs and never exposes raw Git output.

## Verification

```bash
cd apps/backend && go test ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -run 'Test(CreateWorktree|PullBaseBranch|PreferRefreshedRemoteRef|ResolveBaseRefWithFallback|Prepare.*Worktree|.*Refresh)' -count=1
```

## Files likely touched

- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/manager_pull_fallback_test.go`
- `apps/backend/internal/worktree/manager_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree_multi_test.go`

## Dependencies

None.

## Risks

- The fallback can incorrectly admit remote-only branches if local verification
  is omitted.
- The fallback can ignore cancellation if it classifies every command error as
  recoverable.
- Warning details can leak credentials if they reuse raw Git output.

## Parallelism

`sequential`

## Inputs

- `REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001`
- Worktree Base Refresh System Design sections for local fallback and required materialization.
- ADR `2026-08-31-local-worktree-refresh-best-effort`.
- Pre-`527cf1e9fa` best-effort manager behavior.

## Results

- Host worktree refresh now verifies the local base first and treats fetch,
  pull, missing-remote-ref, divergence, and uncertain-ancestry failures as
  credential-safe warnings when that local base remains usable.
- Remote-only refs, missing local bases, and cancellation remain strict.
- Worktree and lifecycle tests cover local preservation, warning projection,
  cancellation, provider-managed refresh, recreate, and multi-repository
  continuation.
