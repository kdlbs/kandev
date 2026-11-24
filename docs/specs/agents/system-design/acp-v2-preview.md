---
status: current
system: agents
requirements:
  - REQ-AGENTS-ACP-V2-001
  - REQ-AGENTS-ACP-V2-002
  - REQ-AGENTS-ACP-V2-003
  - REQ-AGENTS-ACP-V2-004
---

# ACP v2 preview system design

## Purpose and boundaries

The Agent system owns ACP negotiation and projection into Kandev's agent events.
The lifecycle manager retains process ownership and the delivery journal retains
prompt submission and event identity.

| Requirement | Design |
| --- | --- |
| `REQ-AGENTS-ACP-V2-001` | [Launch admission and negotiation](#launch-admission-and-negotiation) |
| `REQ-AGENTS-ACP-V2-002` | [Session and turn handling](#session-and-turn-handling) |
| `REQ-AGENTS-ACP-V2-003` | [Recovery and rollback](#recovery-and-rollback) |
| `REQ-AGENTS-ACP-V2-004` | [Diagnostics and verification](#diagnostics-and-verification) |

## Launch admission and negotiation

Use `features.acpV2` and `KANDEV_FEATURES_ACP_V2`. Set all profile defaults
to false. Register an experimental toggle named **ACP v2 preview** with
restart-required metadata. The existing settings route reads the registry and
renders the same toggle on desktop and phone. Runtime precedence follows
[ADR 0018](../../../decisions/0018-runtime-settings-overrides.md).

The lifecycle manager resolves eligibility before it sends the local standalone
launch configuration. Eligibility requires the effective flag, a qualified managed
version of `claude-acp` or `codex-acp`, ordinary local credentials, and ACP chat.
Use the version already resolved for the launch. Initial qualification targets
the pins listed in the [plan](../../../plans/acp-v2-preview/plan.md).
The authenticated agent configuration carries the preference to agentctl, including
when a task starts an agent on a helper that already serves its workspace.
An absent field means v1. Workspace-only operations, utilities, profile probes,
gateway launches, and other executors use their existing paths.

Agentctl owns the protocol selection for the lifetime of each connection.
The existing v1 adapter sends the usual v1 initialize request for a v1 launch.
For a v2 candidate, send `protocolVersion: 2` with v2 `info` and `capabilities`.
Require version 2 and valid v2 `info` before session traffic. The response can
omit optional capabilities. Test the standard v2 contract with Kandev's client
identity.

If the response selects version 1, return a typed fallback result to the
process manager. The same result applies when a peer stamps 2 onto a v1
envelope, or returns a version-negotiation error that explicitly offers v1.
Stop and join the first subprocess. Use the original launch snapshot to start one
fresh process and send full v1 client capabilities. Propagate ambiguous envelopes,
transport errors, and post-handshake failures through the normal error path.
Report the selected ACP version from the adapter that supplies agent info for
the initialize response's process generation.

## Go wire boundary

Keep the v1 bindings at the SDK root. Add separate v2 bindings in
`kdlbs/acp-go-sdk` and pin their provenance to ACP schema
`schema-v2.0.0-beta.1`. Reuse the fork's generic `Connection`, JSON-RPC errors,
notification ordering, and cancellation. Build a typed v2 client for the methods
the pilot uses. Validate these wire types against the pinned schema and captures.
Whole-schema generation is a separate step because the current generator collides
on the v2 state union and injects v1 fields into v2 setup types.

Register the v2 adapter at the existing `agent.ProtocolACP` factory boundary.
Share the existing configuration, MCP attachment, and event contracts.
Keep version-specific wire structures in the version-specific adapter.
The native Codex app-server protocol has its own factory branch.

## Session and turn handling

- Pass supported MCP transports under the v2 session capabilities. Reuse the
  per-session server configuration and attachment evidence. V2 delegates
  agent-side execution to the agent and carries terminal output for display.
- Read setup config options by `configId` and use the typed discriminator
  when applying a model or mode change. Handle auth methods by `methodId`.
  Auth errors use the existing sign-in route for the configured provider.
- Cache setup settings internally while strict startup applies the session's
  configuration. Publish the settled settings even if the setup snapshot
  arrives after the execution becomes ready. Provider-restored sessions
  continue to publish the restored configuration.
- Bind one active foreground submission at a time. Keep the Kandev submission
  ID stable and use it as the v2 `idempotencyKey` when available.
  Correlate the reply and user message update by session and `messageId`.
  Either order can occur. A reply with `messageId` establishes acceptance.
- Process `session/update` in order. Use `state_update` for foreground
  progress, a pending user action, and completion. Publish the terminal
  event once after earlier deltas clear the update queue. Preserve explicit
  failure and cancellation reasons. If the stop reason is missing or unknown,
  record that the turn stopped with an unknown outcome and keep the runtime
  available for the next turn. Release the state lock before waiting for
  ordered event delivery so cancellation and callbacks can still inspect state.
- Scope message and tool updates to their session and entity ID. An omitted
  field preserves the value, `null` clears it, and chunks append to it.
  Keep the latest whole-message or tool snapshot for projection and replay.
  Preserve the prompt generation on each projected message replacement.
  Resolve an omitted generation from the execution at publication.
  Route reported child sessions under the parent with their own IDs.
  Skip well-formed future update variants and keep later updates flowing.
- Use `session/cancel` for a foreground interrupt. The terminal state
  settles cancellation. Use the existing permissions and elicitation handlers
  for validated v2 requests. Keep replies, timeouts, and request cancellation
  scoped to the process generation and session. Advertise a capability only
  if Kandev can satisfy its callbacks.

## Recovery and rollback

Use `session/resume` with the saved native ID for a fresh v2 connection.
When Kandev needs history replay, set `replayFrom: { "type": "start" }`.
Use `replay_complete` and the response to finish attachment. Reconcile the
replay with the saved projection before processing the live update stream.
Keep the saved identity if replay or resume fails. Reuse the existing
`session/load` and `session/resume` paths for a fresh v1 connection.

The agentctl instance reports its selected version through authenticated
instance information. Adoption reads that version and the delivery cursor
while keeping the existing connection. If the effective flag is off for
a retained v2 connection, fence new submissions at the backend dispatch
boundary. Keep delivery reconciliation, pending replies, and Stop available.
A later fresh launch selects v1 and attempts to restore the saved ID.

A correlated RPC error before acceptance can reject the submission. A loss
of transport before a correlated reply leaves the acceptance outcome unknown.
Use the existing recovery contract for ambiguous outcomes. Replaying history
alone cannot establish that the provider did not insert a prompt.

## Diagnostics and verification

Log the agent ID, agent version, requested and selected protocol version, and
the fallback reason with the existing initialize trace. Keep raw capture on the
explicit `acpdbg` path. The debugger defaults to v1 and can request v2 directly
with `--protocol-version 2`. For v2 it waits for foreground completion and can
replay from the start with `session/resume`.

Record the two adapter versions and a v1 comparison in the pilot evidence.
Keep provider qualification independent of the registry's package version or
the agent's numeric initialize echo. The [ACP v2 migration guide](https://agentclientprotocol.com/protocol/v2/migration)
and [Preview schema release](https://github.com/agentclientprotocol/agent-client-protocol/releases/tag/schema-v2.0.0-beta.1)
define the wire behavior.
