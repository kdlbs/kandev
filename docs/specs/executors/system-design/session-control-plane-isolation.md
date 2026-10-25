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

The backend owns token derivation, nonce persistence and the executor guards.
agentctl owns enforcement on its instance listener, the broker relay, and the
broker URL it composes for the processes it starts. The agent only carries what
Kandev writes into its MCP configuration.

Related decisions: the broker lease contract is unchanged
([provider-neutral Git credential broker](../../../decisions/2026-07-31-provider-neutral-git-credential-broker.md));
ownership semantics are unchanged
([per-user ownership](../requirements/per-user-ownership.md)).

## Requirement mapping

| Acceptance criteria | Mechanism |
| --- | --- |
| 001.1, 001.9, 001.10 | Token derivation, nonce and key |
| 001.2, 001.3, 001.4, 001.5, 001.12 | Instance MCP guard |
| 001.6, 001.7, 001.8, 001.14 | Token carriers and redaction sites |
| 001.11, 001.13, 002.12 | Identity advertisement, adoption and replacement |
| 001.15 | Launch failure, rollback and visible outcome |
| 002.1 to 002.11 | Broker relay |
| 003.1 to 003.7 | Executor route guards and web transport |
| 004.1, 004.2 | Bind decision |

## Token derivation, nonce and key

The instance ID stays the execution ID: it is already the create request `ID`,
the `X-Instance-ID` header that `instanceIDGuard` checks, and the key recovery
uses to rebuild `StandaloneInstanceID` from `AgentExecutionID` for every executor
type. Changing it would be a persistence and recovery refactor that this
capability does not need. Because recovery reuses an execution ID for a
replacement instance, the ID alone cannot make tokens distinct, so each create
adds a 16-byte random nonce.

`token = base64url(HMAC-SHA256(key, "kandev-mcp-v1:" + instanceID + ":" + hex(nonce)))`.
The nonce is held in the in-memory launch, sent only through the token (agentctl
never needs the nonce), and persisted in the execution record `Metadata` map
(`ExecutorRunning.Metadata`, already JSON-persisted and already read back by
recovery as `record.Metadata`) under `mcp_token_nonce` by the existing
execution-record persistence after the create succeeds. If that write is lost the
recomputed fingerprint cannot match, the instance is replaced (fail-safe), and a
legacy row without the key is treated the same way.

