---
id: "01-create-replacement-worktrees"
title: "Create replacement worktrees without changing sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.7
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.9
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.10
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Create Replacement Worktrees Without Changing Sessions

## Summary

Prove that normal resume propagates confirmed branch loss. Add the narrow
backend permission that creates a fresh worktree from the task base branch
without clearing the existing provider conversation identity.

## In scope

- Start with a failing resume test that matches
  `worktree.ErrBranchUnrecoverable` through the executor and orchestrator error
  chain.
- Add `resume_new_branch` to `Service.RecoverSession` validation.
- Keep the `runtime_retry`, `resume`, and `fresh_start` behavior unchanged.
- Pass a branch-replacement permission from explicit recovery through the
  executor resume request and lifecycle workspace preparation.
- Add the permission to `worktree.CreateRequest`.
- Run normal local and remote branch recovery before replacement is eligible.
- When local and tracking refs are absent, run a bounded authoritative remote
  probe and distinguish confirmed absence from authentication or network
  failure.
- Generate the replacement directory and branch with the existing branch
  template and suffix helpers.
- Create the new worktree from the configured task base branch.
- Update the existing task environment repository record.
- Remove a newly created checkout and branch if that record update fails.
- Preserve valid worktrees in a multi-repository task.
- Prove that the task session, ACP session ID, and stored resume token remain
  unchanged.
- Prove the service-level `resume_new_branch` request keeps one task session,
  the configured base branch, and the original provider identity.

## Out of scope

- WebSocket error details.
- Warning message persistence.
- Frontend changes or localized copy.
- Automatic branch replacement.
- Recovery from any error other than confirmed `ErrBranchUnrecoverable`.

## Acceptance

- Normal resume returns an error that matches `ErrBranchUnrecoverable` and does
  not change the branch.
- `resume_new_branch` creates a unique normal-format branch from the configured
  base branch after the same recovery checks confirm branch loss.
- The same `TaskSession`, ACP session ID, and resume token reach provider
  launch.
- **Start fresh** remains the only action that clears the stored conversation
  identity before launch.
- A multi-repository task retains each valid worktree and replaces only each
  confirmed lost branch.
- Network and authentication failures remain errors and create no branch.
- A missing tracking ref does not advertise branch replacement when the remote
  branch still exists.
- Failed replacement persistence leaves the old environment record in place
  and no replacement checkout or branch on disk or in Git.

## Verification

Add the failing tests first and record the failure. Then run:

```bash
# From apps/backend:
rtk go test ./internal/worktree -run 'Test.*(Recreate|BranchUnrecoverable|ReplacementBranch)' -race
rtk go test ./internal/agent/runtime/lifecycle -run 'Test.*(Workspace|Worktree|ReplacementBranch)' -race
rtk go test ./internal/orchestrator/executor ./internal/orchestrator -run 'Test.*(Resume|RecoverSession).*(Branch|Token|Session)' -race
```

## Files likely touched

- `apps/backend/internal/worktree/worktree.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/manager_lifecycle_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_test.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_launch_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`

## Dependencies

None.

## Risks

- Branch templates can omit a suffix. Use the same collision checks and retry
  behavior as normal worktree creation.
- Reusing contribution or pull-request refs as the replacement start point can
  restore the wrong code. Resolve only the configured task base branch.
- Updating a session-owned path would violate task-environment ownership. Keep
  all physical worktree changes in the existing task repository record.
- A token assertion only in memory can miss persistence changes. Reload the
  session and executor records in the test.
- Remote-tracking refs can be pruned while the authoritative remote branch
  remains. Probe the remote before emitting typed branch loss.
- Persistence can fail after `git worktree add`; compensate the exact new path
  and branch without deleting the old recorded checkout.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001`.
- `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003`.
- `docs/specs/agents/system-design/agent-resume-runtime-recovery.md`.
- `ADR-2026-08-31-explicit-new-branch-session-recovery`.
- Existing worktree recreate and executor resume tests.

## Results

- Implemented typed unrecoverable-branch errors and the explicit
  `resume_new_branch` replacement path. Normal resume still returns the typed
  error without changing the saved branch; explicit recovery creates a unique
  branch from the configured base and updates the existing task environment
  repository record.
- Preserved the task session, ACP session ID, resume token, and valid sibling
  worktrees through the executor and lifecycle request chain.
- Added service-level coverage for `RecoverSession("resume_new_branch")`,
  including one-session identity, configured base branch, ACP request identity,
  and persisted resume-token assertions.
- GREEN: `rtk go test ./internal/worktree -run
  'Test.*(Recreate|BranchUnrecoverable|ReplacementBranch)' -race` (17 passed).
- GREEN: `rtk go test ./internal/agent/runtime/lifecycle -run
  'Test.*(Workspace|Worktree|ReplacementBranch)' -race` (124 passed).
- GREEN: `rtk go test ./internal/orchestrator/executor ./internal/orchestrator
  -run 'Test.*(Resume|RecoverSession).*(Branch|Token|Session)' -race` (87
  passed across 2 packages).
