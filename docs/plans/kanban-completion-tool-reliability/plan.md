---
spec: docs/specs/workflow/explicit-completion-signal/spec.md
created: 2026-07-22
status: implemented
---

# Implementation Plan: Kanban Completion Tool Reliability

## Overview

Harden the existing `step_complete_kandev` control signal for Kanban sessions without changing its workflow semantics or exposing it to Office. First lock the mode and metadata contract with regression tests, then clarify canonical versus client-qualified names and verify new/load/reconnect configuration paths. The unknown upstream disconnect trigger is not a license for speculative transport changes.

## Backend

### Task-only discoverable tool contract

Files:
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`

Changes:
- Keep `step_complete_kandev` on the standard MCP definition without `anthropic/alwaysLoad` or other vendor-specific eager-load metadata.
- Keep registration exclusively inside the `ModeTask` branch.
- Test discovery through the task-mode catalog and absence from Office/config/external modes.

### Client-neutral naming guidance

Files:
- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `docs/public/automation-and-mcp.md`
- `docs/public/agent-communication.md`

Changes:
- Describe listed names as canonical MCP protocol names and note that a client may display a server-qualified runtime alias.
- Keep the `step_complete_kandev` instruction conditional on `auto_advance_requires_signal`.
- Do not instruct every client to call Claude's qualified alias directly; the registered tool schema remains authoritative.
- Document that the signal is task-mode only and may require the client's normal tool-search flow. If transport recovery fails, the task stays put for retry or a normal human workflow move.

### Claude ACP session wiring

Files:
- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/mcp_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_test.go`

Changes:
- Preserve the existing unversioned Claude ACP package selection and adapter initialization logging of bridge name/version.
- Assert the local Kandev MCP server is supplied on both `session/new` and `session/load`, with HTTP preferred and SSE retained as fallback.
- Add a reconnect-shaped contract test: initialize task mode, list the completion tool, load the session with the same MCP config, then list/call it again.
- If the test cannot reproduce an actual catalog loss, do not add retry loops or server re-registration. Retain ACP/MCP logging instructions for the next field capture.

## Frontend

No frontend change. ADR 0015's planned manual completion fallback is not currently implemented and remains outside this reliability fix.

## Tests

- **What:** task-mode `tools/list` includes discoverable `step_complete_kandev` without vendor-specific eager metadata; every restricted mode excludes it.
  **File:** `apps/backend/internal/mcp/server/server_test.go`
  **How:** mode table plus JSON serialization of the registered tool.
- **What:** workflow flag controls prompt advertisement but never tool registration.
  **File:** `apps/backend/internal/sysprompt/sysprompt_test.go`, `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
  **How:** format contexts with the flag both ways and compare against mode inventories.
- **What:** both ACP new and load requests receive the local Kandev MCP server and preserve HTTP/SSE selection.
  **File:** `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_test.go`, `apps/backend/internal/agentctl/server/api/agent_test.go`
  **How:** fake ACP connection request capture.
- **What:** reconnect-shaped MCP reinitialization still lists and calls the task-only completion tool.
  **File:** `apps/backend/internal/mcp/server/server_test.go` or a focused sibling integration test.
  **How:** use the streamable HTTP test server; avoid a live model dependency.
- **What:** a signal-gated Kanban step profile selects a runner without selecting Office mode.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`
  **How:** persist the workspace, workflow, step, task, and session in SQLite; assert the runner and Office projections independently, then launch the created session and verify task-mode discovery guidance.

## E2E Tests

No browser behavior changes. A live-provider test is not required in CI. The QA packet records an optional `acp-debug` probe against the currently resolved bridge when credentials and network are available.

## Implementation Waves

Wave 1:
- [x] [task-01-task-only-discoverable-tool](task-01-task-only-eager-tool.md)

Wave 2 (parallel after the Office prompt-context task has landed):
- [x] [task-02-canonical-tool-naming](task-02-canonical-tool-naming.md)
- [x] [task-03-claude-acp-session-wiring](task-03-pin-and-reconnect-claude-acp.md)

Wave 3:
- [x] [task-04-kanban-reliability-verification](task-04-kanban-reliability-verification.md)

## Verification Commands

```bash
rtk make -C apps/backend fmt
cd apps/backend && rtk go test ./internal/mcp/server ./internal/sysprompt
cd apps/backend && rtk go test ./internal/agent/agents ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/acp
rtk make -C apps/backend lint
rtk make -C apps/backend test
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Risks

- Deferred tool discovery is client behavior. Agents must use the active client's search mechanism, and a disconnected MCP transport can still make the tool temporarily unavailable.
- The affected field run lacked ACP/MCP logs, so the precise disconnect trigger remains unproven. Production transport behavior must not be changed without a red reproduction or new trace evidence.
- The unversioned Claude ACP dependency can change independently of a Kandev release, so field reports must include the bridge-reported version.

## Open Questions

None for this pass. The plan deliberately limits remediation to task-mode registration, truthful discovery guidance, and existing reconnect wiring; it does not add eager-load metadata or change Claude ACP package resolution.
