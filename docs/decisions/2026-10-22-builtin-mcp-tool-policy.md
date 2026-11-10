# ADR-2026-10-22-builtin-mcp-tool-policy: Native MCP packages with bounded task selection

**Status:** proposed
**Date:** 2026-10-22 (proposal revised 2026-10-24)
**Area:** backend, frontend, protocol

## Context

[Issue #3808](https://github.com/kdlbs/kandev/issues/3808) requests a smaller task
tool surface. The user's later direction also requests native packages, with
Configuration automatically available in configuration chats and explicitly
selectable for a normal task. This revises the earlier unapproved proposal for
workspace-only exclusions. The existing runtime has one injected `kandev` server,
backend-owned surfaces, capability guards, and separate user-defined catalogs.
Configuration and task registration groups overlap. Simply instantiating both
would duplicate tool names and could confuse dispatch authority.

## Decision proposed for review

Provide five packages: Core, Configuration, Rich output, Walkthroughs, and
Diagnostics. Each is a separate logical MCP server on the existing agentctl
process, using a disjoint registered catalog and package-specific routes. Core
keeps `kandev`; other reserved identities use `kandev-<package>`. Do not create
workspace catalog definitions or one process per package. Preserve canonical
`*_kandev` tool names. Package descriptors and membership have one backend owner.

Core protects eligible lifecycle and workflow capabilities. Optional task tools
remain enabled by default to preserve behavior. Configuration is required in
configuration chats and requires a human task opt-in in ordinary Kanban tasks.
Workspace limits bound task preferences. Profiles keep existing provider and
permission authority; sessions have no independent package override. Agents
cannot modify the package policy or grants, including through Configuration.

Bind selection to an execution at normal process start. A regular task with
Configuration keeps its regular surface and gets a separate trusted package
grant. Registration and backend dispatch enforce that grant; normal resource
checks still apply. Save never restarts or mutates the running agent. Desired,
bound, and observed state remain separate. Legacy retained agents keep the old
catalog until a normal start on a package-capable helper.

The proposal is defined in [requirements](../specs/agents/requirements/builtin-mcp-management.md)
and [system design](../specs/agents/system-design/builtin-mcp-management.md).
It extends reserved host injection without replacing workspace composition or
session-observability decisions. No implementation approval is implied.

## Consequences

Users get clear choices and can omit expensive tool schemas. Configuration has
a visible purpose and a deliberate task opt-in. Core still supplies mandatory
control paths. Package selection is not a sandbox, resource-ownership grant,
or immediate permission revocation. Authorized configuration may affect shared
resources; the UI must explain that boundary.

Multiple MCP connections add handshake and adapter costs. All routes, permission
provenance, prompts, tool discovery, and observation identities must support the
same manifest. Moving a tool changes its client-qualified name even though the
canonical name remains stable. Legacy retained executions therefore stay intact;
new package-mode executions do not provide duplicate monolithic aliases.

## Alternatives considered

- One server with group toggles is cheaper, but does not provide the separate
  Configuration MCP or independently visible package connections requested.
- A separate process or external install for each package adds lifecycle and
  credential complexity without an isolation requirement. Use logical servers.
- Ordinary catalog records imply editable endpoints and deletion and conflict
  with host-owned runtime injection. Use typed built-in descriptors/selections.
- Workspace defaults alone cannot deliberately equip one normal task with
  Configuration. Task preferences provide that explicit choice.
- Profile grants and session overrides add precedence and cross-workspace
  questions. Keep those editors out of this release; profile permissions remain.
- A default-on Configuration option for normal tasks would silently expand
  their mutation surface. Require an explicit human choice per task instead.
- One Presentation package would force rich output and walkthrough schemas to
  travel together. Keep them separate for independent pruning.
- Separate Planning, Delegation, Code review, and Canvas packages could reduce
  more tools, but need workflow contract changes. Preserve those in Core for now.
- Arbitrary tool filters or immediate reconnect can remove required controls
  or leave cached catalogs inconsistent. Use reviewed packages and start binding.
