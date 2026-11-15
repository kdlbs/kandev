---
id: "02-automatic-detection"
title: "Automatic worktree detection"
status: complete
wave: 2
depends_on:
  - "01-reported-adoption"
plan: "plan.md"
requirements:
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-002
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-003
acceptance_criteria:
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-002.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-002.2
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-002.3
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-002.4
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-002.5
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.2
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.5
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.6
system_design:
  - ../../specs/tasks/system-design/agent-worktree-adoption.md
---

# Task 02: Automatic worktree detection

## Summary

agentctl reports linked worktrees inside the task workspace root, and the backend adopts unreported
ones through `AdoptWorktree` with the `detected` source. Detection and reports reconcile to one root,
and a worktree deleted from disk loses its root.

## In scope

- `workspace_worktree_observer.go`: gated `git worktree list --porcelain -z`, containment and
  identity filtering, change-only `worktree_inventory` publication from `gitPollTick`.
- `types.WorkspaceMessageTypeWorktreeInventory` and its payload.
- Lifecycle translation to `events.WorkspaceWorktreesObserved` with execution identity.
- Task service subscriber: adopt new entries, ignore known ones, remove rows for paths gone from
  disk, ignore unsupported executors and non-latest sessions.
- `task_worktree_adoption_total` and `task_worktree_detach_total{reason="removed_from_disk"}`.

## Out of scope

- User detach and the detachment table (Task 03); this task treats it as absent.

## Acceptance

- Unit tests with real temporary repositories show in-root worktrees reported and out-of-root,
  other-repository, submodule, tracked and detached-`HEAD` checkouts excluded.
- Detected-then-reported and reported-then-detected both leave exactly one row and tracker.
- Detection performs no Git write and never calls a lifecycle start, resume or prompt method.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process/ -run 'TestWorktreeObserver' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle/ -run 'TestWorktreeInventory' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/service/ -run 'TestWorktreeObserved|TestAdoptWorktree' -count=1)
bash scripts/lint-go-changed $(git ls-files -mo --exclude-standard -- 'apps/backend/*.go')
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_worktree_observer.go` (new) and test
- `apps/backend/internal/agentctl/server/process/workspace_git_poll.go`
- `apps/backend/internal/agentctl/types/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/events/` (event type)
- `apps/backend/internal/task/service/worktree_observed_subscriber.go` (new) and test

## Dependencies

Task 01.

## Risks

- The process package has `goleak.VerifyTestMain`; the observer must run inside the existing poll
  goroutine, not a new one.

## Parallelism

`sequential`

## Inputs

- Design sections: Candidate identity, Worktree observer, Detection flow, Removal from disk.

## Results

Added the agentctl worktree observer, `worktree_inventory` stream payload, lifecycle `WorkspaceWorktreesObserved` translation, task-service subscriber, removal-from-disk retirement, and task-root containment passed to agentctl as `task_workspace_root`. Covered by observer, lifecycle payload, subscriber, and end-to-end detection tests.
