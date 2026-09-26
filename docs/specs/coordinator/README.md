---
status: draft
system: coordinator
specification_version: 1
migration: complete
owners:
  - kandev
---

# Workspace coordinator

## Purpose

The workspace coordinator system owns an optional, feature-flagged agent that
reads a Kanban workspace, tells a workspace manager what needs them and why,
and proposes work that the manager approves. It is a core product surface for
regular Kanban workspaces; a coordinator plugin remains possible as an optional
extension through the generic plugin Host contracts.

Phase 1 is attended: every coordinator turn starts from a message a workspace
manager sends. The coordinator's Kandev tool surface can only read and propose.

## Terms

- **Workspace coordinator:** a named, workspace-scoped agent configuration
  (agent profile, executor, context text) with one conversation, its own
  proposals and its own attention count. In the UI it is called
  "Coordinator". It is not Office's coordinator role, which is an Office agent
  role defined by the [Office system](../office/README.md).
- **Proposal:** a stored request by a coordinator to create one task. Only a
  person with `workspace.manage` can approve or reject it.
- **Needs you:** the coordinator's list of items that need a person, derived
  from task facts, stall records and pending proposals.
- **Queue:** the coordinator's read-only list of the other open tasks, grouped
  by position.
- **Stall record:** the coordinator's durable copy of one `task.stalled`
  observation for a coordinated workspace.
- **Copilot:** the chat popover on the Coordinator screens backed by the
  coordinator's conversation session.

## Ownership

This system owns:

- coordinator identity, configuration and lifecycle within a workspace;
- the coordinator conversation task, its `coordinator` task origin and the
  `coordinator` MCP surface, mode and allowlist;
- proposals, their states, approval and rejection;
- stall records and their retention;
- the Needs you and Queue projections, the count strip, the sidebar entries and
  the `coordinator.updated` event;
- the `features.coordinator` release toggle.

## Exclusions

- Tasks, workflows, sessions, `statusSummary`, `task.stalled` detection and task
  creation idempotency belong to the [task system](../tasks/README.md).
- Agent profiles, permission policy and agentctl's generic permission handling
  belong to the [agent system](../agents/README.md).
- Workspace membership, scopes and deletion belong to the
  [workspace system](../workspaces/README.md) and the [auth system](../auth/README.md).
- The Needs-you Inbox, its count and its tabs are unchanged by this system.
- Office's coordinator role, heartbeats and routines belong to the
  [Office system](../office/README.md).
- The generic plugin Host boundary stays with the [plugin system](../plugins/README.md).

## Find specifications

Use the catalog command to list this system's current documents:

    python3 scripts/list-docs.py specs --system coordinator --format markdown
    python3 scripts/list-docs.py specs --system coordinator --kind requirement --format paths
    python3 scripts/list-docs.py specs --system coordinator --kind system-design --format paths

Do not copy the command output into this README.

## Migration record

The system is new and has no legacy sources.

## Related systems

- [Tasks](../tasks/README.md): supplies tasks, sessions, status summaries,
  `task.stalled` and idempotent task creation.
- [Agents](../agents/README.md): supplies agent profiles and agentctl.
- [Workspaces](../workspaces/README.md): supplies workspace scope and deletion.
- [UI](../ui/README.md): supplies the sidebar, settings shell, chat primitives
  and Quick Chat session view.
- [Office](../office/README.md): owns a different "coordinator" concept.
- [Plugins](../plugins/README.md): keeps the generic Host contracts a
  coordinator plugin could use.

## Related decisions

- [Workspace coordinator in core](../../decisions/2026-09-26-workspace-coordinator.md)
- [Workspace coordinator implementation plan](../../plans/workspace-coordinator/plan.md)