The key is 32 random bytes in `internal/secrets`. That package's store interface
gains a create-if-absent operation (store-level, one transaction; the loser of a
race rereads the winner's key). The key uses the fixed install-wide ID
`kandev-session-mcp-key-v1`, is excluded from every user-facing list, read,
update and delete path (Build chooses the exclusion mechanism; a store test
observes it), and a read or decrypt failure yields `mcp_token_key_unavailable`,
never a regeneration, because regenerating would orphan every retained instance
silently. A database restore that brings back an older key is detected the same
way as a lost key.

The create request carries the token on the authenticated control channel (the
sole API-payload exception in AC-001.7). agentctl keeps it as an unexported
instance-config field excluded from JSON, `String()` and log fields. A duplicate
live instance ID already fails in `Manager.Create` ("already exists"); the HTTP
layer maps that to 409 `instance_exists` without touching the live instance.

## Identity advertisement, adoption and replacement

The instance server's `/identity` (the router in `agentctl/server/api/identity.go`,
not the control server's) gains, in addition to the existing capability list:
`session_mcp_token` in `capabilities`, `mcp_token_fingerprint =
base64url(SHA-256("kandev-mcp-fp-v1:" + token))`, and
`broker_upstream_fingerprint` (the same construction over the upstream URL, empty
when none).

There is no per-instance replacement path today; the only incompatibility path is
control-server level (`RequiredSurvivalCapabilities`, `capability_incompatible`,
`stopIncompatibleControlServer`). Two mechanisms cover the two cases:

1. Old agentctl build. `session_mcp_token` is added to `SurvivalCapabilities`
   (`agentctl/server/api/identity.go`) and to `RequiredSurvivalCapabilities`, the
   same way `launchwrapper.Capability` was added. On upgrade, a retained control
   server that lacks it is stopped by the existing path and its sessions resume
   on a fresh control server through the existing resume flow. The user sees the
   same reconnect behavior as any earlier capability bump and no new copy.
2. Per-instance mismatch (lost or restored key, missing nonce, changed backend
   port). In `StandaloneExecutor` recovery, where retained instances are listed
   and turned into recovered `ExecutorInstance` values, the backend reads each
   instance's `/identity`, recomputes both fingerprints from the instance ID, the
   persisted nonce and its key, and on any difference deletes that instance and
   leaves it out of the recovered set. The existing handling of a session with no
   recovered instance (resume on a new instance) then applies unchanged.

A fresh create runs the same comparison before the agent starts (AC-001.13)
because an old agentctl would ignore the unknown field and serve MCP
unauthenticated; the agent has not started yet, so nothing used it.

## Instance MCP guard

A middleware on the instance router runs ahead of the current exemption list.
For `/mcp`, `/sse` and `/message` it applies `subtle.ConstantTimeCompare` over
equal-length digests of the presented and expected tokens. An empty expected token
yields 503 `mcp_token_unconfigured`. The token is part of the instance config
passed to the server factory, so it exists before the listener binds.

`activityMiddleware(inst)` (`agentctl/server/instance/idle_reaper.go`) increments
the in-flight count and calls `MarkActivity` before the wrapped handler runs, so a
downstream marker cannot undo it. The MCP guard is therefore installed outside
`activityMiddleware` for those three paths: the instance handler chain routes a
request for an MCP path through the guard first and only a request the guard
accepts reaches `activityMiddleware`. A refused request moves neither counter
(AC-001.3). Request logging redacts the `Authorization` header and never logs
query strings for these routes.

## Token carriers

Inventory by transport class. Each class below is the carrier Build must wire; a
class whose agent build rejects it fails the launch with
`mcp_token_carrier_unavailable`. Build lists every supported agent against this
table in the plan before removing the exemption and may not add a class.

| Transport class | Carrier | Injection site |
| --- | --- | --- |
| ACP HTTP and SSE entries | `McpServer.Headers` `Authorization: Bearer` | `config.injectKandevMcpServerAtHost`, `api.injectKandevMcpServers` |
| Codex app-server (native) | `http_headers` in the JSON-RPC config | `codexappserver/adapter.go` |
| Codex passthrough | `-c mcp_servers.kandev.env_http_headers={"Authorization": "KANDEV_MCP_SESSION_TOKEN"}` plus the variable set in that Codex process only, with the `Bearer ` prefix in its value | `agent/mcpconfig/passthrough.go` `CodexStrategy` |
| Claude / OpenCode passthrough | `headers` in the generated JSON file | `agent/mcpconfig` strategies |
| Cursor (two writers) | `headers` on the `kandev` entry of `<workspace>/.cursor/mcp.json` | `lifecycle/cursor_plugin_mcp_reconcile.go` `composeEffectiveMCPServers`, and `CursorStrategy` in `agent/mcpconfig` |
| Pi (ACP and passthrough) | `headers` on the `kandev` entry of `<workspace>/.pi/mcp.json`; typed failure when the Pi MCP adapter does not forward headers | `ProjectMCPStrategy` (`pi_acp.go`) and the Pi passthrough strategy |
| Custom TUI strategies | the strategy's file format when it supports headers, otherwise typed failure | `agent/mcpconfig` |

Codex passthrough cannot use a file (Codex has no MCP-config-file injection) and
a `-c` header would expose the token in the process argument list, so the token
goes through an environment variable. Passthrough runs `npx -y @openai/codex`,
which is unpinned, so detection is by version, not by build pin: before starting,
the launch resolves the Codex binary the same way the agent command does and runs
`--version`; a version below the minimum, or one that cannot be parsed, fails with
`mcp_token_carrier_unavailable`. The minimum is the lowest Codex release whose
documented configuration accepts `env_http_headers` under `mcp_servers`; Build
records that constant and its source (release note or documentation URL) in the
plan. The carrier is also refused when the Codex process is started through a
wrapper shell or launcher wrapper, because the variable would then be set in the
wrapper. The name contains `TOKEN` so Codex's default exclusion of such names from
tool subprocess environments applies; that exclusion is Codex behavior and is not
asserted (AC-001.7).

File carriers (`.cursor/mcp.json`, `.pi/mcp.json`, custom TUI files) share one
handling, implemented once and used by every file writer:

- A per-path mutex in the backend serializes write, remove and rebuild for one
  file, so a removal never discards a concurrent replacement write.
- Deleting an instance removes only the `kandev` entry whose `Authorization`
  value matches that instance's token (compared through the fingerprint), so a
  replacement instance's entry and user servers survive.
- "Kandev created this file" is durable: the Cursor reconciler's existing
  ownership file records it, and the other writers record the created paths in
  the execution record metadata under `mcp_files_created`. A file is deleted only
  when it is in that record and no entry remains.
- A symbolic link at the target, or a file that is not valid JSON, is never
  followed or rewritten: the launch fails with `mcp_token_carrier_unavailable`.
- Kandev adds the path to the repository's local exclude file (`info/exclude`,
  never a tracked `.gitignore`) of the git repository that contains the file, so
  the token is not committed. A workspace that is not inside a git repository
  gets no exclude entry, because there is nothing to commit; a multi-repository
  root gets an entry in each repository that contains the file.

