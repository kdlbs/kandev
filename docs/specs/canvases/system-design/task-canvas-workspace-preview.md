---
id: canvases-task-canvas-workspace-preview-design
title: Task canvas workspace data preview design
status: draft
system: canvases
owners:
  - canvases
created: 2026-09-23
last_updated: 2026-09-23
requirements:
  - REQ-CANVASES-WORKSPACE-PREVIEW-001
---

# Task canvas workspace data preview System Design

## Boundary and mapping

Canvases owns when a locally requested task canvas gains workspace data scope,
how an older canvas can opt in, and when promotion changes placement. Plugins
persists the instance's effective data scope and enforces it through the shared
web-app runtime. Task services remain authoritative for data and current user
access. This design implements `REQ-CANVASES-WORKSPACE-PREVIEW-001`.

| Criteria | Design sections |
| --- | --- |
| `AC-CANVASES-WORKSPACE-PREVIEW-001.1`, `.2`, `.8` | Data scope and runtime enforcement |
| `AC-CANVASES-WORKSPACE-PREVIEW-001.3`, `.4` | Lifecycle and presentation |
| `AC-CANVASES-WORKSPACE-PREVIEW-001.5`, `.6`, `.7` | Existing canvas upgrade and host flow |

See [the workspace preview decision](../../../decisions/2026-09-23-task-canvas-workspace-data.md)
and [the existing creation authority](local-creation-authority.md).

## Data scope and persistence

Keep `plugin_instances.scope_kind` as the placement/lifecycle scope. Add a
nullable `data_scope_kind` to `plugin_instances`; null means the existing
`scope_kind` so older rows and non-canvas plugins retain their behavior. A
non-null value is valid only when it equals the placement scope, or when a
`local_canvas` task instance uses `workspace` data scope with its existing
trusted `workspace_id` and `task_id`. Reject `instance` and foreign workspace
data scope for canvases. Do not accept the field from agent, manifest, or
browser input. Store it through SQLite and PostgreSQL schema paths, backup,
snapshot, and instance row mapping.

At trusted canvas creation, record workspace data scope only for a new task
canvas with owner-authorized creation provenance. The task remains the canvas
owner and the plugin instance's placement scope. The first valid release
derives grants from exactly its declared supported capabilities with a
`workspace` ceiling, then consumes creation authority and activates in the
existing transaction. Raise `CreationAuthorityPolicyVersion` for newly issued
authority; consumed version-1 rows remain historical evidence and are never
reissued. If provenance or current owner checks fail, publication follows the
normal permission-review path and cannot infer a workspace grant.

All grant-fit, release activation, and runtime-binding validation for a web
app use its effective data scope. A grant ceiling cannot be interpreted as
covering a wider data scope merely because it covers the placement scope. A
later declaration increase still creates a pending release. Revocation bumps
the existing grant generation and invalidates runtime tokens.

## Runtime enforcement

`webapp.CapabilityBinding` carries both placement `ScopeKind` and effective
`DataScopeKind`. Issue both from the persisted instance, and compare both with
the current instance on every token validation. Existing bindings with an
empty data scope resolve to placement scope. The relative
`./_kandev/v1/context` response retains `scope_kind` for placement and adds
`data_scope_kind` for authors to label the available data accurately.

Use the effective data scope for the web-app task list, single-task read,
task update, message dispatch, workflow list and steps, task dependency edge
projection, and subscribed task events. A workspace data scope binds all
queries and mutations to the instance's trusted `workspace_id`, including
explicit `workspace_id` filters. It does not derive workspace IDs from the
request or allow `ScopeInstance`. The task-list route uses its existing
pagination path instead of the bound-task shortcut. Each operation still
checks the declared capability, effective grant, and current user/domain
authorization. Other plugin instances continue to use their existing scope
rules.

Never use the ordinary authenticated Kandev HTTP API as an implicit escape
from the canvas protocol. The [same-origin trust decision](../../../decisions/2026-09-19-trusted-same-origin-canvases.md)
acknowledges that browser code can call those APIs, but the documented canvas
contract and capability-only runtime must remain coherent.

