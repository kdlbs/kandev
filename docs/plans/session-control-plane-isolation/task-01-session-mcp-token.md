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
- `session_mcp_token` in survival capabilities (first addition to the required set); one verifier on every recovered instance of every executor (local adoption and `processRecoveredRemoteInstance`) with the read timeout, retry and unknown-outcome rules; mismatch deletes the instance with the token-fingerprint precondition.
- Nonce carry-forward across every `ExecutorRunning.Metadata` writer (table test), Kubernetes resume that keeps the retained instance, 409 `instance_exists` and `instance_cleaning_up` mapping with the backend retry.
- Carriers for ACP, Codex (native and passthrough with version gate), Claude, OpenCode, Pi, both Cursor writers and custom TUI strategies; shared file handling on `internal/common/passthroughconfig` (cross-process lock, ownership claims, symlink/malformed refusal, local exclude), including the Cursor ownership record `created` field.
- Typed launch failures, unknown-outcome delete-by-ID, public docs for the three codes.

## Exclusions

Broker relay, executor routes.

## Acceptance

1. Missing, wrong and cross-instance tokens are refused on all three paths; the right one is served; refusals do not move activity.
2. The token is absent from logs, the ACP frame debug log, env and argv.
3. The plan records the per-agent carrier inventory (agent, class, adapter test) and the Codex minimum-version constant before the exemption is removed; an agent missing from the inventory fails closed.
4. A stale rollback delete does not remove a replacement instance that reuses the ID.

## Verification

`(cd apps/backend && go test ./internal/agentctl/... ./internal/agent/... -count=1)`

## Likely files

internal/agentctl/server/api/server.go, auth.go; server/config/config.go; api/agent.go; lifecycle/cursor_plugin_mcp_reconcile.go; agent/mcpconfig.
