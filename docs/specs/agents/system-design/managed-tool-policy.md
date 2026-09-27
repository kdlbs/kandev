---
status: draft
system: agents
requirements:
  - REQ-AGENTS-MANAGED-TOOLS-001
  - REQ-AGENTS-MANAGED-TOOLS-002
created: 2026-09-25
owners:
  - kandev
---

# Managed agent tool policy system design

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-MANAGED-TOOLS-001` | [Restricted execution](#restricted-execution) |
| `REQ-AGENTS-MANAGED-TOOLS-002` | [Provider and lifecycle enforcement](#provider-and-lifecycle-enforcement) |

## Purpose and boundaries

The shared agent runtime owns enforceable tool policy. Plugin manifests select
tools and Host capability grants authorize commands. A prompt is never an access
control. Core treats each conversation instance as opaque; it does not define a
chief-of-staff provider or coordinator role.

## Restricted execution

Add a versioned managed execution policy to shared launch requests. It contains
installation/workspace/conversation IDs, allowed namespaced plugin tool IDs,
policy revision, and a host-issued execution generation reference. The Host
validates this policy against current manifest and approval before launch.
Agentctl receives the validated policy through its authenticated runtime channel.

Keep the single Kandev MCP broker from the plugin-tools ADR. Extend manifest tool
applicability with an explicit managed-conversation surface and an instance tool
selection restricted to declared tools. Existing kanban-task/office-task semantics
remain unchanged. Never make all plugin tools visible just because one plugin owns
the conversation. Required protocol controls cannot mutate unrelated task state.

Adapters disable native tools, unrelated ambient MCP, file, shell, and network
access for the agent turn. The broker exposes only the selected plugin tools.
Where an adapter cannot disable one of those channels, it does not support this
policy. Provider account privileges alone are not proof of enforcement. An initial
release may support only the provider proven by launch/resume contract tests.

The broker stamps calls with conversation/session/execution identity derived from
its authenticated process context. Plugin tool parameters cannot override it.
The plugin backend passes an opaque, host-verifiable provenance handle to exact
Host commands. The host validates it against live execution and current policy on
every call. Plugin-scheduled background calls use installation authority without
pretending to be an active agent. Keep those origins distinct in audit receipts.

## Provider and lifecycle enforcement

Expose a provider capability for restricted managed execution only when launch,
resume, tool dispatch, and recovery all enforce the policy. Reject unsupported
profile/executor combinations before creating an execution. Share this result with
capability discovery and the profile selector. Do not silently choose another agent.

Mint a new execution generation for each actual launch. Rotate it on resume or
recovery that replaces the execution. Persist intended policy but validate authority
again before applying it. Reject old process calls after replacement, stop, claim
transfer where relevant, disable, or uninstall. Revocation stops further Host effects
at the authorization boundary and requests normal runtime cancellation. Report an
unconfirmed cancellation rather than claiming the process stopped.

Managed sessions may request only supported modes that preserve this policy.
Permission decisions still flow through native human interaction services. Neither
an `acceptEdits` label nor provider-specific mode can widen the restricted tool set.
The fork's provider login-lock repair is deliberately excluded: normal provider
configuration owns credential recovery.

## Components and persistence

Extend `internal/agent/runtime` launch/lifecycle contracts, the agentctl process
manager, provider adapters, MCP plugin-tool context, and persisted session metadata.
The plugins service resolves declarations and approvals. The runtime creates the
execution reference; agentctl enforces tool exposure; Host adapters enforce effects.
No new public MCP server is introduced. Broker credentials are short-lived, bound
to execution, and never written into plugin-visible prompt text or logs.

Existing adapter tests gain a policy matrix: launch, resume, restricted native-tool
attempt, foreign plugin tool, stale generation, disable, and unknown provider.
A protocol fake proves wire enforcement; a supported real adapter smoke must verify
its documented tool-deny mechanisms before advertising production capability.

## Failure, security, and observability

Policy validation failure returns a typed unsupported/denied result without
starting an agent. An unexpected tool call is rejected and audited, not converted
to a broader fallback route. Startup reconciliation rotates expired handles and
revalidates support. Log provider kind, policy revision, operation, and deny reason;
never log credential material, full prompts, or bearer handles. Native installed
plugin code remains trusted and is outside the managed-agent restriction boundary.

## Related documents

- [Requirements](../requirements/managed-tool-policy.md)
- [Implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
- [Coordination platform decision](../../../decisions/2026-09-25-plugin-coordination-platform.md)
