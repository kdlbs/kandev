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

The backend owns instance-ID minting, token derivation and the executor guards.
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
| 001.1, 001.9, 001.10 | Instance ID and token derivation |
| 001.2, 001.3, 001.4, 001.5, 001.12 | Instance MCP guard |
| 001.6, 001.7, 001.8, 001.14 | Token carriers |
| 001.11, 001.13, 002.12 | Identity advertisement and adoption check |
| 001.15 | Launch failure and rollback |
| 002.1 to 002.11 | Broker relay |
| 003.1 to 003.7 | Executor route guards |
| 004.1, 004.2 | Bind decision |

## Instance ID and token derivation

The backend mints a UUID for every create request and sends it as the existing
`CreateRequest.ID`; it persists that ID where it already persists
`StandaloneInstanceID`. It never passes the execution ID: execution IDs are
reused by recovery (`CandidateExecutionID`, `AgentExecutionID` recreate paths),
so deriving from them would reissue a token. A create whose ID is live returns
409 `instance_exists` and changes nothing, so there is no create idempotency to
reason about: a retry after an unknown outcome mints a new ID and the orphan is
reaped by the existing instance cleanup.

`token = base64url(HMAC-SHA256(key, "kandev-mcp-v1:" + instanceID))`. The key is
32 random bytes in the existing secret store (`internal/secrets`). Creation is a
store-level create-if-absent in one transaction; the loser of a race rereads the
winner's key. A stored key that fails to decrypt is an error, never a trigger to
regenerate, because regenerating would orphan every retained instance silently.
A database restore that brings back an older key is detected the same way as a
lost key: fingerprints stop matching and instances are replaced.

The create request carries the token on the authenticated control channel (the
sole API-payload exception in AC-001.7). agentctl keeps it as an unexported
instance-config field excluded from JSON, `String()` and log fields.

## Identity advertisement and adoption

The instance server's `/identity` (the router in `agentctl/server/api/identity.go`,
not the control server's) gains, in addition to the existing capability list:
`session_mcp_token` in `capabilities`, `mcp_token_fingerprint =
base64url(SHA-256("kandev-mcp-fp-v1:" + token))`, and
`broker_upstream_fingerprint` (the same construction over the upstream URL, empty
when none). Retained-instance adoption recomputes both and requires equality, so a
lost key, restored key, old agentctl build or changed backend port all route to
the existing replacement path. A fresh create runs the same comparison before the
agent starts (AC-001.13) because an old agentctl would ignore the unknown field
and serve MCP unauthenticated; the agent has not started yet, so nothing used it.

## Instance MCP guard

A middleware on the instance router runs ahead of the current exemption list.
For `/mcp`, `/sse` and `/message` it applies `subtle.ConstantTimeCompare` over
equal-length digests of the presented and expected tokens. An empty expected token
yields 503 `mcp_token_unconfigured`. The token is part of the instance config
passed to the server factory, so it exists before the listener binds.

`activityMiddleware(inst)` wraps the whole instance handler, so a refused request
would otherwise count as activity. The guard marks a refused MCP request in the
request context and `activityMiddleware` skips `MarkActivity` for marked
requests; refusals therefore cannot keep an instance alive (AC-001.3).
Request logging redacts the `Authorization` header and never logs query strings
for these routes.

## Token carriers

Inventory by transport class. Each class below is the carrier Build must wire; a
class whose pinned agent build rejects it fails the launch with
`mcp_token_carrier_unavailable`. Build lists every supported agent against this
table in the plan before removing the exemption and may not add a class.

| Transport class | Carrier | Injection site |
| --- | --- | --- |
| ACP HTTP and SSE entries | `McpServer.Headers` `Authorization: Bearer` | `config.injectKandevMcpServerAtHost`, `api.injectKandevMcpServers` |
| Codex app-server (native) | `http_headers` in the JSON-RPC config | `codexappserver/adapter.go` |
| Codex passthrough | `-c mcp_servers.kandev.env_http_headers={"Authorization": "KANDEV_MCP_SESSION_TOKEN"}` plus the variable set in that Codex process only, with the `Bearer ` prefix in its value | `agent/mcpconfig/passthrough.go` `CodexStrategy` |
| Claude / OpenCode passthrough | `headers` in the generated JSON file | `agent/mcpconfig` strategies |
| Cursor | `headers` on the `kandev` entry of `<workspace>/.cursor/mcp.json` | `lifecycle/cursor_plugin_mcp_reconcile.go` `composeEffectiveMCPServers` |
| Custom TUI strategies | the strategy's file format when it supports headers, otherwise typed failure | `agent/mcpconfig` |

