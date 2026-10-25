---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003
---

# Session control-plane isolation design

## Purpose and boundaries

The backend owns token derivation, nonce persistence and the adoption verifier. agentctl owns
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
| 003.1, 003.2 | Bind decision |

## Token derivation, nonce and key

The instance ID stays the create request `ID`: the execution ID for every
executor type except a Kubernetes or SSH resume that reuses the retained agentctl
instance and its recorded ID (`MetadataKeyKubernetesAgentctlInstanceID`,
`executor_kubernetes_environment.go`; `resumedSSHAgentctlInstanceID`, `executor_ssh.go`). It is also the `X-Instance-ID` header that
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

Carry-forward: `mcp_token_nonce` is added to both `persistentMetadataKeys` and
`sessionScopedMetadataKeys` (`executor_backend.go`), so `FilterPersistentMetadata` keeps it and
no unrelated metadata path drops it. The `prior.Metadata` merge in `persistence.go` skips it
(as it skips the passthrough claims and idle-activity keys): the current execution's value
owns it and a prior instance's nonce is never resurrected. Every writer that builds or
clones `ExecutorRunning.Metadata` carries it onto the new value: standalone create
(`executor_standalone.go`), the Kubernetes recovery clone (`executor_kubernetes_recovery.go`),
idle parking (`manager_idle_parking.go`), and reconnect and remote-status updates
(`manager_remote_status.go`). A table test runs one record through each writer and asserts the
key is unchanged. On a Kubernetes or SSH resume that keeps the retained instance, the previous
record's nonce is set explicitly on the new request metadata and the adoption check below
decides whether the token still matches; on a mismatch the instance is deleted and recreated
with a fresh nonce.

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
   port, a retained remote agentctl from an older build). One verifier runs on every
   path that adopts an instance this launch did not create: the local adoption in
   `manager_lifecycle.go`, `processRecoveredRemoteInstance` (`remote_recovery.go`), the
   Kubernetes `reattachKubernetesAgentctl` and the SSH resume reuse
   (`newResumedAgentctlClient`). Build enumerates any other reattach site (Sprites, remote Docker,
   plugin) and a table test covers one per executor type. The verifier reads the instance's
   `/identity` through the client. Remote and launch-path reads use a 5 second timeout and up to
   three attempts, one second apart. The local adoption runs concurrently (at most 8 instances)
   with a 2 second timeout and up to two attempts 500 ms apart, so the pass fits the 30 second
   recovery deadline (`defaultRecoveryDeadline`); a read that exhausts the deadline is unknown.
   Retries cover transport errors, timeouts and 5xx. Outcomes, exhaustive:
   - A JSON object with a `capabilities` array of strings and string
     `mcp_token_fingerprint` and `broker_upstream_fingerprint` fields: compare. Any other body
     (including 200 without the capability, from an older retained build) is a mismatch with no
     retry.
   - 404 (the instance ID guard after a port was reused): absent, omitted from the recovered
     set, nothing deleted. Every retention-capable build serves `/identity`.
   - 401, 403, any other 4xx, exhausted attempts, a nil client, a cancelled context, or a
     key read or decrypt failure: unknown outcome, never a mismatch and never a delete. A key
     failure logs `mcp_token_key_unavailable` at WARN. Recovery reads the key and never creates
     it: key not found means a lost key and every retained instance mismatches.
   On a mismatch the verifier deletes the instance with the precondition set to the
   `mcp_token_fingerprint` observed on it (none when the body had no usable fingerprint; the
   delete is then unconditional, pinned by `X-Instance-ID`), so a replacement that took
   the ID meanwhile survives, and omits it from the recovered set. A launch-path adoption (resume
   reuse) then falls to the existing recreate path with a fresh nonce; unknown outcome fails the
   launch through the launch-failure path with `instance_exists` at the start of the text, deleting
   nothing, and the session stays resumable. A failed delete is retried three times, one second
   apart; if it still fails the instance stays unregistered with a WARN and the next recovery pass
   repeats the check. Until deleted, a replacement create gets 409 `instance_exists`.
   Unknown outcome on recovery: the remote path reports `RecoveryOutcomeUnknown`, keeps the
   record and retries through the existing remote retry path. The local adoption has no retry
   loop: the instance stays unregistered with a WARN and the next backend start repeats the check.
   For a remote executor the expected upstream fingerprint is always empty (no upstream is sent).
   For Kandev-deployed helpers replacement is self-sufficient: SSH and Sprites compare the uploaded binary's SHA-256 on every non-resumed create and
   upload a stale one (`executor_ssh.go` `CreateInstance`, `uploadAgentctl` in
   `executor_sprites_operations.go`). Operator-pinned images (Kubernetes, remote Docker) need the
   operator to update them; until then every launch there fails with `mcp_token_unsupported`.
   A new agentctl under an older backend answers MCP routes 503 `mcp_token_unconfigured`; the
   documentation says to update the backend first.