## Lifecycle and presentation

`canvas_lifecycle_metadata.task_id` and the instance's placement scope retain
task discovery and deletion semantics. Workspace data scope does not add the
canvas to workspace navigation. Promotion remains a reviewed human action:
it changes placement to workspace, clears task placement, preserves the active
release, and leaves workspace data scope intact. The review states that data
access is already workspace-wide when that is true. A task-only legacy canvas
still has its existing promotion behavior, including grant expansion.

Project `data_scope_kind` alongside `scope_kind` in the canvas and runtime
responses. The task host exposes a compact data-scope label in its existing
header/action surface. Releases and permissions and promotion review explain
placement separately from data access. The bundled authoring guidance and
public canvas documentation tell authors to read both context fields and to
declare only capabilities they use. Revise the saved `create-canvas` prompt's
promotion guidance so it describes workspace navigation rather than saying
that workspace data always requires promotion. No generated package rewrite
is needed.

## Existing canvas upgrade

Do not silently widen a retained release. An active task canvas with task-only
data scope offers **Enable workspace data** in the existing host action menu.
The backend returns a review snapshot of the active release, permission
digest, grant generation, current/target data scope, and exact declared
permissions. Only a currently authorized workspace owner may confirm it.

Confirmation rechecks the canvas's task and workspace identity, owner, active
release, permission digest, and grant generation inside one transaction. It
upserts workspace-ceiling grants only for that release's declared permissions,
sets `data_scope_kind=workspace`, increments grant generation, and publishes a
single lifecycle refresh after commit. It neither promotes the canvas nor
republishes the artifact. On stale review or any failed check it changes
nothing. A canceled review has no write. Imported or legacy packages receive
no automatic exemption.

Use the existing canvas HTTP controller's preview/confirm pattern with
`GET /api/v1/canvases/{id}/workspace-data-preview` and
`POST /api/v1/canvases/{id}/workspace-data`. The request carries the same
release ID, permission digest, and grant generation preconditions as promotion.
Return an updated canvas projection after success. Keep the action unavailable
for pending, archived, removed, already workspace-data, or foreign canvases.

## Desktop and phone host flow

Desktop keeps the current canvas panel and action toolbar. The data-scope
label sits with existing host metadata, and a legacy canvas exposes the upgrade
action from its current actions. Phone keeps the focused canvas route and
secondary-action drawer; the same upgrade opens the existing full-height
review surface. The header and actions remain fixed while one body region
scrolls, with safe-area clearance and touch targets. Both presentations share
one review state and backend snapshot; neither relies on canvas-authored UI.

## Failure and observability

Denied or stale access review returns the existing safe error style and leaves
the active release and old scope usable. A missing task or workspace makes
its runtime binding stale. Scope or grant changes invalidate old tokens; the
host reloads the runtime through its existing recovery path. Record bounded
scope-transition result counters and IDs already allowed for canvas lifecycle
logging. Do not log package source, permission bodies, tokens, or task data.

## Verification strategy

- Instance-store tests cover null fallback, invalid combinations, transaction
  rollback, snapshot/backup mapping, and grant-generation invalidation.
- Canvas service tests cover version-2 owner authority, exact declarations,
  legacy review, stale snapshots, owner drift, and promotion without data
  expansion.
- Web-app protocol tests cover list pagination, task read/write, workflow,
  message, event, dependency projection, foreign-workspace denial, and old
  task-scoped behavior.
- Desktop and mobile Playwright exercise an owner-created task canvas with at
  least two tasks, refresh before promotion, and an older canvas's explicit
  access upgrade. The mobile route and review remain touch-usable.

## Related designs

- [Canvas lifecycle and promotion](agent-authored-web-apps.md)
- [Owner-authorized first publication](local-creation-authority.md)
- [Plugin web-app runtime](../../plugins/system-design/isolated-web-app-contributions.md)
- [Plugin canvas data-scope runtime](../../plugins/system-design/canvas-workspace-data-scope.md)
