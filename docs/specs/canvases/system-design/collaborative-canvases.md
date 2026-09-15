---
id: canvases-collaborative-canvases-design
title: Collaborative canvases system design
status: superseded
system: canvases
owners:
  - canvases
created: 2026-08-25
last_updated: 2026-08-30
requirements:
  - REQ-CANVASES-COLLABORATIVE-CANVASES-001
  - REQ-CANVASES-COLLABORATIVE-CANVASES-002
  - REQ-CANVASES-COLLABORATIVE-CANVASES-003
  - REQ-CANVASES-COLLABORATIVE-CANVASES-004
  - REQ-CANVASES-COLLABORATIVE-CANVASES-005
  - REQ-CANVASES-COLLABORATIVE-CANVASES-006
  - REQ-CANVASES-COLLABORATIVE-CANVASES-007
  - REQ-CANVASES-COLLABORATIVE-CANVASES-008
  - REQ-CANVASES-COLLABORATIVE-CANVASES-009
  - REQ-CANVASES-COLLABORATIVE-CANVASES-010
  - REQ-CANVASES-COLLABORATIVE-CANVASES-011
---

# Collaborative canvases system design

This design is not the current canvas design. It is replaced by
[Agent-authored web-app canvases](agent-authored-web-apps.md).

Its blank native-canvas dialog, import flow, and prohibition on sidebar
creation actions are historical. They do not constrain the replacement design.

## Design summary

Kandev stores each canvas in one workspace as an owner-only snapshot. A canvas
can link to several tasks in that workspace and appear in each task's Dockview.

Human controls and agent tools call one typed command service. The web client
shows a closed schema with native components. It never executes canvas code.

Version 1 transfers canvases through a single inert JSON file. An import always
creates an independent canvas. Version 1 has no user sharing, invitations,
collaborator roles, presence, or live instance federation.

This design follows
[the server-owned declarative canvas decision](../../../decisions/2026-08-25-server-owned-declarative-canvases.md).

## System boundary

Add `apps/backend/internal/canvas/` as the owner of canvas storage, workspace
scope, task links, actions, events, leases, export, and import. Other systems
can store a canvas identifier, but they do not store canvas blocks.

The initial adapter boundary is:

```text
HTTP handlers ---------+
WebSocket gateway -----+--> canvas access + command service --> repository
Kandev MCP tools ------+                |
                                        +--> export/import codec
                                        +--> ordered owner events
```

The owner client receives agent changes through the WebSocket gateway. No
other user can subscribe to the canvas.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-001,
REQ-CANVASES-COLLABORATIVE-CANVASES-003,
REQ-CANVASES-COLLABORATIVE-CANVASES-005, and
REQ-CANVASES-COLLABORATIVE-CANVASES-008.

## Persistence

Use five canvas-owned tables.

### `canvases`

- `id`
- `owner_user_id`
- `workspace_id`
- `title`
- `schema_version`
- `revision`
- `compacted_through_revision`
- nullable `source_export_id`
- nullable `imported_at`
- nullable `archived_at`
- `created_at` and `updated_at`

### `canvas_task_links`

- `canvas_id`
- `task_id`
- `linked_by_user_id`
- `created_at`

The primary key is `(canvas_id, task_id)`. A service check requires both records
to use the same workspace. Task removal cascades only to this table. The canvas
remains.

### `canvas_blocks`

- `canvas_id`
- `block_id`
- `block_type`
- `position`
- `state_json`
- `block_revision`
- timestamps

The primary key is `(canvas_id, block_id)`. A stable sortable position avoids a
full rewrite after one block move.

### `canvas_events`

- `canvas_id`
- `revision`
- unique `command_id`
- `actor_kind` as `human` or `agent`
- nullable `actor_user_id`
- nullable `actor_session_id`
- `action`
- nullable `target_block_id`
- bounded `payload_json`
- `created_at`

The primary key is `(canvas_id, revision)`.

### `canvas_command_receipts`

- `command_id`
- `canvas_id`
- normalized result
- resulting revision
- `created_at`

