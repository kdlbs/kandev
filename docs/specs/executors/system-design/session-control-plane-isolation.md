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

The instance ID stays the create request `ID`: the execution ID for every
executor type except a Kubernetes resume, which reuses the retained agentctl
instance and its recorded ID (`MetadataKeyKubernetesAgentctlInstanceID`,
`executor_kubernetes_environment.go`). It is also the `X-Instance-ID` header that
`instanceIDGuard` checks, and the key recovery uses to rebuild
`StandaloneInstanceID` from `AgentExecutionID`. Changing it would be a persistence and recovery refactor that this
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

Carry-forward: the nonce must survive every rewrite of the record. Every writer
that builds or clones `ExecutorRunning.Metadata` carries `mcp_token_nonce` onto the
new value: standalone create (`executor_standalone.go`), the Kubernetes recovery
clone (`executor_kubernetes_recovery.go`), idle parking (`manager_idle_parking.go`),
and reconnect and remote-status updates (`manager_remote_status.go`). A table test
runs one record through each writer and asserts the key is unchanged. On a
Kubernetes resume that keeps the retained instance, the previous record's nonce is
carried onto the new record and the adoption check below decides whether the token
still matches; on a mismatch the instance is deleted and recreated under the same
instance ID with a fresh nonce.

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
instance-config field excluded from JSON, `String()` and log fields. `admitCreate` (called from `Manager.CreateInstance`,
`agentctl/server/instance/manager.go`) refuses a duplicate in two ways today:
"already exists" and "is still cleaning up", both surfaced as HTTP 500 by the
control server. The HTTP layer maps them to 409 `instance_exists` and 409
`instance_cleaning_up` without touching the existing instance. The second is hit
when a recovery replacement reuses the execution ID right after a delete; the
backend retries that create three times, one second apart, then fails the launch
through the existing launch-failure path. It never deletes on either 409.

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
   (`agentctl/server/api/identity.go`) and to `RequiredSurvivalCapabilities`
   (`lifecycle/control_server_adoption.go`, today `{"agent-survival.v1"}`). This is
   the first addition to the required set: `launchwrapper.Capability` is only
   advertised and is checked per use, so it is no precedent. On upgrade every
   retained local control server lacks the capability, so each is stopped by the
   existing `capability_incompatible` / `stopIncompatibleControlServer` path and
   its sessions resume on a fresh control server through the existing resume flow.
   That install-wide interruption is accepted because a retained old server would
   otherwise keep serving MCP without authentication. The public executors
   documentation notes it. No new copy.
2. Per-instance mismatch (lost or restored key, missing nonce, changed backend
   port, a retained remote agentctl from an older build). One verifier runs on
   every `ExecutorInstance` that any executor's recovery returns, at the two
   points that consume them: the local adoption in `manager_lifecycle.go` and
   `processRecoveredRemoteInstance` (`remote_recovery.go`), which cover standalone,
   SSH, Sprites, Kubernetes, remote Docker and plugin executors. It reads the
   instance's `/identity` through the recovered client with a 5 second timeout and
   up to three attempts for transport errors, timeouts and 5xx; a malformed or
   incomplete body is a mismatch with no retry. It recomputes both fingerprints
   from the instance ID, the persisted nonce and its key. On any difference it
   deletes that instance (with the fingerprint precondition below) and leaves it
   out of the recovered set; the existing handling of a session with no recovered
   instance (resume on a new instance) then applies. If the identity cannot be
   read after the attempts the instance is neither adopted nor deleted: recovery
   reports `RecoveryOutcomeUnknown`, the record is kept and retried by the existing
   remote retry path. If the delete of a mismatching instance fails, the
   replacement create gets 409 and the launch fails through the existing
   launch-failure path until the instance is deleted or reaped.
   For a remote executor the expected upstream fingerprint is always empty, because
   no upstream is sent.

A fresh create runs the same comparison before the agent starts (AC-001.13)
because an old agentctl would ignore the unknown field and serve MCP
unauthenticated; the agent has not started yet, so nothing used it.

## Instance MCP guard

The guard is a handler wrapper built where the instance server is started
(`startInstance`), placed outside both `activityMiddleware(inst)`
(`manager.go` wraps the whole router in it) and the router with its
`httpmw.RequestLogger`. For `/mcp`, `/sse` and `/message` it applies `subtle.ConstantTimeCompare` over
equal-length digests of the presented and expected tokens. An empty expected token
yields 503 `mcp_token_unconfigured`. The token is part of the instance config
passed to the server factory, so it exists before the listener binds.

`activityMiddleware(inst)` (`agentctl/server/instance/idle_reaper.go`) increments
the in-flight count and calls `MarkActivity` before the wrapped handler runs, so a
downstream marker cannot undo it. The MCP guard is therefore installed outside
`activityMiddleware` for those three paths: the instance handler chain routes a
request for an MCP path through the guard first and only a request the guard
accepts reaches `activityMiddleware`. A refused request moves neither counter
(AC-001.3). Because the guard sits outside the router, a refused request never reaches
`RequestLogger`; the guard itself logs the refusal at WARN at most once per minute
per instance with the route and reason category only (AC-001.3), and the router's
request logs redact the `Authorization` header and omit query strings for these
routes. A request the guard accepts passes on to `activityMiddleware` and the
router unchanged.

