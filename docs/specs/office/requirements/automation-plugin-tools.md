---
status: active
system: office
created: 2026-11-02
owners:
  - Kandev
---

# Private automation plugin tools

## Overview

Workspace owners need private automations to read plugin data and submit reports without creating visible tasks.
They select exact tools in the existing automation editor and save the selection.
This save approves those tools for that automation.

Office owns this contract because it owns automation configuration and execution authority.
The plugin system owns installation, tool declarations, and plugin execution.
This requirement defines a narrow extension to the current [automation authority](automation-continuity.md).
The extension is implemented by the backend policy boundary and the native automation editor.

Source: [discussion #4997 and its comment](https://github.com/kdlbs/kandev/discussions/4997).

## Terminology

- **Selection:** Exact tools approved for one private automation and workspace.
- **Policy version:** A host-owned value that changes after a plugin installation or admitted configuration save.
- **Review required:** A saved selection no longer matches the installed plugin policy version.

## Requirements

### REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-001: Save an explicit selection

**Intent:** Approve plugin access through the existing configuration workflow.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.1:** New and existing automations shall default to no plugin tool access.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.2:** An authorized native editor save shall approve only the selected tools that explicitly support private automations.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.3:** A selection shall apply only to its automation, workspace, plugin installation, and policy version.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.4:** A plugin installation update or admitted configuration save shall invalidate its earlier selections, including secret-only changes.
  Restarting an unchanged installation shall preserve valid selections.
  A failed update may leave the new policy version in place, but changed configuration shall never become effective under an earlier policy version.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.5:** An omitted selection shall remain unchanged.
  An explicit empty selection shall remove access.
  An unrelated save shall not renew stale access.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.6:** Portable exports shall omit selections and approval values.
  Imports shall start without approved tools.
  Other task modes shall not use this selection.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-001.7:** MCP callers and plugin schedule commands shall not create or renew selections.
  Unknown tools, stale approval values, and foreign workspaces shall fail without changing stored access.

### REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-002: Enforce the selection

**Intent:** Extend the fixed automation surface without granting general plugin or administrative access.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.1:** Discovery and invocation shall use the trusted automation, workspace, task, session, and surface identities.
  Tool arguments shall not replace those identities.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.2:** Discovery shall expose only currently approved tools from active, configured plugins.
  Global catalog updates shall not expose other tools.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.3:** Every invocation shall check the current selection, installation, policy version, declaration, and arguments before plugin dispatch.
  Missing or unreadable approval data shall deny access.
  Calls admitted before removal can finish.
  Later calls shall fail, including calls from stale clients and resumed sessions.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.4:** The fixed core catalog and its administrative restrictions shall remain unchanged.
  Plugin tools shall grant no additional Host API capability.
  Read-only annotations shall not grant authority.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.5:** Explicitly selected report-writing tools shall be supported alongside read tools.
  Results shall remain in automation history, and generated tasks shall stay outside Kanban and the sidebar.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-002.6:** Valid selections shall survive backend restart, continuation, and retries.
  Configuration changes, disabled plugins, and removed selections shall remain effective at the next call admission.

### REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-003: Edit access on desktop and phone

**Intent:** Keep tool approval visible and usable within the existing editor.

#### Acceptance criteria

- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.1:** The private automation editor shall show optional checkboxes grouped by plugin.
  Each tool shall have its name, description, and read or write indication.
  The existing Save action shall save access without another approval dialog.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.2:** Stale selections shall show Review required and the previous tool names.
  Current tools shall require explicit selection before renewed access.
  An unrelated edit shall preserve the stale selection without approving it.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.3:** The phone editor shall provide the same selection and removal actions in its existing vertical scroll surface.
  Selectable rows shall provide touch targets of at least 44 pixels.
  Content shall not cause document horizontal overflow.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.4:** Loading, empty, unavailable, and error states shall remain visible without hover.
  A failed options request shall preserve the saved selection and other editor changes.
- **AC-OFFICE-AUTOMATION-PLUGIN-TOOLS-003.5:** All new copy and accessible labels shall use the supported locale catalogs.

## Out of scope

- Automatic exposure for plugins that do not declare the `automation` surface.
- A separate approval service, approval queue, or permission settings page.
- Wildcards, automatic approval of future tools, or a generic plugin action invoker.
- Changes to the fixed core MCP catalog, task modes, scheduler, or run statuses.
- Changes to ordinary task or managed-conversation tool permissions.
- A sandbox for installed plugin code or changes to existing Host capability grants.
- Modifications or releases in third-party plugin repositories.
