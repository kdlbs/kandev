---
created: 2026-08-31
status: complete
requirements:
  - REQ-TASKS-MCP-TOOL-NAMES-001
system_design:
  - ../../specs/tasks/system-design/mcp-tool-name-stability.md
legacy_specs: []
---

# Implementation Plan: Auggie MCP Tool Names

## Overview

Add one agent runtime capability and carry it to the per-instance MCP server.
Then adapt only the transport-facing tool names and incoming calls of that server.
The implementation order keeps the canonical registry and system prompts stable
while proving the capability reaches every executor before the MCP boundary
uses it.

## Confirmed root cause

Kandev registers built-in tools with a `_kandev` suffix and injects its local
server as `kandev`. Auggie appends the server name to each presented tool.
This operation produces `*_kandev_kandev`. Cursor ACP and other
clients do not add that namespace. Current focused baseline tests pass, so the
regression test must first encode the missing Auggie-specific round trip.

## Scope

### In scope

- Declare the Auggie server-namespacing behavior in the agent runtime
  configuration.
- Propagate the capability through local, Docker, SSH, and Sprites agentctl
  instance creation.
- Translate built-in `*_kandev` names at the per-instance MCP list and call
  boundaries for agents that enable the capability.
- Preserve canonical registry keys, validators, handler names, server identity,
  and system-prompt references.
- Add regression coverage for `ask_user_question_kandev`,
  `get_task_plan_kandev`, all built-in profile tools, and representative
  non-namespacing agents.

### Out of scope

- Modifying the external Auggie CLI.
- Renaming canonical built-in or plugin-contributed tools.
- Changing MCP schemas, profiles, permissions, or prompt content.
- Adding frontend or browser behavior.

## Technical approach

### Agent capability and instance contract

- Add `NamespacesMCPToolsByServer` to
  `apps/backend/internal/agent/agents.RuntimeConfig` and enable it only in
  `Auggie.Runtime()`.
- Copy the value through the lifecycle executor request builders,
  `internal/agent/runtime/agentctl.CreateInstanceRequest`, the public agentctl
  create request, `config.InstanceOverrides`, and `config.InstanceConfig`.
- Keep the zero value false and add producer/serialization/consumer assertions
  so Cursor ACP, Codex ACP, and Claude ACP remain unchanged.

### MCP transport presentation

- Add an immutable per-instance presentation setting to
  `apps/backend/internal/mcp/server.Server`, configured in
  `apps/backend/cmd/agentctl/main.go` before routes are served.
- Use the existing MCP hooks around `tools/list` and `tools/call`.
- For opted-in clients, strip one trailing `_kandev` from built-in definitions.
  Restore the suffix before registry lookup.
- Resolve against the live canonical registry, prefer exact registered names,
  and leave non-suffixed plugin tools untouched.
- Keep `server.MCPServer.ListTools()`, argument validators, `wrapHandler`, and
  system-prompt synchronization canonical.

## Tests

- **AC-TASKS-MCP-TOOL-NAMES-001.1 and .3:** Add MCP transport coverage for task,
  configuration, external, Office, and automation profiles. Simulate the
  server-name append from Auggie. Assert that each built-in result has exactly
  the canonical name. Include explicit assertions for
  `ask_user_question_kandev` and `get_task_plan_kandev`.
- **AC-TASKS-MCP-TOOL-NAMES-001.2:** Assert representative false-capability
  agents receive the unchanged canonical list.
- **AC-TASKS-MCP-TOOL-NAMES-001.4:** Send a bare transport call through the MCP
  request boundary and assert the canonical handler and backend action execute.
- **AC-TASKS-MCP-TOOL-NAMES-001.5:** Extend request, configuration, and executor
  tests so new/load/reset session paths share the configured per-instance MCP
  endpoint behavior.
- Keep the existing sysprompt-to-registry synchronization suite green to prove
  prompt references did not change.

## E2E tests

No browser E2E is required because this is an agent/MCP protocol boundary with
no web UI change. When an authenticated Auggie runtime is available, do the
documented live ACP-log test after the automated suite. This evidence depends
on the environment. It does not replace the regression tests.

## Work orders

- [x] [Task 01: Preserve canonical names for server-namespacing agents](task-01-preserve-canonical-tool-names.md) — complete

## Verification results

- Focused work-order packages passed: 3,278 tests across agents, agentctl,
  lifecycle, instance config, instance manager, MCP server, and sysprompt.
- `go test ./cmd/agentctl`: passed 94 tests.
- Race-enabled MCP and lifecycle tests: passed 2,451 tests.
- The raw `make -C apps/backend test` run was affected by the managed session's
  inherited `KANDEV_INTERNAL_CONFIG_FILE` and failed only unrelated config
  discovery tests. The same target passed after unsetting the managed config
  handoff variables, with all backend packages green.
- `make -C apps/backend lint`: passed with zero issues.
- `python3 scripts/lint-spec-files.py --all`, `gofmt -l`, and `git diff --check`:
  passed.

## Risks

- A propagation omission in one executor will leave that launch mode broken.
  every request builder needs a focused assertion.
- Hook ordering must restore the canonical name before MCP lookup and argument
  validation while reporting the definitions actually returned to the client.
- Dynamic tool replacement must not cache stale aliases.
- Plugin tools use a different canonical namespace and must not be rewritten by
  the trailing-suffix adapter.
