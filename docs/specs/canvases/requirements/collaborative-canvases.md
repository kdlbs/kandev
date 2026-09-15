---
id: canvases-collaborative-canvases
title: Collaborative canvases
status: deprecated
system: canvases
owners:
  - canvases
created: 2026-08-25
last_updated: 2026-08-30
---

# Collaborative canvases

This requirement is not the current canvas contract. It is replaced by
[Agent-authored web-app canvases](agent-authored-web-apps.md).

Its native-block creation, import, and sidebar-action rules do not apply to the
current product. The replacement contract uses a normal task and an agent to
create a canvas application.

## Summary

A canvas is a durable work surface in one workspace. The workspace owner and
trusted agents use it. The canvas can link to tasks, but task state does not own
canvas state.

Version 1 uses native blocks and actions. A portable file transfers a canvas
snapshot between users or Kandev instances. Version 1 has no live sharing
between users.

## Goals

- Give a user and trusted agents one persistent work artifact.
- Support Markdown, checklists, Kanban boards, metrics, and timelines.
- Show task-linked canvases in a Dockview panel.
- Show workspace canvases in a first-party sidebar section.
- Manage canvases on a dedicated workspace settings page.
- Transfer a safe canvas snapshot through export and import.
- Preserve native desktop and mobile interaction paths.

## Non-goals

- Viewer, editor, guest, or collaborator roles.
- Canvas invitations or shared links.
- Live collaboration between Kandev users.
- Live synchronization or federation between Kandev instances.
- An import that updates or merges with an existing canvas.
- A general application or dashboard builder.
- Arbitrary iframe, page, script, or plugin execution.
- Reusable templates or plugin block types in version 1.
- Access to linked task, workspace, repository, file, or secret content through
  an export.

## Requirements

### REQ-CANVASES-COLLABORATIVE-CANVASES-001: Durable independent canvas

The backend must persist each canvas as independent product state. Each canvas
must have one owner, one workspace, a title, a schema version, a revision,
ordered blocks, and timestamps.

A canvas can link to zero or more tasks. One task can link to zero or more
canvases in the same workspace. Removing a task must remove only its links. The
canvas must remain on the workspace Canvases page until the owner removes it.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-001.1:** After a browser reload and a backend restart, the owner shall see the same ordered blocks and revision.
- **AC-CANVASES-COLLABORATIVE-CANVASES-001.2:** When a linked task is removed, the system shall remove its links and preserve each canvas.
- **AC-CANVASES-COLLABORATIVE-CANVASES-001.3:** When a user links one canvas to several same-workspace tasks, each task shall open the same canvas state.
- **AC-CANVASES-COLLABORATIVE-CANVASES-001.4:** When a user changes the active workspace, the sidebar shall show canvases from the new workspace only.

### REQ-CANVASES-COLLABORATIVE-CANVASES-002: Closed native block contract

Version 1 must accept only these host-rendered block types:

- Markdown
- checklist
- Kanban
- metrics
- timeline

Each block must have a stable identifier, position, type, state, and revision.
The backend must reject unknown types, actions, executable content, and content
that exceeds a defined limit. Markdown must use the existing sanitized host
pipeline. The host must not fetch a remote resource during rendering.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-002.1:** When a client creates each supported block type, the system shall persist and show its typed state with a stable identifier and revision.
- **AC-CANVASES-COLLABORATIVE-CANVASES-002.2:** When input contains an unknown type, action, raw HTML, executable content, or automatic remote resource, the system shall reject it before a state change.

### REQ-CANVASES-COLLABORATIVE-CANVASES-003: One command model

Human controls and agent tools must call the same backend command service.
Each command must contain a unique command identifier, a base canvas revision,
an action, a target block when required, and validated input.

The service must apply a command once. It must append an event, update the
snapshot, increment the revision, and record the actor in one transaction.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-003.1:** When a human and an agent apply actions, both actions shall create ordered events through the same service.
- **AC-CANVASES-COLLABORATIVE-CANVASES-003.2:** When a caller retries one command identifier, the system shall return the first result without another event or revision.

