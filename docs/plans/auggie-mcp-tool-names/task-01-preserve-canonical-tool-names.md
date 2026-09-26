---
id: "01-preserve-canonical-tool-names"
title: "Preserve canonical names for server-namespacing agents"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MCP-TOOL-NAMES-001
acceptance_criteria:
  - AC-TASKS-MCP-TOOL-NAMES-001.1
  - AC-TASKS-MCP-TOOL-NAMES-001.2
  - AC-TASKS-MCP-TOOL-NAMES-001.3
  - AC-TASKS-MCP-TOOL-NAMES-001.4
  - AC-TASKS-MCP-TOOL-NAMES-001.5
system_design:
  - ../../specs/tasks/system-design/mcp-tool-name-stability.md
---

# Task 01: Preserve Canonical Names for Server-Namespacing Agents

## Summary

Declare and propagate the MCP server-namespacing capability of the agent. Then
add a reversible transport-name adapter to the per-instance Kandev MCP server.
The canonical registry, prompts, validation, and handler routing remain
unchanged.

## In scope

- Add the runtime capability and enable it for Auggie only.
- Propagate it through every persistent agentctl instance creation path.
- Adapt `tools/list` and `tools/call` at the local Kandev MCP endpoint.
- Add red-green regression coverage for all built-in MCP profiles, canonical
  dispatch, and non-namespacing agents.

## Out of scope

- External agent-client changes.
- Prompt, schema, profile, permission, or canonical tool-name changes.
- Plugin tool-name redesign.
- Browser E2E coverage.

## Acceptance

- The effective model-facing Auggie catalog contains
  `ask_user_question_kandev`, `get_task_plan_kandev`, and every other built-in
  canonical tool without a doubled `_kandev_kandev` suffix.
- Cursor ACP, Codex ACP, Claude ACP, and other default-false agents retain their
  canonical `*_kandev` catalog unchanged.
- The bare MCP call from a namespacing client is restored to the canonical
  registered name before validation and dispatch. All backend tests and lint
  pass.

## Verification

Follow TDD, then run:

```bash
rtk go test ./internal/agent/agents ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/mcp/server ./internal/sysprompt
rtk make -C apps/backend test
rtk make -C apps/backend lint
```

Run the first command from `apps/backend`. Run the two `make` commands from the
repository root.

When a live Auggie login is available, trigger a user question and inspect the
ACP logs with:

```bash
rtk grep -rho "ask_user_question[a-z_]*\|get_task_plan[a-z_]*" ~/.kandev/logs/acp/
```

## Files likely touched

- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/auggie.go`
- `apps/backend/internal/agent/agents/new_acp_agents_test.go`
- `apps/backend/internal/agent/runtime/agentctl/control.go`
- `apps/backend/internal/agent/runtime/agentctl/control_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/tool_name_presentation_test.go`
- Relevant existing executor, instance, config, and sysprompt synchronization
  tests alongside those files.

## Dependencies

None.

## Risks

- Every executor request builder must carry the capability.
- The list and call mappings must remain one-to-one across live tool-set
  replacement.
- Exact registered names must win over aliases to avoid dispatching a plugin or
  future built-in tool incorrectly.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-MCP-TOOL-NAMES-001` and all acceptance criteria.
- `docs/specs/tasks/system-design/mcp-tool-name-stability.md`.
- `ADR-2026-08-31-agent-aware-mcp-tool-names`.
- Existing MCP registration, hook, instance configuration, executor request,
  and sysprompt synchronization tests.

## Results

Completed on 2026-08-31.

- Added the Auggie-only runtime capability and propagated it through standalone,
  container, Docker reconnect, Sprites, and SSH agentctl instance requests.
- Added per-instance MCP transport translation for list and call boundaries.
  Canonical registry names, validators, handlers, prompts, and plugin names
  remain unchanged.
- Focused packages passed 3,278 tests; `cmd/agentctl` passed 94 tests; and
  race-enabled MCP/lifecycle tests passed 2,451 tests.
- The full backend test target passed after removing the managed-session config
  handoff variables. Backend lint passed with zero issues.
