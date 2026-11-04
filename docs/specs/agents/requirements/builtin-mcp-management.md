---
status: draft
system: agents
created: 2026-10-22
owners:
  - kandev
---

# Built-in MCP packages and tool selection

## Intent and boundary

Users can choose coherent packages of native Kandev tools and understand them in
the same management experience as other MCP servers. Configuration chats receive
Configuration automatically. An authorized user can also give Configuration to a
regular task without converting that task into a configuration chat.

This draft implements the user's 2026-10-24 scope revision and
[issue #3808](https://github.com/kdlbs/kandev/issues/3808). It supersedes this
package's earlier workspace-only, three-group pruning proposal. It extends
[workspace MCP configuration](workspace-mcp-configuration.md) and the default
availability of [agent rich output](agent-rich-output.md). Agents owns package
selection and delivery. Platform retains ownership of session observations.
Names, package boundaries, and defaults below are recommendations for review,
not previously approved behavior. Implementation needs a later explicit request.

## Terms

- **Operator:** A human caller authorized to manage the affected workspace or task.
- **Package:** A named set of native tools exposed as one host-injected MCP server.
- **Core:** The required built-in package, retaining the reserved `kandev` identity.
- **Agent profile:** Saved provider, model, permissions, and runtime preferences.
- **Tool profile:** Backend-selected surface and eligible capabilities for an execution.
- **Workspace policy:** Limits and defaults for optional packages used by tasks.
- **Task preference:** A human-set choice to inherit, enable, or disable a package.
- **Bound manifest:** The package selection frozen for one agent execution.

## Requirements

### REQ-AGENTS-BUILTIN-MCP-001: Built-in identity and authority

**Intent:** Make packages selectable without transferring runtime ownership.

- **AC-AGENTS-BUILTIN-MCP-001.1:** MCP management shall show a Built-in section
  containing Core, Configuration, Rich output, Walkthroughs, and Diagnostics,
  including when the workspace has no user-defined server definitions. Each
  detail shall explain purpose, tool membership, ownership, and eligible contexts.
- **AC-AGENTS-BUILTIN-MCP-001.2:** Each selected package shall be a distinct
  host-injected MCP server. Core shall retain `kandev`. Package identities shall
  be reserved. Packages shall not become ordinary workspace catalog records or
  offer editable endpoints, credentials, rename, delete, or marketplace install.
- **AC-AGENTS-BUILTIN-MCP-001.3:** Only an authorized human shall change workspace
  package policy or task preferences. Agent MCP callers, including Configuration
  callers, shall not grant themselves or another task packages, change the
  operator policy, or bypass resource authorization. Denied changes shall leave
  saved state unchanged and shall not disclose foreign-workspace state.
- **AC-AGENTS-BUILTIN-MCP-001.4:** Workspace restrictions shall bound task choices.
  Agent profiles shall retain provider and permission controls but shall not
  grant packages in this release. A session shall use its task's preferences at
  its next process start; it shall have no independent package override. Task
  creation by an agent shall not copy Configuration from the caller.

### REQ-AGENTS-BUILTIN-MCP-002: Package boundaries and safe defaults

**Intent:** Reduce optional tools while preserving required control paths.

- **AC-AGENTS-BUILTIN-MCP-002.1:** Regular Kanban tasks shall receive Core, Rich
  output, Walkthroughs, and Diagnostics by default. Configuration shall be off
  by default and require an explicit human task choice. Configuration chats
  shall receive Core and Configuration automatically. Office tasks shall retain
  their existing Core capabilities and default Rich output. Other fixed or
  restricted surfaces shall retain their existing exact catalogs and grants.
- **AC-AGENTS-BUILTIN-MCP-002.2:** Core shall preserve eligible lifecycle,
  completion, delegation, question barriers, title ownership, plan/recovery,
  task relationship, provider change-request, review, branch/source, canvas,
  and Office-document capabilities. Required means required when eligible;
  selecting a package shall not grant title, question, provider, or canvas rights.
- **AC-AGENTS-BUILTIN-MCP-002.3:** Workspace operators shall be able to block an
  optional task package, permit explicit task selection, or enable it by default.
  Configuration shall support only blocked or explicit selection for regular
  tasks. Task choices shall be Inherit, On, or Off within those limits. Invalid,
  duplicate, unknown, required-package, arbitrary-tool, and stale-revision saves
  shall reject the whole request. Reset shall restore the stated defaults.
- **AC-AGENTS-BUILTIN-MCP-002.4:** An excluded package shall have no registered
  tools or usable alternate built-in route for that execution. Direct calls
  shall fail before effects. Generated instructions and built-in UI actions
  shall use the same bound availability. Existing output and walkthroughs shall
  remain readable after the authoring package is disabled.
- **AC-AGENTS-BUILTIN-MCP-002.5:** A regular task with Configuration shall gain
  its authorized configuration operations while remaining a regular task.
  Configuration shall cover workflows/steps, agents/profiles, MCP settings,
  shared prompts, executors, settings discovery/update, and automation setup.
  It shall not grant task-transfer privileges, permission-answering authority,
  unrestricted resource access, or authority to alter package grants. Each
  canonical tool shall appear at most once across injected packages for an execution.

### REQ-AGENTS-BUILTIN-MCP-003: Predictable application and recovery

**Intent:** Keep the running agent's tool contract stable and explain changes.

- **AC-AGENTS-BUILTIN-MCP-003.1:** Saving policy or preferences shall not start,
  stop, reset, reconnect, or alter a running agent. Changes shall apply at the
  next normal agent process start. Idle, a new turn, and backend adoption shall
  not count as a new process start. The UI shall state this timing.
- **AC-AGENTS-BUILTIN-MCP-003.2:** Each start shall bind one validated package
  manifest to injection, registration, dispatch authorization, and generated
  guidance. Existing executions shall retain it through title/capability changes,
  context reset, same-process resume, reconnect, and backend adoption.
- **AC-AGENTS-BUILTIN-MCP-003.3:** Session detail shall distinguish desired
  selection, bound packages, and observed tools for each server. A save during
  launch shall not relabel an older launch as current. Failed starts and delayed
  observations shall not replace newer accepted execution state.
- **AC-AGENTS-BUILTIN-MCP-003.4:** Missing policy or preferences on upgrade shall
  preserve the stated default tool capabilities. Invalid or unreadable explicit
  state shall fail a new start closed. Retained legacy executions shall keep
  their existing monolithic catalog and show unavailable historical selection
  evidence as Unknown. No automatic restart or fabricated snapshot is permitted.
- **AC-AGENTS-BUILTIN-MCP-003.5:** A helper or adapter that cannot enforce the
  package contract shall report an explicit unsupported state before a new
  package-mode start. A collision with a user-defined server identity shall
  preserve that definition and require resolution. Package selection shall not
  silently fall back to an unrestricted monolithic server.

### REQ-AGENTS-BUILTIN-MCP-004: Evidence and responsive management

**Intent:** Make choices and actual session behavior understandable on any screen.

- **AC-AGENTS-BUILTIN-MCP-004.1:** Workspace details shall show configuration
  without global health badges. Session details shall show attachment/tool
  observations with execution identity and time, or a neutral Unobserved/Unknown
  explanation. Included, selected, registered, and delivered shall not imply
  connected or healthy. One package's evidence shall not certify another.
- **AC-AGENTS-BUILTIN-MCP-004.2:** Workspace management shall explain limits and
  defaults. Task Customize shall offer a separate Built-in packages section
  with effective choices, inheritance, blocked/required reasons, and next-start
  timing. It shall remain separate from inherited or additional catalog servers.
  Configuration detail shall explain its mutation authority, including authorized
  shared resources, before the user saves an explicit task opt-in.
- **AC-AGENTS-BUILTIN-MCP-004.3:** Desktop and phone shall support the same read,
  edit, save, discard, reset, conflict, and denied-change flows. Drafts shall
  survive viewport changes. Denied writes shall remain unsaved. Read-only users
  shall see values and reasons without misleading editable controls.
- **AC-AGENTS-BUILTIN-MCP-004.4:** Phone navigation shall show one detail level
  at a time, with visible Back/Close actions, one content scroller, safe areas,
  and at least 44px active targets. Desktop fine-pointer controls shall retain
  28px ordinary sizing. Reasons and errors shall not depend on hover or color.
- **AC-AGENTS-BUILTIN-MCP-004.5:** Loading, empty catalog, unavailable evidence,
  pending changes, conflict, and denial shall work at 320px, 393px, and 767px
  phone widths, the 768px boundary, and on a 900px touch tablet. Copy shall use
  all shipped locales and support keyboard navigation and focus return.

## Exclusions and delivery

Inherited-server exclusions, assignment after registration, and general MCP
navigation/status redesign stay in sibling packages. This work does not add
arbitrary tool-name filters, new probes, immediate revocation, new roles,
workflow/profile package editors, or a general sandbox. Selecting Configuration
never implies user authorization to run a particular destructive operation.
Existing agent instructions and operation-specific user consent still apply.

[System design](../system-design/builtin-mcp-management.md) defines the technical
contract. [Implementation plan](../../../plans/builtin-mcp-management/plan.md)
contains sequential work orders and evidence requirements.