### REQ-CANVASES-COLLABORATIVE-CANVASES-004: Edit conflicts

Structured blocks must address items by stable item identifier. The service can
apply a stale command when its target item still matches the command
precondition.

A full Markdown replacement must match the current block revision. A human tab
or agent can hold a renewable 30-second Markdown lease. Another writer must
receive the current block and lease state instead of overwriting content.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-004.1:** When two writers change different structured items, the system shall preserve both changes.
- **AC-CANVASES-COLLABORATIVE-CANVASES-004.2:** When commands conflict on one item, the rejected command shall receive the current item and revisions.
- **AC-CANVASES-COLLABORATIVE-CANVASES-004.3:** When one writer holds a Markdown lease, another writer shall receive a busy state until release or expiry.

### REQ-CANVASES-COLLABORATIVE-CANVASES-005: Live agent updates and recovery

An authorized owner client must subscribe to a canvas through the existing
WebSocket gateway. The backend must send ordered events after the last known
revision. If the history is unavailable, the backend must send the current
snapshot.

The interface must show agent changes, connection loss, recovery, and edit
conflicts. Version 1 must not publish user presence or focus state.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-005.1:** When an authorized client reconnects, the server shall send missed retained events or one complete snapshot.
- **AC-CANVASES-COLLABORATIVE-CANVASES-005.2:** When a non-owner client subscribes, the server shall deny the request without canvas content or history.
- **AC-CANVASES-COLLABORATIVE-CANVASES-005.3:** When an agent changes a canvas, an open owner client shall show the ordered change without a reload.

### REQ-CANVASES-COLLABORATIVE-CANVASES-006: Portable export and import

The owner must be able to export the current snapshot as one inert
`.kandev-canvas` JSON file. The file must contain a format version, export
identifier, export time, canvas title, canvas schema version, ordered blocks,
and block state.

The export must not contain events, actor records, user identifiers, task links,
session links, repository data, files, secrets, or server addresses.

An import must validate the complete file before persistence. A successful
import must create a new canvas in the selected workspace. It must create new
canvas and block identifiers. The imported canvas is an independent fork.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-006.1:** When an owner exports a canvas, the downloaded file shall contain the complete current block snapshot and no excluded data.
- **AC-CANVASES-COLLABORATIVE-CANVASES-006.2:** When a user imports a valid file, the system shall create a workspace canvas with new identifiers and equivalent block content.
- **AC-CANVASES-COLLABORATIVE-CANVASES-006.3:** When a file has an unknown version, invalid block, or exceeded limit, the system shall reject the complete import without partial state.
- **AC-CANVASES-COLLABORATIVE-CANVASES-006.4:** When a user imports the same file twice, the system shall create two independent canvases.

### REQ-CANVASES-COLLABORATIVE-CANVASES-007: Owner and task authorization

Only the workspace owner and trusted agents that act for the owner can read or
change a canvas. A task agent can access a canvas only when the canvas links to
its task. An Office agent can access canvases in its trusted workspace context.

Any operation on a linked task or repository must use the owning system's
current access decision. Canvas ownership must not replace that decision.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-007.1:** When another local user requests a canvas, the system shall deny read, edit, export, and subscription access.
- **AC-CANVASES-COLLABORATIVE-CANVASES-007.2:** When a task agent requests an unlinked canvas, the system shall deny the request.
- **AC-CANVASES-COLLABORATIVE-CANVASES-007.3:** When linked-resource access changes, the next operation shall use the current owning-system decision.

### REQ-CANVASES-COLLABORATIVE-CANVASES-008: Agent access

Task and Office sessions must receive a small canvas MCP tool group. The tools
must list accessible canvases, create a canvas, read a canvas, and apply one
canvas action.

