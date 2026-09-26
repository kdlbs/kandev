---
id: plugins-canvas-workspace-data-scope-design
title: Canvas placement and data scope in the web-app runtime
status: draft
system: plugins
owners:
  - kandev
created: 2026-09-23
last_updated: 2026-09-23
requirements: []
---

# Canvas placement and data scope in the web-app runtime

## Boundary

This is the Plugins runtime implementation contract for the
[Canvas workspace preview requirement](../../canvases/requirements/task-canvas-workspace-preview.md).
Canvases decides when an owner-authorized local app may use workspace data.
Plugins persists and enforces that decision; it does not infer authority from
the manifest or browser request. The existing
[isolated web-app design](isolated-web-app-contributions.md) remains the general
runtime contract.

## Instance model and grants

Add nullable `plugin_instances.data_scope_kind`. A null value resolves to
`scope_kind` for retained rows and non-canvas instances. A non-null workspace
data scope is valid only for a `local_canvas` instance placed in a task or
workspace with a trusted `workspace_id`. Reject any attempt to use an
installation-wide data scope for a canvas. Creation, reviewed upgrade, and
promotion update the field and grants through transactions owned by the
canvas lifecycle service.

Grant fit, release activation, and runtime validation compare each declared
permission with the effective data scope. An exact task grant cannot cover
workspace data. A workspace grant authorizes only the bound workspace and is
still intersected with current user and resource access. Revocation and scope
changes increment the existing grant generation.

## Runtime binding and protocol

Add effective `DataScopeKind` to `webapp.CapabilityBinding`, sourced from the
persisted instance and checked against it on every request. Keep `ScopeKind`
for placement and task lifecycle. The relative context response exposes both
`scope_kind` and `data_scope_kind`; empty historical data scope resolves to
placement scope.

Task lists, single-task reads, task writes, messages, workflow reads, task
dependency projections, and events use data scope. Workspace-data task
canvases use the paginated workspace path, bounded by the trusted workspace
ID, rather than the task-only shortcut. Explicit foreign `workspace_id`
filters fail. Non-canvas instances retain current behavior.

## Compatibility and failure

The schema migration does not widen old rows. Historical runtime bindings
and grants remain task-scoped until a reviewed update changes them. A stale
binding, missing workspace/task, revoked grant, or mismatched data scope fails
closed through existing safe errors. Current plugin manifests and immutable
canvas packages need no rewrite.

## Verification

Test SQLite and PostgreSQL mapping, legacy null fallback, invalid scope
combinations, runtime token invalidation, exact grants, all workspace task
pages, task read/write and message paths, workflow and event scope, dependency
redaction, and foreign-workspace denial.

## Related decision

- [Task canvas workspace data](../../../decisions/2026-09-23-task-canvas-workspace-data.md)
