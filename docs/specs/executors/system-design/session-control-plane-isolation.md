---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004
---

# Session control-plane isolation design

## Purpose and boundaries

The backend owns token derivation and the executor guards. agentctl owns
enforcement on its instance listener and the broker relay. The agent only
carries what Kandev writes into its MCP configuration.

## Requirement mapping

| Acceptance criteria | Mechanism |
| --- | --- |
| 001.1, 001.9, 001.10 | Token derivation |
| 001.2, 001.3, 001.4, 001.5, 001.12 | Instance MCP guard |
| 001.6, 001.7, 001.8 | Token carriers |
| 001.11 | Capability advertisement |
| 002.1 to 002.11 | Broker relay |
| 003.1 to 003.7 | Executor route guards |
| 004.1, 004.2 | Bind decision |

## Token derivation

The backend computes `token = base64url(HMAC-SHA256(key, "kandev-mcp-v1:" + instanceID))`.
The key is 32 random bytes held in the existing secret store, created on first
use. The instance create request carries the token; agentctl keeps it in the
instance config as an unexported field excluded from JSON, `String()` and log
fields. Because the derivation is keyed by instance ID, a token from another
instance fails the comparison, and a new instance ID yields a new token.
Retained-instance adoption recomputes the token for comparison with
the instance's advertised capability only; it never reads the token back.

## Instance MCP guard

A middleware on the instance router runs ahead of the current exemption list.
For `/mcp`, `/sse` and `/message` it applies `subtle.ConstantTimeCompare` over
equal-length digests of the presented and expected tokens, so length does not
leak. An empty expected token yields 503 `mcp_token_unconfigured`. The token
is part of the instance config passed to the server factory, so it exists
before the listener binds. Request logging redacts the `Authorization` header
and never logs query strings for these routes.

## Token carriers

Header-capable entries (ACP `McpServer.Headers`, Codex `http_headers`, Claude
passthrough JSON, Cursor `mcp.json` `headers`) receive `Authorization: Bearer`.
The injection sites are `config.injectKandevMcpServerAtHost`,
`api.injectKandevMcpServers`, `lifecycle/cursor_plugin_mcp_reconcile.go` and
the passthrough writers in `agent/mcpconfig`. Comparisons that match the
injected entry by URL (`managed_tool_policy.go`, `manager_permission_policy.go`)
keep matching on name and URL only, ignoring headers. Config files written to
the workspace are tracked for deletion on instance delete. Where a transport
cannot carry a header the launch fails with `mcp_token_carrier_unavailable`.
The Build step must list every supported agent and its carrier in the plan
before it removes the exemption.

## Capability advertisement

agentctl `/identity` gains `session_mcp_token: true`. Lifecycle adoption of a
retained instance requires it, otherwise the existing replacement path runs.

## Broker relay

agentctl registers `GET` and `POST` `/git-credentials/resolve` on the instance
listener, outside the MCP guard. The helper must not hold the session MCP token
(AC-001.7), so the relay carries no token; the credential lease the helper
already sends is the capability, and the upstream broker validates it. The
upstream URL arrives in the create request and is fixed for the instance. The
relay uses a dedicated `http.Client` with a 15 second timeout, no redirects, a
64 KiB body limit, and a header allow-list. The orchestrator sets
`KANDEV_GITHUB_CREDENTIAL_BROKER_URL` to
`http://<instance host>:<port>/git-credentials/resolve` when the executor type
is same-host, and keeps the existing URL for remote types.

## Executor route guards

`registerHTTP` in `executor_handlers.go` and `executor_profile_handlers.go`
wrap mutating routes in `authz.RequireOrgScope(authz.ScopeOrgConfigManage)`
followed by the settings interlock, the same pair as `agent/settings`.
The `ws.ActionExecutor*Create|Update|Delete` registrations are removed. The
web client already sends the interlock header on mutations
(`lib/api/client.ts`) and uses HTTP for these calls; the Build step must
confirm no web caller uses the removed actions.

## Bind decision

`server.host` stays `0.0.0.0`. Changing it breaks LAN, container and
`kandev service` users with no migration path, and AC-001 plus AC-003 remove
the reason the change was proposed for the session channels. A gated loopback
default is filed as a follow-up.

## Ordering, idempotency, concurrency, errors

- Ordering: token installed, listener bound, config written, agent started.
- Repeated create with the same instance ID yields the same token.
- Concurrent guard checks read immutable state and need no lock.
- Missing key, empty token and unreachable upstream fail closed, as in the AC list.
