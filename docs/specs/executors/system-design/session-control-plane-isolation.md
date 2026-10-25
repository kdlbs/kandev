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

The backend owns token derivation, nonce persistence and the executor guards. agentctl owns
enforcement on its instance listener, the broker relay, and the broker URL it composes for the
processes it starts. The agent only carries what Kandev writes into its MCP configuration.

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
`instanceIDGuard` checks and the key recovery uses to rebuild `StandaloneInstanceID`;
changing it would be a recovery refactor this capability does not need. Recovery reuses an
execution ID for a replacement, so the ID alone cannot make tokens distinct: each create
adds a 16-byte random nonce.

`token = base64url(HMAC-SHA256(key, "kandev-mcp-v1:" + instanceID + ":" + hex(nonce)))`, with `base64url` unpadded and `hex` 32 lowercase characters.
The nonce is held in the in-memory launch (agentctl never needs it) and persisted in
`ExecutorRunning.Metadata` (already JSON-persisted and read back by recovery as
`record.Metadata`) under `mcp_token_nonce` by the existing execution-record persistence after
the create succeeds. If that write is lost the recomputed fingerprint cannot match and the
instance is replaced (fail-safe); a legacy row without the key, or with a nonce that is not 32
lowercase hex characters, is treated the same way.
Each create attempt, including each 409 retry, mints a fresh nonce. A `crypto/rand`
failure, or a stored key that is not 32 bytes, yields `mcp_token_key_unavailable`. A failure of the
execution-record write that persists the nonce is the existing launch failure and rolls the instance back (AC-001.15).

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

The key is 32 random bytes in `internal/secrets`, whose store interface gains a
create-if-absent operation (one transaction; the loser of a race rereads the winner's key).
It uses the fixed install-wide ID `kandev-session-mcp-key-v1`, is excluded from every
user-facing list, read, update and delete path (Build chooses the mechanism; a store test
observes it), and a read or decrypt failure yields `mcp_token_key_unavailable`, never a
regeneration, which would silently orphan every retained instance. A restored older key is
detected like a lost one.

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
   (`lifecycle/control_server_adoption.go`, today `{"agent-survival.v1"}`), the first addition
   to the required set (`launchwrapper.Capability` is only advertised and checked per use, so
   it is no precedent). On upgrade every retained local control server lacks it, so each is
   stopped by the existing `capability_incompatible` / `stopIncompatibleControlServer` path and
   its sessions resume on a fresh control server. That install-wide interruption is accepted
   because a retained old server would otherwise keep serving MCP unauthenticated; the public
   executors documentation notes it. No new copy.
2. Per-instance mismatch (lost or restored key, missing nonce, changed backend
   port, a retained remote agentctl from an older build). One verifier runs on
   every `ExecutorInstance` that any executor's recovery returns, at the two
   points that consume them: the local adoption in `manager_lifecycle.go` and
   `processRecoveredRemoteInstance` (`remote_recovery.go`), which cover standalone,
   SSH, Sprites, Kubernetes, remote Docker and plugin executors. It reads the
   instance's `/identity` through the recovered client with a 5 second timeout and
   up to three attempts, one second apart, for transport errors, timeouts and 5xx.
   Outcomes, exhaustive:
   - Body is a JSON object with a `capabilities` array of strings and string
     `mcp_token_fingerprint` and `broker_upstream_fingerprint` fields: compare. A body
     missing any of them, or not that shape, is a mismatch with no retry.
   - 404 (the instance ID guard after a port was reused, or the route is gone): the instance
     is treated as absent, omitted from the recovered set, nothing deleted.
   - 401, 403, any other 4xx, exhausted attempts, a nil client, a cancelled context, or a
     failure to read the key while recomputing: unknown outcome, never a mismatch and never a
     delete (a transient fault must not delete live instances). A key-read failure also logs
     `mcp_token_key_unavailable` at WARN.
   On a mismatch the verifier deletes the instance with the precondition set to the
   `mcp_token_fingerprint` it just read from that instance's `/identity` (recovery minted
   nothing; the observed value pins the delete to the exact instance examined, so a
   replacement that took the ID meanwhile survives) and leaves it out of the recovered
   set; the existing handling of a session with no recovered instance (resume on a new
   instance) then applies. A failed delete is retried three times, one second apart; if it
   still fails the instance stays unregistered with a WARN and the next recovery pass repeats the
   verification. Until it is deleted the replacement
   create gets 409 `instance_exists` and the launch fails through the existing
   launch-failure path.
   Unknown outcome: the remote path reports `RecoveryOutcomeUnknown`, keeps the record and
   retries through the existing remote retry path. The local adoption has no retry loop: the
   instance stays unregistered with a WARN, the next backend start repeats the verification, and a
   resume create before then gets 409 `instance_exists` and fails the launch.
   For a remote executor the expected upstream fingerprint is always empty, because
   no upstream is sent.
   For Kandev-deployed helpers replacement is self-sufficient: SSH and Sprites compare the uploaded binary's SHA-256 on every non-resumed create and
   upload a stale one (`executor_ssh.go` `CreateInstance`, `uploadAgentctl` in
   `executor_sprites_operations.go`), so the replacement instance runs the current build.
   Operator-pinned images (Kubernetes, remote Docker) need the operator to update them;
   until then every launch there fails with `mcp_token_unsupported` (documented, AC-001.15).

