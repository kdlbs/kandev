---
id: "02-derive-runtime-contract"
title: "Derive the autopilot runtime contract"
status: done
wave: 2
depends_on:
  - "01-persist-task-contract"
plan: "plan.md"
spec: "../../specs/tasks/requirements/autopilot-mode.md"
---

# Task 02: Derive the Autopilot Runtime Contract

## Acceptance

- Every task launch/resume transport carries the persisted autopilot value into
  system-prompt construction and agentctl task MCP configuration.
- Autopilot sessions receive the autonomy/last-action guidance and discover
  `ask_parent_question_kandev` only when a direct parent exists; autopilot root
  sessions receive no question tool; normal task sessions keep only
  `ask_user_question_kandev`.
- No session receives both question tools, and the remaining task/provider tools
  follow the existing mode allowlists.
- The profile registry composes base surfaces (`kanban-task`, `office-task`,
  `configuration`, `external`) with additive title, question, and provider groups.
  It replaces duplicated mode branches and manual tool counts.
- Top-level, nested, restart, and unsupported-mode behavior is deterministic and
  covered by prompt/tool inventory tests.

## Verification

```bash
cd apps/backend && go test ./internal/sysprompt ./internal/mcp/server ./internal/agent/runtime/lifecycle/... ./internal/agentctl/server/... ./internal/orchestrator/...
```

## Files likely touched

- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/agent/runtime/agentctl/control.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/tool_profiles.go`
- `apps/backend/internal/mcp/server/tool_profiles_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker_test.go`
- `apps/backend/internal/agentctl/server/session_config_test.go`
- `apps/backend/internal/mcp/server/server_test.go`

## Dependencies

- Task 01 provides the persisted property and compatible-profile validation.

## Parallelism

Can run in parallel with Task 04 after Task 01. It owns backend launch, prompt, and
tool-inventory transport; Task 04 owns frontend files.

## Inputs

- Spec sections `Autonomous behavior`, `Parent question protocol`, and `Permissions and boundaries`.
- ADR `2026-08-03-provider-scoped-task-mcp-tools.md` for backend-owned capability transport.
- Existing `KandevContextOptions`, task launch wrapping, lifecycle configuration,
  `disableAskQuestion` behavior, and task parent lookup.

## Output contract

Report the launch configuration shape, each transport updated, exact prompt text
source, MCP registration predicate, tool-list behavior across resume, tests run,
and any client that cannot receive a changed inventory.

## Results

Done. Added typed backend-owned MCP surfaces and additive capability groups, carried
the resolved profile through launch/resume/interaction transports, and derived the
autopilot prompt plus mutually exclusive question tools for normal, child, and root
tasks. Focused prompt, profile, server, and orchestrator tests pass.
