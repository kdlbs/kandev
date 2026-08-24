# ADR-2026-08-17-plugin-workspace-agent-conversations: Managed workspace agent conversations

**Status:** accepted (amended 2026-08-24)
**Date:** 2026-08-17
**Area:** backend, frontend, protocol, security

## Context

Coordinator needs a durable interactive agent conversation for a workspace without
creating a visible Kanban task. Existing routes and Integrations navigation can place
the UI, but existing task creation and utility-agent invocation do not supply the
required ownership, lifecycle, transcript, or authorization boundary.

## Decision

Kandev will expose capability-gated `AgentConversations` Host operations and a
host-owned `WorkspaceAgentChat` UI primitive. A host-managed conversation is uniquely
identified by plugin, workspace, and conversation key. It uses a hidden workflowless
ephemeral backing task/session, stamps its provenance server-side, is excluded from
ordinary task and Quick Chat projections, and is deleted only by its owning plugin's
successful uninstall.

The Coordinator plugin owns reports, workflow workstep policy, prompt composition,
and its durable Coordinator product state. It must not own a cron/scheduler:
Kandev Automations trigger Coordinator cycles. The generic conversation remains
transport only. Disabling, upgrading, or changing configuration retains managed
conversation data; uninstall stops the plugin and fails visibly if
provenance-safe cleanup cannot complete.

### Amendment: Coordinator product identity is separate

This ADR defines a generic plugin conversation and chat compatibility contract.
It does not define the Coordinator product identity, authority, audit history,
or wake ownership. The Coordinator product is now specified as a first-class,
host-owned workspace object in
[ADR-2026-08-24-first-class-workspace-coordinator](2026-08-24-first-class-workspace-coordinator.md).

For Coordinator, the backing task/session is a replaceable Stage 0 execution
adapter. It is not an immortal session, a visible product identity, or a source
of implicit authority. The generic Integrations recipe stays available to other
workspace-agent plugins; Coordinator uses a generic workspace-agent destination
placement after Integrations rather than a Coordinator-specific core surface.

## Consequences

The host owns generic conversation visibility, profile/executor resolution,
dispatch idempotency, and lifecycle cleanup. Plugin authors receive a narrow
public contract rather than database or private-API access. The Host protobuf,
SDK, manifest, task service, and native UI surfaces require additive
compatibility tests. Coordinator-specific policy, reports, run ledger, audit
projection, and wake interpretation remain plugin-owned. Any additional core
contract must be generic workspace-agent identity/authority, inbox, logical
conversation, or Automation delivery rather than Coordinator behavior.

## Alternatives Considered

- **Visible task plus a client filter.** Rejected because visibility and cleanup would
  not be an enforceable server-side ownership boundary.
- **Utility-agent invocation.** Rejected because it is sessionless and cannot retain
  interactive history or run task-capable MCP loops.
- **Office taskless runs.** Rejected because they are not an interactive chat contract
  and would couple a Kanban plugin to Office runtime behavior.
