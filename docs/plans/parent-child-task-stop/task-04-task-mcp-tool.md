---
id: "04-task-mcp-tool"
title: "Task MCP stop tool"
status: done
wave: 4
depends_on: ["03-mcp-stop-handler"]
plan: "plan.md"
spec: "../../specs/tasks/parent-child-task-stop.md"
---

# Task 04: Task MCP stop tool

## Acceptance

- `stop_task_kandev` is registered only in task mode with required `task_id`
  and accurate halt-only versus stop-and-steer guidance.
- Server forwarding injects immutable current task identity and exposes no
  sender, reason, or force-stop field to the agent.
- The `message_task_kandev` description presents the reciprocal three-way
  choice: queue ordinary information, interrupt urgent replacement work, stop
  halt-only work.
- Injected Kandev context advertises optional `delivery_mode` and the
  queue/interrupt/stop choice. `registerKanbanTools` count becomes 15 and total
  task-mode count becomes 27; Config, Office, and External still omit stop.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/server
```

## Files Likely Touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/config/prompts/kandev-context.md`

## Dependencies

- Task 03 provides `mcp.stop_task` and its backend response contract.

## Inputs

- Spec: `What`, `Choosing the control`, `API surface`, and `Permissions`.
- Tool/forwarder precedent: `message_task_kandev`.
- Mode registration/count tests in `internal/mcp/server/server_test.go`.

## Output Contract

Update this task to `done`, update `plan.md`, and report tool schema, modes,
identity injection, files changed, tests run, blockers, and risks.

## Completion

- Added task-only `stop_task_kandev` with a single required `task_id` and a
  fresh trusted `{task_id, sender_task_id}` backend payload.
- Updated task counts to 15 Kanban / 27 total and proved Config, Office, and
  External omission.
- Rewrote message and stop descriptions plus injected context around the
  queued / interrupt-and-steer / halt-only decision; prompt-schema sync now
  protects `delivery_mode` discoverability.
- Server and WebSocket packages pass focused tests.
- Existing agentctl instances retain their old per-instance tool registry and
  need restart/recreation to see the new tool; old first-turn prompt text is
  likewise not retroactive.
