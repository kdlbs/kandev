# ADR-2026-08-11-live-agent-permission-authority: Keep Pending Agent Permission Authority in the Live Runtime

**Status:** accepted
**Date:** 2026-08-11
**Area:** backend, agentctl, protocol, security

## Context

Agent permission prompts exist in two forms: a live request held by agentctl while the
provider waits for an answer, and a task-session message used by Kandev's UI and history.
The message can outlive the provider request, while provider-supplied pending IDs may be
reused or collide across executions. External MCP clients need to enumerate and resolve
requests without treating a stale UI projection as executable authority.

## Decision

The agentctl process manager remains authoritative for whether an agent permission request
is currently answerable and for the immutable options originally offered by the provider.
Each live request receives a Kandev-generated `request_id` in addition to the provider's
`pending_id`. Strict resolution requires the backend-owned task and task-session IDs plus
both request identities and one offered `option_id`; agentctl validates the complete tuple
and consumes it atomically before forwarding the selected option.

The orchestrator exposes a narrow authorized permission service above the runtime. MCP,
the existing WebSocket UI path, and automation callers use that service rather than reading
frontend state or calling agentctl directly. It authorizes the task/session pair before any
runtime lookup, presents an allowlisted and redacted snapshot, and records resolution
attempts and outcomes on the existing permission-request message. A compare-and-set claim
on that message serializes concurrent resolvers and records actor, source, request identity,
option identity, timestamps, and result without persisting credentials or raw environment
values.

`external_mcp` is a transport-attested audit source, not a caller-authored field. The backend's
external MCP dispatcher bridge adds a process-local context marker only after the request enters
through the external `/mcp` transport. The shared dispatcher handler requires that marker before
assigning the source, so raw WebSocket and in-session MCP dispatch cannot forge external-MCP
attribution.

The transcript remains the durable audit projection, not a source of live permission
authority. Listing returns only requests still pending in agentctl. A historical message can
explain an already-resolved or expired request, but it can never make a request actionable.

## Consequences

- Reused provider pending IDs cannot redirect a stale approval to a newer prompt because the
  Kandev request generation must also match.
- Every transport shares task/session authorization, option validation, concurrency control,
  and audit behavior.
- Agentctl gains a typed list/snapshot operation and a strict resolution operation, while the
  orchestrator and persistence layers gain a reusable permission service and audit claim.
- Permission messages may retain an indeterminate dispatch result if the backend stops between
  the durable claim and the runtime response. That state is honest and replay-safe: a retry
  cannot create a second resolution, and recovery can report the recorded attempt rather than
  silently guessing.
- External presentation cannot reuse raw `action_details`; adding a new provider action shape
  requires an explicit safe projection.
- Adding another trusted external transport requires an explicit in-process attestation at its
  boundary; registering the shared dispatcher action or accepting a wire field is insufficient.

## Alternatives Considered

### Enumerate pending permission messages from the database

Rejected because a message can be stale after the provider withdraws a prompt, and a durable
row cannot prove that an agent is still waiting for that exact request.

### Let the MCP handler query agentctl directly

Rejected because it would duplicate authorization and audit behavior, couple a public
transport to runtime internals, and leave the existing web path with different safety rules.

### Use only the provider's pending ID

Rejected because provider IDs have provider-specific lifetimes and uniqueness guarantees.
They cannot protect against replacement or reuse across execution generations.

### Persist every raw action detail for faithful presentation

Rejected because command arguments, MCP arguments, headers, and environment-derived values
can contain credentials. The public contract uses an allowlisted, redacted presentation while
the provider-native response continues to use the untouched internal request.
