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

This makes them attributable and separable; it does not confine child tasks or
change the MCP tool surface, Git hooks, or the backend REST API.

## Definitions

- **Instance**: one agentctl instance. Its **instance ID** is the existing create request ID and `X-Instance-ID`: the execution ID, except on a Kubernetes resume that reuses the retained instance (the ID recorded on the previous execution). It is unchanged.
- **Token nonce**: 16 random bytes minted per instance create, persisted in the execution record metadata as `mcp_token_nonce`.
- **Session MCP token**: a secret bound to one instance and nonce, carried in the agent's MCP configuration.
- **MCP routes**: `/mcp` (every HTTP method), `/sse` and `/message` on the
  instance listener. Control routes under `/api/v1/mcp/` are unchanged.
- **Same-host executor**: an executor type for which
  `executorNeedsResolvedCredentials` is false (today every type except SSH and
  the containerized types). Every other type is a **remote executor**.
- **Broker URL environment value**: the value the backend places in the
  `KANDEV_GITHUB_CREDENTIAL_BROKER_URL` environment variable of the create
  request. Its presence is the existing managed-Git-launch signal.
- **Configuration route**: a `POST`, `PUT`, `PATCH`, or `DELETE` route registered
  in `executor_handlers.go` or `executor_profile_handlers.go` whose path is
  `/api/v1/executors` or begins with `/api/v1/executors/` or
  `/api/v1/executor-profiles/` (today including `DELETE /api/v1/executor-profiles/:profileId`).
- **Configuration manager**: a caller holding `org.config.manage`, or the synthetic identity when authentication is disabled.

## Requirements

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001: Per-session MCP token

**Intent:** An MCP route accepts only its own instance's token, so reaching another session's port grants no identity.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.1:** When the backend creates an instance, it shall mint a fresh token nonce, derive a token of at least 256 bits from the instance ID, the nonce and a backend-held secret key (HMAC), and supply the token in the create request. It shall persist the nonce in the execution record metadata so it can recompute the token after a restart without storing the token. agentctl shall hold the token in memory only.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.2:** While an instance holds a token, each MCP route shall serve a request only when its `Authorization: Bearer` value equals that token under a constant-time comparison. This shall hold whether or not control authentication is enabled. `GET /sse` and `POST /message` are checked independently: a client that presents the token on one and not the other is refused on the other.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.3:** A request with a missing, empty, malformed, wrong, or other-instance token, or only the control credential, shall receive HTTP 401 with a generic JSON error naming no token detail. It shall open no SSE stream, create no MCP session, dispatch no tool call, and not change the instance's in-flight count or last-activity time. Refusals are logged at most once per minute at WARN with route and reason (`missing`, `malformed`, `mismatch`), never the value.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.4:** An instance that holds no token shall answer every MCP route with HTTP 503 and the code `mcp_token_unconfigured`, dispatching nothing.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.5:** The token shall be installed before the instance listener accepts its first connection. `/health` and non-MCP routes keep their authorization.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.6:** Every Kandev MCP entry that Kandev composes for an agent shall carry the token as an `Authorization: Bearer` header, for every class in the design's carrier table. Codex passthrough instead uses `env_http_headers` naming a reserved `TOKEN` environment variable set only in the environment of the command Kandev starts for Codex, never in argv, under the design's conditions. A carrier is available only for an agent the plan's inventory records as verified, by the design's evidence rule, to forward the header; otherwise, including for an agent absent from the inventory, the launch shall fail with the typed error `mcp_token_carrier_unavailable` instead of emitting an unauthenticated entry. An agent with neither HTTP nor SSE MCP capability keeps today's behavior (entry omitted with the existing warning).
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.7:** The token shall appear in no agent process argument list, log line at any level (including request, injection debug and ACP frame debug logs and the served frame output), diagnostic bundle, persisted row, or API or event payload, except the create request on the authenticated control channel, the AC-001.6 placements, and the reserved Codex variable. It shall appear in no other environment variable of any process Kandev starts.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.8:** When an instance is deleted, Kandev shall remove from every configuration file it wrote the Kandev MCP entry that carries that instance's token, and shall delete a file only when Kandev created it and no other entry remains. It shall not alter an entry owned by the user or carrying a different instance's token. Writes and removals to one file shall be serialized across processes. A symbolic-link target or invalid-JSON file is never written or removed through: at launch the launch fails with `mcp_token_carrier_unavailable`; at removal the file is left untouched with a warning naming the path and the instance delete still succeeds. A file so left, or left by a crash, holds a token that authorizes nothing once its instance is deleted or reaped.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.9:** Within one instance, the token shall stay the same across agent restart, context reset, new ACP session, and backend restart with the instance retained, including a resume that keeps a retained Kubernetes instance (the nonce is carried onto the new record; AC-001.11 applies). Any instance a launch creates, including from resume or recovery replacement, shall hold a different token because it has a new nonce, even when it reuses the ID. A create whose instance ID is live shall receive HTTP 409 `instance_exists`, and one still being cleaned up HTTP 409 `instance_cleaning_up`; neither changes the existing instance. The backend retries the second three times, one second apart, then fails the launch, deleting nothing.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.10:** The backend's secret key shall be created atomically if and only if none exists (two concurrent first uses end with one key), stored install-wide, and absent from every user-facing secret operation. If it cannot be read, decrypted, or created, the launch shall fail with the typed error `mcp_token_key_unavailable` and shall not fall back to an unauthenticated or unrecoverable token. If the key is lost or replaced (including by a database restore), every retained instance shall be detected (AC-001.11) and replaced.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.11:** Each instance shall advertise on `/identity` the capability `session_mcp_token` and a token fingerprint (a one-way hash). The backend shall not adopt a retained instance unless the capability is present and the fingerprint equals the one it recomputes from the instance ID, the persisted token nonce and its key; a missing nonce is a mismatch. The check runs for every instance any executor's recovery returns (standalone, SSH, Sprites, Kubernetes, remote Docker, plugin) before it is registered. An instance whose identity cannot be read within the design's timeout and attempts is neither adopted nor deleted: the record is kept and retried through the existing unknown-outcome path, and a malformed response is a mismatch. A mismatching instance shall be deleted (fenced as in AC-001.15) and omitted from the recovered set, so the session continues through the existing resume path on a new instance; while that delete has failed, the replacement create is refused with 409 and the launch fails through the existing launch-failure path. The capability also joins the survival capabilities required at control-server adoption, so a retained older control server is replaced and its sessions resume.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.12:** Concurrent requests with the valid token are all served and those without it all refused; a refusal changes no state.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.13:** For a freshly created instance, the backend shall read the instance's `/identity` capability and fingerprint before starting the agent. If the capability is absent, the fingerprint differs, or the identity stays unreadable (design), it shall delete the instance and fail the launch with `mcp_token_unsupported`, with no agent started.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.14:** The Kandev entry that agentctl's managed-tool policy matches or rebuilds shall be recognized when it carries exactly the one token `Authorization` header, and the rebuilt entry shall keep it. Any other header or token placement stays unrecognized, as today.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.15:** A launch that fails with `mcp_token_carrier_unavailable`, `mcp_token_unsupported`, or `mcp_token_key_unavailable` shall surface through the session's existing launch-failure path with that code at the start of the error text and one fixed English diagnostic sentence (no translated copy), and leave no token-bearing file, instance, or listener once the instance delete succeeds, except a file AC-001.8 refused to touch. Every rollback delete carries the minted token's fingerprint as a precondition; a different fingerprint means no delete and counts as success. When the create outcome is unknown, the backend shall delete by instance ID with that precondition (three attempts within ten seconds), else log a warning and leave it to the idle reaper; the Kubernetes ambiguous-create adoption remains and the adopted instance shall pass AC-001.13 or be deleted likewise. The public executors documentation shall name the three codes, the recovery action, and the one-time control-server restart on upgrade.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002: Git credential broker through the session's agentctl