A fresh create runs the same comparison before the agent starts (AC-001.13), with the same
5 second timeout and three attempts; an identity still unreadable after them is treated as
unsupported (delete with the minted fingerprint, `mcp_token_unsupported`), because the launch
owns the instance and a retry is the launch's own. It is needed because an old agentctl would ignore the unknown field and serve MCP
unauthenticated; the agent has not started yet, so nothing used it.

## Instance MCP guard

The guard is a handler wrapper built where the instance server is started
(`startInstance`), placed outside both `activityMiddleware(inst)`
(`manager.go` wraps the whole router in it) and the router with its
`httpmw.RequestLogger`. For `/mcp`, `/sse` and `/message` it applies `subtle.ConstantTimeCompare` over
equal-length digests of the presented and expected tokens. An empty expected token
yields 503 `mcp_token_unconfigured`. A path is guarded when it equals `/mcp`, `/sse` or
`/message`, or begins with one of them followed by `/`; the path is not normalized, so a
variant (trailing dot segments, case change) is not guarded and falls to the router, which
serves no MCP handler for it. Header grammar: no `Authorization` header or an empty value is
reason `missing`; more than one `Authorization` header, a scheme other than `Bearer`
(case-insensitive, exactly one space) or an empty token after it is `malformed`; a token
that differs is `mismatch`. The once-per-minute refusal log uses an atomic compare-and-swap
on a last-logged timestamp per instance, so concurrent refusals log at most once. The token is part of the instance config
passed to the server factory, so it exists before the listener binds.

`activityMiddleware(inst)` (`agentctl/server/instance/idle_reaper.go`) increments the in-flight
count and calls `MarkActivity` before the wrapped handler runs, so a downstream marker cannot
undo it. The guard is therefore installed outside it for those paths: only a request the guard
accepts reaches `activityMiddleware` and the router, so a refused request moves neither counter
(AC-001.3) and never reaches `RequestLogger`. The guard logs the refusal itself (WARN, at most
once per minute per instance, route and reason only), and the router's request logs redact the
`Authorization` header and omit query strings for these routes.

## Token carriers

Inventory by transport class. Each class below is the carrier Build must wire. "Available" is
decided per agent: before removing the exemption Build lists every supported agent in the plan
with its class and evidence. Evidence is a recorded request (the agent, or its adapter against a
stub MCP HTTP server, is driven and the test observes the `Authorization` header arrive on the
`/message` or `/mcp` request) or, where the agent cannot be driven in a test, the emitted
`session/new` frame or file plus a cited vendor document or source line stating it forwards
configured MCP headers; a frame or file alone does not qualify. An agent not in the list, or
whose evidence shows the header dropped, launches with `mcp_token_carrier_unavailable`: the
fail-closed default, so an unverified agent never gets an unauthenticated Kandev entry. An agent
whose adapter rewrites ACP MCP entries into its own configuration (Muse writes a temporary
overlay, `muse_acp.go`) is verified through that overlay, and Build adds it as a row under the
class its overlay uses; it may not add a class. An agent that passes but drops the header at run
time shows as the guard's rate-limited `missing` refusal log and as Kandev MCP tool calls failing
with the agent's own tool error; no new copy, and the public documentation's troubleshooting
names the log line.