Managed policy: `canonicalManagedMCPServers` and `injectedKandevMCPConfigured`
(`agentctl/server/process/managed_tool_policy.go`, `manager_permission_policy.go`)
today skip any Kandev entry with a header and rebuild entries without headers.
They change to accept exactly one `Authorization` header and `adapterMCPServers`
keeps it in the rebuilt entry (AC-001.14); name and URL still decide identity.

Redaction sites: request logs (above), injection debug logs, and the ACP frame
debug log (`adapter/transport/shared/acplog.go`), which records `session/new`
frames including `mcpServers[].headers` and is served at
`/api/v1/debug/acp/{session}`; `Authorization` header values in recorded frames
are replaced before they are written, so neither the file nor the served output
contains the token.

## Broker relay

agentctl registers `GET` and `POST` `/git-credentials/resolve` and `POST`
`/git-credentials/reissue` on the instance listener. These paths are exempt from
`instanceAuth` (control credential) and from the MCP guard: the helper holds
neither the control credential nor the MCP token, and the lease in the JSON body
is the capability, validated by the upstream broker. `instanceIDGuard` already
allows a missing `X-Instance-ID`, and `deliveryProtocolGuard` applies only to
agent stream and submission paths. The reissue path is needed because the helper
(`cmd/agentctl/github_credential.go`) derives its reissue URL by replacing the
trailing `/resolve` of the broker URL with `/reissue`; the relay does the same to
the upstream URL, and rejects at create time an upstream that does not end in
`/resolve`.

The managed-launch signal stays where it is. `hasManagedGitCredentialBrokerEnv`
(`remote_github_env.go`), `applyGitCredentialSnapshot` (`executor_credentials.go`),
the `managed :=` filters (`instance_git_env.go`, `manager.go`, `profile_env.go`)
and `activateManagedGitToolsForCurrentAuthorization` (`manager.go`) all key on the
broker URL environment value plus the lease, so the backend keeps placing the
backend broker URL in the create request environment for a same-host executor,
and also sends it as `credential_broker_upstream_url`. agentctl has one effective
broker URL for the instance: the relay URL
`http://<loopback>:<port>/git-credentials/resolve` (composed after binding, when
the upstream is present), else the value from the environment. Every place that
turns the broker URL into process-visible state (process environment, the helper
and Git configuration overlays, readiness probe command, CLI shim) reads the
effective URL. The backend value remains only in agentctl's stored instance
environment, which is reachable only on the authenticated control channel.
The remote executor path is unchanged: the operator URL stays in the request
environment, no upstream field is sent, and `validateGitHubCredentialBrokerURL`
still applies.

Relay behavior is exactly the order in AC-002.4. The `http.Client` has a 15 second
total timeout, no redirect following (a 3xx is returned unchanged), a transport
with keep-alives disabled and retries off (one forward per request), and the
caller's request context (a caller disconnect cancels the upstream request). The
request body is read through a 64 KiB `MaxBytesReader` before forwarding, the
response through a 1 MiB limit buffered before writing, only `Content-Type` and
`Accept` are copied upstream, and only `Content-Type` is copied back. For a
same-host executor the readiness probe's curl limit is 20 seconds and
`waitAgentctlProcess` (`lifecycle/broker_reachability.go`) waits 25 seconds, both
above the relay's 15 seconds; remote executors keep 15.

## Executor route guards

Authorization matrix (applied to the target and, for a profile route, also its parent executor):

