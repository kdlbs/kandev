---
status: shipped
created: 2026-07-17
owner: kandev
---

# Direct Sidebar View Creation

## Why

Creating a sidebar view currently requires changing an existing view before `Save as…` becomes available. This makes a basic organizational action feel hidden and risks teaching users that editing an existing view is part of creating a new one.

## What

- The desktop Tasks view picker ends with a separated `New view` action. The label has no ellipsis because selection creates the view immediately rather than opening a pre-creation dialog.
- The mobile task-switcher sheet exposes a fixed `New view` action beside the horizontally scrolling saved-view chips and filter control. It remains reachable without hover or horizontal page scrolling and has at least a 40×40 px hit area.
- Selecting `New view` immediately appends and activates a saved view with canonical defaults: no filters, `state` ascending sort, `repository` grouping, and no collapsed groups.
- Canonical defaults do not inherit from the active view, an active draft, or a user-modified `All tasks` view.
- The automatic name is the lowest exact name not already present: `New view`, then `New view 2`, `New view 3`, and so on. Existing manually duplicated names remain valid.
- After creation, the existing filter popover opens with its rename input focused and prefilled with the automatic name. Saving a rename keeps the popover open so the user can configure filters, sort, and grouping next.
- Rename is optional. Canceling rename, pressing Escape, or closing the popover keeps the already-created view under its automatic name.
- `Save as…` remains available for deriving a view from unsaved changes; direct creation does not replace or change that workflow.
- When the active view has unsaved changes, direct creation is disabled and explains that the user must save or discard those changes first. No draft is silently cleared.
- A user may save at most 50 sidebar views. At 50 views, direct creation is disabled with a clear limit reason; backend validation remains authoritative.
- The filter popover fits narrow viewports without document-level horizontal overflow.

## Persistence and failure behavior

- Creation persists the new view and active-view selection immediately through existing backend-owned user settings. A successful create survives reload and is available on the user's other clients.
- A failed create restores the previous saved views, active view, and draft, then surfaces the existing sidebar-view sync error. It never leaves a selected view that is absent from persisted settings.
- Each new view owns independent filter, sort, and collapsed-group data; later changes to it cannot mutate the canonical default or another view by shared reference.

See [ADR 0041](../../decisions/0041-backend-owned-portable-user-settings.md) for backend ownership of portable user settings.

## Scenarios

- **GIVEN** fewer than 50 saved views and no unsaved draft, **WHEN** the user selects `New view`, **THEN** a canonical default view is appended, activated immediately, and offered for focused rename.
- **GIVEN** the active saved view has custom filters, sort, grouping, or collapsed groups, **WHEN** the user creates a new view, **THEN** the new view still uses the canonical defaults rather than cloning the active state.
- **GIVEN** saved views named `New view` and `New view 3`, **WHEN** the user creates a new view, **THEN** its automatic name is `New view 2`.
- **GIVEN** a newly created view is in rename mode, **WHEN** the user cancels or closes the popover, **THEN** the view remains saved and active under its automatic name.
- **GIVEN** a newly created view is in rename mode, **WHEN** the user saves a nonblank name, **THEN** the name persists and the filter popover remains open for customization.
- **GIVEN** an unsaved sidebar-view draft, **WHEN** the user inspects direct creation on desktop or mobile, **THEN** `New view` is disabled and communicates that the draft must be saved or discarded first.
- **GIVEN** 50 saved sidebar views, **WHEN** the user inspects direct creation on desktop or mobile, **THEN** `New view` is disabled and communicates the saved-view limit.
- **GIVEN** the create settings write fails, **WHEN** retry handling finishes, **THEN** the prior sidebar-view state is restored and the sidebar-view error is shown.
- **GIVEN** a narrow mobile viewport, **WHEN** the task-switcher sheet opens, **THEN** `New view` is touch-reachable, the chip row still scrolls, and neither the bar nor popover causes horizontal page overflow.

## Out of scope

- Changing `Save as…`, duplicate-view, rename, delete, or saved-view ordering semantics.
- A pre-creation naming dialog, cancel-to-delete behavior, or delayed persistence until rename.
- Cloning the active view from the direct-create action.
- Backend schemas, new endpoints, or a new cross-tab conflict model for full-array user-settings writes.
- Broad sidebar visual redesign or animation dependencies.
