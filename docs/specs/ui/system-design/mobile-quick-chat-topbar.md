---
status: draft
system: ui
requirements:
  - REQ-UI-MOBILE-QUICK-CHAT-TOPBAR-001
---

# Mobile Workspace Topbar System Design

## Purpose and boundaries

This UI-owned reusable composition covers phone Kanban, List, and Threads.
It replaces the Kanban/List scrolling action strip with the quiet Threads
header while keeping listing state and workspace tools in their current owners.
`useResponsiveBreakpoint().isMobile` selects this composition; tablet,
desktop, task-session headers, and plugin-owned routes retain their layouts.

The closest shipped exemplars are `MobileColumnTabs` for visible current
context and `MobileMenuSheet` for inset, safe-area-aware navigation. The
current Threads page contributes the 56-pixel topbar, unboxed stacked title,
and 44-pixel ghost menu button. The always-visible header carries orientation;
the drawer carries temporary navigation and less frequent workspace tools.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-MOBILE-QUICK-CHAT-TOPBAR-001` | [Shared header](#shared-header), [Menu capabilities](#menu-capabilities), [Responsive geometry and focus](#responsive-geometry-and-focus) |

## Shared header

`KanbanHeader` continues to own responsive dispatch and passes existing page,
workspace, search, and listing-control inputs to `KanbanHeaderMobile`.
`KanbanHeaderMobile` renders one `PageTopbar` composition for all phone listing
pages: height/min-height 56 pixels, `freeWidth="lead"`, no separate brand or
home breadcrumb, a shrinking title slot, and a fixed 44-pixel ghost menu button.
No conditional phone branch retains the old action strip.

The title slot uses the same two-line typography, spacing, truncation, and
chevron treatment, with these mode-specific contents:

| Mode | Context line | Primary line | Tap destination |
| --- | --- | --- | --- |
| Kanban | Existing workspace label | Localized Kanban label | Existing listing menu |
| List | Existing workspace label | Localized List label | Existing listing menu |
| Threads | Localized Threads label | Existing active saved-view name | Existing saved-view drawer |

Kanban/List reuse the already-resolved workspace label, including its loading
or no-workspace fallback. Labels are rendered with existing i18n keys and are
not used as routing discriminants. The current route determines the visible
mode; the phone's effective Pipeline fallback never overwrites desktop state.

Threads supplies `ThreadsViewControls` and `MobileThreadPagination` through
the existing `taskListingControls` slot. Its
[deck design](threads-conversation-deck.md#phone-position-feedback) owns swipe
position. The shared header knows neither transcript state nor thread order.
Kanban/List get a small presentation-only context button, not another saved
view model. Shared appearance is factored only where needed to prevent the
three phone controls drifting; no new global configuration is introduced.

## Menu capabilities

Generalize `MobileThreadsMenuActions` into `MobileListingMenuActions` and pass
the actual `currentPage` instead of a Threads constant. `MobileMenuSheet`
retains its inset drawer and existing display/state hook. Its phone content
owns these capabilities:

- `AppNavSections` exposes Home through the existing manifest. Do not omit the
  whole primary section after removing the brand link. Omit redundant Tasks
  and Threads destinations already covered by the mode selector, without
  changing manifest routing or tablet navigation policy.
- Workspace picker, repository/workflow filters, board column visibility,
  List sort/group/archive/detail controls, and mode switching keep their
  existing handlers in `useMobileMenuSheetState` and listing owners.
- Quick Chat and Quick Terminal reuse their launcher hooks and workspace IDs.
  Rows close the drawer before opening the destination on the next frame,
  following the existing `useAppNavDialogs` pattern. Their dialog ownership
  remains outside the unmounted menu content.
- The existing phone search toggle moves into the menu. On Kanban/List it
  closes the menu before revealing/focusing `MobileSearchBar`, which retains
  the existing `mobileKanban.isSearchOpen` state and query callbacks. Closing
  search clears the query. Do not leave an independently editable duplicate
  phone search input in the drawer; the tablet menu search remains unchanged.
- `MainTopBarPluginActions` retains the `main-top-bar` slot, workspace label,
  `presentation="mobile"`, and actual page identity. Contributions remain
  mounted only in their intended presentation, not hidden in a second header.
  `MobileWorkspaceActionsSection` retains its separate slot and canvas links.
- Status continues through the existing `AppNavSections` integration. When
  the ordinary status surface is off but metrics are opted in, the open menu
  uses `StatusSurfaceMetrics` with `presentation="mobile-drawer"`, compact
  density, and the actual drawer-open value. Do not force-enable status,
  hide opted-in metrics, or keep a closed-phone-menu subscription alive.

The persistent menu button retains the current connection warning. It also
mirrors existing `useQuickChatActivity` feedback so moving the launcher into a
drawer does not hide background work/completion while the drawer is closed.
Reuse `QuickChatActivityIndicator` on the menu button and its Quick Chat row;
keep connection and activity cues distinguishable and do not cover either
with the other. Accessible menu copy includes applicable warning/activity
descriptions. No notification ledger or subscription is added.

## Responsive geometry and focus

The header remains one fixed-height in-flow row. The page's existing content
retains vertical scrolling; the Threads board retains its own horizontal snap
scrolling. The header never owns a horizontal scroller. Search adds space only
while explicitly opened, not to the normal idle topbar.

`MobileMenuSheet` remains the single vertical scroll owner while open, with
dynamic viewport height, safe-area clearance, and a fixed drawer header.
Long labels shrink or wrap inside their rows. Context and menu controls have
44-pixel active targets; menu utility rows use that same minimum. Tablet and
fine-pointer controls retain their current density.

Both the context button and menu button can open the same drawer on Kanban or
List. Remember the actual opener locally so dismiss returns focus correctly;
do not always focus the right-hand button. Launching a dialog or search must
not return focus into a closing drawer. Pass that persistent opener's ref to
the existing Quick Chat/terminal focus capture so closing the launched surface
returns there, not to a detached menu row. No separate focus owner is added.
Threads keeps its saved-view and task pickers' existing focus-return behavior.

## Failure, persistence, and security

No backend, API, schema, saved preference, plugin contract, or permission
changes occur. Missing workspaces omit unusable launchers. Existing menu
fallback labels, connection severity, metrics availability, plugin errors,
and disabled actions retain their existing owners. The explicit default-Home
preference requested by the user belongs to a separate Kandev subtask.

## Verification design

`kanban-header-mobile.test.tsx` becomes a three-mode contract test, including
route-derived labels, menu capability delegation, no-workspace behavior,
activity/connection feedback, and direct slot composition. Focused menu tests
cover page-specific plugin props, search clearing, close-before-launch, and
focus return from each opener. Existing `mobile-menu-sheet.test.tsx`,
`mobile-menu-utility-actions.test.tsx`, and listing navigation tests protect
unchanged business logic.

`mobile-kanban-topbar.spec.ts` checks the shared 56-pixel geometry on Kanban,
List, and Threads, mode/context actions, Home routing, long labels, and zero
document overflow. `mobile-plugin-topbar.spec.ts` checks menu contribution
reachability, touch targets, enabled-metrics fallbacks, and absence of a
horizontal header strip. Existing mobile search, Quick Chat, terminal,
activity, saved-prompt delivery, and status tests switch to menu entry points
without dropping their original outcome assertions.

Phone screenshot checks at 360 and 393 pixels compare all three modes;
820-pixel coarse-pointer and desktop regressions protect the unchanged
responsive branches. The existing isolated Tailscale demo is rebuilt and
refreshed after implementation without reseeding or restarting its backend.

## Related decisions

- [Navigation manifest boundaries](../../../decisions/2026-08-04-navigation-manifest-boundaries.md)
- [Viewport activation owns thread streams](../../../decisions/2026-08-28-viewport-activation-owns-thread-streams.md)