The backend must derive the user, task, and session context from the trusted
MCP connection. Tool input must not select another identity. Agents must not
export, import, unlink, or remove a canvas in version 1.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-008.1:** Task and Office agents shall list, create, read, and change only canvases in their trusted context.
- **AC-CANVASES-COLLABORATIVE-CANVASES-008.2:** Agent tool input shall not claim another identity or perform export, import, unlink, or delete operations.

### REQ-CANVASES-COLLABORATIVE-CANVASES-009: Native canvas surfaces

The desktop sidebar must contain a folded Canvases section for the active
workspace. It must follow the Automations and Integrations section pattern. The
section must show canvas rows and a count. Its header shortcut must open the
workspace Canvases settings page.

The sidebar must not show New canvas or Import canvas actions. An empty section
must show a setup row that opens the workspace Canvases settings page.

The workspace Canvases settings page must support creation, import, search,
archive state, export, removal, and task associations.

The creation dialog must accept a title and an optional current-task link. A
task can also create and open a linked canvas in Dockview. The direct canvas
route and the Dockview panel must use the same state and command hooks.

On mobile, the workspace page must show canvases as vertical cards. New canvas
and Import must remain visible on that page. The focused canvas must use a
full-height route with an inset block and action drawer.

Mobile controls must have 44 by 44 CSS pixel targets, safe-area padding, one
vertical scroll owner, and no page-level horizontal scroll.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-009.1:** The desktop sidebar shall show a folded workspace canvas section with rows, count, and a settings-page shortcut.
- **AC-CANVASES-COLLABORATIVE-CANVASES-009.2:** Task users shall create or open linked canvases in a Dockview panel and open the same canvas on its direct route.
- **AC-CANVASES-COLLABORATIVE-CANVASES-009.3:** Users shall create, import, find, open, export, archive, and remove canvases on the workspace Canvases settings page.
- **AC-CANVASES-COLLABORATIVE-CANVASES-009.4:** Mobile actions shall have 44 by 44 CSS pixel targets and no page-level horizontal overflow.
- **AC-CANVASES-COLLABORATIVE-CANVASES-009.5:** The sidebar shall contain no New canvas or Import canvas action.

### REQ-CANVASES-COLLABORATIVE-CANVASES-010: Security and limits

The backend must enforce title, block, item, command, canvas, export, and import
limits before persistence or broadcast. Logs and metrics must not contain
Markdown bodies, block state, or imported file content.

The client must treat canvas and imported content as untrusted data. Blocks
must use host components and theme tokens. No block can load arbitrary remote
code, open an unrestricted URL, or request credentials.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-010.1:** When input exceeds a defined limit, the backend shall reject it before persistence or broadcast.
- **AC-CANVASES-COLLABORATIVE-CANVASES-010.2:** Canvas rendering shall use host components and shall not execute canvas or import code.
- **AC-CANVASES-COLLABORATIVE-CANVASES-010.3:** Logs and metrics shall omit Markdown bodies, block state, and imported file content.

### REQ-CANVASES-COLLABORATIVE-CANVASES-011: Observability and history bounds

The backend must record counters for accepted, duplicate, conflicted, denied,
exported, imported, and rejected-import operations. It must record active
canvas subscriptions and recovery mode without content labels.

The system must keep the current snapshot and the newest 1,000 events for each
canvas. It must compact older events without breaking reload or recovery.

#### Acceptance criteria

- **AC-CANVASES-COLLABORATIVE-CANVASES-011.1:** After compaction, a canvas shall retain at most 1,000 recent events and one complete current snapshot.
- **AC-CANVASES-COLLABORATIVE-CANVASES-011.2:** Metrics shall distinguish command, export, import, and recovery results without canvas content.
- **AC-CANVASES-COLLABORATIVE-CANVASES-011.3:** A client older than the compaction revision shall recover from a complete snapshot.

## Open questions

- A later requirement can define templates and plugin block types.
- A later requirement can define signed bundles or provenance verification.
- Live user sharing and instance federation require a separate authorization
  and synchronization design.
