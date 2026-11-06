---
status: active
system: plugins
created: 2026-11-06
owners:
  - Kandev
---
# Workspace-Scoped Plugin Tool Requirements

## Overview

External MCP clients can call plugin tools only as instance reads, which carry
no workspace. This capability lets Kandev authorize a workspace for the caller
and pass the verified workspace to the plugin.

## Requirements

### REQ-PLUGINS-AGENT-TOOL-WORKSPACE-001: Workspace-authorized external reads

**Intent:** Plugins own workspace data, but an external MCP client can reach
plugin tools only as instance reads, which carry no workspace. As a plugin
author, I want Kandev to verify which workspace an external caller may read
and pass that workspace to my tool, so that I can expose workspace-owned data
without building a separate authentication system.

#### Acceptance criteria

- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.1:** A plugin may declare
  `invocation_scope: workspace` for an agent tool. Omitted, `task`, and
  `instance` declarations shall keep their existing behavior.
- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.2:** When a manifest declares a workspace tool
  for the `external` surface without a true read-only annotation, for an
  `automation`, `coordinator`, or `managed-conversation` surface, with an input
  schema whose top level is not an object, or with a top-level input property
  named `kandev_workspace_id`, installation shall fail with a validation error.
- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.3:** When an external client lists tools, each
  eligible workspace tool shall advertise a required string argument
  `kandev_workspace_id` in addition to the plugin-declared inputs.
- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.4:** When an external caller invokes a workspace
  tool with the ID of an existing workspace that the caller may read, the
  plugin shall receive that workspace ID, the `external` surface, and empty
  task, session, and execution identity. The plugin arguments shall not
  contain `kandev_workspace_id`.
- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.5:** When `kandev_workspace_id` is absent, empty,
  not a string, unknown, or names a workspace that the caller cannot read, the
  call shall fail before plugin execution. Unknown and unreadable workspaces
  shall return the same error.
- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.6:** Workspace access shall be decided by the
  caller identity that the external endpoint already authenticated (personal
  access token or browser session) and the caller's current workspace
  permissions. While authentication is disabled, any existing workspace shall
  be accepted.
- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.7:** Each invocation shall re-evaluate workspace
  access, so a revoked membership or token fails the next call. Caller
  credentials shall never be sent to the plugin.
- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.8:** When a workspace tool also declares a
  `kanban-task` or `office-task` surface, a task-backed call shall receive the
  host-resolved task workspace and the tool shall advertise no
  `kandev_workspace_id` argument on that surface.
- **AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.9:** An external call to an instance tool shall
  continue to carry no workspace, and a caller-supplied identity field outside
  the tool arguments shall still be rejected before plugin execution.

#### Exclusions

- Workspace scope for automation, coordinator, and managed-conversation
  surfaces. Those contexts already bind a workspace through their own grants.
- Workspace-scoped writes, and filtering the external tool list by the
  caller's workspace memberships.
- A plugin-side permission model, plugin-visible user identity, and UI changes.

## Related

Extends [plugin-contributed agent tools](agent-tools.md)
(REQ-PLUGINS-AGENT-TOOLS-001 and REQ-PLUGINS-AGENT-TOOLS-002).
Design: [Workspace-scoped plugin tools](../system-design/agent-tool-workspace-scope.md).
Plan: [Workspace-scoped external plugin reads](../../../plans/plugin-tool-workspace-scope/plan.md).
