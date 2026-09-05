---
id: canvases-agent-authored-web-apps
title: Agent-authored web-app canvases
status: draft
system: canvases
owners:
  - canvases
created: 2026-08-26
last_updated: 2026-08-30
---

# Agent-authored web-app canvases Requirements

## Overview

A canvas is a custom web application that an agent creates for one task. A
user can promote a useful task canvas to its workspace. A workspace canvas
appears in workspace navigation and can use workspace-scoped data.

The Canvases system owns the canvas scope, source lineage, release selection,
promotion, editing flow, and discovery. The Plugins system owns the isolated
web-application runtime and its data contract.

## Terminology

- **Task canvas:** A canvas that belongs to one task and appears only in that
  task.
- **Workspace canvas:** A promoted canvas that belongs to one workspace and
  appears in workspace navigation.
- **Draft:** Editable canvas source in an authorized agent workspace.
- **Release:** An immutable package that passed validation.
- **Promotion:** A user action that changes a task canvas to workspace scope.

## Requirements

### REQ-CANVASES-AGENT-WEB-APPS-001: Agent-created task canvas

**Intent:** A task agent creates a purpose-built interface without requiring a
person to assemble predefined blocks.

**User story:** As a user, I want a task agent to create a custom interface for
the task, so that the interface matches the work.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-001.1:** When a user requests a canvas from a
  task, the active task agent shall create the canvas draft in that task
  context.
- **AC-CANVASES-AGENT-WEB-APPS-001.2:** When the agent publishes a first
  release, the open task shall show its ready or permission-review canvas host
  without a browser reload or manual panel action.
- **AC-CANVASES-AGENT-WEB-APPS-001.3:** When an agent session creates a canvas,
  the canvas shall belong to the trusted task and workspace from that session.
- **AC-CANVASES-AGENT-WEB-APPS-001.4:** Agent input shall not select another
  task, workspace, user, or canvas owner.
- **AC-CANVASES-AGENT-WEB-APPS-001.5:** The task interface shall not offer a
  blank manual canvas builder or predefined block editor.
- **AC-CANVASES-AGENT-WEB-APPS-001.6:** When a task agent creates or edits a
  canvas, it shall be able to read the version-matched Kandev canvas-authoring
  skill without adding that skill to the task workspace.
- **AC-CANVASES-AGENT-WEB-APPS-001.7:** When the canvas feature is disabled,
  Kandev shall not expose canvas tools, routes, events, background work, or user
  interface entries.
- **AC-CANVASES-AGENT-WEB-APPS-001.8:** The authoring guidance shall provide a
  complete core workflow and exact file inventory with at most one required
  skill-read operation.
- **AC-CANVASES-AGENT-WEB-APPS-001.9:** Local, container, and remote task
  agents shall use the same authoring contract without access to a Kandev host
  file path.

### REQ-CANVASES-AGENT-WEB-APPS-002: Durable source and releases

**Intent:** A published canvas remains available after the agent workspace and
browser session end.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-002.1:** After a browser reload and a backend
  restart, the canvas shall load the same active release.
- **AC-CANVASES-AGENT-WEB-APPS-002.2:** When an agent publishes an invalid
  draft, the system shall preserve the current active release and show the
  validation errors.
- **AC-CANVASES-AGENT-WEB-APPS-002.3:** When an agent publishes a valid draft,
  the system shall activate one immutable release and preserve one prior valid
  release for rollback.
- **AC-CANVASES-AGENT-WEB-APPS-002.4:** When a user rolls back a release, the
  canvas shall activate the retained prior release without changing its scope
  or identity.
- **AC-CANVASES-AGENT-WEB-APPS-002.5:** When a task is removed, the system shall
  remove its unpromoted canvases and preserve canvases that were promoted.
- **AC-CANVASES-AGENT-WEB-APPS-002.6:** When a workspace is removed, the system
  shall remove all of its canvases, grants, state, runtime tokens, releases, and
  retained artifacts.

### REQ-CANVASES-AGENT-WEB-APPS-003: User-controlled promotion