Receipts preserve idempotency after event compaction. Canvas removal deletes
all canvas-owned rows. An import creates one canvas and its blocks in one
transaction. It does not create events from the source file.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-001,
REQ-CANVASES-COLLABORATIVE-CANVASES-003,
REQ-CANVASES-COLLABORATIVE-CANVASES-006, and
REQ-CANVASES-COLLABORATIVE-CANVASES-011.

## Block and action schema

The backend owns one versioned registry.

| Block     | State                               | Main actions                    |
| --------- | ----------------------------------- | ------------------------------- |
| Markdown  | sanitized Markdown source           | replace content                 |
| Checklist | ordered items with stable IDs       | add, edit, toggle, move, remove |
| Kanban    | columns and cards with stable IDs   | add, edit, move, remove         |
| Metrics   | named values with units and status  | set, remove, reorder            |
| Timeline  | ordered events with time and status | add, edit, move, remove         |

The registry validates all state before a write or import transaction. Version
1 uses these limits:

- 100 active canvases per owner
- 50 blocks per canvas
- 500 structured items per canvas
- 256 KiB of current block state per canvas
- 32 KiB per command input
- 512 KiB per `.kandev-canvas` file
- 200 characters per title or item label
- 1,000 retained events after compaction

One typed backend contract owns these limits. HTTP, WebSocket, MCP, export, and
import tests use the same values.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-002,
REQ-CANVASES-COLLABORATIVE-CANVASES-006,
REQ-CANVASES-COLLABORATIVE-CANVASES-010, and
REQ-CANVASES-COLLABORATIVE-CANVASES-011.

## Command transaction

The canonical command is:

```json
{
  "command_id": "uuid",
  "canvas_id": "uuid",
  "base_revision": 41,
  "action": "checklist.toggle",
  "target_block_id": "uuid",
  "input": { "item_id": "uuid", "completed": true }
}
```

The service does these steps in one transaction:

1. Load the canvas and trusted actor context.
2. Authorize the owner, task agent, or Office agent.
3. Return the stored receipt for a duplicate command.
4. Validate the action, input, limits, and preconditions.
5. Apply the typed change to the block snapshot.
6. Increment the canvas and block revisions.
7. Append the attributed event and command receipt.
8. Commit, then publish to the owner's authorized subscribers.

A failed transaction publishes nothing.

Structured actions contain item preconditions. A stale canvas revision is
valid when the target item still matches. Otherwise, the service returns
`canvas_conflict` with the current block and revisions.

A Markdown replacement requires the current block revision and a lease. The
bounded lease manager keys each lease by canvas, block, and actor. A lease lasts
30 seconds and can renew. A restart removes all leases safely.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-003 and
REQ-CANVASES-COLLABORATIVE-CANVASES-004.

## Portable file contract

Version 1 uses one UTF-8 JSON file with the `.kandev-canvas` extension. It is
not a ZIP file because all version 1 blocks contain bounded structured data.

The top-level contract is:

```json
{
  "format": "kandev.canvas",
  "format_version": 1,
  "export_id": "uuid",
  "exported_at": "2026-08-26T12:00:00Z",
  "canvas": {
    "title": "Release readiness",
    "schema_version": 1,
    "blocks": []
  }
}
```

The codec emits fields in a deterministic order. The file contains no internal
canvas ID, block ID, user ID, task link, event, actor, session, server address,
repository data, file content, or credential. Each exported block uses a
portable position and typed state without its internal identifier.

The import codec must parse with unknown-field rejection. It validates the
format version, schema version, block registry, text encoding, and all limits.
It completes validation before it starts the transaction.

The importer creates new canvas and block identifiers. It records `export_id`
as `source_export_id` for local provenance. It assigns the current user and
selected workspace. The user can select a same-workspace task link before the
transaction.

Each import creates a new fork, even when `source_export_id` already exists.
There is no update, merge, refresh, or synchronization action in version 1.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-006 and
REQ-CANVASES-COLLABORATIVE-CANVASES-010.

