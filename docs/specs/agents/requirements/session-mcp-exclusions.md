---
status: active
system: agents
created: 2026-10-22
owners:
  - kandev
---

# Session MCP Exclusions Requirements

## Overview

A workspace member can exclude an inherited catalog server from one existing
task agent session. Restoring inheritance reverses that choice. Agents owns
the saved selection, composition, and application contract, including its UI.

This extends [workspace MCP configuration](workspace-mcp-configuration.md).
It updates the session-only restrictions in AC-003.3/.7 and
AC-006.13/.16/.20/.21 of that document. Other scopes retain additive selection.
The original delivery ledger remains historical evidence.

## Terminology

- **Session exclusion:** A saved exception for one catalog definition and one
  Kandev task agent session. It lasts until restored or the session is deleted.
- **Inherited server:** A definition contributed by a task, any attached
  repository, or the effective profile in the task workspace.
- **Saved setup:** Current catalog choices after session exclusions. Definition
  enablement and provider policy still govern delivery.
- **Last accepted setup:** The setup accepted at a successful attachment.
  This is not proof of connection health or current tool availability.

## Requirements

### REQ-AGENTS-SESSION-MCP-EXCLUSIONS-001: Reversible session scope

**Intent:** Temporarily reduce one session's inherited catalog setup without
changing the owners that supplied it.

#### Acceptance criteria

- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.1:** An authorized member shall be able
  to exclude an inherited definition from one existing session. Repository,
  profile, task, and sibling-session selections shall remain unchanged.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.2:** One exclusion shall cover every
  contribution of that definition, including multiple repositories and a saved
  session addition. All contributors shall remain visible. Restoring shall
  remove only the exclusion and use the then-current selections.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.3:** Saved exclusions shall survive
  closing the editor, page reload, backend restart, normal agent restart, and
  reopening the same session. A new or duplicated session shall start without
  these exclusions. Session deletion shall remove them.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.4:** If all contributors disappear,
  the exclusion shall remain visible and reversible as an inactive exception.
  If the same definition contributes again, the exclusion shall apply again.
  Deleting the definition shall remove its exclusion. Recreating a server with
  the same name shall not transfer the deleted definition's exception.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.5:** Excluding or restoring a disabled
  inherited definition shall not enable it workspace-wide. Re-enabling its
  definition shall preserve the session exclusion. The UI shall distinguish
  workspace disablement from session exclusion.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.6:** The system shall reject unauthorized,
  foreign-workspace, missing, or newly non-inherited exclusion targets without
  changing saved state. Concurrent exclusion edits shall not silently overwrite
  a newer saved choice. Existing additive clients shall preserve exclusions.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.7:** Required Kandev built-in capabilities
  shall remain outside this exclusion control. Unmigrated legacy and independently
  configured provider servers shall not acquire catalog exclusion controls or
  a false claim of removal from the provider.

### REQ-AGENTS-SESSION-MCP-EXCLUSIONS-002: Safe application and recovery

**Intent:** Separate a saved session preference from the setup used by an agent.

#### Acceptance criteria

- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.1:** Excluding or restoring during an
  active turn shall save a pending choice without changing that turn's delivered
  MCP set. Saving shall not interrupt or start an agent.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.2:** At an eligible idle boundary, the
  system shall use the existing safe reconnect path and complete desired set.
  Unsupported or project-file-based paths shall retain the choice for the next
  actual start. The session identity, transcript, profile, and runtime settings
  shall retain their existing continuity guarantees.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.3:** Successful application shall accept
  the exact setup sent by that operation. An older success shall not clear a
  newer exclusion or restoration. Reopening shall recover the accepted baseline.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.4:** Failed application shall preserve
  the last accepted baseline and saved choice. The UI shall offer retry and
  explain next-start application. Retry shall retain an unsaved addition draft
  and remain unavailable after a rejected save until choices reload successfully.
  It shall not claim rollback or verified absence
  when a provider partially accepted a failed operation.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.5:** Pending comparison shall identify
  exclusion as removal from the saved setup and restoration as addition when
  applicable. Search shall not hide pending actions. Unknown comparison shall
  remain unknown. Reversing a choice before application shall not invent a
  removal or addition when the saved and accepted setups match.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-002.6:** Excluded definitions shall not be
  prepared, have secrets resolved, or be delivered by Kandev for the new setup.
  Safe attachment evidence shall identify session exclusion separately from
  disabled definitions, transport filtering, and connection failure.

### REQ-AGENTS-SESSION-MCP-EXCLUSIONS-003: Clear desktop and touch controls

**Intent:** Make the scope, timing, and reversal of exclusions understandable.

#### Acceptance criteria

- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.1:** MCP plug > Customize shall show
  an explicit Exclude for this session action on eligible inherited rows and
  Restore inheritance on excluded rows. The editor shall state that exclusions
  last for this session until restored. It shall explain that workspace-wide
  disabling affects other sessions' future setups.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.2:** Each row shall retain its name,
  metadata, all authorized origin names, saved inclusion or exclusion, and
  relevant pending action. Current attachment evidence shall remain in Connections.
  Saved state shall never imply connected, disconnected, or unavailable tools.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.3:** Desktop shall retain the composer
  dialog. Phone and coarse-pointer users shall receive one sheet with fixed
  heading and view controls, one scroll owner, and safe-area clearance. Actions
  shall be reachable at 320px width without horizontal document overflow.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.4:** Exclude, restore, retry, and dismiss
  shall work with keyboard and touch. Phone and coarse-pointer action hit areas
  shall measure at least 44 by 44 CSS pixels. Status and error text shall not
  depend on hover, motion, or color alone. Focus shall return to the plug on dismiss.
- **AC-AGENTS-SESSION-MCP-EXCLUSIONS-003.5:** Initial load, refresh, save, conflict,
  rejection, pending, deferred, and failed states shall remain distinct. Background
  updates shall preserve search, focus, scroll, and row order. Context changes
  shall hide the previous session's state. All new copy shall use shipped locales.

## Out of scope

- Exclusions in repository, profile, task, or new-session creation editors.
- Registration assignment, catalog usage navigation, and global-profile UI.
- Built-in catalog management, tool pruning, or new mandatory-tool policy.
- Native provider configuration editing, legacy migration redesign, or a security
  revocation guarantee for already delivered tools or independent configuration.
- New provider reconnection capabilities, automatic restarts, or endpoint probes.

## Implementation plan

- [Session exclusions package](../../../plans/session-mcp-exclusions/plan.md)
