---
id: "02-mcp-title-tool"
title: "Task MCP tool and first-turn instruction"
status: done
wave: 2
depends_on: ["01-backend-title-lifecycle"]
plan: "plan.md"
spec: "../../specs/tasks/agent-generated-titles.md"
---

# Task 02: Task MCP tool and first-turn instruction

> Continuation note: Task 05 refines pending-only gating into durable ownership by exactly one
> first-launched task-mode session. This completed task records the original pending-title rollout.

## Acceptance

- `set_task_title_kandev(title)` is discoverable only in the title-pending task-mode variant and is
  absent from ordinary task, Config, Office, and External modes. It targets only the server-bound
  current task and returns the specified success/idempotent responses. Its tool and argument
  descriptions target about six words in sentence case, use a short title phrase rather than a sentence
  or progress update, and make clear that the agent replaces the provisional title before beginning
  work.
- Every structured first-turn path includes the before-other-work instruction only for pending tasks;
  the instruction repeats the same word-count guidance and says to call despite the provisional title.
  Later sessions and non-pending tasks omit both the instruction and tool schema, and
  sysprompt/tool-catalog sync tests pass.
- Passthrough launch receives the equivalent short instruction only while pending, and a failed MCP
  persistence call leaves the provisional title and marker intact.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/handlers ./internal/mcp/server ./internal/sysprompt ./internal/orchestrator ./internal/task/handlers -run 'Test.*(SetTaskTitle|AgentTitleInstruction|AgentTitlePending|Sysprompt)'
```

## Files likely touched

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- focused MCP handler/integration test files
- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- focused executor MCP-mode tests
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- focused orchestrator prompt tests
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_blocked_test.go`

## Dependencies

Task 01 service mutation and pending metadata contract.

## Parallelism

Sequential. It consumes Task 01's service contract and changes shared MCP/prompt registries.

## Inputs

- Spec sections: **Task MCP**, **Permissions**, **Failure modes**, pending-title scenarios.
- ADR decision on pending-only registration and task-bound identity.
- Existing patterns: `step_complete_kandev` mode registration/prompt gating and raw `mcp.*` WebSocket
  rejection.

## Risks

- Audit every first-turn composition path and MCP mode-resolution path so prompt text and tool catalogs
  cannot disagree.
- Preserve system-context canonicalization against forged user `<kandev-system>` blocks.
- Do not accept a task ID from tool arguments or register the tool in External/Office/Config mode.

## Output contract

Report behavior implemented, files changed, the exact test command/result, blockers or risks, and update
this task plus `plan.md` status in the same conversation.
