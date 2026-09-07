# ADR-2026-08-26-plugin-backed-web-app-canvases: Use plugin-backed web applications for canvases

**Status:** accepted
**Date:** 2026-08-26
**Updated:** 2026-08-30
**Area:** backend, frontend, protocol, plugins, security

## Context

The first canvas design used server-owned Markdown and structured blocks. That
design limited canvas interfaces to types that Kandev implemented in advance.
It did not provide the flexibility of GitHub Copilot App Canvas extensions.

The intended canvas is a custom application that an agent creates for one
task. A user can promote a useful application to its workspace. The application
can then appear in workspace navigation and serve more than one task.

Kandev already has a plugin system for permissions, Kandev data access, events,
state, agent tools, process lifecycle, and UI contributions. A separate canvas
runtime duplicates those contracts and blocks later plugin authoring in
Kandev.

Agent-generated code must not run in the Kandev SPA. The current native plugin
bundle runs with the host React instance and application context. That trust
level is not valid for arbitrary canvas code.

## Decision

Kandev will implement a canvas as a scoped instance of a plugin web-application
contribution.

An agent creates and publishes a canvas from a task session. The first canvas
scope is that task. A user can promote the same canvas instance to workspace
scope after a permission review. Only workspace canvases appear in workspace
navigation.

The web application can contain arbitrary packaged HTML, CSS, and JavaScript.
Kandev will show it in a sandboxed iframe. Kandev will not load this code as a
native plugin bundle or give it the host React, DOM, Zustand store, or cookies.
The runtime document will also use the CSP `sandbox` response directive. This
rule keeps the document opaque when a person opens its URL outside the iframe.

Kandev will not inject a canvas JavaScript object. The application will use
relative HTTP and Server-Sent Events endpoints that the plugin runtime owns.
Those endpoints will expose only the effective manifest capabilities and the
authorized canvas scope.

The host can send a versioned, one-way appearance envelope to the iframe. The
envelope contains only the resolved color mode and bounded semantic colors. It
does not create a host API or grant authority.

The effective permission set is the intersection of these values:

- the package declarations
- the grants that the user approved for the canvas instance
- the current Kandev access decision for the user and resource

Each published release is immutable. A valid release replaces the active
release atomically. A rejected release does not change the active release.
Kandev keeps one prior valid release for rollback and editing.

Release artifacts remain files under the Kandev data directory. The Kandev
home, not a database snapshot alone, is the recovery boundary for these files.
Startup marks a release unavailable before execution when its artifact is
missing or has a different digest.

The first delivery supports static web-application packages. It supports
Kandev data, canvas state, and live Kandev events through the host gateway. It
does not run an agent-generated backend process on the Kandev host.

Installed managed plugins can add web-application contributions through the
same package and placement contract. A later decision can add a sandboxed
custom backend runtime for locally authored plugins.

The user edits a workspace canvas through a Quick Chat agent. Kandev gives the
agent a draft copy of the current source. The agent publishes a new immutable
release through the same validation path.

The canvas-authoring skill uses a canvas-owned embed and a canvas-owned system
skill directory under the Kandev home. It does not use the Office bundled-skill
embed or Office workspace deployment.

Implementation starts from `main`. PR #3061 is the superseded declarative
implementation and is not a prerequisite. The first implementation ships
behind `features.canvases`, with all profile defaults set to `false`.

## Consequences

- Canvas interfaces can use arbitrary packaged HTML, CSS, and JavaScript.
- Canvas data access follows the plugin permission and data contracts.
- Task canvases and workspace canvases use one package and release format.
- Promotion changes scope and discovery. It does not convert the application.
- Future plugin authoring can reuse the package, release, permission, and web
  application runtime.
- Kandev must add scoped plugin instances and grants to its instance-global
  plugin model.
- Kandev must add an isolated web-application contribution beside trusted
  native React contributions.
- The HTTP gateway becomes a public plugin protocol and needs versioned data
  shapes, limits, conflict behavior, and event recovery.
- Arbitrary custom backend logic remains unavailable to static-only canvases.
  A managed plugin backend can still provide custom plugin actions.
- A canvas must bundle its executable frontend assets. Remote scripts do not
  become part of the active release.
- Runtime isolation applies to iframe and direct capability-URL navigation.
- A complete canvas backup includes the database and retained artifacts.
- Database-only recovery can leave a release unavailable but cannot execute a
  missing or changed artifact.
- Canvas authoring does not add a bundled skill to Office workspaces.
- Operators can keep every canvas entry point disabled through one backend
  feature gate.
- Isolated applications can match live Kandev appearance without sharing host
  state or authority.

## Alternatives Considered

### Keep the closed native block model

Rejected. It gives Kandev consistent rendering but cannot provide arbitrary
application interfaces or agent-authored functionality.

### Load canvas JavaScript as a native frontend plugin

Rejected. Native bundles share the Kandev JavaScript process, React context,
DOM, and application origin. Agent-generated code needs a stronger boundary.

### Add a canvas-specific JavaScript SDK

Rejected. It creates another extension contract and couples canvas code to an
injected global object. Standard web requests are sufficient for the isolated
runtime.

### Require an agent-generated Go plugin binary

Rejected for the first delivery. Kandev does not ship a compiler on every
installation. An unsandboxed generated binary also has excessive host access.

### Store only an iframe URL

Rejected. A remote URL does not give Kandev immutable releases, offline use,
permission review, source editing, or dependable rollback.

## Related records

- [Agent-authored web-app canvas requirements](../specs/canvases/requirements/agent-authored-web-apps.md)
- [Agent-authored web-app canvas design](../specs/canvases/system-design/agent-authored-web-apps.md)
- [Isolated plugin web-application requirements](../specs/plugins/requirements/isolated-web-app-contributions.md)
- [Isolated plugin web-application design](../specs/plugins/system-design/isolated-web-app-contributions.md)
- [GitHub Copilot App Canvas reference](../copilot-canvas-reference.md)
- [Plugin Host data API](0043-plugin-host-data-api.md)
- [Plugin agent tools through Kandev MCP](2026-08-11-plugin-tools-through-kandev-mcp.md)
- [Superseded declarative canvas decision](2026-08-25-server-owned-declarative-canvases.md)
