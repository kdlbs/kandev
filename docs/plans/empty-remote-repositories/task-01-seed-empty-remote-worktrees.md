---
id: "01-seed-empty-remote-worktrees"
title: "Seed Empty Remote Worktrees"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.1
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.2
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.3
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.4
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.5
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.6
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.7
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8
system_design:
  - ../../specs/workspaces/system-design/empty-remote-repositories.md
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Seed Empty Remote Worktrees

## Summary

Add typed empty-remote evidence and a deterministic local baseline. Create normal task worktrees from that baseline without a remote write.

## In scope

- Add a typed remote-ref state to strict clone and refresh results.
- Add the shared deterministic baseline and marker-ref contract.
- Seed the local base under the repository lock.
- Preserve required-refresh failures for every unproven empty state.
- Cover initial launch, recreation, and multi-repository isolation.

## Out of scope

- Push a remote ref.
- Change agentctl Push or Create PR.
- Add frontend copy.

## Acceptance

- Authenticated zero-ref evidence creates one empty marked baseline and a normal task worktree.
- Missing branch, authentication, network, timeout, and unknown states do not create a baseline.
- Launch and recreation leave the remote unchanged, including multi-repository preparation.

## Verification

```bash
cd apps/backend && go test ./internal/gitbootstrap ./internal/repoclone ./internal/worktree ./internal/orchestrator/executor
```

## Files likely touched

- `apps/backend/internal/gitbootstrap/`
- `apps/backend/internal/repoclone/clone.go`
- `apps/backend/internal/repoclone/clone_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`
- `apps/backend/internal/worktree/worktree.go`
- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/manager_empty_remote_test.go`

## Dependencies

None.

## Risks

- The provider-managed refresh callback needs typed evidence without weakening exact credential scope.
- Ref creation must remain atomic under concurrent task launches.
- The commit must remain deterministic across repeated clones that use the same Git object format.

## Parallelism

`sequential`

## Inputs

- `REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001`
- Empty-remote design sections for classification, baseline, launch, persistence, and security.
- Existing strict-refresh and local initial-commit tests.

## Results

Completed. Added typed remote-ref-state propagation, deterministic local baseline creation, marker validation, local-origin probing, and conflict-safe launch/recreation behavior. The focused backend suite passed: `go test ./internal/gitbootstrap ./internal/repoclone ./internal/worktree ./internal/orchestrator/executor` (954 tests).