## HTTP API

Add `/api/v1/canvases` endpoints for:

- workspace-scoped list, create, fetch, rename, archive, restore, and remove
- task link list, create, and remove
- block list and initial snapshot
- Markdown lease acquire, renew, and release
- `GET /api/v1/canvases/{id}/export`
- `POST /api/v1/canvases/import` with a bounded file and optional task ID

The export response uses a safe file name and
`application/vnd.kandev.canvas+json`. The import response returns the new
canvas ID and direct route. Both endpoints require the current owner identity.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-001,
REQ-CANVASES-COLLABORATIVE-CANVASES-006,
REQ-CANVASES-COLLABORATIVE-CANVASES-007, and
REQ-CANVASES-COLLABORATIVE-CANVASES-010.

## Access policy

Version 1 has one canvas role: owner.

- The authenticated workspace owner can read, change, export, link, archive,
  and remove the canvas.
- The synthetic user has the same capability when authentication is disabled.
- A task agent can read or change only canvases linked to its trusted task.
- An Office agent can use canvases in its trusted workspace context.
- Another Kandev user has no canvas access.
- Import creates a new canvas in the selected owned workspace.

The canvas service uses the task system's access decision for every task-link
operation. Canvas ownership never grants access to a linked task or repository.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-007 and
REQ-CANVASES-COLLABORATIVE-CANVASES-008.

## WebSocket protocol

Extend the gateway with a canvas subscription map and an authorization callback.
Do not route canvas traffic through workspace broadcasts.

Client actions:

- `canvas.subscribe` with `canvas_id` and `after_revision`
- `canvas.unsubscribe`
- `canvas.command`

Server events:

- `canvas.snapshot`
- `canvas.event`
- `canvas.lease`
- `canvas.error` with a stable code and safe recovery data

Subscription authorization requires the current owner. Every publish path
repeats the access decision as a backstop. Disconnect removes leases owned by
that socket. Version 1 has no presence or focus messages.

The server replays events after `after_revision` when the event range exists.
Otherwise, it sends a snapshot. Clients apply only the next revision and
request recovery after a gap.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-005 and
REQ-CANVASES-COLLABORATIVE-CANVASES-011.

## Agent tools

Add a `canvas` MCP profile group for Task and Office task surfaces:

- `list_canvases_kandev`
- `create_canvas_kandev`
- `get_canvas_kandev`
- `apply_canvas_action_kandev`

The MCP server derives the actor from its backend connection. Tool schemas do
not expose another actor, export, import, task unlink, canvas removal, arbitrary
URL, or raw code input.

Task sessions list only canvases linked to their task. A task-created canvas
links to that task. Office sessions list canvases for their trusted workspace.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-007 and
REQ-CANVASES-COLLABORATIVE-CANVASES-008.

## Desktop information architecture

Add Canvases as a first-party workspace sidebar section. Model it on
`AutomationsSection` and `IntegrationsSection`. The section starts folded,
shows the workspace canvas count, and fetches rows only for the active
workspace.

The section header toggles the rows. A persistent header shortcut opens the
workspace Canvases settings page. Each row opens the canvas direct route. The
sidebar contains no creation or import actions.

An empty section shows Set up a canvas. This row opens the workspace Canvases
settings page.

Routes:

- `/settings/workspaces/:workspaceId/canvases` manages workspace canvases
- `/canvases/:canvasId` shows the focused canvas

Add `canvases` to `WORKSPACE_SETTINGS_TABS`. The settings tree and workspace
tab strip derive the row from that catalog. The workspace page follows the
Automations list page and Integrations index page patterns.

The task surface registers a first-party `canvas` Dockview component. Opening a
linked canvas reuses the same canvas surface and store as the direct route.

### Sidebar preview

```text
+----------------------------+
| KANDEV                     |
|                            |
| [ ] Home                   |
| [ ] Tasks                  |
|                            |
| [#] Canvases v      3  [=] |
|     Release readiness      |
|     Incident 482           |
|     API migration          |
|                            |
| [ ] Stats                  |
| [ ] Settings               |
+----------------------------+
```