Codex passthrough cannot use a file (Codex has no MCP-config-file injection) and
a `-c` header would expose the token in the process argument list, so the token
goes through an environment variable instead; the name contains `TOKEN` so
Codex's default exclusion of such names from tool subprocess environments
applies, which Build verifies against the pinned Codex build. If that build lacks
`env_http_headers`, the launch fails with the typed error; argv is never used.

Cursor file handling: `.cursor/mcp.json` also holds user-owned servers that the
reconciler already classifies via its ownership file. Deleting an instance removes
only the `kandev` entry whose `Authorization` value matches that instance's token
(compared through the fingerprint), so a replacement instance's entry and the
user's servers survive; the file is deleted only when Kandev created it and no
entry remains. Kandev adds the path to the repository's local exclude file
(`info/exclude`, never a tracked `.gitignore`) so the token is not committed.

Managed policy: `canonicalManagedMCPServers` and `injectedKandevMCPConfigured`
(`agentctl/server/process/managed_tool_policy.go`, `manager_permission_policy.go`)
today skip any Kandev entry with a header and rebuild entries without headers.
They change to accept exactly one `Authorization` header and `adapterMCPServers`
keeps it in the rebuilt entry (AC-001.14); name and URL still decide identity.

## Broker relay

agentctl registers `GET` and `POST` `/git-credentials/resolve` on the instance
listener. The path is exempt from `instanceAuth` (control credential) and from the
MCP guard: the helper holds neither the control credential nor the MCP token, and
the lease in the JSON body is the capability, validated by the upstream broker.
`instanceIDGuard` already allows a missing `X-Instance-ID`, and
`deliveryProtocolGuard` applies only to agent stream and submission paths.

The upstream URL arrives in the create request (`credential_broker_upstream_url`)
and is fixed for the instance. For a same-host executor the backend omits
`KANDEV_GITHUB_CREDENTIAL_BROKER_URL` from the request environment; agentctl
knows its own port only after binding, so it composes
`http://<loopback>:<port>/git-credentials/resolve` and sets the variable for every
process it starts (agent, terminals, readiness probe), overriding any inherited
value. The remote executor path is unchanged: the operator URL stays in the
request environment and `validateGitHubCredentialBrokerURL` still applies.

Relay behavior is exactly the order in AC-002.4. The `http.Client` has a 15 second
timeout and no redirect following (a 3xx is returned unchanged). The request
body is read through a 64 KiB `MaxBytesReader` before forwarding, the response
through a 1 MiB limit buffered before writing, only `Content-Type` and `Accept`
are copied upstream, and only `Content-Type` is copied back (the helper sends
`Content-Type` only; the lease is in the body). The readiness probe's curl limit
rises above 15 seconds for same-host executors.

## Executor route guards

A route middleware in the executor handler packages implements AC-003.1: pass
when the caller holds `org.config.manage` (or is synthetic), otherwise pass only
when the target's owner is the caller (resolved through the existing
`authorizeExecutorOwners` rules) or the request creates a resource owned by the
caller; refuse everything else with 403 `forbidden`. It runs before
`httpmw.InterimSettingsInterlock`, the same order as `agent/settings`. The
interlock refuses every `Authorization: Bearer` credential, so PAT and CLI callers
of these routes are refused, which matches agent-settings routes and is the
interim intent. The `ws.ActionExecutor*Create|Update|Delete` and
`ActionExecutorProfile*Create|Update|Delete` registrations are removed; the web
only consumes the `*.created|updated|deleted` events and uses HTTP for these
calls (`lib/api/client.ts` sends the interlock header on mutations when a token
exists). The enumeration test (AC-003.5) matches registered routes by the path
prefixes in the definition, not by substring, so the SSH probe and remote-docker
build routes are not in its set.

## Launch ordering, rollback and idempotency

Order: key read or created; instance ID minted; create request sent with token,
upstream URL and no same-host broker URL; token installed; listener bound;
`/identity` verified (AC-001.13); configuration files written; broker readiness
probe; agent started. Any failure after the create request deletes the instance
and removes the files written for it (AC-001.15, AC-002.9). Concurrent guard checks
read immutable state and need no lock. Delete of an instance is idempotent.

## Bind decision

`server.host` stays `0.0.0.0`. Changing it breaks LAN, container and
`kandev service` users with no migration path, and AC-001 plus AC-002 remove the
reason the change was proposed for the session channels. The follow-up is a gated
runtime flag that defaults the bind to loopback for single-user installs, with
public documentation and a migration note for LAN, container and service users.
Build files it (not started, EMBARGO section copied verbatim) and records its ID
in the task plan.

## Work orders

The plan keeps three independent work orders (token, relay, executor guards).
They share one card because they close one sandbox-contract layer, but each ships
and tests alone and has no code dependency on another except that the relay
reuses the create-request extension from the token order.
