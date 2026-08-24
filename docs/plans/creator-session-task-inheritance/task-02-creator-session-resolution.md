---
id: "02-creator-session-resolution"
title: "Creator-session resolution"
status: done
wave: 2
depends_on: ["01-initial-runtime-seed"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/mcp-task-agent-profile-default.md"
---

# Task 02: Creator-session resolution

## Acceptance

- Session-bound `create_task_kandev` calls use the verified creating session's
  profile and effective runtime configuration for top-level tasks and subtasks.
- Explicit, workflow, and workspace-default profiles do not receive copied
  creator runtime values. Calls without session context keep the task fallback.
- Executor inheritance remains parent-owned for subtasks and source-owned for
  top-level tasks. Invalid creator attribution creates no task.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers
```

## Files likely touched

- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/create_task_creator_session_test.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`

## Dependencies

- Task 01 supplies the typed runtime seed and initial-session mapping.

## Parallelism

Sequential. This task changes the profile-resolution boundary used by Tasks 03
and 04.

## Inputs

- Spec: **What**, **Permissions**, **Failure modes**, **Scenarios**
- Plan: **Trusted creator context**
- Existing pattern:
  `apps/backend/internal/mcp/handlers/spawn_session.go::resolveSpawnerSession`

## Output contract

Report the trusted identity path, precedence table, files changed, exact tests
run, results, blockers, risks, and synchronized task/plan status.

## Results

Implemented trusted creator-session resolution for session-bound MCP task
creation. The MCP server now forwards its bound session ID as internal
`source_session_id`; external mode omits it. The handler verifies that the
session exists and belongs to `source_task_id` before resolving the request.

`current_task` now prefers the verified creator session profile and effective
runtime seed for top-level and subtask creation. Workflow-selected profiles,
explicit profiles, and `workspace_default` suppress the copied runtime. The
existing executor inheritance chain remains unchanged, and invalid creator
context fails before task persistence.

The server forwards the bound session ID only when the bound task ID is also
present, so the backend never receives an unpairable creator identity.

Files changed:

- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/create_task_creator_session_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`

Verification:

```text
rtk go test ./internal/mcp/server ./internal/mcp/handlers ./internal/task/models ./internal/task/service ./internal/orchestrator/executor -count=1  PASS
```
