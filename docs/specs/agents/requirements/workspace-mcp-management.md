---
status: active
system: agents
created: 2026-10-22
owners:
  - kandev
---

# Workspace MCP management requirements

## Overview

Members need to find where a catalog definition is selected and reach the
correct owner. Global profiles need an explicit workspace when selecting MCPs.
Session evidence must distinguish saved choices from observed connections.

Agents owns catalog associations and their management experience. Platform
retains ownership of attachment evidence. This package extends
[workspace MCP configuration](workspace-mcp-configuration.md) and consumes
[session observability](../../platform/requirements/mcp-session-observability.md).
It does not replace either contract.

## Terminology

- **Used in:** Direct saved selections of a catalog definition. This does not
  count runtime connections or enumerate every session that inherits it.
- **Owner:** A repository, workspace-profile pair, task, or task agent session
  that directly selects the definition.
- **Unobserved:** Kandev has no current connection evidence. This is not failure.

## Requirements

### REQ-AGENTS-MCP-MANAGEMENT-001: Named usage and owner navigation

**Intent:** Let members understand a definition's saved usage without guessing
from aggregate counts or navigating to unrelated settings.

#### Acceptance criteria

- **AC-AGENTS-MCP-MANAGEMENT-001.1:** A catalog definition shall expose Used in
  with authorized owner names grouped by scope. Each direct association shall
  appear once, including saved associations for disabled definitions.
- **AC-AGENTS-MCP-MANAGEMENT-001.2:** An authorized owner row shall navigate to
  that owner's existing management surface with the correct workspace and,
  where applicable, task session. Opening a row shall not change any selection.
  A view-only destination shall not imply permission to edit it. A failed
  session-list read shall offer retry for the same requested conversation;
  confirmed missing sessions shall never open a fallback conversation.
- **AC-AGENTS-MCP-MANAGEMENT-001.3:** Names, identifiers, links, and usage totals
  shall not reveal inaccessible owners. Missing or removed owners shall not
  produce broken links. An authorized owner with no name shall use a localized
  scope label, never its internal identifier.
- **AC-AGENTS-MCP-MANAGEMENT-001.4:** Loading, a successful empty result, a failed
  read, and an incomplete result shall be distinct. Members shall be able to
  retrieve all visible usage. Refresh shall reconcile renamed or removed owners
  without discarding catalog edits or showing another workspace's result.
  Failed continuation shall keep accepted pages usable. A failed refresh shall
  disable retained destinations until a successful retry replaces them.

### REQ-AGENTS-MCP-MANAGEMENT-002: Workspace-aware global profile selection

**Intent:** Give a saved global profile independent catalog choices in each
workspace while preserving settings save ownership and legacy behavior.

#### Acceptance criteria

- **AC-AGENTS-MCP-MANAGEMENT-002.1:** A saved MCP-capable global profile shall
  expose the shared catalog picker with a visible, authorized workspace choice.
  With no valid workspace, it shall request a workspace and disable selection.
  A workspace-owned profile shall remain fixed to its owning workspace.
- **AC-AGENTS-MCP-MANAGEMENT-002.2:** Changing workspace with unsaved settings
  shall offer the existing Save changes, discard, and continue-editing choices.
  A failed or blocked save shall retain the current workspace and draft. No
  selection shall save merely because the member changes workspace.
- **AC-AGENTS-MCP-MANAGEMENT-002.3:** A save shall use the workspace and owner
  captured for that draft. Late responses shall not replace another context's
  data or clear newer edits. Shared settings save/discard shall remain the only
  persistence controls for profile selections. Session customization shall keep
  its existing autosave behavior.
- **AC-AGENTS-MCP-MANAGEMENT-002.4:** The picker shall explain that choices apply
  to this profile in the selected workspace. Render one inline searchable
  selection list, without a second opener, duplicate description, or catalog
  shortcut. Changing these defaults shall not claim immediate
  application to an existing agent or alter its provider timing.
- **AC-AGENTS-MCP-MANAGEMENT-002.5:** Catalog navigation and selection shall
  preserve legacy fallback until successful import for that workspace-profile
  pair. An incomplete or unknown import shall not appear complete. Authorized
  legacy recovery shall remain available and clearly identify its wider scope.
  Opening the picker shall not import, rewrite, or clear legacy configuration.

### REQ-AGENTS-MCP-MANAGEMENT-003: Neutral evidence beside saved changes

**Intent:** Explain the difference between setup delivery, connection evidence,
and pending selection changes using the existing session evidence authority.

#### Acceptance criteria

- **AC-AGENTS-MCP-MANAGEMENT-003.1:** Delivery without observed connection and
  unobserved configuration shall use neutral presentation in Connections,
  server detail, and hover/focus summaries. These states shall not resemble
  pending-selection warnings or assert healthy or failed connections.
- **AC-AGENTS-MCP-MANAGEMENT-003.2:** A configured-only server shall remain
  unobserved unless its own current attachment evidence proves delivery.
  Evidence for another server, session, or earlier attachment shall not promote
  it. Existing explicit failures, exclusions, connected state, and verified
  tools shall retain their evidence meanings and available diagnostics.
- **AC-AGENTS-MCP-MANAGEMENT-003.3:** Pending, next-start, failed-application,
  and applied selection states shall remain independent of connection evidence.
  A neutral delivered row may coexist with a pending selection marker. Clearing
  a pending marker shall not imply connection success or create a tool count.

### REQ-AGENTS-MCP-MANAGEMENT-004: Complete responsive management

**Intent:** Make the same management outcomes accessible on desktop and phone.

#### Acceptance criteria

- **AC-AGENTS-MCP-MANAGEMENT-004.1:** Used in shall provide a focused phone
  detail surface with named navigation, one vertical scroll owner, fixed
  navigation controls, and safe-area clearance. Short workspace choices shall
  use the existing touch picker pattern. Required actions shall not need hover.
- **AC-AGENTS-MCP-MANAGEMENT-004.2:** Phone and coarse-pointer controls shall
  have at least 44px hit targets. Long names, translations, and errors shall
  wrap without clipping controls or causing horizontal page overflow. Desktop
  ordinary controls shall retain the standard 28px density.
- **AC-AGENTS-MCP-MANAGEMENT-004.3:** Keyboard activation, visible focus,
  predictable back/dismiss behavior, and focus return shall work for all new
  destinations. Resizing shall preserve drafts, accepted usage pages and their
  continuation, and the single editor owner.
  New copy shall exist in all shipped locales and use text alongside state icons.

## Out of scope

- Session-only inherited exclusions, assignment after registration, and built-in
  MCP management. Separate follow-ups own these features.
- New probes, endpoint tests, telemetry producers, runtime reconnection, or
  changes to the additive resolver, import process, and provider timing.
- A second catalog hub, automatic selection, global MCP definitions, broad
  inventory discovery, and bulk removal of usage associations.

## Implementation plans

- [MCP management clarity](../../../plans/mcp-management-clarity/plan.md)
