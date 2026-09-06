---
id: canvases-agent-authored-web-apps-design
title: Agent-authored web-app canvases system design
status: draft
system: canvases
owners:
  - canvases
created: 2026-08-26
last_updated: 2026-09-05
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-002
  - REQ-CANVASES-AGENT-WEB-APPS-003
  - REQ-CANVASES-AGENT-WEB-APPS-004
  - REQ-CANVASES-AGENT-WEB-APPS-005
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-007
  - REQ-CANVASES-AGENT-WEB-APPS-008
  - REQ-CANVASES-AGENT-WEB-APPS-009
---

# Agent-authored web-app canvases system design

## Purpose and boundaries

The Canvases system owns the lifecycle that turns an agent-authored task
application into a workspace application. It owns task creation context,
promotion, release selection, editing sessions, discovery, and canvas host
surfaces.

The Plugins system owns package validation, iframe isolation, data access,
state, events, grants, and runtime tokens. This design uses
[the isolated plugin web-application contract](../../plugins/system-design/isolated-web-app-contributions.md).

This design supersedes
[the declarative canvas design](collaborative-canvases.md).

## Requirement mapping

| Requirement                       | Design sections                        |
| --------------------------------- | -------------------------------------- |
| `REQ-CANVASES-AGENT-WEB-APPS-001` | Agent authoring lifecycle, Agent tools |
| `REQ-CANVASES-AGENT-WEB-APPS-002` | Canvas model, Publish and rollback     |
| `REQ-CANVASES-AGENT-WEB-APPS-003` | Promotion and permissions              |
| `REQ-CANVASES-AGENT-WEB-APPS-004` | Quick Chat editing                     |
| `REQ-CANVASES-AGENT-WEB-APPS-005` | Desktop information architecture       |
| `REQ-CANVASES-AGENT-WEB-APPS-006` | Mobile design contract                 |
| `REQ-CANVASES-AGENT-WEB-APPS-007` | Host state and recovery, Observability |
| `REQ-CANVASES-AGENT-WEB-APPS-008` | Authoring admission and limits         |
| `REQ-CANVASES-AGENT-WEB-APPS-009` | Guided canvas task launch              |

## System relationship

```text
Task or Quick Chat agent
        |
        | create, inspect, publish
        v
Canvas lifecycle service
        |
        | owns scope and source lineage
        v
Scoped plugin instance --------------+
        |                             |
        | active immutable release    | effective grants
        v                             v
Sandboxed web application <---- Plugin web runtime
        |                             |
        | HTTP + Server-Sent Events   | Host data and state services
        +-----------------------------+
                                      |
                                      v
                           Tasks, workflows, and events
```

Canvas code does not call a canvas-specific JavaScript object. It uses the
standard web protocol that the Plugins system owns.

## Implementation baseline and feature gate

Implementation starts from current `main`. PR #3061 contains the superseded
declarative canvas implementation and must not merge as a prerequisite. The
new implementation creates the canvas model and host surfaces directly from
this design.

The runtime flag identity is `features.canvases`. Its environment variable is
`KANDEV_FEATURES_CANVASES`. The first implementation sets `prod`, `dev`, and
`e2e` profile defaults to `false`. Focused development and end-to-end tests set
the environment variable explicitly.

The flag requires a restart because it changes MCP tool registration and
backend composition. When the flag is off, Kandev does not register backend
canvas entry points. These entry points include MCP, HTTP, WebSocket, SSE,
background workers, and boot data. The frontend does not register canvas
routes, navigation, or settings surfaces. Database migrations can run while
the feature is off. No canvas operation can read or change canvas data.

## Canvas model

Use a small lifecycle record instead of declarative canvas blocks.

### `canvases`

- `id`
- `plugin_instance_id`
- `workspace_id`
- nullable `task_id` while task-scoped
- nullable `origin_task_id` for promoted-canvas provenance
- `title`
- `created_by_session_id`
- nullable `promoted_by_user_id`
- nullable `promoted_at`
- nullable `archived_at`
- `created_at` and `updated_at`