**Intent:** A useful task canvas becomes a workspace application only after a
user reviews its scope and permissions.

**User story:** As a user, I want to promote a useful task canvas, so that I
can open it from the workspace sidebar.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-003.1:** When a user starts promotion, the
  system shall show the requested data, write, event, state, and network
  permissions before confirmation.
- **AC-CANVASES-AGENT-WEB-APPS-003.2:** When the user confirms promotion, the
  same canvas identity and active release shall change to workspace scope.
- **AC-CANVASES-AGENT-WEB-APPS-003.3:** When promotion completes, the canvas
  shall appear in navigation for that workspace only.
- **AC-CANVASES-AGENT-WEB-APPS-003.4:** An agent shall not promote, demote, or
  grant permissions to a canvas.
- **AC-CANVASES-AGENT-WEB-APPS-003.5:** When a release requests new permissions,
  the current release shall remain active until a user approves the new set.

### REQ-CANVASES-AGENT-WEB-APPS-004: Agent-assisted workspace editing

**Intent:** A person changes a workspace canvas by describing the change to an
agent instead of editing source in Kandev.

**User story:** As a user, I want an agent to edit a workspace canvas, so that
I do not need to maintain its source manually.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-004.1:** When a user selects Edit canvas, the
  system shall launch a Quick Chat agent with a draft of the active source.
- **AC-CANVASES-AGENT-WEB-APPS-004.2:** The edit agent shall receive the canvas
  identity, current manifest, current source, validation rules, and existing
  permission grants.
- **AC-CANVASES-AGENT-WEB-APPS-004.3:** Agent edits shall not change the active
  release until the publish operation succeeds.
- **AC-CANVASES-AGENT-WEB-APPS-004.4:** When a Quick Chat edit session expires,
  the active and prior releases shall remain available.
- **AC-CANVASES-AGENT-WEB-APPS-004.5:** When the edit adds permissions, the
  system shall use the permission review in
  `AC-CANVASES-AGENT-WEB-APPS-003.5`.

### REQ-CANVASES-AGENT-WEB-APPS-005: Task and workspace discovery

**Intent:** Users find a canvas at the scope where it is useful.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-005.1:** A task shall list its task canvases and
  workspace canvases that apply to the task workspace.
- **AC-CANVASES-AGENT-WEB-APPS-005.2:** The desktop sidebar shall list active
  workspace canvases in a Canvases section that starts folded. Opening a canvas
  route shall not expand the section.
- **AC-CANVASES-AGENT-WEB-APPS-005.3:** The sidebar shall not list task-only,
  archived, disabled, invalid, or pending-permission canvases.
- **AC-CANVASES-AGENT-WEB-APPS-005.4:** When a user opens a canvas from a task,
  the desktop task workbench shall show it in a canvas panel.
- **AC-CANVASES-AGENT-WEB-APPS-005.5:** When a user opens a workspace canvas
  from navigation, the system shall show the same active release on its direct
  route.
- **AC-CANVASES-AGENT-WEB-APPS-005.6:** Host controls for Edit, Promote,
  Permissions, Releases, Archive, and Remove shall remain outside canvas code.
- **AC-CANVASES-AGENT-WEB-APPS-005.7:** When a user explicitly expands or
  folds the Canvases sidebar section, Kandev shall retain that preference.
- **AC-CANVASES-AGENT-WEB-APPS-005.8:** When canvases are enabled, workspace
  settings shall show a Canvases tab and shall include the active canvas count
  in workspace summaries that have room for it.
- **AC-CANVASES-AGENT-WEB-APPS-005.9:** A narrow workspace summary shall keep
  the Canvases tab available without compressing the summary tiles into
  unreadable labels.

### REQ-CANVASES-AGENT-WEB-APPS-006: Responsive host surface

**Intent:** Desktop and phone users can open, manage, promote, and edit the same
canvas through native Kandev navigation.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-006.1:** On a phone, a task shall open one
  focused canvas in a full-height route instead of Dockview.
- **AC-CANVASES-AGENT-WEB-APPS-006.2:** On a phone, workspace canvases shall
  appear as labeled entries in the mobile navigation for the active workspace.
