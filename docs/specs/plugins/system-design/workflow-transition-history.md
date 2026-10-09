---
id: plugins-workflow-transition-history-design
title: Workflow transition history for plugin and canvas reads
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-WORKFLOW-HISTORY-001
  - REQ-PLUGINS-WORKFLOW-HISTORY-002
  - REQ-PLUGINS-WORKFLOW-HISTORY-003
owners:
  - kandev
created: 2026-09-22
last_updated: 2026-09-22
---

# Workflow transition history for plugin and canvas reads System Design

## Purpose and boundaries

The Plugins system publishes a capability-gated read model for recorded task
moves. The [task transition ledger](../../tasks/system-design/workflow-task-step-transition-ledger.md)
remains the only source of historical facts. This design does not change its
writers or infer pre-activation history. The [canvas lifecycle](../../canvases/system-design/agent-authored-web-apps.md)
continues to own task-to-workspace promotion and grant review.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-WORKFLOW-HISTORY-001` | [Task trail read](#task-trail-read), [Authorization](#authorization) |
| `REQ-PLUGINS-WORKFLOW-HISTORY-002` | [Workflow route summary](#workflow-route-summary), [Authorization](#authorization) |
| `REQ-PLUGINS-WORKFLOW-HISTORY-003` | [Canvas view](#canvas-view), [Refresh and failure](#refresh-and-failure) |

## Components and responsibilities

1. The task repository adds bounded read queries over
   `task_step_transitions`. The task service exposes narrow methods and keeps
   task/workflow ownership checks above the repository.
2. The Plugin Host data adapter calls those task service methods. The typed
   Host gRPC API, SDK, and canvas JSON routes share the same DTO definitions
   and order. No plugin or canvas reads SQLite directly.
3. The existing issue #3583 canvas uses those browser routes. Its task view
   works before promotion; its workspace view becomes available only after the
   user promotes the same canvas and workspace grants cover its manifest.

## Task trail read

Add `ListTaskStepTransitions(task_id, page)` to the Host data API and
`GET ./_kandev/v1/data/tasks/{task_id}/step-transitions?limit=&cursor=` to the
canvas protocol. Both require `api_read:tasks`. The browser route first uses
the same task scope check as `getWebAppTask`; it never queries the ledger for a
task the binding cannot read.

The public item contains `id` as a decimal string, nullable
`from_workflow_id`, `from_workflow_step_id`, `to_workflow_id`, and
`to_workflow_step_id`, plus `trigger` and RFC3339 `occurred_at`. It omits
`actor_id`, `actor_kind`, and `session_id`. The nullable endpoints represent
creation, attachment, and detachment without invented steps. The browser
response uses the existing `{items,page_info}` envelope.

The repository reads `WHERE task_id = ? AND id < ? ORDER BY id DESC LIMIT
limit+1`, with the cursor omitted on the first page. `id` is the ledger's
monotonic identity, so equal timestamps and late writes do not reorder older
pages. The server treats the cursor as opaque, rejects malformed or foreign
cursor forms, and caps `limit` at 200. An empty page is valid. New moves
arriving during pagination appear on the next refresh, not in an older page.

## Workflow route summary

Add `ListWorkflowTransitionGroups(workflow_id, page)` to the Host data API and
`GET ./_kandev/v1/data/workflows/{workflow_id}/transition-groups?limit=&cursor=`
to the canvas protocol. Both require `api_read:tasks` and
`api_read:workflows`. The browser route is available only to a
workspace-scoped canvas whose workspace contains the workflow. The Host
adapter retains the gRPC plugin's existing instance-level workspace model.

The task repository groups retained rows joined to their task by task ID and
workspace ID. It includes archived tasks and excludes deleted tasks because
task deletion removes their ledger rows. Each group has `kind` (`within`,
`entry`, or `exit`), nullable `from_step_id` and `to_step_id`, and `count`.
`within` means both endpoints name the selected workflow. `entry` means only
the destination names it; `exit` means only the source names it. The other
workflow's identifiers are omitted from entry/exit groups. Removed step IDs
remain in the group. Groups have a stable `(kind, from_step_id, to_step_id)`
order and bounded pagination; counts are recomputed on each new overview
refresh. Add indexes for source- and destination-workflow grouping in both
SQLite and Postgres schema initialization, with a repository test for existing
stores.

The canvas gets unarchived current tasks from the existing paginated task list
filtered by `workflow_id` and derives step counts in memory. It does not use
route counts as current task counts. It fetches all task-list pages; if a read
fails, it marks the overview stale instead of presenting a partial count as
complete. The route summary is read separately, so concurrent moves can make
the two reads briefly disagree. The next refresh reconciles them.

## Authorization

Use the existing manifest resources. Per-task history needs
`api_read:tasks`; the workflow summary needs both `api_read:tasks` and
`api_read:workflows`. No new permission name or write capability is needed.
The canvas protocol checks effective grants, capability binding, and current
scope on every request. Out-of-scope tasks and workflows return no data. A
task-scoped canvas receives `plugin_permission_denied` for workflow-wide
groups even when its own task uses that workflow. Promotion remains an
explicit user action and may require workspace grant review.

The gRPC Host methods use the existing resource gates and service-layer reads.
The summary method checks both gates before calling the service. Host DTOs
are additive to protocol v1; generated proto code and SDK mappings remain in
sync. No arbitrary SQL or database path is exposed.

## Canvas view

The task canvas replaces the sample path in its primary view with the scoped
task trail. It keeps a clear `Recorded history` caption because ledger data
starts at its activation point. A deleted step is labeled `Removed step` with
its recorded identifier available on inspection. A move spanning workflows
is displayed but is not classified as forward or backward.

The workspace view adds a workflow picker, ordered steps, unarchived task
markers, grouped route counts, and selected task trail. The canvas determines
`return` only when both endpoints are known steps of the same workflow and
`to.position <= from.position` in the current step list. This is a positional
label, not a stored rejection verdict; a step reorder can change the label.
The history remains unchanged. Audio is off by default and unlocked by an
explicit `Enable sound` control after a user gesture.

Desktop places workflow steps beside the selected task trail. Phone uses a
single focused view with a visible workflow/task selector and `Steps` / `Trail`
controls; it does not squeeze the desktop columns or require horizontal page
scrolling. The closest Kandev phone precedent is `mobile-column-tabs.tsx`:
one selected step at a time, a visible navigator, and at least 44 px touch
targets. The canvas has one document scroll owner and safe-area bottom space.
Its standalone copy uses a bundled `t()` catalog for supported product
languages because the host does not inject i18next into the iframe.

## Refresh and failure

The canvas polls the selected task every five seconds and the workspace
overview every fifteen seconds while visible. It refreshes the trail after an
observed step change and offers a manual refresh. It compares steps only after
the first successful task read, so initial load and reconnect never ring.
Polling stops when the frame is hidden or removed. A 401 stale runtime token
or 403 grant denial is shown as an authority error. A 404 from the new history
route on an older host shows an unsupported-host state while the existing
current-task read remains usable; publication must not break the active task
canvas before the host is upgraded. Transient read failures show retry and
never relabel cached data as current. Empty history, empty workflow, and no
active tasks have separate messages.

## Persistence and observability

No new domain rows or writes are introduced. Only read indexes are added.
Task-scoped canvas publication remains an immutable release; the existing
release stays active if a later publish is rejected. The app stores no copy
of task history in canvas state. Host route tests measure paging and scope
denials; query tests cover SQLite and Postgres parity. Existing bounded JSON
responses, safe error codes, and request logs apply without a new metric label
containing task or workflow IDs.

## Related decisions

- [ADR 0043: capability-gated Host data API](../../../decisions/0043-plugin-host-data-api.md)
- [Plugin-backed web-app canvases](../../../decisions/2026-08-26-plugin-backed-web-app-canvases.md)

## Implementation plan

- [Live workflow transition canvas](../../../plans/workflow-transition-canvas/plan.md)
