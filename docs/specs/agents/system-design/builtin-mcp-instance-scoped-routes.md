---
status: draft
system: agents
created: 2026-11-16
requirements:
  - REQ-AGENTS-MCP-INSTANCE-001
  - REQ-AGENTS-MCP-INSTANCE-002
  - REQ-AGENTS-MCP-INSTANCE-003
  - REQ-AGENTS-MCP-INSTANCE-004
owners:
  - kandev
---

# Instance-scoped built-in MCP routing design

## Ownership and current behavior

Agents owns this endpoint contract. Existing
[package authority](builtin-mcp-management.md),
[project delivery](builtin-mcp-project-delivery.md), and
[runtime recovery](agent-resume-runtime-recovery.md) remain separate contracts.
Task-owner authorization still derives from the execution carrying the stream.

Backend source paths in this document are relative to `apps/backend`.
Names marked **proposed** do not exist in the baseline.

Current grounding at `b4c6a197129b5b4a5e99c3e1dab132ada9fb433f`:

- `cmd/agentctl/main.go` creates every production instance through
  `instance.Manager`. `admitCreate` supplies an ID when the request omits one.
  `NewInstanceConfig` has no other production caller. Empty IDs occur in direct
  configuration/test construction and remain valid only in legacy mode.
- `api.Server.setupRoutes` passes its root router to `mcp.Server.RegisterRoutes`.
  `builtinMCPExemptPaths` exempts bare transports from control authentication.
- `config.MCPServerURL` builds most URLs. `process.PassthroughOwner.Snapshot`
  and `ProjectMCPFilesOwner.Target` also format bare loopback URLs directly.
- Lifecycle's `builtinPassthroughEndpoint` rejects every path except `/mcp`.
  `createRemoteAgentInstance` currently discards the create response except its port.
- `Client.InstanceID()` names the helper pinned by `X-Instance-ID`.
  `ExecutorInstance.InstanceID` can instead name the host execution. Kubernetes
  recreation paths reuse the recorded helper ID for the same execution.
- Control `InstanceInfo` and create responses already carry the helper ID and port.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-MCP-INSTANCE-001 | Shared URL contract, Instance server and transports |
| REQ-AGENTS-MCP-INSTANCE-002 | Delivery and exact-URL policy, Verification boundaries |
| REQ-AGENTS-MCP-INSTANCE-003 | Per-instance negotiation, Compatibility |
| REQ-AGENTS-MCP-INSTANCE-004 | Recovery and identity, Authority and observability |

## Per-instance negotiation

Add **proposed** `ScopedMCPRoutes bool` with JSON name `scoped_mcp_routes` to
both create-request DTOs: runtime `agentctl.CreateInstanceRequest` and
`instance.CreateRequest`. Carry the value through `InstanceOverrides` into
`InstanceConfig`, before native servers are injected. It is immutable for that
helper lifetime and independent of package selection.

Add the same effective field to both create-response DTOs and both `InstanceInfo`
DTOs. The create response owns initial negotiation. `InstanceInfo` is the single
mode source on retained adoption. Report explicit true/false from new helpers.
A missing field from an old peer decodes to false. Invalid field types remain
decode errors. No session-association DTO, capability endpoint, or credential
route changes are needed.

Current backend create builders request true after all consumers migrate.
These include standalone/worktree, container, reconnect, SSH, Sprites, and
plugin remote creation. Kubernetes and Remote Docker use these carriers too.
SSH and Sprites direct HTTP paths must preserve the full response, not only
the port. Create reconciliation and `getOrCreateSharedKubernetesInstance` must
copy the effective field when synthesizing a response from `InstanceInfo`.

The backend `Client` gains a **proposed** `BuiltinMCPScopedRoutes()` reader.
Populate the effective value from the accepted create response or authenticated
`InstanceInfo` before publishing the client or materializing MCP configuration.
Never infer support from the requested bool, helper version, package capability,
listener port, or backend execution ID. An omitted field in a successful old
response means legacy. An unsuccessful read does not mean an omitted field.

## Compatibility

| Backend | Helper and instance state | Routing and delivery |
| --- | --- | --- |
| Current | Current, new create with true echoed | Scoped only, scoped URLs |
| Current | Old, request ignored and response field absent | Bare routes and bare URLs |
| Old | Current, request field absent | Bare routes and helper-generated bare URLs |
| Current | Current, retained instance created without opt-in | Preserve bare mode |
| Current | Current, retained scoped instance | Preserve scoped mode and ID |
| Current | Malformed or unreadable instance information | Stop dependent delivery without fallback |