The plugin instance owns `scope_kind`, active release, status, and grants. The
canvas service is the only service that changes the task or workspace scope of
a canvas instance.

The canvas and plugin instance have a one-to-one relationship. Canvas removal
removes the plugin instance, runtime tokens, grants, state, pending releases,
and retained artifacts through the plugin lifecycle service.

Task removal removes each task canvas in the same cleanup flow. A promoted
canvas has no current `task_id`, so task cleanup preserves it. The
`origin_task_id` can become null after task removal.

Workspace removal removes task and promoted canvases, plugin instances,
grants, state, pending releases, and runtime capabilities. The workspace delete
transaction records artifact-cleanup jobs before it removes release ownership.
The plugin cleanup worker removes retained files after commit and retries after
restart.

## Canvas state machine

```text
draft
  | first valid release
  v
task_active ---- archive ----> archived
  | promote                     |
  v                             | restore
workspace_active <--------------+
  |
  +---- edit draft ---- publish ----> workspace_active
  |
  +---- remove ----------------------> removed
```

A pending release does not change the canvas state. It becomes active after a
permission approval or disappears after replacement or removal.

There is no workspace-to-task demotion in the first delivery.

## Authoring admission and limits

The canvas service enforces these initial limits before it creates or restores
a canvas:

- 10 non-removed task canvases for one task
- 100 non-removed canvases for one workspace across task and workspace scopes

Archived canvases count toward both limits. Create and restore use the same
repository transaction and lock as the count. Concurrent operations cannot
admit more than the limit.

The publish service permits 10 attempts for one agent session in a rolling
five-minute window. One canvas can have only one publish in progress. Publish
admission reserves plugin artifact bytes before source transfer and validation.
The plugin runtime also enforces its workspace and installation storage limits.

A limit error returns a stable safe code. It does not change the active
release, grants, state, archive status, or retained release set.

## Agent authoring lifecycle

The normal flow starts in a task conversation:

1. The user asks the task agent to create a canvas.
2. The agent calls `create_canvas_kandev` with a title and application summary.
3. The backend derives the task, workspace, session, and user from MCP context.
4. The service creates a task-scoped canvas and inactive plugin instance.
5. The tool returns the canvas ID, source directory, manifest scaffold,
   current permission ceiling, and the command to read the bundled authoring
   skill.
6. The agent reads the version-matched authoring skill through Canvas MCP.
7. The agent writes source and built assets under the returned directory.
8. The agent calls `publish_canvas_kandev` with the canvas ID and source path.
9. Kandev reads that directory through the trusted task execution file API.
10. The Plugins service validates and stores an immutable release.
11. The canvas activates immediately when its permissions fit existing grants.
12. The task client receives a lifecycle event and opens the new canvas.

The source directory is workspace-relative and scoped to the current execution.
The publish service rejects traversal, links, another canvas identity, and a
directory outside the assigned canvas root.

The backend must use the execution file API for local, Docker, and remote
executors. It must not assume that the agent workspace exists on the Kandev
host filesystem.

Agentctl adds one authenticated streaming tar endpoint for a workspace-relative
root. The canvas service supplies only the assigned canvas source root. The
endpoint rejects absolute paths, traversal, links, devices, and files outside
that root. It permits at most 512 files, 25 MiB of file data, and 30 MiB on the
wire. Cancellation stops traversal and closes the stream. The backend sends the
stream directly to the plugin validator and does not make one JSON request for
each file.

## Guided canvas task launch

The desktop sidebar and workspace Canvases settings page use one shared canvas
task preset. The preset opens the standard `TaskCreateDialog`. It does not
create canvas metadata or add a canvas-only form.

The preset supplies:

- a localized task title and canvas-authoring prompt
- repository-free source mode with an empty scratch path
- a preference for an eligible local executor profile
- the selected workspace