| Transport class | Carrier | Injection site |
| --- | --- | --- |
| ACP HTTP and SSE entries | `McpServer.Headers` `Authorization: Bearer` | `config.injectKandevMcpServerAtHost`, `api.injectKandevMcpServers` |
| Codex app-server (native) | `http_headers` in the JSON-RPC config | `codexappserver/adapter.go` |
| Codex passthrough | `-c mcp_servers.kandev.env_http_headers={"Authorization": "KANDEV_MCP_SESSION_TOKEN"}` plus the variable set in that Codex process only, with the `Bearer ` prefix in its value | `agent/mcpconfig/passthrough.go` `CodexStrategy` |
| Claude / OpenCode passthrough | `headers` in the generated JSON file | `agent/mcpconfig` strategies |
| Cursor (two writers) | `headers` on the `kandev` entry of `<workspace>/.cursor/mcp.json` | `lifecycle/cursor_plugin_mcp_reconcile.go` `composeEffectiveMCPServers`, and `CursorStrategy` in `agent/mcpconfig` |
| Pi (ACP and passthrough) | `headers` on the `kandev` entry of `<workspace>/.pi/mcp.json`; typed failure when the Pi MCP adapter does not forward headers | the `ProjectMCPStrategy` field and `MCPStrategy` of the Pi agent (`pi_acp.go`), both set to `mcpconfig.PiStrategy`, for both the ACP and passthrough paths |
| Custom TUI strategies | the strategy's file format when it supports headers, otherwise typed failure | `agent/mcpconfig` |

Codex passthrough cannot use a file (Codex has no MCP-config-file injection) and a `-c` header
would expose the token in argv, so the token goes through an environment variable. Passthrough
runs the unpinned `npx -y @openai/codex`, so detection is by version. The carrier is available
only when the effective agent command starts with `npx` or `codex` (by basename) and no operator
launch wrapper or wrapper shell applies; otherwise `mcp_token_carrier_unavailable`. The probe
runs inside agentctl on the host that runs the agent, with the instance environment: the
command's own resolver form plus `--version` (default `npx -y @openai/codex --version`), 20
second timeout. A timeout (including a cold `npx` download), non-zero exit, unparseable version
or version below the minimum fails with `mcp_token_carrier_unavailable`; failures are not
cached, a success is cached per resolved command and host for ten minutes, and a package update
between probe and start is a residual the guard's refusal log would surface. The probe runs after
the `/identity` check and before files are written; its failure rolls the instance back (Launch
ordering). The minimum is the lowest Codex release whose documented configuration accepts
`env_http_headers` under `mcp_servers`; Build records the constant and its source in the plan.
`KANDEV_MCP_SESSION_TOKEN` is set in the environment of the command Kandev starts, after the
profile environment is merged (a profile value of that name is overwritten); the `npx` to
`codex` exec chain inherits it, which is what "only in the Codex launch command's environment"
means. It travels as an unexported field of the process start configuration on the
authenticated control channel, never in `PassthroughArtifacts.Env`, so it never reaches the
`passthrough_mcp_env` metadata or `MetadataSnapshot()`; a test asserts this. The name contains
`TOKEN`, so Codex's default exclusion of such names from tool subprocess environments applies;
that is Codex behavior and is not asserted (AC-001.7).

