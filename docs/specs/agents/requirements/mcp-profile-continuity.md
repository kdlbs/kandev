---
status: active
system: agents
created: 2026-11-04
owners:
  - kandev
---

# MCP tool profile continuity

## Purpose and ownership

Agents owns the MCP tool profile that a managed agent receives at process start.
Workspace preparation and session recovery must preserve that contract.
The same task context must produce the same eligible capability groups through each start path.

A tool profile contains a base surface, capability groups, and provider capabilities.
It can also contain a built-in package binding.
Tool eligibility remains subject to its existing owner and authorization rules.

## Requirements

### REQ-AGENTS-MCP-PROFILE-CONTINUITY-001: Complete profile delivery at process start

**Intent:** A resumed conversation receives its eligible Kandev tools without a manual model change or a new task session.

#### Acceptance criteria

- **AC-AGENTS-MCP-PROFILE-CONTINUITY-001.1:** For equivalent authorized task context, a new process shall receive the same eligible tool groups through fresh launch, resume, or workspace promotion.
  A workspace helper created before the launch request shall not cause missing groups.
- **AC-AGENTS-MCP-PROFILE-CONTINUITY-001.2:** Before the first request reaches a new process, its Kandev catalog shall reflect the complete resolved tool profile.
  A base mode alone shall not remove eligible capability or provider groups.
- **AC-AGENTS-MCP-PROFILE-CONTINUITY-001.3:** Canvas availability shall respect the existing canvas gate and session surface.
  The title tool shall remain limited to the pending title's owner session.
  Question capabilities shall retain their existing human, parent, and autopilot boundaries.
- **AC-AGENTS-MCP-PROFILE-CONTINUITY-001.4:** Generated startup instructions shall use the same effective profile as registration.
  Instructions shall omit unavailable optional groups and retain the existing tool-discovery guidance.
- **AC-AGENTS-MCP-PROFILE-CONTINUITY-001.5:** If a helper cannot accept or confirm the required profile, the new process shall not start.
  Recovery shall retain the task session and saved provider conversation identity for retry.
  A mode-only fallback shall not report successful profile delivery.
- **AC-AGENTS-MCP-PROFILE-CONTINUITY-001.6:** Backend adoption or reconnect of an existing process shall preserve its accepted catalog and package binding.
  The system shall not restart that process, apply new package preferences, or invent historical selection evidence.

## Related contracts

- [Agent resume and runtime recovery](agent-resume-runtime-recovery.md) owns conversation identity and recovery state.
- [Built-in MCP management](builtin-mcp-management.md) owns package selection, frozen bindings, and retained legacy execution compatibility.
- [Agent-generated titles](../../tasks/requirements/agent-generated-titles.md) owns title eligibility and session ownership.
- [Agent-authored web apps](../../canvases/requirements/agent-authored-web-apps.md) owns canvas authority and authoring outcomes.
- [MCP discovery guidance](mcp-tool-discovery-guidance.md) owns the selected-tool instruction format.

## Exclusions

This contract does not require identical numeric tool counts across different task contexts or releases.
It does not grant title rights to every resumed session.
It does not migrate an active legacy process to package mode or repair provider-owned discovery caches during a turn.
It adds no public UI, package preference, feature flag, or plugin registration mechanism.

## Delivery

- [System design](../system-design/mcp-profile-continuity.md)
- [Workspace MCP profile continuity plan](../../../plans/workspace-mcp-profile-continuity/plan.md)
