---
id: "01"
title: "Session MCP token"
status: todo
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001
acceptance_criteria:
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.3
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.4
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.5
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.6
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.7
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.8
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.9
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.10
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.11
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.12
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.13
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.14
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.15
system_design:
  - ../../specs/executors/system-design/session-control-plane-isolation.md
---

# Task 01: Session MCP token

## Summary

Derive, inject, carry and enforce the per-instance token on /mcp, /sse, /message.

## Scope

- Backend HMAC derivation over instance ID plus a persisted per-create nonce; key in the secret store (create-if-absent, hidden from user secret paths); create-request field.
- agentctl guard outside activity accounting, 503 when unset, header redaction (request logs and the ACP frame debug log), /identity capability and fingerprint.
- `session_mcp_token` in survival capabilities; per-instance fingerprint mismatch deletes the instance during standalone recovery.
- Carriers for ACP, Codex (native and passthrough with version gate), Claude, OpenCode, Pi, both Cursor writers and custom TUI strategies; shared file handling (mutex, symlink/malformed refusal, local exclude).
- Typed launch failures, unknown-outcome delete-by-ID, public docs for the three codes.

## Exclusions

Broker relay, executor routes.

## Acceptance

1. Missing, wrong and cross-instance tokens are refused on all three paths; the right one is served; refusals do not move activity.
2. The token is absent from logs, the ACP frame debug log, env and argv.
3. The plan records the per-agent carrier inventory and the Codex minimum-version constant before the exemption is removed.

## Verification

`(cd apps/backend && go test ./internal/agentctl/... ./internal/agent/... -count=1)`

## Likely files

internal/agentctl/server/api/server.go, auth.go; server/config/config.go; api/agent.go; lifecycle/cursor_plugin_mcp_reconcile.go; agent/mcpconfig.