File carriers (`.cursor/mcp.json`, `.pi/mcp.json`, custom TUI files) share one
handling, built on the existing `internal/common/passthroughconfig` package
(`Prepare`/`Cleanup` with per-file owner claims in the workspace registry
`.kandev/passthrough-claims.json`, `Acquire`, `CreateNoFollow`, `MergeJSONUnderKey`) and
used by every file writer, in whichever process writes: agentctl for passthrough
preparation and cleanup (`server/process/passthrough_config.go`), and the backend for the
Cursor reconciler (`cursor_plugin_mcp_reconcile.go`) and `lifecycle/manager_passthrough.go`.
Build moves any writer that does not yet use `Prepare`/`Cleanup` onto them.

- One lock key. Every read-modify-write or removal of a carrier file, from any process, is
  a `Prepare` or `Cleanup` call and so holds `Acquire` on the workspace registry path
  (`<workspace>/.kandev/passthrough-claims.json`). No writer locks the config file path
  itself; a writer that does is moved. A per-path in-process mutex sits in front. All writes
  go through a temporary file and rename. A removal therefore never discards a concurrent
  replacement write, whichever process issued either.
- Per-entry removal is a new `Cleanup` contract. Today `Cleanup` deletes a whole file when Kandev
  created it and its fingerprint still matches, and does nothing for a user-created file. Build
  extends it: for a claim released by an instance, under the same registry lock, it removes the
  `kandev` entry whose `Authorization` value matches that instance's token (compared through the
  fingerprint), leaves every other entry, and deletes the file only when Kandev created it and no
  entry remains. A replacement instance's entry and user servers survive.
- One live instance per carrier file. The file holds one `kandev` key. `Prepare` already
  refuses a second owner whose proposed content differs from the file ("MCP configuration is
  in use by another session"). That refusal is a launch failure with
  `mcp_token_carrier_unavailable` and the diagnostic naming the file, for the Cursor and Pi
  (and custom TUI) file carriers; concurrent sessions sharing one workspace directory
  therefore cannot both use a file-carrier agent. The same owner re-preparing (agent
  restart, same token) is idempotent and succeeds. A session that stops removes its claim,
  freeing the file. This is the stated cost of keeping the token out of argv; the public
  documentation names it.
- "Kandev created this file" is durable. Writers that go through `Prepare` use its registry
  claims. The Cursor reconciler's ownership record (`cursorImportOwnershipRecord`, today
  only `Version` and `Imported`) gains a `created` boolean; a legacy, missing, or
  unparseable record reads as false, so such a file is never deleted. The reconciler creates
  the file with `CreateNoFollow`, which reports whether it created it, and writes the record
  with `created` set to that result, under the registry lock, afterwards. A crash between
  the two leaves a created file with no record, which reads as not created: the file stays,
  holding a token that authorizes nothing once its instance is deleted or reaped
  (AC-001.8). No new execution-record metadata key is used.
- A symbolic link at the target, or a file that is not valid JSON, is never
  followed or rewritten: at launch the launch fails with
  `mcp_token_carrier_unavailable`; at removal time the file is left untouched with a
  warning, the instance delete still succeeds, and the token left in it authorizes
  nothing once the instance is gone (AC-001.8).
- A target the repository tracks (`git ls-files --error-unmatch`, run in the git repository
  that contains it) fails the launch with `mcp_token_carrier_unavailable`: an exclude
  rule has no effect on a tracked file, so the token would reach `git diff` and commits.
- For an untracked target Kandev adds the path to the repository's local exclude file
  (`info/exclude`, never a tracked `.gitignore`) of the git repository that contains the
  file, so the token is not committed. In a linked worktree that file lives in the common git
  directory, so the entry also hides a same-named untracked file in the main checkout; this
  is accepted and documented. The entry is removed by the same `Cleanup`, under the same
  lock, when the last claim on the file is released. A workspace that is not inside a git
  repository gets no exclude entry; a multi-repository root gets an entry in each repository
  that contains the file.

Managed policy: `canonicalManagedMCPServers` and `injectedKandevMCPConfigured`
(`agentctl/server/process/managed_tool_policy.go`, `manager_permission_policy.go`)
today skip any Kandev entry with a header and rebuild entries without headers.
They change to accept exactly one `Authorization` header and `adapterMCPServers`
keeps it in the rebuilt entry (AC-001.14); name and URL still decide identity.

