---
id: "03-keepalive-floor-guidance"
title: "Keepalive, harness floor, and agent guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-MCP-WAIT-002
  - REQ-PLATFORM-DETACHED-MCP-WAIT-006
acceptance_criteria:
  - AC-PLATFORM-DETACHED-MCP-WAIT-002.1
  - AC-PLATFORM-DETACHED-MCP-WAIT-002.2
  - AC-PLATFORM-DETACHED-MCP-WAIT-002.3
  - AC-PLATFORM-DETACHED-MCP-WAIT-006.1
  - AC-PLATFORM-DETACHED-MCP-WAIT-006.2
  - AC-PLATFORM-DETACHED-MCP-WAIT-006.3
system_design:
  - ../../specs/platform/system-design/detached-mcp-call-wait.md
---

# Task 03: Keepalive, harness floor, and agent guidance

## Summary

Send progress notifications during every Kandev call, raise the harness tool
timeout to cover the budget, and tell agents calls can wait.

## In scope

- Generic keepalive wrapper around Kandev tool handlers, replacing the
  `ask_user_question` pinger; one monotonic stream, token synthesized when the
  client sent none, send failure ignored.
- `RuntimeConfig.ToolTimeoutEnvKey`, floor computation, and
  `ErrToolTimeoutBelowOfflineBudget`. The budget is read from launch metadata
  with a 15-minute fallback, so this task does not depend on Task 02.
- `kandev-context.md` connection-loss line through `sysprompt`.

## Out of scope

Agents with no declared tool-timeout key.

## Verification

- `go test -trimpath ./internal/mcp/server/ ./internal/agent/agents/ ./internal/agent/runtime/lifecycle/ ./internal/sysprompt/ -run 'KeepAlive|ToolTimeout|ConnectionLoss' -count=1`