The normal dialog continues to own workflow, workflow step, agent profile, and
executor compatibility. The workflow and agent profile remain editable. The
executor preference uses capability-based selection and never stores a profile
identifier in the preset.

Successful task creation follows the normal task route. The user continues the
conversation there, and the task agent uses the authoring lifecycle. The same
full-screen task dialog serves the workspace settings action on a phone. The
desktop sidebar does not exist at that viewport.

All launch surfaces remain behind `features.canvases`. A disabled client does
not request canvas counts, add a settings tab, or register the task preset.

## Agent authoring guidance

Kandev bundles a read-only `kandev-canvas-authoring` system skill in a canvas
embed that is separate from the Office skill embed. At startup, Kandev writes
the current skill and its supporting files to
`<kandev-home>/system-skills/kandev-canvas-authoring/`. A Kandev update replaces
these Kandev-owned files with the version from the new binary. It does not
change user-owned skills.

Kanban task agents do not receive this skill in `.agents/skills`,
`.claude/skills`, or another task-workspace skill directory. The Canvas MCP
server reads the canonical files from the Kandev home directory and returns
them through `read_canvas_authoring_skill_kandev`. A call without a path returns
one compact core bundle. The bundle contains the complete normal workflow,
manifest contract, browser protocol summary, appearance rules, minimal
scaffold, and exact supporting-file inventory. The optional `path` input reads
one detailed reference from that inventory.

The handler rejects absolute paths, traversal, links, and files outside the
inventory. It does not return a Kandev host path to the agent. This rule keeps
local, Docker, and remote executors on one contract without copying the skill
into the task workspace.

`create_canvas_kandev` materializes the version-matched minimal scaffold in the
assigned source directory. It returns the system-skill slug, version, exact
scaffold inventory, and an instruction to make at most one core skill-read
call. Canvas edit Quick Chat prompts give the same instruction. After the core
read, agents use their native file tools for the scaffold and request a
detailed reference only when the core bundle directs them to it.

Office role and operating skills keep their existing per-task workspace
deployment. The canvas-authoring read path does not change Office skill
selection, injection, or synchronization. Office skill discovery and
`SyncSystemSkills` do not inspect the canvas system-skill directory.

The canvas-authoring skill tells the agent to:

- use the generated manifest and source directory
- bundle executable frontend dependencies
- add a responsive viewport declaration
- use relative `./_kandev/v1` data, state, action, and event paths
- keep Kandev domain data as the source of truth
- store only application-specific shared state in instance state
- use memory for temporary values because opaque-origin browser storage is not
  available
- publish after local build checks
- read validation diagnostics and correct rejected releases
- use semantic appearance variables and apply live host appearance messages

Supporting files include the browser API, manifest, data and state, events and
recovery, security, and UI pattern references. The scaffold includes a minimal
no-build HTML application and bundled Kandev-compatible canvas styles. It can
also include optional build examples without making Node a host-runtime
dependency.

## Agent tools

Replace the declarative block tools with these task-aware tools:

- `list_canvases_kandev`
- `read_canvas_authoring_skill_kandev`
- `create_canvas_kandev`
- `get_canvas_kandev`
- `publish_canvas_kandev`
- `get_canvas_state_kandev`
- `set_canvas_state_kandev`

Task agents can create and publish task canvases for their trusted task. An edit
Quick Chat session can publish only the canvas that started that session.

State tools derive the plugin instance from the canvas. They use the plugin
state service and revision preconditions. They do not accept another user,
workspace, task, session, or plugin instance.

Agents cannot promote, approve permissions, roll back, archive, restore, or
remove a canvas. Agents use existing Kandev tools to change tasks and workflows
that a canvas displays.

## Publish and rollback

The publish command resolves the source through trusted session context. It
streams a bounded package to the plugin release validator.

If the declaration does not exceed existing grants, the release activates in
one transaction. The service publishes `canvas.release.activated` after commit.

If the declaration needs more permission, the release gets
`pending_permission` status. The service publishes
`canvas.release.permission_required`. The active release remains unchanged.

