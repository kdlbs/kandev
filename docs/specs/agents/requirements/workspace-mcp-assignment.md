---
status: active
system: agents
created: 2026-10-22
owners:
  - kandev
---

# Assignment after workspace MCP registration

## Overview

After saving a workspace MCP definition, a member can leave it saved or choose
where to use it. Agents owns this outcome because it owns definitions, scope
selections, and application timing. Existing owner editors retain their save
behavior. Registration never requires an assignment.

This extends [workspace MCP configuration](workspace-mcp-configuration.md).
It does not replace its additive composition, workspace isolation, legacy
fallback, or runtime application contract.

## Terminology

- **Registration:** Saving a custom or marketplace definition in a workspace.
- **Assignment:** Selecting that definition at an existing owner.
- **Owner:** A repository, a profile in one workspace, or one task agent session.
- **Use in:** An optional handoff from the saved definition to an owner editor.
  Opening the editor does not select or save the definition.

## Requirements

### REQ-AGENTS-MCP-ASSIGNMENT-001: Independent registration completion

**Intent:** Make successful registration clear even when assignment is skipped
or cannot finish.

#### Acceptance criteria

- **AC-AGENTS-MCP-ASSIGNMENT-001.1:** After successful custom or marketplace
  registration, the system shall identify the saved definition and workspace,
  offer optional Use in, and provide Done without requiring an owner.
- **AC-AGENTS-MCP-ASSIGNMENT-001.2:** Done, dismissal, back navigation, and
  assignment failure shall preserve the saved definition. Returning to the
  catalog shall provide Use in for that same definition, including after reload.
- **AC-AGENTS-MCP-ASSIGNMENT-001.3:** Registration, discovery, owner search, and
  navigation shall not connect to MCP endpoints, download or execute packages,
  start agents, or create tasks or sessions. Registration success shall not
  imply assignment, runtime delivery, or connection health.
- **AC-AGENTS-MCP-ASSIGNMENT-001.4:** Retrying assignment shall use the saved
  definition identity and shall not repeat registration. If the registration
  response is lost, the system shall offer catalog reconciliation before another
  create attempt and shall not infer success from an unrelated name match.

### REQ-AGENTS-MCP-ASSIGNMENT-002: Explicit owner and scope

**Intent:** Let members choose an authorized destination with its consequences
visible before a selection changes.

#### Acceptance criteria

- **AC-AGENTS-MCP-ASSIGNMENT-002.1:** Use in shall offer repository, agent
  profile in this workspace, and existing task agent session. It shall show
  authorized names and enough task, agent, and workspace context to distinguish
  identical names. No owner shall be selected automatically.
- **AC-AGENTS-MCP-ASSIGNMENT-002.2:** Before opening a repository or profile
  editor, the system shall explain that its saved choices affect future
  attachments using that owner. They do not change an already delivered turn.
  The destination shall retain shared Save changes, discard, and dirty-route
  guards. Opening it shall not save or replace its existing draft.
- **AC-AGENTS-MCP-ASSIGNMENT-002.3:** A global profile shall use the registering
  workspace explicitly. Its choices in other workspaces shall remain separate.
  Missing, unauthorized, or incompatible workspace context shall prevent a
  selection change and shall not fall back to a global or active-workspace write.
- **AC-AGENTS-MCP-ASSIGNMENT-002.4:** Existing-session assignment shall name
  the exact task and session. The destination shall use its existing Customize
  selection control. Only explicit selection there shall autosave. Saved,
  pending-idle, next-start, and failed-application states shall remain distinct.
  The flow shall not restart an agent or replace a conversation.
- **AC-AGENTS-MCP-ASSIGNMENT-002.5:** The system shall revalidate access and
  eligibility at the destination and at save. Removed owners, disabled profiles,
  archived tasks, unavailable sessions, and disabled or removed definitions
  shall have a readable unavailable outcome. Selecting another owner or keeping
  the definition saved shall remain possible. No automatic enable or restore
  operation shall occur.

- **AC-AGENTS-MCP-ASSIGNMENT-002.6:** An optional definition hint shall reveal
  an available definition once without changing selection, search, or save
  state. It shall use the authorized destination workspace and existing picker
  inventory. Invalid or unavailable hints shall not select a fallback or widen
  that inventory. Session navigation shall not open Customize automatically;
  the hint shall wait until the user opens it.

### REQ-AGENTS-MCP-ASSIGNMENT-003: Safe assignment recovery

**Intent:** Preserve other choices and distinguish saved configuration from an
unsuccessful assignment or runtime application.

#### Acceptance criteria

- **AC-AGENTS-MCP-ASSIGNMENT-003.1:** Saving from a stale owner selection
  snapshot shall reject a conflicting replacement without losing another
  writer's choices. The editor shall retain the draft and offer reload and
  explicit reconciliation. It shall not automatically resubmit the stale set.
- **AC-AGENTS-MCP-ASSIGNMENT-003.2:** An already selected or inherited
  definition shall be identified without creating duplicate effective entries
  or implying that an inherited selection is removable here. Adding a new
  definition shall preserve unrelated direct selections.
- **AC-AGENTS-MCP-ASSIGNMENT-003.3:** A denied, failed, or uncertain selection
  save shall not display assignment success. The saved definition shall remain
  recoverable through the existing catalog. A successful selection save followed
  by runtime failure shall remain saved and show the existing retry or next-start
  recovery, without repeating registration.
- **AC-AGENTS-MCP-ASSIGNMENT-003.4:** A workspace, owner, or session change
  shall invalidate old asynchronous results. An old completion shall not expose
  another workspace's data, overwrite a new draft, or open an incorrect editor.

### REQ-AGENTS-MCP-ASSIGNMENT-004: Desktop and phone completion

**Intent:** Make saved-only and assignment paths equally usable on desktop,
phone, keyboard, and touch.

#### Acceptance criteria

- **AC-AGENTS-MCP-ASSIGNMENT-004.1:** Desktop and phone shall both support
  saved-only completion, all three owner types, scope explanation, and partial
  success recovery. Phone choice shall use one focused surface and direct owner
  navigation, without stacked dialogs or a second management destination.
- **AC-AGENTS-MCP-ASSIGNMENT-004.2:** Phone and coarse-pointer actions shall
  have at least 44px targets. Ordinary fine-pointer desktop controls shall retain
  28px sizing. Long names, loading, empty results, unavailable targets, and
  failures shall fit without horizontal page overflow. An overflowing chooser
  shall have one scroll owner and safe-area clearance.
- **AC-AGENTS-MCP-ASSIGNMENT-004.3:** Keyboard activation, visible focus,
  dismissal, and focus return shall work. Viewport changes shall retain the
  current definition, owner choice, and search without another mutation.
- **AC-AGENTS-MCP-ASSIGNMENT-004.4:** All new interface text shall use the
  current locale. Every shipped locale shall include the new copy. Saving and
  failure feedback shall be readable without hover, color, or motion alone.

## Out of scope

- Task-level assignment, bulk assignment, owner creation, or automatic selection.
- A second management hub, Used in projection, or global-profile management UI.
  Management clarity owns the latter two destinations and navigation contracts.
- Session inherited exclusions and built-in MCP management, each owned by a
  separate sibling follow-up.
- Endpoint probes, package materialization, new reconnect logic, provider
  capability claims, or secret-value previews.

## Implementation plans

- [Assignment after registration](../../../plans/mcp-assignment-after-registration/plan.md)