- **AC-CANVASES-AGENT-WEB-APPS-006.3:** On a phone, a canvas picker and
  secondary host actions shall use an inset bottom drawer.
- **AC-CANVASES-AGENT-WEB-APPS-006.4:** Host controls shall use safe-area
  padding, one vertical scroll owner, and touch targets of at least 44 CSS
  pixels.
- **AC-CANVASES-AGENT-WEB-APPS-006.5:** The host page shall have no horizontal
  overflow at supported phone widths.
- **AC-CANVASES-AGENT-WEB-APPS-006.6:** The application viewport shall receive
  the available size without a compressed desktop workbench.
- **AC-CANVASES-AGENT-WEB-APPS-006.7:** Release, permission, and promotion
  controls shall explain their effect through pointer and keyboard help on
  desktop and visible descriptions on touch surfaces.

### REQ-CANVASES-AGENT-WEB-APPS-007: Visible runtime and release state

**Intent:** A user can understand whether the canvas is loading, offline,
blocked by permissions, invalid, or using a prior release.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-007.1:** The host shall show distinct loading,
  ready, offline, permission-review, invalid-release, and unavailable states.
- **AC-CANVASES-AGENT-WEB-APPS-007.2:** When live events disconnect, the canvas
  shall keep its last rendered content and show the connection state.
- **AC-CANVASES-AGENT-WEB-APPS-007.3:** When a canvas release becomes invalid or
  unavailable, the host shall show recovery actions without executing that
  release.
- **AC-CANVASES-AGENT-WEB-APPS-007.4:** A release history shall identify the
  active release, author kind, creation time, validation result, and permission
  change without showing source content in logs.

### REQ-CANVASES-AGENT-WEB-APPS-008: Bounded agent authoring

**Intent:** A task agent cannot fill canvas storage through repeated create or
publish operations.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-008.1:** A task shall have at most 10
  non-removed task canvases. A workspace shall have at most 100 non-removed
  canvases across task and workspace scopes.
- **AC-CANVASES-AGENT-WEB-APPS-008.2:** One agent session shall make at most 10
  canvas publish attempts in five minutes, and one canvas shall have at most one
  publish operation in progress.
- **AC-CANVASES-AGENT-WEB-APPS-008.3:** A create, publish, archive, or restore
  limit error shall return a stable code and preserve the active release.
- **AC-CANVASES-AGENT-WEB-APPS-008.4:** Archived canvases shall count toward
  canvas and storage limits until a user removes them.

### REQ-CANVASES-AGENT-WEB-APPS-009: Guided canvas task launch

**Intent:** A user starts canvas authoring through a normal task without
configuring repository state that the canvas does not need.

**User story:** As a user, I want a guided canvas task, so that I can choose an
agent and review the canvas in the same task.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-009.1:** When canvases are enabled, the desktop
  sidebar and workspace Canvases settings shall offer a Create canvas action.
- **AC-CANVASES-AGENT-WEB-APPS-009.2:** The action shall open the standard task
  creation flow with a localized canvas title and prompt, no repository, an
  empty scratch path, and an eligible local executor preference.
- **AC-CANVASES-AGENT-WEB-APPS-009.3:** The user shall be able to change the
  workflow and agent profile before task creation.
- **AC-CANVASES-AGENT-WEB-APPS-009.4:** After task creation, Kandev shall open
  the normal task details surface where the user can interact with the agent
  and review the canvas.
- **AC-CANVASES-AGENT-WEB-APPS-009.5:** On a phone, workspace Canvases settings
  shall expose the same task creation flow without a canvas-only form.

## Out of scope

- A manual visual application builder.
- A direct source-code editor in Kandev.
- Canvas invitations, collaborator roles, or multi-user live editing.
- Demotion from workspace scope to task scope.
- A marketplace or cross-instance package import flow.
- Automatic publication of a release that requests new permissions.
- General top-bar, sidebar-widget, or arbitrary-slot plugin contributions.
- A custom server-side runtime for agent-generated backend code.
