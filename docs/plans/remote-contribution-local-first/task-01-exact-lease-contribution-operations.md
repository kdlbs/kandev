---
id: "01-exact-lease-contribution-operations"
title: "Exact-lease contribution operations"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 01: Exact-Lease Contribution Operations

Add explicit backend operations that replace the contribution branch or adopt its current provider
head. Keep generic contribution force-push requests rejected.

## Inputs

- Spec sections: What, API surface, Permissions, Failure modes, and Persistence guarantees.
- Plan sections: Backend and Tests.
- Decision: `docs/decisions/2026-08-12-local-first-contribution-replacement.md`.
- Existing pattern: `GitOperator.Push`, agentctl Git API handlers, runtime client methods, and
  `GitHandlers` WebSocket forwarding.

## Acceptance

1. An exact leased replacement succeeds only when the contribution branch still matches the expected
   provider head. A mismatch changes no remote refs.
2. Provider adoption requires a clean tree, checks the fetched head, creates a recovery branch, and
   resets the task branch only after all guards pass.
3. The new session-scoped actions forward one repository scope and remain absent from MCP and automatic
   operation registrations.

## Files Likely Touched

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_remote_contribution_resolution.go` (new)
- `apps/backend/internal/agentctl/server/process/git_remote_contribution_resolution_test.go` (new)
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/api/git_handlers_test.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git_test.go`
- `apps/backend/internal/agent/handlers/git_handlers.go`
- `apps/backend/internal/agent/handlers/git_handlers_test.go`
- `apps/backend/pkg/websocket/actions.go`

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/handlers
```

## Dependencies

None.

## Parallelism

Sequential. This task defines the contracts that every later task consumes.

## Risks

- Quote and validate the exact lease ref without constructing a shell command.
- Do not create the recovery branch before fetch and clean-tree guards pass.
- Do not use the generic multi-repository fan-out path.

## Output Contract

Report the request and response contracts, Git arguments, files changed, exact test results, and any
remaining provider-specific behavior. Update this task and `plan.md` in the same conversation.

## Results

Completed on 2026-08-12.

- Added provider-neutral `ReplaceRemoteContribution` and `UseRemoteContribution` process
  operations. Replacement uses an exact `--force-with-lease` refspec and rejects stale provider
  heads without changing the remote. Adoption rejects staged, unstaged, and untracked changes,
  verifies the fetched provider head, creates a recovery branch at the task HEAD, and resets the
  task branch while retaining its upstream.
- Added the agentctl HTTP endpoints, typed runtime client methods, and session-scoped WebSocket
  actions `worktree.replace_contribution` and `worktree.use_contribution`. Each forwards one
  optional repository scope and the full expected provider head. The adoption result forwards the
  recovery branch name.
- Added malformed-request, missing-field, exact-payload, response-decoding, handler forwarding,
  stale-lease, dirty-tree, recovery, and real bare-repository tests.
- Verification: `cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/handlers`
  passed: 1,753 tests in 4 packages.
- Provider-specific behavior remains outside this task. The operations consume the existing
  validated remote-contribution binding.