Rollback selects the retained prior valid release. It does not restore old
grants that the user revoked. If the current grants do not cover the old
release, rollback needs a permission review.

## Promotion and permissions

Promotion is a human-only canvas operation. The backend authorizes the current
user for the canvas workspace before it returns promotion data.

The promotion dialog shows:

- the canvas title and origin task
- the active release and source actor
- each Kandev data read
- each Kandev data write
- each event subject
- shared state access
- each exact external network origin
- the change from task scope to workspace scope
- the new workspace navigation placement

The user can cancel without a state change. Confirmation creates the approved
workspace grants and changes the plugin instance scope in one transaction. The
canvas keeps its ID, plugin instance, active release, state, and release
history.

Promotion publishes one lifecycle event after commit. The task and workspace
navigation projections then refresh.

A release that adds permissions uses the same review component. Removing a
declaration does not need approval. The effective grant contracts to the new
declaration after activation.

## Quick Chat editing

The workspace canvas host has an Edit canvas action. It calls a canvas edit
endpoint instead of invoking a one-shot utility agent.

The endpoint performs these steps:

1. Authorize the canvas and workspace.
2. Create a normal Quick Chat session with `origin: canvas_edit` metadata.
3. Record the target canvas and active release in trusted session metadata.
4. Materialize the active release source in the Quick Chat workspace.
5. Open Quick Chat with a prompt that describes the requested edit workflow.
6. Restrict canvas publish tools to the target canvas.

The user describes changes in Quick Chat. The agent edits the materialized
source, builds static assets, and calls the normal publish tool. The active
release changes only after validation and permission checks succeed.

The Quick Chat workspace remains disposable. The immutable release store owns
published source. Quick Chat cleanup cannot remove an active or prior release.

## Runtime data and state

The iframe uses the plugin web protocol. For a task board, the expected data
flow is:

```text
Initial page load
  -> GET ./_kandev/v1/data/tasks
  -> GET ./_kandev/v1/data/workflows/{id}/steps
  -> read each task.workflow_step_id
  -> render tasks grouped by workflow and workflow step

User continues a task
  -> POST ./_kandev/v1/data/tasks/{id}/messages { text: "continue" }
  -> the shared message service queues, resumes, or starts the task session
  -> normal task and session events reach the iframe

User moves a card
  -> choose the next ordered workflow step
  -> PATCH ./_kandev/v1/data/tasks/{id} { workflow_step_id: nextStepId }
  -> task service updates the task
  -> task.state_changed event is published
  -> the iframe receives the event through Server-Sent Events

Agent moves a task
  -> existing Kandev task MCP tool
  -> task service updates the task
  -> the same event reaches the iframe
```

The task store remains the source of truth. Canvas instance state stores only
data that has no Kandev domain owner. Examples include saved board filters,
custom lane definitions, or application annotations.

The agent can read and change instance state with the canvas state tools. The
iframe uses the relative state routes. Both paths use the same revision and
conflict service.

## HTTP API

Add owner-authorized canvas endpoints for:

- list task canvases
- list workspace canvases
- get canvas metadata and host state
- request a runtime URL
- get promotion preview
- confirm promotion
- get release history
- approve a pending release
- roll back a release
- start an edit Quick Chat session
- archive, restore, and remove a workspace canvas

The first delivery has no manual create, source upload, package import, package
export, or demotion endpoint.

The publish path is agent-facing through MCP. It can use a narrow internal HTTP
or WebSocket transfer between the backend and agent execution layer. It is not
a browser upload route.

## WebSocket lifecycle events

Canvas metadata changes use the existing Kandev WebSocket gateway. Add these
owner-scoped events:

- `canvas.created`
- `canvas.release.activated`
- `canvas.release.permission_required`
- `canvas.promoted`
- `canvas.archived`
- `canvas.restored`
- `canvas.removed`

Events contain canvas identity, scope, host state, and release metadata. They
do not contain source files, HTML, JavaScript, state values, or runtime tokens.

