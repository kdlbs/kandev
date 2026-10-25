---
id: "11-session-token-broker-relay"
title: "Authenticate ordinary-session MCP and relay the Git broker"
status: in_progress
wave: 8
depends_on:
  - "07-mcp-connectivity"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003
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
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.3
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.4
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.5
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.6
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.7
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.8
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.9
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.10
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.11
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.12
system_design:
  - ../../specs/executors/system-design/session-control-plane-isolation.md
  - ../../specs/agents/system-design/sandbox-policies.md
---

# Task 11: Authenticate ordinary-session MCP and relay the Git broker

## Relationship to the sandbox private route

A sandboxed execution reaches Kandev MCP only through the per-attempt relay in Task 07 and never receives the instance token (`sandboxedMCPRoute`). This task covers sessions that run without a sandbox: the plain agentctl `/mcp`, `/sse` and `/message` routes require the per-instance token, and same-host agents reach the Git credential broker through their own agentctl listener.

## Open

A sandboxed child cannot reach the broker today. Its environment is filtered to granted keys and the fixed baseline, `KANDEV_*` keys cannot be granted, and the backend, standalone and instance ports are all protected. A sandboxed Git fetch or push through the broker needs its own private route, a grant to select it, and a launch-supplied credential-helper environment. That slice is not delivered here and is not claimed by this work order.


### Session MCP token

Derive, inject, carry and enforce the per-instance token on /mcp, /sse, /message.

#### Scope

- Backend HMAC derivation over instance ID plus a persisted per-create nonce; key in the secret store (create-if-absent, hidden from user secret paths); create-request field.
- agentctl guard outside activity accounting, 503 when unset, header redaction (request logs and the ACP frame debug log), /identity capability and fingerprint.
- `session_mcp_token` in survival capabilities (first addition to the required set); one verifier on every path that adopts an instance (local adoption, `processRecoveredRemoteInstance` for Sprites, remote Docker and plugin; table test per executor type) with the local and remote timing, retry and unknown-outcome rules; the unknown outcome calls the new `POST /api/v1/instances/:id/mcp-token/revoke` control route (agentctl clears the in-memory token, so MCP answers 503); Kubernetes and SSH retained-instance reuse is refused with `mcp_token_unsupported` / `retained_instance_deferred` after a revoke; mismatch deletes the instance with the fingerprint observed on its `/identity` as the `X-Expected-Token-Fingerprint` precondition (header absent = unconditional delete).
- Nonce persisted as `mcp_token_nonce` in both metadata key maps, skipped by the `prior.Metadata` merge, carried by every `ExecutorRunning.Metadata` writer (table test), with the nonce minted for the create written after every other merge so a copied sibling value is overwritten; 409 `instance_exists` and `instance_cleaning_up` mapping with the backend retry.
- Carriers for ACP, Codex (native and passthrough with version gate, variable set by agentctl in the Codex process) and Claude/OpenCode passthrough via the private `TempConfigPath` file; Cursor, Pi and custom TUI fail closed (`workspace_file_carrier_deferred`, `custom_tui_deferred`); Codex variable never in `PassthroughArtifacts.Env`.
- Single-flight ensure-key that replaces an undecryptable or wrong-length key at startup (before recovery) and at launch, logging `mcp_token_key_replaced`.
- Typed launch failures with `cause` tokens, unknown-outcome delete-by-ID, public docs for the three codes, causes, upgrade order and stuck-resume outcome.
- Bind decision: file the local-only follow-up card (REQ-003) with the EMBARGO section verbatim and record its ID in the task plan.

#### Exclusions

Broker relay, executor routes, workspace-file carriers, Kubernetes and SSH retained-instance adoption (follow-up `5c04885e-9899-4db6-992d-241cacd90dba`).

#### Acceptance

1. Missing, wrong and cross-instance tokens are refused on all three paths; the right one is served; refusals do not move activity.
2. The token is absent from logs, the ACP frame debug log, env and argv.
3. The plan records the per-agent carrier inventory (agent, class, recorded-request or cited-vendor evidence) and the Codex minimum-version constant before the exemption is removed; an agent missing from the inventory fails closed.
4. A stale rollback delete does not remove a replacement instance that reuses the ID.

#### Verification

`(cd apps/backend && go test ./internal/agentctl/... ./internal/agent/... -count=1)`

#### Likely files

internal/agentctl/server/api/server.go, auth.go; server/config/config.go; api/agent.go; lifecycle/executor_backend.go, persistence.go; agent/mcpconfig.


### Broker relay

Relay the credential broker through the instance listener for same-host executors.

#### Scope

- Relay handler for resolve and reissue, upstream URL in the create request alongside the unchanged broker URL environment value, agentctl effective broker URL for every process-visible use, readiness probe limits (20 s curl, 25 s wait).

#### Exclusions

Remote executor transport; lease semantics.

#### Acceptance

1. Same-host agents see only the instance URL; remote executors still require HTTPS.
2. Limits, timeouts, no-retry, caller-cancel and method/path rules hold; reissue works through the relay.
3. Managed-launch signals (readiness probe, credential snapshot, helper activation) still fire.

#### Verification

`(cd apps/backend && go test ./internal/agentctl/... ./internal/orchestrator/executor/... ./internal/backendapp/... -count=1)`

#### Likely files

internal/backendapp/orchestrator.go; orchestrator/executor/executor_credentials.go; agent/runtime/lifecycle/broker_reachability.go; agentctl/server/api.