The number is the workspace canvas count. The `[=]` header shortcut opens
workspace settings. The expanded list uses the Automations row density. Each
row opens the direct route.

### Workspace settings preview

```text
+--------------------------------------------------------------+
| Workspace settings > Main workspace > Canvases              |
+--------------------------------------------------------------+
| Canvases                                                     |
| Create, import, and manage canvases for this workspace.      |
|                                      [Import] [New canvas]    |
+--------------------------------------------------------------+
| Search [_______________________________________________]      |
|                                                              |
| Name                 Linked tasks     Updated        Actions |
| Release readiness    Prepare release  4 minutes ago     [...] |
| Incident 482         2 tasks          Yesterday         [...] |
| API migration        No tasks         12 Aug            [...] |
+--------------------------------------------------------------+
```

The page owns creation and import. Row actions contain Open, Export, Archive,
and Remove. The table becomes a vertical card list on phones.

### New canvas preview

```text
+------------------------------------------------+
| New canvas                                  [x] |
|                                                |
| Title                                          |
| [ Release readiness                         ]  |
|                                                |
| Link to task (optional)                        |
| [ No task                                   v] |
|                                                |
| The canvas also appears in the Canvases list.  |
|                                                |
|                    [Cancel] [Create canvas]     |
+------------------------------------------------+
```

The workspace settings action leaves the task option empty. A task Dockview
action selects the current task by default. The dialog creates a blank canvas.
Users add the first block after creation. Version 1 does not include templates.

### Dockview panel preview

```text
+---------------- Task workbench -------------------------------+
| [Agent] [Changes] [Files] [Canvas: Release readiness]      [x] |
+---------------------------------------------------------------+
| Release readiness             Saved at 12:44   [Open] [Export] |
| Linked task: Prepare release 0.52                    [...]     |
+----------------+----------------------------------------------+
| Blocks         | Checklist: Release gates             [+ Add] |
|                |                                              |
| > Release gates| [x] Backend tests                            |
|   Metrics      | [x] Web typecheck                            |
|   Timeline     | [ ] Desktop package                          |
|                | [ ] Release notes                            |
| + Add block    |                                              |
|                | Agent updated this block at 12:43            |
+----------------+----------------------------------------------+
```

The panel header contains Open full canvas, Export, and an overflow menu. The
overflow menu contains Rename, task links, Archive, and Remove. The canvas
body owns internal scrolling.

## Import interaction

The Import action on the workspace Canvases settings page opens a file picker
for `.kandev-canvas`. Kandev parses the file before it shows the preview.

The preview shows title, block count, block types, format version, file size,
and an optional task link. It does not render block content before full
validation. The primary action is Import as new canvas.

A successful import opens the new direct route. The interface states that the
new canvas is an independent copy with no synchronization.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-006 and
REQ-CANVASES-COLLABORATIVE-CANVASES-009.

## Mobile design contract

- **Outcome and entry:** users open the workspace Canvases settings page from
  mobile settings navigation. A task can open the focused canvas route.
- **Exemplars:** use `task-layout.tsx` for full-height ownership. Use
  `mobile-picker-sheet.tsx` for the block and action drawer. Use
  `kanban-with-preview.tsx` for direct detail navigation.
- **Hierarchy:** show the canvas title, save state, focused block, and primary
  block action. Put secondary canvas actions in an overflow control.
- **Surface:** use a direct `h-dvh` route. Do not mount Dockview on a phone.
- **Navigation:** use vertical cards on the workspace page. Keep New canvas and
  Import visible as 44-pixel actions. Use an inset drawer for editor blocks,
  task links, export, archive, and remove.
- **Geometry:** use 44 by 44 CSS pixel action targets and safe-area padding.
  Use one internal vertical scroll owner. Prevent page-level horizontal scroll.
- **Dense content:** show one Kanban column at a time. Wrap metrics. Keep
  checklist and timeline content vertical.