Application data updates do not use this channel. The iframe receives its
declared events through the plugin Server-Sent Events protocol.

## Desktop information architecture

Keep the first-party Canvases section folded on first use. A direct canvas
route does not force it open. An explicit user toggle remains in the persisted
sidebar preference. The section lists active workspace canvases for the active
workspace. It does not list task canvases.

The section contains:

- a workspace canvas count
- one row for each active workspace canvas
- a settings shortcut
- an empty setup row that opens canvas guidance

The sidebar does not create canvases directly. The empty setup row opens the
workspace Canvases settings page, where the guided task launch is available.
There is no package import or blank canvas action in the sidebar.

Routes are:

- `/canvases/:canvasId` for the focused host surface
- `/settings/workspaces/:workspaceId/canvases` for workspace canvas management

The shared feature-aware workspace tab catalog includes Canvases. The settings
tree, tab strip, headings, and links derive from this catalog. The Canvases page
uses `WorkspaceSettingsShell`, shows active and archived workspace canvases,
and supports open, create through a task, edit, permissions, releases, archive,
restore, and remove. It does not import packages or create a blank canvas.

Wide workspace cards add an active-canvas count tile and include that count in
the resource total. Narrow cards retain the current readable tile widths and
omit the canvas tile. The settings tab remains available at every supported
width. Canvas count requests do not run while the feature is disabled.

The task workbench uses one generic canvas panel. The panel receives
`canvasId`, resolves the active release, and renders the shared web-application
host. Multiple task canvases use one picker and one panel type.

The task workbench watches committed canvas lifecycle invalidations. When the
active task receives its first activated or pending-permission release, it adds
and activates `canvas:<canvasId>`. It does not add a duplicate panel or take
focus for another task. A pending release opens the host recovery state instead
of an empty iframe. On a phone, the same event navigates to the focused canvas
route because Dockview is not mounted.

Host chrome stays outside the iframe. Desktop chrome shows the title, runtime
state, Edit, Promote for task canvases, Open full canvas, and an overflow menu.
Release, permission, promotion, and disabled controls have pointer and keyboard
help. The mobile action drawer shows equivalent descriptions without hover.

The Dockview canvas renderer provides a full-height flex boundary around the
persistent portal. The shared host and iframe then fill the area below host
chrome. This change does not alter the direct route or phone viewport formula.

## Mobile design contract

- **Outcome and entry:** A user opens task canvases from the task mobile
  navigation. A user opens workspace canvases from the workspace mobile menu.
- **Exemplars:** Use `task-layout.tsx` for full-height composition. Use
  `mobile-picker-sheet.tsx` for the canvas and action picker. Use
  `kanban-with-preview.tsx` for direct phone navigation.
- **Hierarchy:** Show the canvas title, runtime state, focused application, and
  one primary host action. Put secondary host actions in a visible overflow
  control.
- **Presentation:** Use a direct `h-dvh` route on phones. Do not mount Dockview
  or a desktop sidebar.
- **Navigation:** Use one focused canvas. Use an inset bottom drawer to choose
  another canvas or a host action.
- **Creation:** Keep Create canvas visible on workspace Canvases settings. Open
  the standard full-screen task dialog with the shared canvas preset.
- **Action help:** Show lifecycle action descriptions inside the action drawer.
  Do not require hover.
- **Geometry:** Give the iframe the remaining viewport after fixed host chrome.
  Use one host scroll owner, safe-area padding, and 44-pixel host controls.
- **Application responsibility:** The canvas package owns responsive layout
  inside the iframe. The scaffold and acceptance fixture must support a phone
  viewport without document-level horizontal overflow.

Desktop and mobile use the same canvas metadata, plugin instance, active
release, grants, runtime URL service, and lifecycle actions.

## Host state and recovery

The shared host component has these states:

- `loading_metadata`
- `pending_first_release`
- `pending_permission`
- `loading_runtime`
- `ready`
- `offline`
- `invalid_release`
- `unavailable`
- `archived`

