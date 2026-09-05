# Server-owned declarative canvases

- **Status:** superseded by [ADR-2026-08-26-plugin-backed-web-app-canvases](2026-08-26-plugin-backed-web-app-canvases.md)
- **Date:** 2026-08-25
- **Updated:** 2026-08-26
- **Area:** backend, frontend, protocol, security

## Context

This decision is not the current direction. The replacement decision uses
isolated plugin web applications for canvases.

GitHub Copilot App Canvas lets a person and an agent act on one interactive
state. The public sample loads repository JavaScript, starts a loopback server,
and shows an iframe.

That model fits a local development runtime. It creates a large trust and
deployment boundary in Kandev.

Kandev needs a durable native work surface. It also needs a simple way to move
that work between users and self-hosted instances. Live user sharing adds roles,
invitations, presence, revocation, and synchronization before the core canvas
experience exists.

Task documents, rich output, plugin panels, and task snapshots do not own this
contract. A canvas is structured, editable, persistent, and independent from a
task.

## Decision

Kandev will implement canvases as server-owned product state with a closed,
versioned block and action schema.

Human controls and agent MCP tools will call one typed command service. The
backend will persist the snapshot, bounded events, task links, command receipts,
and actor provenance. The WebSocket gateway will send agent changes and recovery
state to the owner.

Version 1 has one role: owner. It will not include collaborators, invitations,
shared links, user presence, or cross-instance live synchronization.

Each canvas will belong to one workspace. It can link to several tasks in that
workspace. The workspace settings page will own creation, import, export,
archive, and removal.

The desktop sidebar will follow the Automations and Integrations section
pattern. It will show workspace canvas rows and a settings-page shortcut. It
will not show creation or import actions.

Users will transfer canvases through one inert `.kandev-canvas` JSON file. The
file will contain the current typed snapshot and no user, task, event, server,
repository, file, or secret data. Each import will create an independent canvas
with new identifiers and local ownership.

The web and desktop clients will show blocks with Kandev-native components.
Version 1 will not execute canvas-supplied HTML, CSS, JavaScript, remote pages,
repository extensions, arbitrary component identifiers, or plugin block types.

## Consequences

### Positive

- One durable state model supports users, agents, reload, restart, and task
  handoff.
- A portable file works across self-hosted instances without server trust.
- Import is atomic and creates a clear fork without synchronization conflicts.
- Native components preserve theme, accessibility, desktop, and mobile behavior.
- The first version avoids collaborator ACLs, invitations, presence, and
  federation.
- Workspace scope gives the sidebar and task links one stable query boundary.

### Costs

- Kandev must define and maintain a portable file version.
- Imported canvases do not receive later changes from the source canvas.
- Users must export and import again to transfer a later snapshot.
- Users must open workspace settings to create or import a canvas outside a
  task.
- A closed block set supports less visual freedom than executable extensions.
- Live sharing needs a later authorization and synchronization decision.

## Alternatives

### Add same-instance collaborators now

Deferred. This requires viewer and editor roles, invitations, revocation,
presence, conflict UX, and tests across authenticated users. Portable files
provide handoff without these contracts.

### Add cross-instance federation

Deferred. Federation requires server identity, remote user identity, signed
requests, key rotation, availability policy, revocation, and conflict recovery.

### Use a ZIP bundle

Rejected for version 1. The initial block set contains bounded structured data.
A single JSON file is easier to inspect, validate, and transfer. A future format
version can add an archive when native assets become necessary.

### Execute repository canvas extensions

Rejected for version 1. This requires code trust, dependency installation,
process isolation, network isolation, content policy, and upgrade compatibility.

### Extend task documents

Rejected. Documents have a Markdown contract and one-writer behavior.
Structured actions and task-independent lifecycle need a separate owner.

### Store canvas state in the browser

Rejected. Browser state cannot provide restart durability, agent access,
portable server validation, or ordered recovery.

### Let plugins own canvases

Deferred. Plugins can contribute panels, but the host first needs stable canvas
storage, actions, and file contracts.

## Related records

- [Collaborative canvas requirements](../specs/canvases/requirements/collaborative-canvases.md)
- [Collaborative canvas system design](../specs/canvases/system-design/collaborative-canvases.md)
- [GitHub Copilot App Canvas reference](../copilot-canvas-reference.md)
- [Opt-in authentication](2026-07-24-opt-in-authentication.md)
- [Plugin task panels](2026-08-01-plugin-task-panel-contributions.md)
- [Agent rich output](../specs/agents/requirements/agent-rich-output.md)
- [Task documents](../specs/tasks/requirements/documents.md)
