---
id: "02-return-materialized-paths"
title: "Return materialized paths from the MCP tool"
status: completed
wave: 2
depends_on: ["01-restore-live-materialization"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/attach-workspace-sources.md"
---

# Task 02: Return materialized paths from the MCP tool

## Acceptance

- A live `add_branch_to_task_kandev` result contains `worktree_path`,
  `task_workspace_path`, and `agent_cwd_changed: false` alongside all existing fields.
- Deferred pre-launch materialization omits both path fields and still returns
  `agent_cwd_changed: false`.
- The tool description tells agents that the worktree is a sibling, their CWD/processes remain
  unchanged, and `worktree_path` is the exact location to use.

## Verification

```bash
cd apps/backend && rtk go test ./internal/mcp/handlers -run 'Test.*AddBranchToTask' -count=1
cd apps/backend && rtk go test ./internal/mcp/server -run 'Test.*AddBranchToTask' -count=1
```

## Files Likely Touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/server/server_test.go`

## Dependencies

- Task 01 provides `AddBranchToTaskResult` and materialized path values.

## Parallelism

`sequential`. This task depends on the service result contract and owns the agent-facing protocol
surface.

## Inputs

- Spec **API surface** response shape.
- ADR-2026-07-27-legacy-add-branch-live-rescan.
- Existing patterns:
  `Handlers.handleAddBranchToTask`,
  `Server.addBranchToTaskHandler`,
  `TestAddBranchToTask_ForwardsRepositoryURL`, and the real service fixtures in
  `internal/mcp/handlers/handlers_test.go`.

## TDD Sequence

1. Add a handler → service → SQLite regression with an active turn and path-returning legacy
   materializer; confirm the current handler result lacks the three new fields.
2. Add a task-mode MCP result/description regression and confirm RED.
3. Map the Task 01 result without changing request arguments or task scoping.
4. Run the exact verification commands above.

## Output Contract

Report both RED failures, response/description changes, durable integration evidence, files changed,
exact command results, risks, and blockers. Mark this task `done` and update its plan checkbox in the
primary conversation.
