---
status: draft
system: executors
created: 2026-10-25
owners:
  - kandev
---

# Session control-plane isolation requirements

## Overview

A running agent session reaches Kandev through two channels this document
covers: the MCP endpoint served by its own agentctl instance and the Git
credential broker. Each channel must be attributable to exactly one session, and
the agent's only required loopback destination for Git must be its own agentctl
port.

This does not confine child tasks or change the MCP tool surface, Git hooks, the
backend REST API, or executor configuration authorization (deferred).

## Definitions

- **Instance**: one agentctl instance. Its **instance ID** is the existing create request ID and `X-Instance-ID`: the execution ID. It is unchanged.
- **Adopted instance**: an instance a launch or recovery pass did not create itself: a retained one found by recovery or resume, or one a Kubernetes ambiguous create found already present.
- **Deferred executors**: Kubernetes and SSH, whose retained-instance reuse is out of scope (see Out of scope).
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

## Requirements

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001: Per-session MCP token

**Intent:** An MCP route accepts only its own instance's token, so reaching another session's port grants no identity.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.1:** When the backend creates an instance, it shall mint a fresh token nonce, derive a token of at least 256 bits from the instance ID, the nonce and a backend-held secret key (HMAC), and supply the token in the create request. It shall persist the nonce under `mcp_token_nonce` in the execution record metadata so it can recompute the token after a restart without storing the token. The key is persisted, is not copied to a sibling session's launch, and is never inherited from a prior record by a launch that creates a new instance. agentctl shall hold the token in memory only.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.2:** While an instance holds a token, each MCP route shall serve a request only when its `Authorization: Bearer` value equals that token under a constant-time comparison. This shall hold whether or not control authentication is enabled. `GET /sse` and `POST /message` are checked independently: a client that presents the token on one and not the other is refused on the other.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.3:** A request with a missing, empty, malformed, wrong, or other-instance token, or only the control credential, shall receive HTTP 401 with a generic JSON error naming no token detail. It shall open no SSE stream, create no MCP session, dispatch no tool call, and not change the instance's in-flight count or last-activity time. Refusals are logged at most once per minute at WARN with route and reason (`missing`, `malformed`, `mismatch`) only.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.4:** An instance that holds no token shall answer every MCP route with HTTP 503 and the code `mcp_token_unconfigured`, dispatching nothing.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.5:** The token shall be installed before the instance listener accepts its first connection. `/health` and non-MCP routes keep their authorization.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.6:** Every Kandev MCP entry that Kandev composes for an agent shall carry the token as an `Authorization: Bearer` header, for every class in the design's carrier table. Codex passthrough instead uses `env_http_headers` naming a reserved `TOKEN` environment variable that agentctl sets only in the environment of the Codex process it starts, never in argv, under the design's conditions. A carrier is available only for an agent the plan's inventory records as verified, by the design's evidence rule, to forward the header, and only for a class in the table. Otherwise, including an agent absent from the inventory and any agent whose Kandev entry is delivered through a workspace file (today Cursor and Pi, deferred) and any custom TUI agent whose strategy writes a Kandev entry (deferred), the launch shall fail with the typed error `mcp_token_carrier_unavailable` instead of emitting an unauthenticated entry. An agent with neither HTTP nor SSE MCP capability keeps today's behavior (entry omitted with the existing warning).
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.7:** The token shall appear in no agent process argument list, log line at any level (including request, injection debug and ACP frame debug logs and the served frame output), diagnostic bundle, persisted row, or API or event payload, except the create request on the authenticated control channel, the authenticated passthrough-preparation request that delivers a generated per-session MCP configuration file, the AC-001.6 placements, and the reserved Codex variable. It shall appear in no other environment variable of any process Kandev starts.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.8:** Kandev shall write no token to a file inside the workspace. A per-session private file that carries the token (the generated Claude and OpenCode MCP configuration) shall be removed when its instance is deleted; a file that cannot be removed is logged at WARN with its path and does not fail the delete, and the token it holds authorizes nothing once its instance is deleted or reaped.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.9:** Within one instance, the token shall stay the same across agent restart, context reset, new ACP session, and backend restart with the instance retained. Any instance a launch creates, including from resume or recovery replacement, shall hold a different token because it has a new nonce, even when it reuses the ID. A create whose instance ID is live shall receive HTTP 409 `instance_exists`, and one still being cleaned up HTTP 409 `instance_cleaning_up`; neither changes the existing instance. The persisted nonce is the one minted for the create; a copied value is overwritten. The backend retries either three times, one second apart, then fails the launch, deleting nothing.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.10:** The backend's secret key shall be created atomically if and only if none exists (two concurrent first uses end with one key), stored install-wide, and absent from every user-facing secret operation. A stored key that cannot be decrypted (for example after the master key was regenerated) or is not 32 bytes shall be replaced by a fresh key under the same atomic rule, at backend startup before any recovery pass and again at launch, logged once at WARN as `mcp_token_key_replaced`. Retained instances mismatch (AC-001.11) and relaunch; a relaunch that fails shows `mcp_token_key_replaced` first in its error text with one fixed English sentence. A read that fails for any other reason (store unavailable) fails the launch with `mcp_token_key_unavailable` and never falls back to an unauthenticated or unrecoverable token. Recovery only reads the key: absent or undecryptable is a mismatch, other failures unknown.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.11:** Each instance shall advertise on `/identity` the capability `session_mcp_token` and a token fingerprint (a one-way hash). The backend shall not use an adopted instance unless the capability is present and the fingerprint equals the one it recomputes from the instance ID, the stored nonce and its key; a missing nonce or key is a mismatch. The check runs before an instance is registered or used on recovery of standalone (Local, Worktree, Docker), Sprites, remote Docker and plugin instances (design). An instance whose identity cannot be read within the design's timeout and attempts is an unknown outcome, neither adopted nor deleted: the backend asks it over the authenticated control channel to revoke its token (idempotent; three attempts, one second apart, else WARN and the next pass repeats the check), after which it answers every MCP route 503 `mcp_token_unconfigured` (AC-001.4). The survival rules then govern the instance unchanged (AC-EXECUTORS-SURVIVAL-003.7 and 002.15; no new stop or retention rule), and a remote record keeps its retry path. A launch that needs an unknown instance fails through the launch-failure path with the 409 code `instance_exists`, leaving the session resumable. A malformed response is a mismatch. A mismatching instance shall be deleted with the fingerprint observed on it as precondition (none when none is observable) and omitted from the recovered set, so the session continues through the existing resume path on a new instance with a fresh nonce; while that delete has failed, the replacement create is refused with 409 and the launch fails through the existing launch-failure path. For a deferred executor, any path that would adopt or reuse a retained instance (recovery, resume, refresh, reconnect, shared-environment adoption) shall not adopt it: the backend revokes its token as above, deletes nothing, and fails the launch with `mcp_token_unsupported` and cause `retained_instance_deferred`; a fresh create there carries the token and passes AC-001.13. The capability also joins the survival capabilities required at control-server adoption, so a retained older control server is replaced and its sessions resume.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.12:** Concurrent requests with the valid token are all served and those without it all refused; a refusal changes no state.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.13:** For a freshly created instance, the backend shall read the instance's `/identity` capability and fingerprint before starting the agent. If the capability is absent or the identity stays unreadable (design), it shall delete the instance with the minted fingerprint as precondition and fail the launch with `mcp_token_unsupported`, with no agent started. If the capability is present and the fingerprint differs (an instance this launch did not mint, found by the Kubernetes ambiguous create), it shall delete that instance with the observed fingerprint and retry the create under the AC-001.9 retry with a fresh nonce.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.14:** The Kandev entry that agentctl's managed-tool policy matches or rebuilds shall be recognized when it carries exactly the one token `Authorization` header, and the rebuilt entry shall keep it. Any other header or token placement stays unrecognized, as today.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-001.15:** A launch that fails with `mcp_token_carrier_unavailable`, `mcp_token_unsupported`, `mcp_token_key_unavailable` or `mcp_token_key_replaced` shall surface through the session's existing launch-failure path with that code at the start of the error text and one fixed English sentence per code (no translated copy), plus one `cause=` token from the closed set (`retained_instance_deferred` for `mcp_token_unsupported`, else one of) `agent_not_verified`, `workspace_file_carrier_deferred`, `custom_tui_deferred`, `codex_wrapper`, `codex_version`, `codex_probe_failed`. It shall leave no token-bearing file or instance once the delete succeeds. Every rollback delete of an instance the launch created carries the minted token's fingerprint as a precondition; a different fingerprint means no delete and counts as success, and a delete that succeeded or answered not found is not repeated. When the create outcome is unknown, the backend shall delete by instance ID with that precondition (three attempts within ten seconds), else log a warning and leave it to the idle reaper; a Kubernetes ambiguous-create adoption shall pass AC-001.13. The public executors documentation shall describe each code, cause and action, and the upgrade and deferral notes listed in the design.

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

### REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003: Default backend bind address

**Intent:** The default bind decision is recorded with its reason, and no
default changes without a gated rollout.

#### Acceptance criteria

- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.1:** The default of `server.host` shall remain `0.0.0.0`.
- **AC-EXECUTORS-SESSION-CONTROL-ISOLATION-003.2:** The design shall record why the default is unchanged and the gated loopback follow-up's scope. The follow-up is tracked outside this change.

## Out of scope

- Child-task confinement and the MCP tool surface, including the `mcp.*_executor_profile` tools.
- Git hooks; enabling authentication by default.
- Token rotation while an instance is live, and planned key rotation.
- Network policy rendering by a sandbox provider; a loopback bind default (REQ-003 follow-up); remote-executor broker transport (stays operator HTTPS).
- Other environment variables giving the agent the backend URL (Office `KANDEV_API_URL`, plugin runtime API URL); the REST API stays reachable until confinement work.
- **Executor configuration authorization** (deferred to a follow-up). Scope plus the settings interlock on mutating routes under `/api/v1/executors` and `/api/v1/executor-profiles/` (`DELETE /api/v1/executor-profiles/:profileId` has no path `:id`), an ownership matrix that keeps owners' per-user grants, removal of the WebSocket executor actions, and the web move to HTTP. A follow-up spec must decide: an update-assignment matrix row, undecodable body against 404, the profile-delete alias, the owner-snapshot comparison inside the write, refusal of Office bearer and `kandev_pat_*` callers, and the settings-operations executor cases. SSH probe and remote-docker build routes stay outside it.
- **Workspace-file MCP carriers** (deferred, same follow-up). Cursor (reconciler and `CursorStrategy`), Pi (`PiStrategy`, `ProjectMCPStrategy`) and custom TUI strategies that write the `kandev` entry into a workspace file. Until it lands they fail with `mcp_token_carrier_unavailable` (`workspace_file_carrier_deferred`). A follow-up spec must decide: the backend claim owner string and reclaim of crash-orphaned claims, the Cursor `created` record against registry ownership, collision with a user-owned `kandev` entry, tracked-file refusal, `info/exclude` handling, and the accepted ADR `2026-10-08-pi-project-mcp-ownership`.
- **Kubernetes and SSH retained-instance adoption** (deferred to a follow-up). Verifier sites for task-owned Kubernetes adoption (`getOrCreateSharedKubernetesInstance`), `connectKubernetesRefresh`, `connectRestartedKubernetesAgentctl`, `reattachKubernetesAgentctl`, SSH resume reuse (`newResumedAgentctlClient`) and disposal of the old remote agentctl process on mismatch (`stopRemoteAgentctl`); the instance ID and nonce kept on resume; the Kubernetes task environment record that clones instance metadata to siblings. Until it lands these executors fail closed (AC-001.11).
- **Custom TUI MCP carriers** (deferred, with the workspace-file carriers above). Custom TUI agents with a strategy that writes a Kandev entry fail with `mcp_token_carrier_unavailable` (`custom_tui_deferred`); strategy `none` emits no entry, as today.
