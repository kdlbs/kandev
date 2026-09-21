---
id: canvases-agent-authored-web-apps
title: Agent-authored web-app canvases
status: draft
system: canvases
owners:
  - canvases
created: 2026-08-26
last_updated: 2026-09-23
---

# Agent-authored web-app canvases Requirements

## Overview

A canvas is a custom web application that an agent creates for one task. A
user can promote a useful task canvas to its workspace. A workspace canvas
appears in workspace navigation. An owner-authorized task canvas can already
use its workspace's data while the user reviews it in the originating task.

The Canvases system owns the canvas scope, source lineage, release selection,
promotion, editing flow, and discovery. The Plugins system owns the web-application
runtime and its data contract.

## Terminology

- **Task canvas:** A canvas that belongs to one task and appears only in that
  task. Its placement does not determine its approved data scope.
- **Workspace canvas:** A canvas that belongs to one workspace and appears in
  workspace navigation. Agent-authored canvases reach this scope by promotion;
  [distribution](marketplace-sharing.md) also defines reviewed package installs.
- **Draft:** Editable canvas source in an authorized agent workspace.
- **Release:** An immutable package that passed validation.
- **Promotion:** A user action that changes a task canvas to workspace placement.

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
- **AC-CANVASES-AGENT-WEB-APPS-001.10:** Canvas-capable task instructions shall
  identify the create, skill-read, and publish operations, including discovery
  guidance when those tools are not immediately callable.
- **AC-CANVASES-AGENT-WEB-APPS-001.11:** Authoring instructions shall require
  creation in Kandev, edits inside the assigned source directory, and a publish
  result before a completion claim.
- **AC-CANVASES-AGENT-WEB-APPS-001.12:** Authoring instructions shall distinguish
  an active release, a release awaiting permission review, and an unsuccessful
  publish. Local files or a successful build shall not imply publication.
- **AC-CANVASES-AGENT-WEB-APPS-001.13:** When a user returns after publication,
  the task shall show eligible task canvases not previously presented in that
  browser tab. This shall also work after reload or a missed publication event.
  Eligible canvases have an active valid release or await permission review.
  Draft-only, archived, disabled, removed, invalid, foreign-task, and workspace
  canvases shall not open automatically.
- **AC-CANVASES-AGENT-WEB-APPS-001.14:** Desktop shall add new canvas panels to
  the main editor group and focus one new panel. Existing panels shall retain
  their placement. Repeated discovery shall not duplicate panels or steal focus.
- **AC-CANVASES-AGENT-WEB-APPS-001.15:** Closing a presented canvas shall prevent
  automatic reopening in that browser tab, including after reload or publication
  of another release. Manual reopening shall remain available.
- **AC-CANVASES-AGENT-WEB-APPS-001.16:** On phones, automatic presentation shall
  open one focused canvas route. Returning to the task shall not redirect to
  that canvas again. Other eligible canvases shall remain accessible through
  the existing picker.

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

**Intent:** A useful task canvas becomes available in workspace navigation only
after a user reviews its placement and permissions.

**User story:** As a user, I want to promote a useful task canvas, so that I
can open it from the workspace sidebar.

#### Acceptance criteria

- **AC-CANVASES-AGENT-WEB-APPS-003.1:** When a user starts promotion, the
  system shall show the requested data, write, event, state, and network
  permissions before confirmation.
- **AC-CANVASES-AGENT-WEB-APPS-003.2:** When the user confirms promotion, the
  same canvas identity and active release shall change to workspace placement.
  A release that already has workspace data access shall keep that access.
- **AC-CANVASES-AGENT-WEB-APPS-003.3:** When promotion completes, the canvas
  shall appear in navigation for that workspace only.
- **AC-CANVASES-AGENT-WEB-APPS-003.4:** An agent shall not promote, demote, or
  directly grant permissions to a canvas. Owner-authorized first publication
  shall follow [local creation authority](local-creation-authority.md).
- **AC-CANVASES-AGENT-WEB-APPS-003.5:** Except for owner-authorized first
  publication, a release that requests new permissions shall await user review.
  Any current release shall remain active until the user approves the new set.

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
- **AC-CANVASES-AGENT-WEB-APPS-006.8:** Release review shall keep its primary
  actions visible outside the scrolling content. On desktop, the surface shall
  use the available viewport; on phones, it shall provide a focused full-height
  view with one content scroll region and safe-area clearance.
- **AC-CANVASES-AGENT-WEB-APPS-006.9:** A two-permission review shall expose
  both permissions and its actions without scrolling at 1280 by 720 CSS pixels.
  Longer reviews shall keep actions reachable at 390 by 844 CSS pixels.

- **AC-CANVASES-AGENT-WEB-APPS-006.10:** An embedded canvas shall have one
  host action toolbar aligned with other task panels under the shared
  [panel toolbar contract](../../ui/requirements/panel-toolbars.md).
  A standalone canvas shall retain one page navigation/action header.
