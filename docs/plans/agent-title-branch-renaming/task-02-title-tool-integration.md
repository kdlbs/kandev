---
id: "02-title-tool-integration"
title: "Title tool integration"
status: done
wave: 2
depends_on: ["01-branch-rename-runtime"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/agent-generated-titles.md"
---

# Task 02: Title tool integration

## Acceptance

- `set_task_title_kandev` invokes the branch runtime only after the title-owner compare-and-set is
  accepted; disabled/manual-title tasks and rejected, non-owner, or repeated calls never invoke it.
- A successful response keeps the existing accepted task/title contract and adds deterministic
  repository-scoped branch outcomes, including preserved and partial/failed states.
- Git failure cannot roll back the accepted title or restore pending ownership metadata, and the MCP
  tool remains exposed only to the existing eligible owner session.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp -run 'Test.*(SetTaskTitle|TaskTitle)'
```

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/set_task_title_test.go`
- `apps/backend/internal/mcp/server/server.go`
- focused MCP server tests
- `apps/backend/internal/backendapp/helpers.go`
- focused backendapp wiring tests

## Dependencies

Task 01's orchestrator contract and outcome types.

## Parallelism

Sequential in the primary conversation because the handler response is defined by Task 01's runtime
results.

## Inputs

- Spec sections: **Task MCP**, **Permissions**, and **Failure modes**.
- The shipped `task.Service.SetPendingAgentTitle` authorization and compare-and-set behavior.
- Task 01's repository-scoped renamed/preserved/failed outcomes.

## Risks

- Returning an MCP transport error for a Git failure would incorrectly imply that the title was not
  accepted. Keep title acceptance separate from branch outcome status.
- Do not broaden tool registration or allow the agent to provide task/session IDs.

## Results

- Wired `TaskTitleBranchRenamer` into `set_task_title_kandev` only after the title compare-and-set
  accepts the owner session. Rejected and repeated calls retain their existing response shape and do
  not invoke the branch runtime.
- Accepted responses now include repository-scoped `branch_rename` outcomes; Git/runtime failures are
  reported as failed branch outcomes without undoing the accepted title or pending-marker cleanup.
- Updated the MCP tool description and production dependency wiring without broadening tool
  registration or allowing caller-supplied task/session IDs.
- Exact verification: `cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp -run 'Test.*(SetTaskTitle|TaskTitle)'` — 12 passed in 3 packages.
