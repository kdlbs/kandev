---
id: "03-pin-and-reconnect-claude-acp"
title: "Verify the Claude ACP bridge and reconnect path"
status: done
wave: 2
depends_on: ["01-task-only-eager-tool"]
plan: "plan.md"
spec: "../../specs/workflow/explicit-completion-signal/spec.md"
---

# Task 03: Verify the Claude ACP bridge and reconnect path

## Acceptance

- Claude ACP command construction consistently uses the unversioned `@agentclientprotocol/claude-agent-acp` package.
- ACP `session/new` and `session/load` both receive the local Kandev MCP configuration, preferring streamable HTTP with SSE fallback.
- A reconnect-shaped test lists and calls `step_complete_kandev` after session load; no speculative retry or tool re-registration is added without a failing reproduction.

## Verification

```bash
cd apps/backend && rtk go test ./internal/agent/agents ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/acp -run 'Test.*(Claude|Mcp|MCP|LoadSession|NewSession)' -count=1
```

## Files Likely Touched

- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/agents/claude_acp_test.go`
- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/mcp_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_test.go`

## Inputs

- Existing HTTP-first Kandev MCP injection in `internal/agentctl/server/api/agent.go`.
- Existing initialization log records bridge name/version.
- Field evidence proves catalog loss during reconnect but does not prove the disconnect trigger.

## Output Contract

Report argv consistency, request-capture evidence, whether a red transport repro was obtained, files changed, commands/results, residual unknowns, and task status. Use TDD and do not edit `plan.md`.

## Implementation Evidence

- RED: `rtk go test ./internal/agent/agents -run TestClaudeACPCommandsUseUnversionedBridgePackage -count=1` failed because normal, runtime, inference, and install paths still selected the previously introduced versioned package.
- GREEN: normal, runtime, inference, and install command paths consistently use the unversioned `@agentclientprotocol/claude-agent-acp` package.
- API request-capture tests prove `agent.session.new` and `agent.session.load` both receive local Kandev HTTP and SSE entries in HTTP-first order.
- ACP request-capture tests prove both protocol requests select HTTP when the agent supports both transports and select SSE when only SSE is supported.
- A reconnect-shaped Streamable HTTP test initializes and lists/calls `step_complete_kandev`, loads the ACP session, then establishes a fresh MCP session and successfully lists/calls the tool again.
- No deterministic catalog-loss or transport defect reproduced, so no retry loop, tool re-registration, or other transport production change was added.
- Packet verification with an isolated writable Go cache passed 45 tests across the three owned packages.
- A full-package run passed 433 tests and timed out only in the pre-existing `TestLoadReplayBurst_HandlesLargeReplay` stress test after 10 minutes; that test passed immediately when rerun alone. A subsequent full API rerun was blocked by the sandbox denying `httptest` a loopback listener. These full-suite limitations are unrelated to the bridge command or session request paths.

### Review Correction

- Renamed the in-process API test to
  `TestMCPToolCatalogRemainsAvailableAfterAgentSessionLoad`. It proves server
  catalog continuity after `agent.session.load`; it does not simulate or prove
  a Claude ACP bridge reconnect.
- Both opt-in real Claude bridge wakeup probes retain their pre-existing
  `@agentclientprotocol/claude-agent-acp@0.31.1` fixture version; production
  commands intentionally remain unversioned.
- The focused API test passes, and both `wakeup_e2e` packages compile with the
  build tag and an empty test selector. The network- and credential-dependent
  probes were intentionally not executed.
- Residual uncertainty remains: a real Claude bridge catalog loss across an
  observed disconnect/reconnect has not been reproduced, so this task still
  does not justify transport retry or tool re-registration changes.