Redaction sites: request logs (above), injection debug logs, and the ACP frame debug log
(`adapter/transport/shared/acplog.go`), which records `session/new` frames including
`mcpServers[].headers` and is served at `/api/v1/debug/acp/{session}`; `Authorization` values in
recorded frames are replaced before they are written, so neither the file nor the served output
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
the upstream URL. At create, agentctl parses the upstream with `url.Parse` and rejects with 400 an
upstream that is not absolute `http` or `https`, carries userinfo, a query or a fragment, has leading or
trailing whitespace, or whose parsed path does not end in `/resolve`.

The managed-launch signal stays where it is. `hasManagedGitCredentialBrokerEnv`
(`remote_github_env.go`), `applyGitCredentialSnapshot` (`executor_credentials.go`), the
`managed :=` filters (`instance_git_env.go`, `manager.go`, `profile_env.go`) and
`activateManagedGitToolsForCurrentAuthorization` (`manager.go`) all key on the broker URL
environment value plus the lease, so the backend keeps placing the backend broker URL in the
create request environment for a same-host executor and also sends it as
`credential_broker_upstream_url`. agentctl has one effective broker URL per instance: the relay
URL `http://<loopback>:<port>/git-credentials/resolve` (composed after binding, when the
upstream is present), else the environment value. Every place that turns the broker URL into
process-visible state (process environment, helper and Git configuration overlays, readiness
probe command, CLI shim) reads the effective URL; the backend value stays only in agentctl's
stored instance environment, reachable only on the authenticated control channel. A remote
executor is unchanged: the operator URL stays in the request environment, no upstream field is
sent, and `validateGitHubCredentialBrokerURL` still applies.

Relay behavior is exactly the order in AC-002.4. The `http.Client` has a 15 second total timeout
that starts when forwarding starts, no redirect following (a 3xx is returned unchanged), a
transport with keep-alives disabled and retries off, and the caller's request context (a
disconnect cancels upstream). The request body is read through a 64 KiB `MaxBytesReader` before
forwarding (an over-limit error is checked before any other read error, so it wins with 413),
the response through a 1 MiB limit buffered before writing; only `Content-Type` and `Accept` are
copied upstream and only `Content-Type` back. For a same-host executor the readiness probe's
curl limit is 20 seconds and `waitAgentctlProcess` (`lifecycle/broker_reachability.go`) waits 25
seconds, both above the relay's 15; remote executors keep 15.

## Executor route guards

Authorization matrix, applied to the target. For a profile route the target is the profile and the
parent executor is the one the profile row stores (`profile.ExecutorID`), not the path `:id`;
if the path `:id` differs from it, or the profile is missing, the answer is 404. The profile is evaluated
first, then the parent. The parent is evaluated only against the rows "Owned by someone else" (404 for a
non-manager) and "Owned by the caller"; a shared parent executor passes for every caller, so an owner of
a profile under a shared executor is authorized (AC-EXECUTORS-OWNERSHIP-001.2 requires both checks only
when both have owners). Profile create takes its parent from the path `:id`, with the same treatment:

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


