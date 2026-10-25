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
agent's only required loopback destination for Git must be its own agentctl port.

This capability makes those channels attributable and separable. It does not
confine child tasks, change the MCP tool surface, or change Git hooks. The
backend REST API stays reachable from the agent host; this capability removes the
need for the agent to use it for Git credentials and MCP, it does not close it.

## Definitions

- **Instance**: one agentctl instance. Its **instance ID** is a random UUID that
  the backend mints for every create request and supplies in the request. It is
  never an execution ID and is never reused.
- **Session MCP token**: a secret bound to one instance, carried by the agent in
  its MCP configuration.
- **MCP routes**: `/mcp` (every HTTP method), `/sse` and `/message` on the
  instance listener. The control routes under `/api/v1/mcp/` are not MCP routes
  and keep their current instance authorization.
- **Same-host executor**: an executor type for which the backend does not need
  resolved credentials (`executorNeedsResolvedCredentials` is false), which today
  is every type except SSH and the containerized types. Every other executor type
  is a **remote executor**.
- **Configuration route**: a `POST`, `PUT`, `PATCH`, or `DELETE` route whose path
  is `/api/v1/executors`, `/api/v1/executor-profiles`, or begins with
  `/api/v1/executors/:id` (including `/profiles`), as registered in
  `executor_handlers.go` and `executor_profile_handlers.go`.
- **Configuration manager**: a caller who holds `org.config.manage`, or the
  synthetic identity when authentication is disabled.

## Requirements

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001: Per-session MCP token

**Intent:** A request on an MCP route is accepted only when it carries the
token of the instance that serves it, so reaching another session's port does not
grant that session's identity.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.1:** When the backend creates an instance, it shall mint a fresh instance ID, derive a token of at least 256 bits from that ID and a backend-held secret key (HMAC), and supply both in the create request, so the backend can recompute the token after a restart from the persisted instance ID without storing the token. agentctl shall hold the token in memory for the instance's lifetime.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.2:** While an instance holds a token, each MCP route shall serve a request only when its `Authorization: Bearer` value equals that token under a constant-time comparison. This shall hold whether or not control authentication is enabled. `GET /sse` and `POST /message` are checked independently: a client that presents the token on one and not the other is refused on the other.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.3:** A request with a missing, empty, malformed, wrong, or other-instance token, or one carrying only the control credential, shall receive HTTP 401 with a generic JSON error that names no token detail. It shall open no SSE stream, create no MCP session, dispatch no tool call, and shall not extend the instance's idle deadline.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.4:** An instance that holds no token shall answer every MCP route with HTTP 503 and the code `mcp_token_unconfigured`, and shall dispatch nothing.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.5:** The token shall be installed before the instance listener accepts its first connection, so no interval exists in which an MCP route is served unauthenticated. `/health` and the non-MCP routes keep their current authorization.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.6:** Every Kandev MCP entry that Kandev composes for an agent shall carry the token, by transport class: ACP HTTP and SSE entries, Codex app-server configuration, Claude and OpenCode passthrough files, Cursor project configuration, and file-based custom TUI strategies carry it as an `Authorization: Bearer` header. Codex passthrough carries it as an `env_http_headers` mapping to a reserved environment variable that contains `TOKEN` in its name, set only in that Codex process, never in argv. When a transport or agent build cannot carry the token by its class carrier, the launch shall fail with the typed error `mcp_token_carrier_unavailable` instead of emitting an unauthenticated entry. A launch where the agent has neither HTTP nor SSE MCP capability keeps today's behavior (the Kandev entry is omitted with the existing warning) and emits no unauthenticated entry.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.7:** The token shall appear in no agent process argument list, no log line at any level (including request logs and injection debug logs), no diagnostic bundle, no persisted row, and no API or event payload, with these exact exceptions: the backend-to-agentctl create request on the authenticated control channel, the MCP configuration placements in AC-001.6 (ACP session parameters, Codex app-server configuration, and the files Kandev writes), and the reserved Codex passthrough environment variable. It shall appear in no other environment variable of any agent or tool process.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.8:** When an instance is deleted, Kandev shall remove the Kandev MCP entry that carries that instance's token from every configuration file it wrote, and shall delete a file only when Kandev created it and no other entry remains. It shall not remove or alter an entry owned by the user or an entry carrying a different instance's token. A file left behind after a crash or while the backend is down carries a token that authorizes nothing (AC-001.9).
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.9:** Within one instance, the token shall stay the same across agent process restart, context reset, new ACP session, and backend restart with the instance retained. A new instance, including one created by resume, recovery replacement, or a fresh launch for the same session, shall hold a different token because it has a new instance ID. After its instance is deleted, the old token shall authorize nothing. A create request whose instance ID is already live shall receive HTTP 409 with the code `instance_exists`, and shall change neither the live instance nor its token.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.10:** The backend shall create its secret key atomically if and only if none exists (two concurrent first uses shall end with one key). If the key exists but cannot be read or decrypted, or cannot be created, the launch shall fail with a typed error and shall not fall back to an unauthenticated or unrecoverable token. If the key is lost or replaced (including by a database restore), every retained instance shall be detected as holding a token from another key (AC-001.11) and replaced.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.11:** Each instance shall advertise on its `/identity` response the capability string `session_mcp_token` and a token fingerprint (a one-way hash of the token that reveals no token bytes). The backend shall not adopt a retained instance unless the capability is present and the fingerprint equals the fingerprint it recomputes from the persisted instance ID and its key. That instance follows the existing incompatible-instance replacement path, with the same effect on an in-flight turn as any incompatible-instance replacement today.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.12:** Concurrent requests that carry the valid token shall all be served, and concurrent requests that do not shall all be refused. A refusal shall not change the state of the instance or of any other request.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.13:** For a freshly created instance, the backend shall read the instance's `/identity` capability and fingerprint before starting the agent. If the capability is absent or the fingerprint differs (an agentctl build that ignores the token), the backend shall delete the instance and fail the launch with the typed error `mcp_token_unsupported`, and no agent shall have started.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.14:** The Kandev entry that agentctl's managed-tool policy matches or rebuilds (managed conversations, auto-approval of Kandev tools) shall be recognized when it carries exactly the one token `Authorization` header, and the rebuilt entry shall keep that header. An entry that carries any other header or a different token placement shall remain unrecognized, as it is today.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.15:** A launch that fails with `mcp_token_carrier_unavailable`, `mcp_token_unsupported`, or the key error of AC-001.10 shall surface through the existing launch-failure path of the session with that stable code in the error, shall leave no instance, listener, or token-bearing file behind, and shall add no new user-facing copy.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002: Git credential broker through the session's agentctl