| Target | Caller | Result |
| --- | --- | --- |
| Missing | any | 404 (existing not-found response) |
| Owned by someone else | configuration manager | authorized |
| Owned by someone else | any other caller | 404 (existing not-found response; no existence or owner disclosure) |
| Owned by the caller | the caller | authorized |
| Shared (no owner) | configuration manager | authorized |
| Shared (no owner) | any other caller | 403 `forbidden` |
| Create naming the caller as owner | any caller | authorized |
| Create naming no owner or another owner | configuration manager | authorized |
| Create naming no owner or another owner | any other caller | 403 `forbidden` |


A route middleware in the executor handler packages implements the matrix in
AC-003.1. It resolves the target from the path (missing target: the handler's own
404), decides authorization from the matrix (profile routes also check the parent
executor), then lets `httpmw.InterimSettingsInterlock` run, the same order as
`agent/settings`. Foreign owned resources answer not found for non-managers
because `authorizeExecutorOwners` (`task/service/executor_ownership.go`) already
does; that helper skips shared resources, so the middleware adds the
shared-resource manager requirement and the create and assignment rows. The
service layer's own owner check stays as the second line when a reassignment races
a mutation. The interlock refuses every `Authorization: Bearer` credential with
`forbidden` (`common/httpmw/interim_settings_interlock.go`), so PAT and CLI
callers of these routes are refused, which matches agent-settings routes and is
the interim intent.

The `ws.ActionExecutor*Create|Update|Delete` and
`ActionExecutorProfile*Create|Update|Delete` registrations are removed. The web
today sends three of them through the WebSocket client:
`app/settings/executor/new/page.tsx` (`executor.create`) and
`app/settings/executor/[id]/page.tsx` (`executor.update`, `executor.delete`);
profile pages already use HTTP. Work order 03 moves those three call sites to the
existing `createExecutor`, `updateExecutor` and `deleteExecutor` in
`lib/api/domains/executor-api.ts`; `lib/api/client.ts` sends the interlock header
on mutations when a token exists. The `*.created|updated|deleted` events are
unchanged and keep updating the stores. The enumeration test (AC-003.5) matches
registered routes by the path rules in the requirements Definitions, not by
substring, so the SSH probe and remote-docker build routes are not in its set.

## Launch ordering, rollback and idempotency

Order: key read or created (`mcp_token_key_unavailable` on failure); nonce
minted; create request sent with token, upstream URL and the broker URL
environment value; token installed; listener bound; `/identity` verified
(AC-001.13); configuration files written; broker readiness probe; agent started;
nonce persisted with the execution record. Any failure after the create request
deletes the instance and removes the files written for it (AC-001.15, AC-002.9).
When the create outcome is unknown, the backend deletes by the instance ID, which
it already knows because the ID is the execution ID: three attempts within ten
seconds, then a warning log and the idle reaper (default one hour) as the backstop.
Concurrent guard checks read immutable state and need no lock. Delete of an
instance is idempotent.

User-visible outcome of a typed launch failure: the code and one fixed English
diagnostic sentence travel in the existing launch-failure error of the session
(no new translated copy). The public executors documentation names the three codes
and the action: upgrade or restart the agentctl that serves the executor
(operator-pinned images for Kubernetes and remote Docker must be updated), or
repair the secret store.

## Bind decision

`server.host` stays `0.0.0.0`. Changing it breaks LAN, container and
`kandev service` users with no migration path, and AC-001 plus AC-002 remove the
reason the change was proposed for the session channels. The follow-up is a gated
runtime flag that defaults the bind to loopback for single-user installs, with
public documentation and a migration note for LAN, container and service users.
Build files it (not started, with the whole `EMBARGO: LOCAL ONLY` section of the
originating task description copied verbatim at its start) and records its ID in
the task plan.

## No kill switch

The capability ships default-on with no runtime flag. A switch that disables token
enforcement is the bypass this capability removes, and the launch-failing paths are
limited to the three typed codes. The runtime-feature-flags convention targets
risky behavior that can be rolled out gradually; here the only safe rollback is a
revert.

## Work orders

The plan keeps three independent work orders (token, relay, executor guards).
They share one card because they close one sandbox-contract layer, but each ships
and tests alone and has no code dependency on another except that the relay
reuses the create-request extension from the token order. Work order 03 includes
the web transport change in AC-003.4.