Desktop and mobile use the same API, store, command hooks, file codec responses,
and access decisions. All user copy uses translation keys in every required
locale.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-009.

## Compaction and recovery

After a command leaves more than 1,000 events, a bounded operation removes the
oldest events and advances `compacted_through_revision`. The block rows already
contain the current snapshot.

A client older than the compaction revision receives the full snapshot. A
client inside the retained range receives ordered events. Command receipts
preserve idempotency after event removal.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-003,
REQ-CANVASES-COLLABORATIVE-CANVASES-005, and
REQ-CANVASES-COLLABORATIVE-CANVASES-011.

## Observability

Add content-free metrics for:

- command result
- event or snapshot recovery
- active owner subscriptions
- lease result
- export, import, and rejected-import results

Logs can include canvas ID, command ID, action, revision, actor kind, format
version, file size, and a safe error code. They must omit titles, block state,
Markdown, task data, file content, and user-supplied file names.

This maps REQ-CANVASES-COLLABORATIVE-CANVASES-010 and
REQ-CANVASES-COLLABORATIVE-CANVASES-011.

## Failure handling

- Duplicate commands return the stored receipt.
- Conflicts return the current target state and revisions.
- A busy Markdown block returns safe lease state.
- A lost connection leaves local state visible and marks it offline.
- Editing remains disabled until recovery is complete.
- A non-owner request returns one generic forbidden error.
- An invalid import returns one safe reason and creates no rows.
- An unknown schema version shows an unsupported-state screen.
- Export failure does not change canvas state.

## Test strategy

- Repository tests cover migrations, task links, removal, compaction, receipts,
  and database replay.
- Service tests cover block actions, limits, owner access, task-agent access,
  conflicts, leases, export, and atomic import.
- Codec tests cover deterministic output, excluded fields, new identifiers,
  repeated import, unknown fields, versions, and limits.
- Gateway tests cover owner authorization, event order, replay, snapshots, and
  lease cleanup.
- MCP tests cover trusted actor context and the absence of portability tools.
- Frontend tests cover the folded sidebar, settings catalog, workspace page,
  creation, import preview, recovery, conflicts, Dockview, and mobile layout.
- Desktop Playwright covers workspace settings, creation, task links, Dockview,
  agent updates, export, import, reload, restart, and archive behavior.
- Mobile Playwright covers navigation, creation, import, block selection,
  export, 44-pixel targets, safe areas, and zero horizontal overflow.

## Requirement traceability

| Requirement                             | Main design sections                                                         |
| --------------------------------------- | ---------------------------------------------------------------------------- |
| REQ-CANVASES-COLLABORATIVE-CANVASES-001 | Persistence, HTTP API, Desktop information architecture                      |
| REQ-CANVASES-COLLABORATIVE-CANVASES-002 | Block and action schema                                                      |
| REQ-CANVASES-COLLABORATIVE-CANVASES-003 | Persistence, Command transaction, Compaction and recovery                    |
| REQ-CANVASES-COLLABORATIVE-CANVASES-004 | Command transaction, Failure handling                                        |
| REQ-CANVASES-COLLABORATIVE-CANVASES-005 | WebSocket protocol, Compaction and recovery                                  |
| REQ-CANVASES-COLLABORATIVE-CANVASES-006 | Portable file contract, HTTP API, Import interaction                         |
| REQ-CANVASES-COLLABORATIVE-CANVASES-007 | Access policy, Agent tools                                                   |
| REQ-CANVASES-COLLABORATIVE-CANVASES-008 | Access policy, Agent tools                                                   |
| REQ-CANVASES-COLLABORATIVE-CANVASES-009 | Desktop information architecture, Import interaction, Mobile design contract |
| REQ-CANVASES-COLLABORATIVE-CANVASES-010 | Block and action schema, Portable file contract, Observability               |
| REQ-CANVASES-COLLABORATIVE-CANVASES-011 | Persistence, WebSocket protocol, Compaction and recovery, Observability      |