**Intent:** For same-host executors the agent's Git credential helper reaches
the broker through its own agentctl port, so a network rule can allow Git
operations without allowing the backend's REST API or external `/mcp`.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.1:** For a same-host executor, the broker URL in every process agentctl starts for the instance (agent, terminals, readiness probe) shall be `http://<loopback listen address of the instance>:<instance port>/git-credentials/resolve`, composed by agentctl from its own listener. It shall not name the backend host or port, and the backend shall not place a broker URL for a same-host executor in the create request environment.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.2:** The relay shall forward `GET` (readiness) and `POST` (resolve) at exactly `/git-credentials/resolve` to the upstream broker URL that the backend supplied in the create request, and return the upstream status and body unchanged, including 3xx statuses, which it shall not follow. Any other method on that path shall receive 405. Any other path under `/git-credentials/` shall receive 404.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.3:** The caller shall not influence the upstream URL, path, or query. The relay shall forward only the request headers `Content-Type` and `Accept` (the credential lease travels in the request body), shall return only the response header `Content-Type` and the status, and shall add no backend credential.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.4:** The relay shall evaluate a request in this order and stop at the first failure: path (404), method (405), upstream configured (404, AC-002.6), request body larger than 64 KiB (413, nothing forwarded). It shall then forward, and answer 504 when the upstream does not respond within 15 seconds, 502 when the upstream is unreachable, when its response body exceeds 1 MiB, or when reading the response fails (nothing partial is returned). Error bodies shall be generic JSON that omits the upstream URL.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.5:** The relay shall forward each request exactly once, with no retry and no caching. Concurrent requests shall be independent.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.6:** An instance created without an upstream broker URL shall answer the relay path with 404.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.7:** Lease scope, lease lifetime, reissue, and backend-side validation shall be unchanged. The backend broker route shall keep its current behavior.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.8:** For a remote executor, the broker URL shall remain the operator-configured URL, shall still be validated by `validateGitHubCredentialBrokerURL`, and shall still require HTTPS. No relay shall be used.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.9:** The launch-time broker readiness probe shall run against the URL the agent will use, with an overall time limit longer than the relay's 15 second upstream limit so a relay 504 is observable. An unreachable relay or upstream shall fail the launch as an unreachable broker does today, and the failed launch shall leave no instance behind.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.10:** For a same-host executor, no broker or Git credential environment variable, Git configuration entry, helper configuration, or CLI shim given to the agent shall contain the backend host or port. When the operator sets a public base URL, that URL shall be the relay's upstream and shall not be visible to the agent. Environment variables unrelated to Git credentials that carry a backend URL are outside this criterion (see Out of scope).
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.11:** The relay shall not require or forward the session MCP token and shall be served without the instance control credential. Authorization is the credential lease in the request body, which the upstream broker validates.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.12:** Each instance shall advertise on `/identity` a one-way fingerprint of its upstream broker URL. The backend shall not adopt a retained instance whose fingerprint differs from the broker URL it would supply now (for example after the backend port changed), and that instance follows the incompatible-instance replacement path.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003: Executor configuration authorization

