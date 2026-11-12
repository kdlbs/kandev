---
status: current
system: agents
requirements:
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-001
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-002
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-003
---

# Built-in MCP Caller Authentication System Design

## Purpose and boundaries

The agents system owns the built-in MCP server entries, their delivery to each
provider, and the host-injected server identity used by permission policy. This
design adds an instance-scoped caller credential to that contract. It enforces
the credential on the built-in MCP endpoint.

The design uses these contracts without changing them:

- The agentctl control credential, its rotation, and `instanceAuth`. These are
  owned by [agentctl authentication modes](../../executors/system-design/agentctl-authentication-modes.md)
  and the executor survival designs.
- Port allocation and leases, owned by the executor system.
- [Built-in MCP project delivery](builtin-mcp-project-delivery.md), which keeps
  its record and fingerprint rules. Generated file content now includes one
  more header.
- [Passthrough MCP injection strategies](../../../decisions/0014-passthrough-mcp-injection-strategies.md).
- Session binding of tool calls in `internal/mcp/server`. Identity still comes
  from the server instance. This design only controls who can reach it.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-001` | [Credential lifecycle](#credential-lifecycle), [Endpoint enforcement](#endpoint-enforcement) |
| `REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-002` | [Built-in entry construction](#built-in-entry-construction), [Delivery paths](#delivery-paths), [Policy recognition](#policy-recognition) |
| `REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-003` | [Rollout control](#rollout-control), [Security](#security), [Observability](#observability) |

## Current behavior

- `builtinMCPExemptPaths` in `internal/agentctl/server/api/builtin_mcp.go`
  exempts `/health`, `/sse`, `/message`, `/mcp`, and every
  `/{mcp,sse,message}/packages/<id>` route from `instanceAuth`.
- `internal/mcp/server.Server.RegisterRoutes` and `registerBuiltinRoutes`
  serve those routes with the instance's `sessionID` and `taskID`.
- Every built-in entry is built as `{Name, Type, URL}`. Several policy checks
  require an empty header set.

## Credential lifecycle

`instance.Manager.CreateInstance` generates the credential during startup,
before the instance's server handler is built. It uses 32 bytes from
`crypto/rand`, encoded as unpadded base64url. It stores the value on
`config.InstanceConfig` as `BuiltinMCPCredential`. The value lives only in agentctl
memory. A new instance always gets a new value, including when it reuses a port.
Deleting the instance destroys the value. Agent survival keeps the agentctl
process alive, so the credential survives a backend restart unchanged.

The backend learns the credential through one control-authenticated instance
read, `GET /api/v1/mcp/builtin-credential`
(`types.BuiltinMCPCredentialResponse{credential, enforced}`, `Cache-Control:
no-store`). The route is under `/api/v1`, so `instanceAuth` requires the
control credential, and the MCP credential never satisfies it. The backend
reads it on demand through the execution's own agentctl client
(`agentctl.Client.BuiltinMCPCredential`) each time it builds a delivery
artifact. It does not cache the value, so a replaced or adopted instance
always yields its own credential, for every executor, without threading the
value through create responses or recovery inventories.

Instance create, get, and list responses do not carry the credential. In
tokenless agentctl startup, the instance routes are reachable only through the
executor transport, such as Sprite, which already exposes every control route.

## Endpoint enforcement

A new middleware, `builtinMCPCallerAuth`, runs only on the built-in MCP route
set. That set is the result of `builtinMCPExemptPaths` without `/health`. The
paths stay exempt from `instanceAuth`, so the control credential is not accepted
there. The middleware:

1. Lets the request pass unchanged when the instance's `BuiltinMCPAuthEnforced`
   is false.
2. Reads `Authorization: Bearer <value>` and compares it with
   `BuiltinMCPCredential` using `crypto/subtle`.
3. On a missing or different value, it aborts with 401 and a JSON body
   `{"error": "kandev built-in MCP credential required"}`. It sends no
   `WWW-Authenticate` resource-metadata parameter.

`/message` and `/message/packages/<id>` are gated the same way. Supported MCP
clients send configured headers on both the SSE stream and the message POST. The
mcp-go client applies them to both requests. `instanceAuth` and the MCP
middleware cover disjoint paths. The MCP credential therefore never satisfies a
control route, and the control credential never satisfies an MCP route.

The agentctl port proxy (`/api/v1/port-proxy/:port/*`) already strips the
control `Authorization` through `credentialScopedRequest`. A proxied request to
the instance's own `/mcp` is therefore rejected under enforcement.

## Built-in entry construction

The credential becomes a header on every built-in entry at its single
construction point:

- `config.BuiltinMCPServers(manifest, host, port, credential)` and
  `injectKandevMcpServerAtHost(servers, port, host, credential)` add
  `Headers: {"Authorization": "Bearer <credential>"}`. With an empty credential,
  they produce the current header-less entry. That case only occurs in tests and
  in older backend/agentctl pairs.
- `api.Server.injectKandevMcpServers` and `builtinSessionMCPServers` copy
  `Headers` together with `Name`, `Type`, `URL`, and `BuiltinPackage`.
- `process.Manager.PrepareBuiltinProfile` rebuilds `cfg.McpServers` through the
  same constructor.

The backend uses the same constructor through
`internal/agent/runtime/lifecycle/builtin_mcp_passthrough.go`, which passes the
credential it read from the execution's instance.

## Delivery paths

| Path | Carrier | Change |
| --- | --- | --- |
| ACP `session/new`, `session/load`, `session/resume`, reset | ACP `McpServerHttp`/`McpServerSse` `headers` | None beyond entry construction. `toACPMcpServers` and `mapToHTTPHeaders` already forward `Headers` |
| Native Codex app-server | `mcp_servers.<name>.http_headers` | None beyond entry construction. `codexConfig` already emits `http_headers` |
| Passthrough Claude and OpenCode | Generated JSON file `headers` | Pass the credentialed entries from `builtin_mcp_passthrough.go` |
| Passthrough Codex | `-c mcp_servers.<name>.http_headers=...` argv | Same. The credential becomes visible in argv, as already noted in `passthrough.go` |
| Passthrough Cursor and Cursor ACP | `.cursor/mcp.json` `headers` | `composeEffectiveMCPServers` and `composeLegacyMCPServers` emit `headers` for Kandev entries |
| Pi project file | `.pi/mcp.json` `headers` | Pass credentialed entries into `ProjectMCPStrategy` |
| Remote passthrough and project files | Same generated files, written through the helper | The backend reads the credential from the helper through the instance read, as for local delivery. `PassthroughSnapshot` and `ProjectMCPTarget` are unchanged |
| Mock agent ACP and TUI | mcp-go `WithHeaders` / `WithHTTPHeaders` | Forward headers from ACP session setup and from `--mcp-config` |

`builtinPassthroughEndpoint` keeps rejecting query, userinfo, and fragment URLs.
The credential never appears in a URL.

Cursor approves built-in entries by `canonicalJSONFingerprint`. A new instance
changes the header, so the fingerprint changes and the existing
`cursor-agent mcp enable` path approves it again. Implementation must confirm
that the fingerprint covers headers and that re-approval needs no user action.

## Policy recognition

Managed-tool and permission checks compare an entry with the full expected
entry, including headers:

- `injectedKandevMCPConfigured` (`manager_permission_policy.go`) and
  `canonicalManagedMCPServers`/`adapterMCPServers` (`managed_tool_policy.go`).
  These accept an entry only when its URL matches the current port and its
  headers equal exactly `{"Authorization": "Bearer <own credential>"}`.
- `builtinPermissionAllowed`/`builtinEndpointOnly` (`builtin_mcp_permission.go`)
  compare `Name`, `Type`, `URL`, and `Headers` against `BuiltinMCPServers` output.

A user-defined server with the same URL but different headers stays a
third-party server.

## Rollout control

A runtime release toggle `features.builtinMcpCallerAuth`
(`KANDEV_FEATURES_BUILTIN_MCP_CALLER_AUTH`) follows the runtime feature flag
contract. The rollout is:

- When merged: `prod: "false"`, `dev: "false"`, `e2e: "true"`. E2E is a
  test-only exception, so the enforcement path runs in CI.
- Promotion to `prod: "true"` is a separate change. It needs the compatibility
  evidence in the plan.

The flag is restart-required. Backend startup records its effective value once
(`agentctl.SetBuiltinMCPAuthEnforced`), and `ControlClient.CreateInstance` sends
it as `builtin_mcp_auth_enforced` on every create request, from every
executor.
`instance.Manager` copies that value to `config.InstanceConfig`. Remote
agentctl processes never read the backend environment. When the flag is
disabled, credentials are still issued and delivered. Agents therefore have
the header before enforcement starts.

An older agentctl ignores `builtin_mcp_auth_enforced` and returns no
credential. The backend then builds header-less entries, as today. An older
backend sends no flag. A newer agentctl then issues a credential but does not
enforce it.

## Failure and recovery

- A provider that drops the header gets 401 on every Kandev tool under
  enforcement. Its session then reports a failed MCP server through existing MCP
  attachment evidence. The operator recovers by disabling the flag. New
  instances then run without enforcement.
- A backend that cannot read the credential from an attached instance shall not
  relaunch passthrough or rewrite project files with header-less entries. It
  reports the existing relaunch or preparation failure instead. An instance
  without the read route (404) predates caller authentication and enforces
  nothing, so header-less entries are correct for it. An execution with no
  agentctl client has no instance to enforce a credential.
- A stale process that keeps calling an old port gets 401 after the port moves
  to a new instance, even with the old credential.

## Persistence

There is no schema change. The credential is held only in agentctl memory. The
backend reads it per artifact build and does not store it in `AgentExecution`,
execution metadata, or SQLite. Generated passthrough and project files contain
it until the existing cleanup removes them or the instance ends. After that, the
value grants nothing.

## Security

Threat model: accidental or blind cross-session calls by local processes,
including agents that scan ports, and reuse of a port by a later instance. Any
process of the same OS user can read argv, environment, and workspace files.
This design does not defend against a process that copies a credential on
purpose. Generated files keep their current permissions.

The credential is an MCP-scope secret. It never authorizes control, git, shell,
file, or process routes. Code that logs `types.McpServer` values or MCP launch
arguments shall redact header values. That code includes `zap` fields in
agentctl and backend lifecycle code, and passthrough argv logging. The ACP debug
frame logger records only agent-to-client notifications today. It shall not start
recording outbound session setup frames without redaction.

## Observability

`builtinMCPCallerAuth` logs a warning for each rejection with route,
`session_id`, `task_id`, and reason (`missing` or `invalid`). Logs are
rate-limited to one per route and reason per minute, with a suppressed count.
The instance's MCP attachment evidence is unchanged. The existing failed-server
signal shows a provider that cannot authenticate.

## Verification

- `internal/agentctl/server/api`: route-table tests for 401 without a
  credential, with another instance's credential, and with the control
  credential, and for success with the own credential. Also test `/health`
  exemption, disabled-flag pass-through, and port reuse with two sequential
  instances on one port.
- `internal/agentctl/server/config` and `process`: entry construction and exact
  policy recognition, including rejection of foreign headers.
- `internal/agentctl/server/adapter/transport/acp` and `codexappserver`:
  header forwarding on new, load, and resume.
- `internal/agent/runtime/lifecycle`: every passthrough strategy, Cursor
  composition, and remote snapshot and target use.
- Web E2E: clarification specs pass with the mock agent and enforcement enabled.
- Provider compatibility: an ACP debug probe per supported provider records that
  the built-in server connects with enforcement enabled.

## Related decisions

- [ADR-2026-11-11-instance-scoped-builtin-mcp-credential](../../../decisions/2026-11-11-instance-scoped-builtin-mcp-credential.md)
- [ADR-2026-07-19-reject-mcp-actions-on-raw-websocket](../../../decisions/2026-07-19-reject-mcp-actions-on-raw-websocket.md)
- [ADR-0014 passthrough MCP injection strategies](../../../decisions/0014-passthrough-mcp-injection-strategies.md)
- [ADR-0007 runtime feature flags](../../../decisions/0007-runtime-feature-flags.md)
