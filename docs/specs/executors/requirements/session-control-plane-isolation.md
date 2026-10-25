---
status: draft
system: executors
created: 2026-10-25
owners:
  - kandev
---

# Session control-plane isolation requirements

## Overview

A running agent session reaches Kandev through three channels: the MCP endpoint
served by its own agentctl instance, the Git credential broker, and the settings
REST surface. Each channel must be attributable to exactly one session, and the
agent's only required loopback destination must be its own agentctl port.

This capability makes those channels attributable and separable. It does not
confine child tasks, change the MCP tool surface, or change Git hooks.

## Definitions

- **Instance**: one agentctl instance, identified by its instance ID. One
  session execution owns one instance at a time.
- **Session MCP token**: a secret bound to one instance, carried by the agent in
  its MCP configuration.
- **MCP routes**: `/mcp` (every HTTP method), `/sse` and `/message` on the
  instance listener. The control routes under `/api/v1/mcp/` are not MCP routes
  and keep their current instance authorization.
- **Same-host executor**: an executor type for which the backend currently
  permits a plain-HTTP loopback broker URL (local and worktree). Every other
  executor type is a **remote executor**.

## Requirements

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001: Per-session MCP token

**Intent:** A request on an MCP route is accepted only when it carries the
token of the instance that serves it, so reaching another session's port does not
grant that session's identity.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.1:** When the backend creates an instance, it shall supply a token of at least 256 bits derived from the instance ID and a backend-held secret key (HMAC), so the backend can recompute it after a restart without storing it. agentctl shall hold the token in memory for the instance's lifetime.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.2:** While an instance holds a token, each MCP route shall serve a request only when its `Authorization: Bearer` value equals that token under a constant-time comparison. This shall hold whether or not control authentication is enabled.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.3:** A request with a missing, empty, malformed, wrong, or other-instance token, or one carrying only the control credential, shall receive HTTP 401 with a generic JSON error that names no token detail. It shall open no SSE stream, create no MCP session, and dispatch no tool call.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.4:** An instance that holds no token shall answer every MCP route with HTTP 503 and the code `mcp_token_unconfigured`, and shall dispatch nothing.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.5:** The token shall be installed before the instance listener accepts its first connection, so no interval exists in which an MCP route is served unauthenticated. `/health` and the non-MCP routes keep their current authorization.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.6:** Every Kandev MCP entry that Kandev composes for an agent (ACP HTTP and SSE entries, Codex and Claude passthrough configuration, Cursor plugin configuration, utility-agent sessions) shall carry the token as an `Authorization: Bearer` header. When an agent or transport cannot carry a header, the launch shall fail with the typed error `mcp_token_carrier_unavailable` instead of emitting an unauthenticated entry.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.7:** The token shall appear in no agent process environment, no process argument list, no log line at any level (including request logs and injection debug logs), no diagnostic bundle, no persisted row, and no API or event payload. It may appear only in the agent's MCP configuration: ACP session parameters and the configuration files Kandev writes for passthrough and Cursor agents.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.8:** A configuration file that contains the token shall be removed when its instance is deleted.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.9:** Within one instance, the token shall stay the same across agent process restart, context reset, new ACP session, and backend restart with the instance retained. A new instance, including one created by resume, recovery replacement, or a fresh launch for the same session, shall hold a different token. After its instance is deleted, the old token shall authorize nothing.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.10:** If the backend cannot read or create its secret key, the launch shall fail with a typed error and shall not fall back to an unauthenticated or unrecoverable token. If the key is lost, retained instances shall be treated as incompatible and replaced.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.11:** The backend shall not adopt a retained agentctl instance that does not advertise session MCP token support. That instance follows the existing incompatible-instance replacement path.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.12:** Concurrent requests that carry the valid token shall all be served, and concurrent requests that do not shall all be refused. A refusal shall not change the state of the instance or of any other request.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002: Git credential broker through the session's agentctl

