# ADR-2026-08-17-plugin-workspace-agent-conversations: Managed workspace agent conversations

**Status:** accepted
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

The Coordinator plugin owns scheduling, reports, workflow workstep policy, and prompt
composition. It uses existing route and Integrations navigation registration. Disabling,
upgrading, or changing configuration retains managed conversation data; uninstall stops
the plugin and fails visibly if provenance-safe cleanup cannot complete.

## Consequences

The host owns identity, visibility, profile/executor resolution, dispatch idempotency,
and lifecycle cleanup. Plugin authors receive a narrow public contract rather than
database or private-API access. The Host protobuf, SDK, manifest, task service, and
native UI surfaces require additive compatibility tests.

## Alternatives Considered

- **Visible task plus a client filter.** Rejected because visibility and cleanup would
  not be an enforceable server-side ownership boundary.
- **Utility-agent invocation.** Rejected because it is sessionless and cannot retain
  interactive history or run task-capable MCP loops.
- **Office taskless runs.** Rejected because they are not an interactive chat contract
  and would couple a Kanban plugin to Office runtime behavior.
