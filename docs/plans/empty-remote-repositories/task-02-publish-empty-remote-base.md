---
id: "02-publish-empty-remote-base"
title: "Publish the Empty Remote Base"
status: done
wave: 2
depends_on:
  - "01-seed-empty-remote-worktrees"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002
acceptance_criteria:
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.1
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.2
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.3
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.4
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.5
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.6
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.7
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.9
system_design:
  - ../../specs/workspaces/system-design/empty-remote-repositories.md
---

# Task 02: Publish the Empty Remote Base

## Summary

Publish the marked base before the task branch during explicit Push and Create PR actions. Stop safely when the remote changes or publication fails.

## In scope

- Validate the exact marker and task base branch in `GitOperator`.
- Advertise the target remote through task runtime credentials.
- Publish the marked base without force before the current branch.
- Reuse the helper in Push and Create PR.
- Return bounded race and partial-result error codes.
- Delay provider change-request creation until both refs exist.

## Out of scope

- Publish during task launch.
- Reconcile changed remote history automatically.
- Change remote-contribution and fork destination rules.
- Add another provider API.

## Acceptance

- Push and Create PR initialize a still-empty remote in base-first order.
- A changed remote stops bootstrap without force or local-history loss.
- Failures identify the publication phase and leave the task branch available for retry.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/orchestrator/handlers
```

## Files likely touched

- `apps/backend/internal/gitbootstrap/`
- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_empty_remote.go`
- `apps/backend/internal/agentctl/server/process/git_empty_remote_test.go`
- `apps/backend/internal/agentctl/server/process/git_pr_providers_test.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/orchestrator/handlers/git_handlers.go`
- `apps/backend/internal/orchestrator/handlers/git_handlers_test.go`

## Dependencies

- Task 01 supplies the marker and deterministic baseline contract.

## Risks

- A non-atomic base-first flow can leave a published base after task-branch failure.
- The ordinary Push path has no explicit base argument and must use the repository-scoped task base.
- Multi-repository and contribution remotes must not share bootstrap state.

## Parallelism

`sequential`

## Inputs

- `REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002`
- Empty-remote design sections for publication, races, change requests, and security.
- Existing Push, Create PR, and credential-environment tests.

## Results

Completed. Push and Create PR now validate and publish the marked base before the task branch, retire the marker after a verified base publication, recover when unrelated refs coexist with the marked base, reject mismatched PR bases before any push, stop on remote races, preserve retryable partial results, and defer provider creation until both refs exist. The first base push uses an absence lease and an immutable baseline commit refspec. A post-push advertisement and local marker revalidation retain the marker and all remote refs when a race is detected, so the task branch is not published accidentally. Recreate and local empty-probe race paths restore from the verified local baseline without requiring `origin/<branch>`. Probe failures return bounded credential-safe errors, and baseline commits ignore inherited Git identity variables. The original focused backend suite passed: `go test ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/handlers ./internal/orchestrator/handlers` (1,573 tests). The review-round process suite passed: `go test ./internal/agentctl/server/process` (733 tests), including the new regressions. The focused worktree, repoclone, and gitbootstrap regressions also pass.
