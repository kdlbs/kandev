---
status: draft
system: orchestration
specification_version: 1
migration: in_progress
owners:
  - Kandev
---

# Orchestrator

The Orchestrator is Kandev's single central coordination and assistant product.
Workspace assignments, the central task view, owner-level objectives, attention,
memory, capability discovery and managed work are one feature set behind
`features.orchestration`. The older Personal Assistant name remains in some
requirement IDs and file paths for compatibility, but it is not a second product
or rollout gate. Office remains the separate legacy autonomous-agent fleet.

## Purpose and ownership

Orchestration owns reusable coordinator roles, workspace assignments, persistent
coordinator conversations and the coordinator's view of workspace work. It has
its own lifecycle and runtime contract, as recorded in the
[ownership decision](../../decisions/2026-09-07-workspace-orchestration.md).
This system owns the central task overview because it combines a workspace
coordinator assignment with task observations; the task system remains the
source of truth for every task and session state.

## Specification map

- [Coordinator view requirements](requirements/coordinator-view.md): authoritative
  draft for the newly proposed central workspace task view and side conversation.
- [Coordinator view system design](system-design/coordinator-view.md): implementation
  boundaries grounded in v0.94.0 and the existing local coordinator prototype.
- [Delivery plan](../../plans/workspace-coordinator-view/plan.md): pending work orders.
- [Personal assistant requirements](requirements/personal-assistant.md) and
  [system design](system-design/personal-assistant.md): authoritative contracts
  for completing the partly implemented assistant, replacing the legacy spec.
- [Complete delivery plan](../../plans/orchestration-delivery/plan.md): repository
  review, rollout isolation, candidate validation, dogfooding and upstream export.

The view is not implemented. Draft status concerns this design package; it does
not reclassify the previously implemented coordinator foundation as new work.

## Legacy contracts and migration boundary

The following documents remain authoritative for their existing capabilities
until migrated. This package adds the task overview; it does not copy or replace
their editable requirements:

- [Workspace orchestrators](../workspace-orchestrators/spec.md): global roles,
  assignments, runtime, conversation, task callbacks and feature gating.
- [Unified workspace orchestration](../unified-workspace-orchestration/spec.md):
  existing Kanban execution and workspace identity.
- [Chief of staff](../chief-of-staff/spec.md) and
  [workspace agents](../workspace-agents/spec.md): earlier Office integration,
  superseded by workspace orchestrators for the first-class coordinator surface.
- [Personal assistant legacy pointer](../personal-assistant/spec.md): redirects
  to the requirements/design above. Its app-level UI and automatic input
  resolution remain outside the workspace task-view package.

## Related systems and exclusions

- [Tasks](../tasks/README.md) owns workflow state, sessions, pending input,
  status projections, queues, delivery and review gates.
- [Workspaces](../workspaces/README.md) owns workspace access and selection.
- [Agents](../agents/README.md) owns execution profiles and provider configuration.
- [Office](../office/README.md) retains its product, personas and heartbeat.
- [Plugins](../plugins/README.md) owns host/plugin capabilities. No plugin API
  expansion or plugin installation is implied by this package.

Automations remain the existing optional schedule/delivery mechanism. The view
introduces no scheduler, task engine, grant model or durable task-state copy.