This compatibility applies to route mode. It does not waive existing
`builtin-mcp-packages-v1` or `project-mcp-delivery-v1` admission requirements.
An old helper without the required package channel can still be unsupported
for reasons independent of routing.

Missing create fields select legacy mode even when an ID is nonempty.
For scoped mode, an empty ID is an error. A retained instance never changes mode
through configure, profile binding, title transition, or backend adoption.
A scoped transport error or 404 never triggers a bare-route retry.

## Shared URL contract

Place the shared implementation in **proposed** `internal/common/mcpurl`, as
required by the backend's cross-tier dependency rule. Reuse the current host
normalization and `net/url` behavior from `config/mcp_url.go`. Preserve empty
host legacy behavior, wildcard-to-loopback mapping, IPv6 brackets, and zone escaping.
No new dependency is necessary.

Define one prefix constant with value `/i/` and one built-in URL entry point:
**proposed** `BuiltinURL(host, port, instanceID, scoped, endpoint)`. Give it
typed Go parameters and an error result. Its companion path function supplies
router registration, auth exemptions, and transport options. All native URL
construction calls this builder, including legacy mode. Thin existing wrappers
can delegate, but cannot retain separate path or host assembly.

Scoped IDs use one nonempty ASCII segment matching `[A-Za-z0-9_-]+`.
Reject slashes, backslashes, dots, whitespace, percent escapes, query markers,
and fragments. Validate caller-provided IDs in `admitCreate` before port
allocation. Validate direct scoped `NewInstanceConfig` construction as well.
Legacy mode preserves its prior ID admission. Generated UUIDs satisfy the rule.

The endpoint argument is a relative transport contract expressed with a leading
slash: `/mcp`, `/sse`, `/message`, or a package variant. Prefix assembly must not
use path cleaning to repair an invalid identity. Errors propagate before native
configuration writes or server publication.

## Instance server and transports

`api.Server` registers native routes on a Gin group for the effective prefix.
Legacy mode uses the root group. `/api/v1/...` control routes remain outside it.
Use exact current-instance paths in `builtinMCPExemptPaths`, plus `/health`.
An arbitrary `/i/` prefix must not exempt a control route.

Install a narrow native-route rejection middleware before `instanceAuth`.
For scoped instances, it rejects bare built-in paths and wrong-ID native paths
with 404. It must cover core and package transports and all HTTP methods.
Legacy instances reject scoped native paths, avoiding a misleading auth error.
Non-native requests retain existing control-auth behavior.

The rejection JSON contains a stable error code and a short stale-endpoint hint.
It does not disclose the expected ID, full endpoint, task, or session. Rejection
happens before transport handling or backend stream dispatch. Requests with an
unrelated `X-Instance-ID` still fail through the existing guard.

Add an instance route-prefix option to `mcp.Server`. The default remains empty
for existing callers. Pass it before transport creation from
`cmd/agentctl/main.go`. Apply it to initial construction and
`BindInitialProfile`, including rebuilt package transports.

For streamable HTTP, `WithEndpointPath` receives the complete effective path.
For SSE, retain the host-only `WithBaseURL`, use `WithStaticBasePath(prefix)`,
and retain relative `/sse` and `/message` endpoint suffixes. Apply this to each
package. Do not put the prefix into both base URL and base path.
The pinned `mcp-go v1.0.0-beta.1` supports this composition. A Gin group alone
does not change the SSE endpoint event.

`NewExternal` and `RegisterBackendRoutes` retain the backend's `/mcp`,
`/mcp/sse`, and `/mcp/message` contract. Package transport sessions, profile
binding, shutdown ownership, and protocol compatibility wrappers remain intact.

## Delivery and exact-URL policy

Agentctl-owned consumers use `InstanceConfig.InstanceID` and its effective bool:

- `injectKandevMcpServerAtHost`, `injectInstanceBuiltinMCP`, and `BuiltinMCPServers`.
- `injectKandevMcpServers` and `builtinSessionMCPServers` for new/load/resume/reset.
- `canonicalManagedMCPServers` and `injectedKandevMCPConfigured` for exact approval.
- `PassthroughOwner.Snapshot` and `ProjectMCPFilesOwner.Target`, including
  their preparation/read-back responses. Preserve their executor-local host.

Lifecycle consumers receive the selected helper client explicitly:

- `agentMCPURL` and `builtinPassthroughMCPServers` supply Claude/OpenCode files,
  Codex arguments, Pi project delivery, and legacy/bound Cursor entries.