- **AC-CANVASES-AGENT-WEB-APPS-006.11:** Neither presentation shall add a
  separate status-only toolbar. Loading and blocking errors shall appear once
  in the body, with recovery actions. Ready shall show the application and an
  accessible status announcement. Nonblocking offline state may appear inline
  in the existing header while retaining the application.

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
- **AC-CANVASES-AGENT-WEB-APPS-007.5:** A runtime URL or iframe load event
  alone shall not display Ready. Until the current frame acknowledges startup,
  the host shall display Loading. After 15 seconds without acknowledgement,
  it shall show the runtime-startup-failure state of
  `AC-CANVASES-AGENT-WEB-APPS-007.9` with Retry and Releases actions.
- **AC-CANVASES-AGENT-WEB-APPS-007.6:** Retry, release replacement, token
  renewal, and canvas navigation shall ignore acknowledgements from previous
  frame attempts. An unavailable frame shall not cover recovery controls.
- **AC-CANVASES-AGENT-WEB-APPS-007.7:** Release and promotion review shall
  show readable release dates, status, and source labels. Internal release,
  task, and session identifiers shall not appear as visible labels or fallback
  copy. Missing sources shall have a readable unavailable label.
- **AC-CANVASES-AGENT-WEB-APPS-007.8:** Review shall describe each permission
  once in plain language, identify newly requested access, and show exact
  external origins. Ordinary permission review shall not appear as a validation
  failure. Active and retained valid releases shall have distinct labels.

- **AC-CANVASES-AGENT-WEB-APPS-007.9:** When a canvas application fails its
  startup acknowledgement while its active release is valid, the host shall
  show a runtime-startup-failure state whose title and description attribute
  the failure to the canvas application or its runtime. That state shall be
  distinct from the release-unavailable state of
  `AC-CANVASES-AGENT-WEB-APPS-007.1`, and the host shall not describe a valid
  active release as unavailable.
- **AC-CANVASES-AGENT-WEB-APPS-007.10:** The runtime-startup-failure state
  shall distinguish at least these reported causes through its description: an
  error raised by the canvas application while starting, a canvas application
  that cannot reach its runtime capability API, a browser failure to load the
  application document, and an absent acknowledgement at the
  `AC-CANVASES-AGENT-WEB-APPS-007.5` deadline. A cause that the host cannot
  determine shall not be reported as a release problem.

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

- **AC-CANVASES-AGENT-WEB-APPS-009.1:** When canvases are enabled, selecting
  Set up a canvas in the empty desktop sidebar shall open task creation
  directly without changing the current route. Workspace Canvases settings
  shall retain its Create canvas action and the sidebar its settings shortcut.
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
- **AC-CANVASES-AGENT-WEB-APPS-009.6:** The localized preset shall contain a
  short editable request for a coordinator view listing existing tasks,
  followed by a blank line and the exact reference `@create-canvas`.
  Detailed authoring instructions shall be available as that saved prompt.
- **AC-CANVASES-AGENT-WEB-APPS-009.7:** On desktop and phone, the user shall
  be able to read and edit the preset before submission. The submitted task
  shall retain those edits.
- **AC-CANVASES-AGENT-WEB-APPS-009.8:** The shipped saved prompt shall direct
  agents through tool discovery, draft creation, one core skill read,
  assigned-directory editing, and publication. It shall require authorized
  live data and accurate release, permission-review, and promotion reporting.
- **AC-CANVASES-AGENT-WEB-APPS-009.9:** When a structured task starts with
  `@create-canvas`, the agent shall receive the saved definition as hidden
  context. The task description shall retain the user's short request and
  reference. Create without starting shall defer expansion until launch.
- **AC-CANVASES-AGENT-WEB-APPS-009.10:** Users shall be able to customize
  `create-canvas` through Settings > Prompts. Startup shall preserve an
  existing same-name prompt and user edits. Removing the reference shall
  remove its expansion from the next submitted request.
- **AC-CANVASES-AGENT-WEB-APPS-009.11:** Desktop and phone users shall be able
  to edit the goal, cancel, retry a failed submission, and start the task.
  Failure shall preserve edits; cancellation shall create no task or canvas.
  The phone action shall remain reachable without horizontal page overflow.

Saved-prompt resolution follows
[Saved Prompt Delivery](../../tasks/requirements/saved-prompt-delivery.md),
including missing references, lookup failures, and passthrough exclusions.

## Implementation plans

- [Canvas runtime and task-entry recovery](../../../plans/canvas-runtime-entry-recovery/plan.md)
- [Direct canvas creation and saved prompt](../../../plans/canvas-direct-creation/plan.md)
- [Task canvas workspace data preview](../../../plans/task-canvas-workspace-preview/plan.md)

## Out of scope

- A manual visual application builder.
- A direct source-code editor in Kandev.
- Canvas invitations, collaborator roles, or multi-user live editing.
- Demotion from workspace scope to task scope.
- Marketplace and cross-instance distribution are owned by the separate
  [marketplace and sharing requirements](marketplace-sharing.md).
- Automatic permission increases after owner-authorized first publication or
  automatic publication of a release that requests new permissions.
- General top-bar, sidebar-widget, or arbitrary-slot plugin contributions.
- A custom server-side runtime for agent-generated backend code.