A route middleware in the executor handler packages implements the matrix in AC-003.1. It
resolves the target from the path (missing target: the handler's own 404), decides authorization
from the matrix, then lets `httpmw.InterimSettingsInterlock` run, the same order as
`agent/settings`. `authorizeExecutorOwners` (`task/service/executor_ownership.go`) already
answers not found for foreign-owned resources but skips shared ones, so the middleware adds the
shared-resource manager requirement and the create and assignment rows. The service layer is the
authority when a reassignment races a mutation: the middleware hands the owner it authorized
against to the service call (request context), and the service compares it with the stored owner
inside the write transaction (for a profile, the profile's and the parent's, each when present).
A difference, including an owner who made the resource shared in between (which
`authorizeExecutorOwners` alone would pass), answers the existing not-found response and changes
nothing. The comparison is an optimistic-concurrency assertion, not an authorization check: a
request context carrying no snapshot skips it, so callers outside the REST routes behave as
today. One is the settings operations dispatcher (`backendapp/settings_domain_operations.go`,
`executor` and `executor_profile` cases), which reaches `UpdateExecutor` and
`UpdateExecutorProfile` without the route guard or the interlock; it belongs to the MCP tool
surface and is named in Out of scope. The interlock refuses every `Authorization: Bearer`
credential with `forbidden` (`common/httpmw/interim_settings_interlock.go`), so PAT and CLI
callers are refused, matching agent-settings routes (the interim intent).

The `ws.ActionExecutor*Create|Update|Delete` and `ActionExecutorProfile*Create|Update|Delete`
registrations are removed. The web sends three of them through the WebSocket client:
`app/settings/executor/new/page.tsx` (`executor.create`) and `app/settings/executor/[id]/page.tsx`
(`executor.update`, `executor.delete`); profile pages already use HTTP. Work order 03 moves those
call sites to `createExecutor`, `updateExecutor` and `deleteExecutor` in
`lib/api/domains/executor-api.ts`; `lib/api/client.ts` sends the interlock header on mutations
when a token exists. The `*.created|updated|deleted` events keep updating the stores. The enumeration test (AC-003.5) matches
registered routes by the path rules in the requirements Definitions, not by
substring, so the SSH probe and remote-docker build routes are not in its set.

## Launch ordering, rollback and idempotency

Order: key read or created (`mcp_token_key_unavailable` on failure); nonce
minted; create request sent with token, upstream URL and the broker URL
environment value; token installed; listener bound; `/identity` verified
(AC-001.13); Codex version probe (Codex passthrough only); configuration files written; broker readiness probe; agent started;
nonce persisted with the execution record. Any failure after a create request that succeeded or has an unknown outcome
deletes the instance and removes the files written for it (AC-001.15, AC-002.9). A
create that was refused with 409 deletes nothing, because it did not make the
instance. Each rollback delete carries the fingerprint of the token this launch
minted as a precondition, and a recovery delete (Identity mechanism 2) carries the
fingerprint observed on that instance. The precondition is the request header
`X-Expected-Token-Fingerprint` on the instance delete. A delete without the header (ordinary
stop, task cleanup, the idle reaper) is unconditional; a header with an empty value is 400; a
matching value deletes; a non-matching value answers success without deleting, so a delayed rollback cannot remove a replacement that
reuses the ID. An agentctl that ignores the header is an old build that the
capability check already refuses. When the create outcome is unknown, the backend
deletes by the instance ID, which it already knows because the ID is the create
request ID: three attempts within ten seconds (3 second timeout each), then a warning log and the idle
reaper (default one hour) as the backstop. A create that completes after all three deletes saw
no instance leaves a listener that only its own token opens, that no agent was ever given; the
reaper collects it. That residual is accepted rather than coordinated with a tombstone. The Kubernetes ambiguous-create
reconciliation (`createOrReconcileKubernetesAgentctlInstance`) keeps adopting the
instance it finds, and the adopted instance goes through the AC-001.13 check, so a
fingerprint mismatch deletes it with the same precondition.
Concurrent guard checks read immutable state and need no lock. Delete of an
instance is idempotent.

User-visible outcome of a typed launch failure: the code and one fixed English diagnostic
sentence travel in the session's existing launch-failure error (no new translated copy). The
public executors documentation names the three codes and the action: update the agentctl that
serves the executor (operator-pinned Kubernetes and remote Docker images), or repair the secret
store.

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

The capability ships default-on with no runtime flag: a switch that disables token
enforcement is the bypass it removes, the launch-failing paths are limited to the three typed
codes, and the only safe rollback is a revert.

## Work orders

Three independent work orders (token, relay, executor guards) share one card because they
close one sandbox-contract layer; each ships and tests alone, except that the relay reuses the
create-request extension from the token order. Work order 03 includes the web transport change
(AC-003.4).
