---
status: active
system: workspaces
created: 2026-10-04
owners:
  - kandev
---

# Per-tab Settings Workspace Context Requirements

## Overview

Settings is shared by Office and Kanban workspaces. A valid workspace that is
active in the current browser tab must remain active when the user opens
Settings. The workspace system owns this behavior because it governs the
identity and lifecycle of the active workspace.

## Requirements

### REQ-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001: Preserve the current tab workspace

**Intent:** Opening Settings must not switch a tab to a workspace selected in another tab.

#### Acceptance criteria

- **AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.1:** When Settings opens in an existing tab and that tab's active workspace is in the available workspace list, Settings shall keep that workspace active even when the shared cookie or saved workspace preference names another workspace.
- **AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.2:** When the current tab has no active workspace in the available workspace list, Settings shall select the first valid candidate in this order: shared active-workspace cookie, saved workspace preference, first available workspace.
- **AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.3:** When no workspace is available, Settings shall have no active workspace.
- **AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.4:** Settings shall apply the same candidate rules to Office and Kanban workspaces.
- **AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.5:** Resolving Settings to the current tab's workspace shall not replace a different shared-cookie value or save the tab's workspace preference.

## Out of scope

- Changing the active workspace cookie or saved preference after an explicit workspace selection.
- Workspace selection rules for cold page loads or Office route bootstraps.

## Traceability

- System design: [Per-tab Settings workspace context](../system-design/per-tab-settings-context.md)
- Decisions: [Active workspace cookie](../../../decisions/0023-active-workspace-cookie.md), [Office mode follows the active workspace](../../../decisions/2026-08-15-office-mode-follows-active-workspace.md)
