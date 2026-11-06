---
id: "01-workspace-scope-declaration"
title: "Workspace scope declaration and dispatch gate"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLUGINS-AGENT-TOOL-WORKSPACE-001
acceptance_criteria:
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.1
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.2
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.8
  - AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.9
system_design:
  - ../../specs/plugins/system-design/agent-tool-workspace-scope.md
---
# Task 01: Workspace scope declaration and dispatch gate

## Summary

Accept `invocation_scope: workspace` in plugin manifests and carry it through
runtime descriptors. Make the plugin dispatch gate require a workspace for
external workspace calls and none for external instance calls.

## In scope

- Manifest constant, normalization, and validation: allowed surfaces, read-only
  rule for `external`, top-level object input schema, reserved
  `kandev_workspace_id` property.
- `external` accepts `instance` or `workspace`; `coordinator` stays
  instance-only.
- `plugintools.InvocationScopeWorkspace`, `Definition.IsWorkspaceRead()`, and
  the external registration filter in `server.go`.
- `validateInstanceSurfaceDeclaration` scope and workspace consistency for
  `external`.
- Workspace tools on `kanban-task`/`office-task` dispatch with the execution
  workspace.

## Out of scope

Schema injection, argument stripping, workspace authorization, and docs
(work order 02).

## Acceptance

- Manifests that break AC-001.2 fail validation with a clear field error. Valid
  workspace declarations install, and existing `task`/`instance` declarations
  validate unchanged.
- External dispatch fails before the RPC for `instance` with a workspace,
  `workspace` without one, and any `task` tool.
- A workspace tool on a task surface receives the execution's task, session,
  and workspace.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/plugins/... ./internal/mcp/...)
bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `apps/backend/internal/plugins/manifest/manifest.go`, `validate.go`, `agent_tools_test.go`
- `apps/backend/internal/plugins/agent_tools.go`, `agent_tools_contexts_test.go`
- `apps/backend/internal/mcp/plugintools/types.go`
- `apps/backend/internal/mcp/server/server.go` (external filter only)
- `apps/backend/internal/mcp/handlers/plugin_tools_contexts_test.go`

## Dependencies

None.

## Risks

`declaredInstanceOnlySurface` is used for both coordinator and external. Split
it carefully so coordinator still rejects `workspace`.

## Parallelism

sequential

## Results

Implemented. Manifests accept `invocation_scope: workspace` with the
`external`, `kanban-task`, and `office-task` surfaces, reject
`automation`/`coordinator`/`managed-conversation`, require a true read-only
hint on `external`, require a top-level object schema without a reserved
`kandev_workspace_id` property, and enforce `min_kandev_version >= 0.100.0`.
Runtime descriptors carry the normalized scope and `IsWorkspaceRead()`. The
external dispatch gate requires a verified workspace for workspace tools and
none for instance tools, while task surfaces dispatch with the execution
workspace. `ProjectLegacySafe` rewrites a surviving workspace descriptor to
task scope so a retained pre-workspace agentctl keeps the catalog.

Verified with `go test -trimpath ./internal/plugins/... ./internal/mcp/...`.
Tests: `TestAgentToolWorkspaceScopeValidation`,
`TestAgentToolCatalogTransportsWorkspaceScope`,
`TestExternalDispatchGateScopeWorkspaceConsistency`,
`TestWorkspaceToolTaskSurfaceUsesExecutionWorkspace`, and
`TestProjectLegacySafeProjectsWorkspaceScopeToTask`.