- Replace `builtinPassthroughEndpoint`'s bare-only assumption. Parse host and
  port, then compare the received path against the canonical core path for the
  selected client's ID and mode. Reject user info, queries, fragments, malformed
  ports, and mismatched paths. Do not recover identity by trusting URL text.
- `composeEffectiveMCPServers`, `composeLegacyMCPServers`, and
  `cursor_plugin_mcp_reconcile.go` use the same selected endpoint. Keep existing
  user entries, merge ownership, fingerprints, and reserved-name protections.
- `materializeProjectMCPPlacement` uses `placement.client`, including local
  placement during staged replacement. The execution's published old client is
  not an acceptable substitute. Remote targets must match that client's mode
  and ID as well as the existing task/session/workspace checks.
- SSH passthrough keeps its snapshot, client, and preparation generation from
  the same helper lease. Never combine a replacement snapshot with the published
  old client's mode, or prepare through a client acquired after snapshot selection.

Endpoint changes do not add MCP headers. Permission checks retain existing
provenance, exact package/tool membership, host/port checks, and allow-option
selection. Tests must observe actual approval, not only generated URL strings.

## Recovery and identity

The prefix uses the accepted helper instance ID, which is unique per execution.
Different tasks therefore have different prefixes even if they reuse a port.
Recreation for the same execution can reuse its ID because the helper still
serves the same task session. Keep existing ID allocation, request carriers,
metadata keys, owner matching, and Kubernetes recreation behavior unchanged.

Retained adoption reads effective mode only from authenticated `InstanceInfo`.
Propagate it through the existing control/list/get results:

- Standalone/worktree: `buildRecoveredInstancesWithLease` and standalone reconnect.
- Docker/Remote Docker: existing `GetInstance`/owner-list results.
- SSH: `readSSHRecoveryInstanceInfo`.
- Sprites: `inspectSavedSprite` and `buildRecoveredSpriteInstance`.
- Plugin remote: `readRecoveredAgentctlInstance`.
- Shared Kubernetes: `getOrCreateSharedKubernetesInstance`, including its
  synthesized create response.

`reattachKubernetesAgentctl` currently checks only instance health. Read
`InstanceInfo` through the existing temporary control forward before publishing
its client. A mode-read failure is not proof of helper death and must not
trigger recreation. No adoption path needs a second mode source on the
session-association endpoint. A client already held by the live execution
retains its accepted mode during in-process reattachment.

A successful older `InstanceInfo` without the field means legacy.
Read failures retain the existing recovery-required behavior and Stop access.
Adoption issues no ACP initialize, load, resume, or new request. The bool needs
no database column, metadata record, or cross-endpoint reconciliation rule.

Staged delivery uses the staged client's `InstanceID()` and effective mode,
including `placement.client` for project files. The published execution client
can still be old. Keep existing prepare/commit/abort and cleanup behavior.
Test separate staged/published clients with different modes, both when their
helper IDs match and when the accepted IDs differ. Preserve delivery streams,
SQL/native-session generations, Pod/PVC identity, and journal ownership.

## Authority and observability

Path scoping prevents accidental cross-target routing. The instance ID is not
a credential. Existing authenticated task-owner dispatch and workflow policies
remain necessary. The design adds no authority based on a path or package name.

Log rejected native paths with bounded reason and route-template fields.
Use one per-server rate limiter, with no background goroutine or unbounded
map keyed by caller IDs. An existing dependency can supply the limiter.
Do not log full URLs, query strings, question arguments, or expected identities
in the rejection event. No new metric, probe, or health aggregate is necessary.

The mock ACP client already consumes the supplied SSE URL. The mock TUI client
already consumes the supplied streamable HTTP URL. TUI agents intentionally
disable interactive questions, so validation uses `/e2e:mcp-context` there.
This preserves question and completion rules on both desktop and phone.

## Verification boundaries and records

The [implementation plan](../../../plans/builtin-mcp-instance-scoped-routes/plan.md)
maps each criterion to named Go tests and browser flows. Tests include both
catalog modes, all native transports, both skew directions, same-port reuse,
actual approval, retained adoption, and staged replacement/rollback.
Installed-provider checks cover Claude ACP, Codex, and Cursor before release.
Mock results cannot certify installed-provider behavior or Cursor approval UX.

Implementation must update the bare-route paragraph in
[`automation-and-mcp.md`](../../../public/automation-and-mcp.md), the transport
section of the existing package design, and scoped agentctl engineering guidance.
The package contract and public endpoint remain unchanged in all other respects.

## Related decision

[ADR: helper-scoped routes](../../../decisions/2026-11-16-instance-scoped-builtin-mcp-routes.md)
records the alternatives and compatibility choice.