A fresh create runs the same comparison before the agent starts (AC-001.13), with the same
5 second timeout and three attempts. For an instance this launch created, a missing capability or an
identity still unreadable after them is unsupported (delete with the minted fingerprint,
`mcp_token_unsupported`; an old build ignores the header, so that delete is unconditional). An adopted
Kubernetes ambiguous-create instance with the capability but a different fingerprint is deleted
with the observed fingerprint and the create retried with a fresh nonce under the AC-001.9 retry.
It is needed because an old agentctl would ignore the unknown field and serve MCP
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
whose evidence shows the header dropped, or whose Kandev entry is written into a workspace file (Cursor, Pi, custom TUI strategies; deferred), launches with `mcp_token_carrier_unavailable`: the
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
| Claude / OpenCode passthrough | `headers` in the generated JSON file at the per-session private `TempConfigPath` (not in the workspace), removed on instance delete | `agent/mcpconfig` strategies |

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
means. agentctl already holds the token from the create request and sets the variable in the Codex process environment itself; nothing new travels on the wire, and the variable is never in `PassthroughArtifacts.Env`, so it never reaches the
`passthrough_mcp_env` metadata or `MetadataSnapshot()`; a test asserts this. The name contains
`TOKEN`, so Codex's default exclusion of such names from tool subprocess environments applies;
that is Codex behavior and is not asserted (AC-001.7).

Cause tokens for `mcp_token_carrier_unavailable` (AC-001.15): `agent_not_verified`,
`workspace_file_carrier_deferred`, `codex_wrapper`, `codex_version`, `codex_probe_failed`.

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
`/git-credentials/reissue` on the instance listener. The whole `/git-credentials/` prefix is exempt from
`instanceAuth` (control credential) and from the MCP guard, so an unregistered subpath reaches the router and answers 404: the helper holds
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
store (check it and restore the install's database backup). Each `mcp_token_carrier_unavailable` cause is named with its action.

## Bind decision

`server.host` stays `0.0.0.0`. Changing it breaks LAN, container and
`kandev service` users with no migration path, and AC-001 plus AC-002 remove the
reason the change was proposed for the session channels. The follow-up is a gated
runtime flag that defaults the bind to loopback for single-user installs, with
public documentation and a migration note for LAN, container and service users.
Build files it as a local Kandev task (`parent_id` self, `start_agent` false, never a public tracker),
with the whole `EMBARGO: LOCAL ONLY` section of the originating task description copied verbatim at
its start, and records its ID in the task plan.

## No kill switch

The capability ships default-on with no runtime flag: a switch that disables token
enforcement is the bypass it removes, the launch-failing paths are limited to the three typed
codes, and the only safe rollback is a revert.

## Work orders

Two independent work orders (token, relay) share one card because they close one
sandbox-contract layer; each ships and tests alone, except that the relay reuses the
create-request extension from the token order. The bind decision (REQ-003) is recorded in the
design and the follow-up card is filed by the token order. Executor route guards and
workspace-file carriers are deferred (requirements Out of scope).