**Intent:** For same-host executors the Git credential helper reaches the broker
through the session's own agentctl port, so a network rule can allow Git without
allowing the backend's REST API or external `/mcp`.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.1:** For a same-host executor whose instance holds an upstream broker URL, the broker URL given to any process agentctl starts for the instance (agent, terminals, readiness probe) in its environment, Git or helper configuration, or CLI shim shall be `http://<loopback listen address>:<instance port>/git-credentials/resolve`, composed by agentctl from its own listener. The create request shall keep the broker URL environment value (the managed-launch signal) and also send it as the upstream broker URL.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.2:** The relay shall forward `GET` (readiness) and `POST` (resolve) at exactly `/git-credentials/resolve` to the upstream broker URL, and `POST` at exactly `/git-credentials/reissue` to the upstream URL with its trailing `/resolve` replaced by `/reissue`. It shall return the upstream status and body unchanged, not following 3xx. Any other method on those paths gets 405; any other path under `/git-credentials/` gets 404. An upstream URL that does not end in `/resolve` is rejected at create with HTTP 400.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.3:** The caller shall not influence the upstream URL, path, or query. The relay forwards only `Content-Type` and `Accept` (the lease is in the body), returns only the status, body and `Content-Type`, and adds no backend credential.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.4:** The relay shall evaluate a request in this order and stop at the first failure: path (404), method (405), upstream configured (404, AC-002.6), request body over 64 KiB (413), body read failure (400); nothing is forwarded for these. It shall then forward, and answer 504 when the upstream has not completed within 15 seconds in total, and 502 when the upstream is unreachable, its response exceeds 1 MiB, or reading it fails (nothing partial is returned). Error bodies are generic JSON that omit the upstream URL.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.5:** The relay shall forward each request once: no retry, replay or caching, and cancel upstream when the caller disconnects.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.6:** An instance created without an upstream broker URL answers an allowed method on the relay paths with 404 (AC-002.4 order) and leaves process-visible broker state untouched.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.7:** Lease semantics, validation, and backend broker routes are unchanged.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.8:** For a remote executor, the broker URL remains the operator-configured URL, still validated by `validateGitHubCredentialBrokerURL` and still HTTPS. No relay is used and no upstream field is sent.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.9:** The launch-time broker readiness probe shall run against the URL the agent will use. For a same-host executor its limits shall exceed the relay's 15 seconds (design) so a relay 504 is observable; remote executors keep today's limits. An unreachable relay or upstream fails the launch as an unreachable broker does today.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.10:** For a same-host executor, no broker or Git credential environment variable, Git configuration entry, helper configuration, or CLI shim given to an agent or tool process shall contain the backend host or port. An operator public base URL is the relay's upstream, not visible to the agent.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.11:** The relay neither requires nor forwards the session MCP token and is served without the control credential; authorization is the lease in the body, validated upstream.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.12:** Each instance shall advertise on `/identity` a one-way fingerprint of its upstream broker URL (empty when none). The backend shall not adopt a retained instance whose fingerprint differs from that of the upstream URL it would send in a create request now (empty when none, and always empty for a remote executor); that instance is handled as in AC-001.11.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003: Executor configuration authorization

