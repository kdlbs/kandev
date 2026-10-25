# ADR-2026-08-20-plugin-task-relations-host-api: Expose Compact Task Relations to Plugins

**Status:** accepted
**Date:** 2026-08-20
**Area:** backend, plugin SDK, protocol, security

## Context

Plugins sometimes need task topology to coordinate work, but `Host.Tasks()` carries broad task fields including descriptions and metadata. A plugin should not need that broader read capability merely to inspect a task's parent, children, siblings, blockers, or blocked tasks.

## Decision

The Host API exposes `Host.TaskRelations().Get(ctx, workspaceID, taskID)`, gated by the independent `api_read:task_relations` manifest capability. The caller must supply the authorized workspace ID. Unknown targets and targets in another workspace fail as `NotFound`, and every projected relation is filtered to that workspace.

The `TaskRelations` DTO contains only compact task identity and lifecycle fields: ID, workspace ID, identifier, title, and state. Its parent, children, siblings, blockers, and blocked-by groups carry relationship topology. It deliberately excludes descriptions, task documents, free-form metadata, and repository details. `api_read:tasks` remains the separate capability for the broader task reader.

## Consequences

Any plugin can consume compact task relationships through a provider-neutral Host/SDK contract without gaining document or description access. Provider-specific orchestration policy and plugin behavior remain outside the Kandev core.

## Alternatives Considered

- Extend `api_read:tasks` with relationship methods. Rejected because it turns a topology read into a backdoor for broad task fields.
- Put relationship traversal in one provider plugin. Rejected because the task graph and workspace authorization are core data responsibilities reusable by all plugins.