**Intent:** Changing an executor or executor profile, which changes what runs
on the next launch, requires the same authorization as changing agent settings,
without taking away what the per-user ownership capability already grants owners.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.1:** Every configuration route shall require, in this order, (a) authorization to mutate the target and (b) the interim settings interlock. Authorization to mutate is: the caller is a configuration manager; or the target is an owned resource whose owner is the caller; or the request creates a resource that names the caller as its owner. Creating or mutating a shared resource, and assigning any owner other than the caller, require a configuration manager. Read routes shall keep their current behavior.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.2:** A caller who is not authorized to mutate the target shall receive 403 `forbidden`. A caller who is authorized and has no valid interlock shall receive 403 `interim_settings_interlock_required`. Neither refusal shall change stored data or publish an event.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.3:** A caller that presents any `Authorization: Bearer` credential (Office bearer or `kandev_pat_*`) shall be refused by the interlock on configuration routes, as agent-settings routes already refuse it. Browser settings sessions are unaffected.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.4:** The WebSocket actions that create, update, or delete an executor or executor profile shall not be reachable. A request for one shall receive the dispatcher's unknown-action error. The list and get actions shall be unchanged.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.5:** A test shall enumerate the registered router and fail when a configuration route lacks either guard.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.6:** With authentication disabled, the synthetic identity shall satisfy the authorization check and the interlock shall still be required.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.7:** The settings pages that create, edit, and delete executors and profiles shall keep working for configuration managers and for owners of their own resources, with no change to what the user does. A browser request that carries no interlock token is refused with the code of AC-003.2 and the existing settings error display applies.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004: Default backend bind address

**Intent:** The default bind decision is recorded with its reason, and no
default changes without a gated rollout.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.1:** The default of `server.host` shall remain `0.0.0.0`.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.2:** The system design shall record why the default is unchanged and the scope of the gated loopback-default follow-up. The Build step shall file that follow-up card (not started, EMBARGO text copied verbatim) and record its identifier in the task plan.

## Out of scope

- Child-task confinement and the MCP tool surface, including the `mcp.create_executor_profile`, `mcp.update_executor_profile`, and `mcp.delete_executor_profile` configuration tools, which also change profiles and are governed by the MCP tool surface card.
- Git hooks, fsmonitor and filter configuration.
- Enabling authentication by default.
- Token rotation while an instance is live, and rotation of the backend derivation key. A lost key makes retained instances unauthenticatable, and they are replaced (AC-001.10, AC-001.11).
- Network policy rendering by a sandbox provider.
- Remote-executor broker transport. It stays operator-configured HTTPS.
- A loopback default for the backend bind. See REQ-004 and the follow-up card.
- Executor action routes that do not change stored configuration: `POST /api/v1/ssh/executors/:id/probe-agents`, `probe-shells`, `reachability/probe`, and `POST /api/v1/remote-docker/executors/:id/build` (already admin-only). They stay as they are.
- Other environment variables that give the agent the backend URL (the Office `KANDEV_API_URL` and the plugin runtime API URL). The backend REST API therefore stays reachable from the agent until the confinement card removes those, and a network rule cannot yet deny it outright.