The iframe can remain visible during an event-stream reconnect. The host shows
the offline state outside the application. If the runtime token expires, the
host requests a new URL and reloads the same active release.

If the release is unavailable, the host offers Edit, Releases, Roll back, or
Remove according to current permissions. It never loads a rejected artifact.

## Authorization

Every human endpoint uses current workspace authorization. A foreign and a
missing canvas return the same not-visible result.

Every agent operation derives user, task, workspace, and session from the MCP
connection. An edit Quick Chat session also verifies its trusted target canvas
metadata.

Task canvases cannot appear in another task. Workspace canvases can serve task
surfaces only inside their workspace. Promotion does not grant access to tasks,
repositories, sessions, or external services by itself.

The plugin runtime repeats resource authorization for each request. Canvas
authorization does not replace task or workspace authorization.

## Baseline from the superseded declarative work

The declarative implementation is not on `main` and is not a released
contract. PR #3061 must close or remain unmerged. Implementation does not merge
that block model before it creates the plugin-backed model.

Development databases that ran PR #3061 can keep unused declarative tables.
The new implementation does not read them. It does not add a destructive
startup migration for those development-only tables.

## Failure handling

- A missing task agent disables the create prompt and shows how to start one.
- A source read error preserves the active release.
- A package validation error returns safe file and rule diagnostics to the
  authoring agent.
- A permission increase creates a pending release.
- A promotion transaction either changes scope and grants together or changes
  neither.
- A Quick Chat launch error leaves releases unchanged.
- A runtime token error reloads through the host before it shows unavailable.
- A state conflict returns the current revision to the writer.
- A removed task removes unpromoted canvases through a retryable cleanup path.
- A removed workspace records artifact cleanup before it removes database
  ownership.
- A count, rate, or storage rejection preserves the active release.

## Observability

Add content-free metrics for:

- canvas create result
- source collection and publish result
- release activation and pending-permission result
- promotion result
- edit Quick Chat launch result
- runtime host state
- archive, restore, rollback, and removal result
- canvas count, publish-rate, and artifact-storage admission result

Logs can include canvas ID, plugin instance ID, release ID, workspace ID, task
ID, actor kind, operation, safe result, and duration. Logs omit canvas title,
prompt, source, application content, state values, package bodies, and runtime
tokens.

## Test strategy

- Canvas repository tests cover task cleanup, promoted preservation, scope
  transitions, and release references.
- Service tests cover trusted agent context, publish authorization, promotion,
  permissions, rollback, edit sessions, workspace removal, and limits.
- Execution integration tests cover source collection from standalone, Docker,
  and remote streaming archive readers.
- MCP tests cover tool inventory, trusted context, assigned source roots, state
  conflicts, and forbidden lifecycle actions.
- Gateway tests cover lifecycle event scope and content limits.
- Frontend tests cover sidebar filtering, task picker, host state, promotion,
  permission review, release history, Quick Chat launch, task presets,
  workspace tab catalogs, card breakpoints, action help, and panel
  deduplication.
- Desktop Playwright covers agent creation, live task data, promotion, sidebar
  discovery, task-based creation, automatic panel opening, host geometry,
  editing, a pending permission, activation, and rollback.
- Mobile Playwright covers task entry, workspace navigation, the focused route,
  task-based creation, bottom drawer help, 44-pixel host controls, safe areas,
  and no host overflow.
- A test web application covers HTML, CSS, JavaScript, state, task reads,
  task writes, Server-Sent Events, appearance changes, desktop layout, and phone
  layout.

## Related decisions

- [Plugin-backed web-app canvases](../../../decisions/2026-08-26-plugin-backed-web-app-canvases.md)
- [Plugin agent tools through Kandev MCP](../../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md)
- [Host utility agentctl for sessionless flows](../../../decisions/0002-host-utility-agentctl-for-sessionless-flows.md)
- [Superseded declarative canvases](../../../decisions/2026-08-25-server-owned-declarative-canvases.md)
