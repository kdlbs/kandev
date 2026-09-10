---
status: current
system: ui
requirements:
  - REQ-UI-MOBILE-TASK-NAVIGATION-001
---

# Mobile Menu Backdrops System Design

## Purpose and boundaries

The UI system owns the reusable responsive menu presentation in
[Mobile Task Navigation](../requirements/mobile-task-navigation.md). Task,
workspace, session, and integration systems continue to own the actions and
state exposed through these menus. This design covers the backdrop portion of
that existing contract, including non-task consumers of the same primitives.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| AC-UI-MOBILE-TASK-NAVIGATION-001.3, 001.9 | Shared presentation |
| AC-UI-MOBILE-TASK-NAVIGATION-001.10 | Nested surfaces |
| AC-UI-MOBILE-TASK-NAVIGATION-001.11 | Lifecycle and interaction |
| AC-UI-MOBILE-TASK-NAVIGATION-001.12 | Shared presentation and compatibility |

## Components and responsibilities

- `apps/packages/ui/src/dropdown-menu.tsx` and
  `apps/packages/ui/src/context-menu.tsx` identify root content with a shared
  `mobile-menu-root` class. Submenu content does not receive the marker.
- `apps/web/app/globals.css` owns responsive geometry and backdrop styling,
  scoped to the generated positioning wrapper immediately containing that
  marked root content.
- `apps/packages/ui/src/drawer.tsx` is the visual reference for the backdrop:
  `bg-black/80` and supported `backdrop-blur-xs`. Its implementation stays
  unchanged.
- Existing Radix roots own open state, presence, accessibility, dismissal,
  focus, and outside interaction. Consumer components retain their handlers.

## Shared presentation

Extend the existing `max-width: 639px` menu rule. Generate one decorative
`::before` backdrop on the positioning wrapper whose direct child has both
`mobile-menu-root` and `data-state="open"`. Use a fixed viewport inset, the
Drawer backdrop utilities, `pointer-events: none`, and a negative stacking
level within the menu wrapper's existing stacking context. Keep the menu and
its children above the backdrop.

The phone positioning rule removes the wrapper transform and resets Radix's
inline `will-change: transform` hint. Both must be cleared so the fixed
pseudo-element covers the viewport instead of using the wrapper as its
containing block. The backdrop belongs outside the menu's scrolling
content so long menus cannot clip it. Do not apply a filter to the document,
application container, positioning wrapper, or menu content itself.

Use the shared marker to scope this behavior. Do not add document-wide
open-menu selectors or per-consumer backdrop implementations. No React state,
observer, portal wrapper, event listener, or package dependency is needed.

## Nested surfaces

Only root content carries the marker. `DropdownMenuSubContent` and
`ContextMenuSubContent` reuse the root backdrop while preserving their
existing bottom-sheet geometry. Opening or closing a submenu does not create
another menu backdrop or change the root backdrop strength.

Menus opened inside a Drawer have two independently owned surfaces: the
existing Drawer backdrop behind the drawer and the menu backdrop behind the
menu. Closing the menu removes only its backdrop. Verify the portaled menu
remains above the parent drawer and that nested menu rows are actual hit
targets.

## Lifecycle and interaction

The `data-state="open"` selector follows Radix state directly. Closed content,
including content retained for exit motion or force mounting, cannot retain
a visible menu backdrop. Unmounting removes the positioning wrapper and its
pseudo-element. Crossing the CSS breakpoint removes or restores the backdrop
without changing menu state.

The pseudo-element is decorative and is absent from the accessibility tree.
It must not receive pointer events or introduce another focus or scroll lock.
Radix continues to handle modal and non-modal menus. In particular,
`MobileWorkspaceSection` passes `modal={false}` to the workspace picker inside
the Home drawer; the backdrop does not change that interaction contract.
Radix's submenu close key returns to the parent menu, while Escape closes the
whole menu hierarchy. Neither operation closes an enclosing Drawer.

## Mobile composition and compatibility

The nearest shipped exemplars are `components/kanban/mobile-menu-sheet.tsx`
and `components/task/mobile/mobile-picker-sheet.tsx`. Their dimmed, blurred
backgrounds establish the visual treatment. Contextual menus keep the existing
inset bottom surface because they expose short, temporary choices.

Entry points, menu hierarchy, and primary actions stay with each consumer.
The current menu content remains the single vertical scroll owner with its
`70dvh` cap, safe-area inset, and 44px minimum rows. The backdrop adds no scroll
region. The action/state model is shared across viewports.

At 640px and above the backdrop is not generated and the current anchored
menu geometry applies. This boundary intentionally differs from the 768px
application layout breakpoint. Drawer, Sheet, ordinary Dialog, Select,
Popover, and full-height Quick Chat contracts are outside this change.

The support-gated blur retains the dark layer when filtering is unavailable.
Verify both light and dark themes; the foreground must not inherit blur.

## Verification

Browser tests inspect the root positioner's computed `::before` style, not
only a class name. They verify a viewport-covering fixed layer, positive blur
when supported, dimming matching an existing Drawer, negative local stacking,
and pointer transparency. A screenshot confirms that actual page content is
blurred and menu text remains sharp.

Exercise real Kanban task options, task-row context submenus, and the non-modal
workspace picker. Assert menu actions still work, parent drawers survive menu
dismissal, outside tap and Escape clean up, and document horizontal overflow
remains zero. Count generated backdrops across root and submenu positioners
to catch accidental duplication. Cover 639px, 640px, and an ordinary desktop
width, including resizing while a menu is open.

Use the focused commands and named regressions in the
[work order](../../../plans/mobile-menu-backdrops/task-01-add-menu-backdrops.md).
No unit test is needed for static marker and CSS changes; browser evidence is
the relevant rendering and interaction boundary.

## Related decisions

No new ADR is required. This change extends the established shared menu CSS
and Drawer visual treatment without changing component APIs, action ownership,
persistence, or runtime boundaries.
