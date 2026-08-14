---
status: building
created: 2026-08-14
owner: Kandev
---

# Active workspace first in settings

## Why

When several workspaces are available, the workspace currently in use can be
buried below another workspace in Settings. Users should see the workspace
that receives their next settings action first in both relevant Settings
surfaces.

## What

- The workspace list on `/settings/workspaces` places the active workspace first.
- The workspace records shown by the Settings sidebar tree place the active
  workspace first.
- The active workspace is the workspace whose id matches the existing
  `workspaces.activeId` state.
- All non-active workspaces retain their existing relative order.
- When there is no active workspace, or the active id is not present in the
  list, the incoming workspace order remains unchanged.
- The active badge and active styling continue to identify the same workspace;
  ordering does not change workspace selection or persistence.
- The same ordering is visible on the phone Settings index, which renders the
  Settings tree, without changing the phone layout or interaction model.

## Scenarios

- **GIVEN** three workspaces in the order `First`, `Active`, `Last`, **WHEN**
  the Settings sidebar tree renders with `Active` as `workspaces.activeId`,
  **THEN** the workspace records appear as `Active`, `First`, `Last` and
  `Active` retains its active badge.
- **GIVEN** three workspaces in the order `First`, `Active`, `Last`, **WHEN**
  `/settings/workspaces` renders with `Active` as `workspaces.activeId`,
  **THEN** the workspace cards appear as `Active`, `First`, `Last` and
  `Active` retains its active styling.
- **GIVEN** the active workspace is already first, **WHEN** either Settings
  surface renders, **THEN** the list stays in its existing order.
- **GIVEN** no active workspace exists, or the active id is not in the returned
  list, **WHEN** either Settings surface renders, **THEN** the returned order is
  preserved and no workspace is incorrectly marked active.
- **GIVEN** the phone Settings index is open in a tree menu mode, **WHEN** the
  Workspaces branch is expanded, **THEN** it shows the same active-first order
  as the desktop Settings sidebar.

## Out of scope

- Reordering the global workspace store or changing the order of workspace
  pickers and non-Settings workspace consumers.
- Persisting a new workspace sort preference.
- Changing which workspace is active, the active-workspace cookie, workspace
  selection behavior, or workspace creation/deletion behavior.
- Redesigning the Settings sidebar or adding a separate mobile navigation
  surface.