**Intent:** For same-host executors the agent's Git credential helper reaches
the broker through its own agentctl port, so a network rule can allow Git
operations without allowing the backend's REST API or external `/mcp`.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.1:** For a same-host executor, the broker URL placed in the agent environment shall name the session's own instance listener and the relay path `/git-credentials/resolve`. It shall not name the backend host or port.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.2:** The relay shall forward `GET` (readiness) and `POST` (resolve) to the upstream broker URL that the backend supplied when it created the instance, and return the upstream status and body unchanged. Any other method shall receive 405. Any other path under the relay shall receive 404.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.3:** The caller shall not influence the upstream URL, path, or query. The relay shall forward only `Content-Type`, `Accept`, and the credential-lease headers the helper sends, and shall add no backend credential.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.4:** The relay shall refuse a request body larger than 64 KiB with 413. It shall answer 504 when the upstream does not respond within 15 seconds, and 502 when the upstream is unreachable. Error bodies shall be generic JSON that omits the upstream URL.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.5:** The relay shall forward each request exactly once, with no retry and no caching. Concurrent requests shall be independent.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.6:** An instance created without an upstream broker URL shall answer the relay path with 404.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.7:** Lease scope, lease lifetime, reissue, and backend-side validation shall be unchanged. The backend broker route shall keep its current behavior.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.8:** For a remote executor, the broker URL shall remain the operator-configured URL, shall still be validated by `validateGitHubCredentialBrokerURL`, and shall still require HTTPS. No relay shall be used.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.9:** The launch-time broker readiness probe shall run against the URL the agent will use. An unreachable relay or upstream shall fail the launch as an unreachable broker does today.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.11:** The relay shall not require or forward the session MCP token. Authorization remains the credential lease that the upstream broker validates.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.10:** For a same-host executor, no environment variable, Git configuration entry, helper configuration, or CLI shim given to the agent shall contain the backend host or port. When the operator sets a public base URL, that URL shall be the relay's upstream and shall not be visible to the agent.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003: Executor configuration authorization

**Intent:** Changing an executor or executor profile, which changes what runs
on the next launch, requires the same authorization as changing agent settings.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.1:** Every `POST`, `PUT`, `PATCH`, and `DELETE` route whose path contains `/executors` or `/executor-profiles` shall require `org.config.manage` and the interim settings interlock, in that order. Read routes shall keep their current behavior.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.2:** A caller without the scope shall receive 403 `forbidden`. A caller with the scope and without a valid interlock shall receive 403 `interim_settings_interlock_required`. Neither refusal shall change stored data or publish an event.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.3:** A caller that presents an Office bearer credential shall be refused by the interlock, as agent-settings routes already refuse it.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.4:** The WebSocket actions that create, update, or delete an executor or executor profile shall not be reachable. A request for one shall receive the dispatcher's unknown-action error. The list and get actions shall be unchanged.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.5:** A test shall enumerate the registered router and fail when a mutating route in the AC-003.1 path set lacks both guards.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.6:** With authentication disabled, the synthetic identity shall satisfy the scope check and the interlock shall still be required.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.7:** The settings pages that create, edit, and delete executors and profiles shall keep working with no change to what the user does.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004: Default backend bind address

**Intent:** The default bind decision is recorded with its reason, and no
default changes without a gated rollout.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.1:** The default of `server.host` shall remain `0.0.0.0`.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.2:** The system design shall record why the default is unchanged, and a follow-up card shall exist for a gated, documented loopback default.

## Out of scope

- Child-task confinement and the MCP tool surface.
- Git hooks, fsmonitor and filter configuration.
- Enabling authentication by default.
- Token rotation while an instance is live, and rotation of the backend derivation key. A lost key makes retained instances unauthenticatable, and they are replaced (AC-001.10, AC-001.11).
- Network policy rendering by a sandbox provider.
- Remote-executor broker transport. It stays operator-configured HTTPS.
- A loopback default for the backend bind. See REQ-004 and the follow-up card.