**Intent:** Changing an executor or executor profile, which changes what runs
on the next launch, requires the same authorization as changing agent settings,
without taking away what the per-user ownership capability already grants owners.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.1:** Every configuration route shall evaluate, in order, (a) authorization to mutate the target, (b) the interim settings interlock, then the handler's own body validation. Authorization follows the design's matrix, applied to the target and, for a profile route, also its parent executor when that has an owner (AC-EXECUTORS-OWNERSHIP-001.2): a missing target is 404; a resource owned by someone else is 404 for a non-manager; a shared resource, and a create naming no owner or another owner, is 403 `forbidden` for a non-manager; owners acting on their own resources and configuration managers are authorized. An owner may set the owner to themselves or clear it (AC-EXECUTORS-OWNERSHIP-002.1). A body that cannot be decoded skips the owner check and falls through to the interlock and the handler's own 400. A reassignment racing a mutation is resolved in the service layer: the owner that authorization observed is compared with the stored owner at write time inside the write; on a difference the mutation answers 404 and changes nothing, including when the resource became shared meanwhile.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.2:** A browser caller who is authorized and has no valid interlock token shall receive 403 `interim_settings_interlock_required`; a caller refused by (a) receives the matrix response. Neither refusal changes stored data or publishes an event.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.3:** A caller that passes (a) and presents any `Authorization: Bearer` credential (Office bearer or `kandev_pat_*`) shall be refused by the interlock on configuration routes with the response it gives such callers on agent-settings routes today (403 `forbidden`). Browser settings sessions are unaffected.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.4:** The WebSocket actions that create, update, or delete an executor or executor profile shall not be reachable: a request for one receives the dispatcher's unknown-action error; list and get actions are unchanged. The web client shall create, update, and delete executors through the existing HTTP functions (`createExecutor`, `updateExecutor`, `deleteExecutor` in `lib/api/domains/executor-api.ts`) so the interlock header travels with them; the `executor.created|updated|deleted` events keep driving the stores.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.5:** A test shall enumerate the registered router and fail when a configuration route lacks either guard, deriving the set from the Definitions' path rules (prefix matching), so `DELETE /api/v1/executor-profiles/:profileId` is in the set and the SSH probe and remote-docker build routes are not.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.6:** With authentication disabled, the synthetic identity satisfies the authorization check and the interlock is still required.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.7:** The settings pages that create, edit, and delete executors and profiles keep working for configuration managers and for owners of their own resources. A web unit test shall assert that executor create, update, and delete use the HTTP functions and no component sends the removed WebSocket actions; a browser request without an interlock token is refused as in AC-003.2.

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004: Default backend bind address

**Intent:** The default bind decision is recorded with its reason, and no
default changes without a gated rollout.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.1:** The default of `server.host` shall remain `0.0.0.0`.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-004.2:** The design shall record why the default is unchanged and the gated loopback follow-up's scope. Build shall file that card (not started, the board card's `EMBARGO: LOCAL ONLY` section copied verbatim at its start) and record its identifier in the task plan.

## Out of scope

- Child-task confinement and the MCP tool surface, including the `mcp.*_executor_profile` tools and the settings-operations executor cases behind them (separate work).
- Git hooks; enabling authentication by default.
- Token rotation while an instance is live, and backend key rotation (a lost key replaces retained instances, AC-001.10/001.11).
- Network policy rendering by a sandbox provider; a loopback default for the backend bind (REQ-004 and its follow-up card); remote-executor broker transport (stays operator HTTPS).
- Executor action routes that do not change stored configuration: `POST /api/v1/ssh/executors/:id/probe-agents`, `probe-shells`, `reachability/probe`, and `POST /api/v1/remote-docker/executors/:id/build` (already admin-only).
- Other environment variables giving the agent the backend URL (Office `KANDEV_API_URL`, plugin runtime API URL); the backend REST API stays reachable until confinement work.
