---
status: current
system: ui
requirements:
  - REQ-UI-DIALOG-CONTENT-CONTAINMENT-001
---

# Growing Dialog Content Containment System Design

## Purpose and boundaries

This design applies one viewport-containment pattern to five dialogs whose
body height grows from user or runtime data. The UI system owns the bounded
shell, scroll ownership, and responsive geometry. Agent settings, marketplace,
system health, plugins, and Office retain ownership of their data and actions.

The design changes local dialog compositions. It does not change the shared
Dialog or AlertDialog primitive and does not introduce a new public component
API.

## Requirement mapping

| Requirement                                 | Design sections                                                                                                                               |
| ------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-DIALOG-CONTENT-CONTAINMENT-001`     | [Composition contract](#composition-contract), [Surface compositions](#surface-compositions), [Responsive interaction](#responsive-interaction) |

## Composition contract

Each covered dialog uses a dynamic-viewport height cap. Its layout has explicit
content-sized rows around a
`minmax(0, 1fr)` body row, or the equivalent flex composition. The body has
`min-height: 0`, vertical auto overflow, overscroll containment, and horizontal
overflow containment.

The body contains growing form content. Transient pickers can extend outside
the dialog shell when their surfaces remain inside the viewport. A clipping
shell must not cut off picker content or its interactive area.

The body is the only region newly responsible for vertical scrolling. Headers,
host close controls, and surface-specific persistent controls stay outside it.
The height cap is a maximum rather than a fixed height, so short content keeps
the current compact presentation.

Stable dialog and body selectors expose these layout boundaries to Playwright.
They are test seams, not product APIs.

## Surface compositions

### Agent-profile deletion conflict

`AgentProfileDeleteConflictDialog` uses an auto/minmax/auto AlertDialog grid.
The title remains in the header, all conflict groups and consequence text move
into the scroll body, and Cancel/Delete Anyway remain in the footer. Routing
tier hard blockers continue to omit Delete Anyway. The conflict body retains
the AlertDialog description relationship.

### Marketplace Sources

`MarketplaceSourcesDialog` keeps its title and description in the header. The
source-item list becomes the scroll body. `AddSourceForm` remains outside that
body in the final auto-sized row, so source name, URL, and Add remain available
while the source list scrolls. Toggle and remove controls stay with their source
items and are reachable through the body.

### System Health Issues

`HealthIssuesDialog` keeps its title and count description in the header. The
issue-card list becomes the scroll body. There is no persistent footer; each
Fix action remains inside its issue card and becomes reachable by scrolling.
Phone Fix actions gain a 44-pixel minimum touch height while desktop can retain
compact sizing.

### Plugin Modal Host

Only `PluginDialog` receives the bounded composition. Its optional host-owned
title and description remain in the header, and `PluginErrorBoundary` plus the
plugin content component move inside the scroll body. Because plugin content is
opaque, plugin-owned buttons scroll with that content; the host guarantees that
all of it can be reached. The host close control remains visible when the modal
is dismissible. Nondismissible outside-click and Escape guards remain intact.

`PluginDrawer` already caps itself at the dynamic viewport and gives plugin
content a bounded scroll region, so its composition remains unchanged.

### Office Create Project

`CreateProjectDialog` keeps the title in the header, places `ProjectFormBody`
in the scroll body, and keeps Cancel/Create Project in the final auto-sized row.
Repository chips remain part of the form body and can grow or wrap without
pushing the footer outside the viewport.

`CreateProjectDialog` retains the existing height cap, grid rows, and scrolling
body. Its shell uses visible overflow so a floating picker can extend beyond
the shell. The body retains horizontal overflow containment and vertical auto
overflow; the shell does not become a horizontal scroll container.

The dialog captures its mounted `DialogContent` element with a callback ref.
`ProjectFormBody` and `ReposField` pass that element to
`ProjectRepositoryPicker` through an optional `portalContainer` prop. The
picker uses the existing `PopoverContent` portal support. Its portal targets
the dialog shell, outside the scrolling body. The project-detail consumer omits
the prop and uses the default document-body portal.

This follows the dialog-local portal pattern in `TaskCreateDialog`. It keeps
the picker inside the modal DOM boundary that owns focus and scroll locking.
It does not require a shared context, a new overlay primitive, or task-specific
context imports into Office.

The picker retains its 420-pixel preferred width. Its maximum width leaves
16 pixels on each side of the viewport. Radix collision padding uses the same
inset. The available-height constraint keeps the search control visible and
lets only the command list scroll. The command layout permits that list to
shrink within the available height.

Phones retain the click/search combobox, with the same filtering and selection
state. This temporary, flat choice fits a contained popover without another
navigation surface. Phone and coarse-pointer controls retain their existing
44-pixel minimum targets. A shorter viewport shrinks the list rather than
clipping the search control or creating document horizontal overflow.

Repository selection, query reset, lazy discovery, exclusions, and project
submission retain their existing callbacks and values. Escape dismisses the
picker before the project dialog and returns focus to its actual trigger.

## Components and responsibilities

- Each dialog component owns its local row composition and stable layout test
  selectors.
- Existing child components continue to own source mutations, health routing,
  plugin rendering, profile deletion callbacks, and Office form state.
- `@kandev/ui/dialog` and `@kandev/ui/alert-dialog` continue to own focus traps,
  portals, accessible names, Escape handling, and outside-interaction behavior.
- Browser tests own geometry assertions. Component tests cover conditional
  actions and callback semantics but do not substitute jsdom for layout proof.

## Data and contracts

No API, persisted state, store, event, or plugin SDK changes. Each surface uses
its existing data shape and callback contract. The only new contracts are local
DOM layout boundaries used for browser verification.

## Responsive interaction

- Centered dialog presentation remains unchanged for the four app-owned
  dialogs and for plugin content that requests dialog presentation.
- Dynamic viewport units account for browser chrome changes on phones.
- Narrow layouts wrap or stack persistent actions without creating document
  horizontal overflow.
- Required actions meet the 44-pixel phone touch minimum; desktop retains its
  existing compact density.
- Desktop and phone expose the same data, item actions, dismissal rules, and
  completion outcomes.

## Control flow

1. Existing data or plugin content opens a covered dialog.
2. The dialog renders its header, designated body, and any persistent controls.
3. If content exceeds the height cap, only the body becomes scrollable.
4. Scrolling changes the body's scroll offset without moving the outer dialog
   or persistent rows.
5. Existing item actions, completion actions, and dismissal callbacks execute
   unchanged.

## Failure and recovery

- A shorter viewport or newly added item shrinks or extends the body scroll
  range while persistent rows remain visible.
- Omitting either `minmax(0, 1fr)` or `min-height: 0` can restore content-sized
  growth and move controls beyond the viewport. Desktop and phone geometry
  regressions guard both constraints.
- Plugin render failures continue through `PluginErrorBoundary` inside the
  bounded body.
- Closing and reopening continues to reset transient scroll state through the
  existing Radix mount lifecycle.

## Persistence

None. Open state and scroll position remain transient.

## Security

No trust boundary changes. Plugin content remains isolated by the existing
error boundary, and all destructive or navigational actions keep their current
authorization and request paths.

## Observability and verification

No production telemetry is added. Focused Chromium and mobile-Chrome browser
tests exercise long content on every surface and assert outer-dialog viewport
containment, a body with real scroll range, reachable final content, visible
persistent controls where applicable, touch-safe required actions, and no
document horizontal overflow. Existing focused component tests continue to
cover callbacks, dismissibility, and conditional action rules.

Office picker regressions also measure the painted interactive area outside
the dialog bounds. They use right-edge hit testing and real click, wheel,
keyboard, and touch input. DOM visibility alone cannot establish that an
ancestor does not clip a picker. Existing long-chip cases continue to verify
the body scroll range, persistent footer, and project submission path.

## Implementation plans

- [Original dialog containment package](../../../plans/dialog-content-containment/plan.md).
- [Office repository picker correction](../../../plans/office-project-picker-clipping/plan.md).

## Related decisions

None. This applies an established local Dialog composition and does not create
a new architectural boundary.
