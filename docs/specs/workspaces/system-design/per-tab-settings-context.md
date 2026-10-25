---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001
---

# Per-tab Settings Workspace Context System Design

## Purpose and boundaries

The workspace system owns active workspace identity. This design covers
workspace selection during Settings hydration. It does not own the shared
cookie format or durable user settings contract. ADR 0023 defines cookie
selection, and the Office-mode decision defines how workspace type controls
shared chrome.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001` | Control flow |

## Components and responsibilities

- `SettingsRouteBootstrap` loads Settings state and reads the current active
  workspace from the tab's Zustand store.
- `loadSettingsInitialState` loads workspace and user-settings data through
  the existing frontend API clients.
- `resolveSettingsActiveWorkspaceId` validates candidate IDs against the
  returned workspace list and applies the Settings precedence.
- The workspace store owns the active ID for this tab. Its `setActiveWorkspace`
  action increments `activeIdRevision` when the active ID changes.
- The backend owns the available workspace list and the saved user preference.
  The browser cookie stores the shared active-workspace fallback.

## Data and contracts

The resolver accepts the available workspace IDs, current tab ID, shared
cookie ID, and saved preference ID. A candidate is valid only when its ID is
present in the available workspace list. The resolver does not filter by
Office or Kanban workspace type.

The order is current tab, shared cookie, saved preference, first available
workspace, then no active workspace. This behavior adds no API, event, backend
handler, or persisted field.

## Control flow

1. The Settings route mounts `SettingsRouteBootstrap`.
2. `loadSettingsInitialState` reads workspaces and user settings with the
   existing frontend API clients.
3. After the requests complete, the bootstrap reads the current tab's active
   workspace and passes it to `buildSettingsInitialStateForRoute`.
4. The resolver checks each candidate against the loaded workspace list and
   selects the first valid candidate in the defined order.
5. Hydration updates workspace and settings state. When the active ID changes,
   `SettingsRouteBootstrap` calls `setActiveWorkspace` so consumers receive a
   new `activeIdRevision`.
6. Settings renders with the resolved active workspace. No backend mutation is
   part of this flow.

## Failure and recovery

Workspace-list and user-settings read failures use the existing initial-state
fallbacks. Invalid candidates do not become active. If the workspace list is
empty, the resolver returns no active workspace. A later successful route load
can resolve the workspace again.

## Persistence

The active ID is tab-local Zustand state. The cookie is shared across tabs and
provides a fallback when the current tab's ID is missing or invalid. The saved
workspace preference remains backend-owned. Settings hydration does not copy a
valid tab-local choice into either shared selection source.

## Related decisions

- [ADR 0023: Active workspace cookie](../../../decisions/0023-active-workspace-cookie.md)
- [ADR 2026-08-15: Office mode follows the active workspace](../../../decisions/2026-08-15-office-mode-follows-active-workspace.md)
