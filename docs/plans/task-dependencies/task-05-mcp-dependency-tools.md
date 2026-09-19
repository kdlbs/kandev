---
id: "05-mcp-dependency-tools"
title: "MCP dependency tools"
status: done
wave: 5
depends_on: ["01-core-dependency-relationship"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/task-dependencies.md"
---

# Task 05: MCP Dependency Tools

## Acceptance

- `create_task_kandev` **declares** `blocked_by` (array of task IDs) and
  `start_when_unblocked` (boolean) in its `mcp.NewTool` schema in
  `internal/mcp/server/server.go`. Today the handler struct at
  `internal/mcp/handlers/handlers.go:576` reads `blocked_by` but the schema never
  declares it, so no agent can discover or reliably pass it. A pinning test
  asserts both parameters are present in the registered tool schema.
- `blocked_by` non-empty plus `start_agent: true` (including the default) records
  a start-when-unblocked intent and launches nothing now. The create response
  reports `started: false` and `start_when_unblocked: true`.
- `blocked_by` non-empty plus explicit `start_when_unblocked: false` creates the
  edges with no launch intent; completing the predecessor starts nothing.
- `add_task_dependency_kandev` is registered: `{task_id?, depends_on_task_id}`,
  `task_id` defaults to the calling task, returns the resulting `depends_on`
  list. It rejects cycles with the cycle path, self-edges, and cross-workspace
  edges, using the single validator from Task 01.
- `remove_task_dependency_kandev` is registered with the same shape. Removing an
  absent edge succeeds. Removing the last edge unblocks the task without
  launching it.
- Both new tools are registered in the same modes as the other task-mutation
  tools, and neither widens the `ModeExternal` surface beyond what
  `create_task_kandev` already exposes there.
- An agent cannot set `start_when_unblocked` on a task it did not create.
- Tool descriptions state: a subtask means "part of" and a dependency means "not
  until"; decomposing a plan into ordered phases is N sibling tasks chained with
  `blocked_by` + `start_when_unblocked`, not N subtasks started at once; and a
  failed predecessor halts the chain and needs human action.
- `list_related_tasks_kandev` populates `blockers` and `blocked_by` with
  `Features.Office` false (inherited from Task 01; asserted here at the MCP
  boundary).
- Both tools authorize through the existing MCP scoping path — the owning task
  comes from the `AgentExecution`, never from an agent-supplied `task_id` used to
  reach another workspace's tasks.
- The MCP tool list in the root `CLAUDE.md` / `AGENTS.md` "Kandev Task Creation"
  guidance is updated if the new tools change how an agent should decompose work.

## TDD sequence

1. Failing pinning test: the registered `create_task_kandev` schema declares
   `blocked_by` and `start_when_unblocked`.
2. Failing test: `blocked_by` + default `start_agent` creates the edge, starts no
   session, and returns `started: false`, `start_when_unblocked: true`.
3. Failing test: `blocked_by` + `start_when_unblocked: false` records no intent
   and completing the predecessor starts nothing.
4. Failing test: three chained `create_task_kandev` calls run in order —
   completing the first starts the second and not the third.
5. Failing tests for `add_task_dependency_kandev`: default `task_id`, returned
   `depends_on`, cycle rejection with path, self-edge, cross-workspace.
6. Failing tests for `remove_task_dependency_kandev`: removal, absent-edge
   success, last-edge removal unblocks without launching.
7. Failing test: an agent cannot set `start_when_unblocked` on a task it did not
   create.
8. Failing test: cross-workspace `task_id` on either tool is denied by the
   scoping path.
9. Failing test: with `Features.Office` false, `create_task_kandev` +
   `blocked_by` succeeds and `list_related_tasks_kandev` reports the edge.
10. Implement schema declarations, the two tools, the start-intent semantics, and
    the descriptions.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/mcp/... -run 'Test.*(Dependenc|BlockedBy|StartWhenUnblocked|ToolSchema)' -count=1
go test -tags fts5 ./internal/mcp/... -count=1
golangci-lint run ./... --new-from-rev=origin/main --timeout=5m
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/server/handoff_handlers.go`
- `apps/backend/internal/mcp/scope/` (if either tool needs a scope entry)
- `CLAUDE.md` / `AGENTS.md` (task-decomposition guidance)
- focused MCP handler and schema-pinning tests

## Dependencies

Task 01 — needs the wired blocker repository, the single validator, and the
`start_when_unblocked` create-request field. It does not need Tasks 02–04, so it
may run alongside Task 06.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the RED tests and `done` only after the
listed commands pass. Record the two new tool names and their registered modes,
the exact `blocked_by` + `start_agent` resolution rule as implemented, the
schema-pinning test name, and test results in this file and `plan.md`.