## Token carriers

Inventory by transport class. Each class below is the carrier Build must wire.
"Available" is decided per agent, not per class: Build lists every supported agent
against this table in the plan before removing the exemption, with for each agent the
class it uses and the test that shows the adapter's emitted `session/new` frame or
file carries the header. An agent that is not in that list, or whose test shows the
header dropped, launches with `mcp_token_carrier_unavailable`; that is the
deliberate fail-closed default, so an agent the inventory did not verify never gets
an unauthenticated Kandev entry. An agent whose adapter rewrites ACP MCP entries
into its own configuration (Muse writes them into a temporary overlay,
`muse_acp.go`) is verified through that overlay, and Build adds that agent as an
inventory row under the transport class its overlay uses; it may not add a class.
An agent that accepts the entry but drops the header is caught by the inventory
test; at run time it shows as the rate-limited refusal log of the MCP guard.

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
the launch runs the agent command's own resolver form with `--version` appended
(for the default, `npx -y @openai/codex --version`), with a 20 second timeout. A
timeout, a non-zero exit, an unparseable version, or a version below the minimum
fails with `mcp_token_carrier_unavailable`. The result is cached per resolved
command for ten minutes, and a package update between the probe and the start is a
residual that the guard's refusal log would surface. The minimum is the lowest Codex release whose
documented configuration accepts `env_http_headers` under `mcp_servers`; Build
records that constant and its source (release note or documentation URL) in the
plan. The variable is set in the environment of the command Kandev starts and is
inherited by the `npx` to `codex` exec chain, which is the same process tree and the
same user. The carrier is refused when Codex is started through a wrapper shell or an
operator launch wrapper, because the variable would then be set in the wrapper. The name contains `TOKEN` so Codex's default exclusion of such names from
tool subprocess environments applies; that exclusion is Codex behavior and is not
asserted (AC-001.7).

File carriers (`.cursor/mcp.json`, `.pi/mcp.json`, custom TUI files) share one
handling, built on the existing `internal/common/passthroughconfig` package
(`Acquire` cross-process lock, `Prepare`/`Cleanup` with owner claims,
`CreateNoFollow`, `MergeJSONUnderKey`) and used by every file writer, in whichever
process writes (agentctl for passthrough preparation and cleanup,
`server/process/passthrough_config.go`; the backend for the Cursor reconciler):

- Every read-modify-write or removal takes `passthroughconfig.Acquire` on the path,
  which is a cross-process advisory lock outside the workspace, and writes through a
  temporary file and rename. A per-path in-process mutex sits in front of it. A
  removal therefore never discards a concurrent replacement write, whichever
  process issued either.
- Deleting an instance removes only the `kandev` entry whose `Authorization`
  value matches that instance's token (compared through the fingerprint), so a
  replacement instance's entry and user servers survive.
- "Kandev created this file" is durable. Writers that go through
  `passthroughconfig.Prepare` use its registry claims, which already record what
  Kandev created; Build moves any file writer that does not onto it. The Cursor
  reconciler's ownership record (`cursorImportOwnershipRecord`, today only `Version`
  and `Imported`) gains a `created` boolean; a legacy record reads as false, so a
  legacy file is never deleted. The reconciler writes `created: true` to the record
  before it creates the file, creates it with `CreateNoFollow`, and rewrites the
  record to `false` when the file turned out to exist already. A file is deleted
  only when the evidence says Kandev created it and no entry remains. No new
  execution-record metadata key is used.
- A symbolic link at the target, or a file that is not valid JSON, is never
  followed or rewritten: at launch the launch fails with
  `mcp_token_carrier_unavailable`; at removal time the file is left untouched with a
  warning, the instance delete still succeeds, and the token left in it authorizes
  nothing once the instance is gone (AC-001.8).
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
service layer is the authority when a reassignment races a mutation: the
middleware hands the owner it authorized against to the service call (request
context), and the service compares it with the stored owner inside the write
transaction. A difference, including an owner who made the resource shared in
between (which `authorizeExecutorOwners` alone would pass), answers the existing
not-found response and changes nothing. The interlock refuses every `Authorization: Bearer` credential with
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
nonce persisted with the execution record. Any failure after a create request that succeeded or has an unknown outcome
deletes the instance and removes the files written for it (AC-001.15, AC-002.9). A
create that was refused with 409 deletes nothing, because it did not make the
instance. Each rollback delete carries the fingerprint of the token this launch
minted as a precondition (a request header on the instance delete); agentctl
deletes only when the instance's own fingerprint matches, and otherwise answers
success without deleting, so a delayed rollback cannot remove a replacement that
reuses the ID. An agentctl that ignores the header is an old build that the
capability check already refuses. When the create outcome is unknown, the backend
deletes by the instance ID, which it already knows because the ID is the create
request ID: three attempts within ten seconds, then a warning log and the idle
reaper (default one hour) as the backstop. The Kubernetes ambiguous-create
reconciliation (`createOrReconcileKubernetesAgentctlInstance`) keeps adopting the
instance it finds, and the adopted instance goes through the AC-001.13 check, so a
fingerprint mismatch deletes it with the same precondition.
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
