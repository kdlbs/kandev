# Kandev Mobile UI Language

Use this reference to turn a desktop capability into a phone-native Kandev experience. Capability parity does not mean layout parity. Preserve user outcomes, permissions, state, and error handling; change composition and interaction when a narrow touch viewport needs it.

This curated guide and its exemplar list define the desired mobile interaction baseline. Other live code remains authoritative for APIs and current behavior, but a nearby surface may predate this language; do not copy a deviation merely because it is close to the new feature.

## Core Principles

1. **One focal task at a time.** Replace multi-pane and multi-column desktop layouts with a focused mobile surface. Make hierarchy visible through a compact navigator, then let the main list or content own vertical scrolling.
2. **Primary tap goes somewhere useful.** When a card or row has an obvious main destination and no competing selection, drag, or inline-control behavior, its body tap should perform that action. Otherwise provide a visible explicit control. Put secondary actions behind an ellipsis or labeled control; do not make long press, hover, or right-click the only path.
3. **Temporary choices rise from the bottom.** Phone pickers, menus, and navigation choices normally use an inset bottom `Drawer`. Keep side sheets and anchored popovers for tablet/desktop unless a nearby phone surface establishes another pattern.
4. **Mobile can simplify presentation, not capability.** It may focus one workflow, hide an unsuitable visualization, or navigate instead of previewing. Required actions remain reachable, and mobile-only fallbacks must not overwrite desktop preferences.
5. **Viewport geometry is product behavior.** Safe areas, browser chrome, on-screen keyboards, scroll ownership, and touch target size are part of correctness.
6. **Share logic, specialize composition.** Reuse store state, domain hooks, view-model derivation, filtering/selection, permissions, and action handlers. Keep viewport wrappers focused on rendering. Use a dedicated mobile layout or responsive surface wrapper when desktop structure itself is wrong for phones.

## Surface Decision Guide

| Desktop interaction | Preferred phone composition | Notes |
|---|---|---|
| Dropdown or context menu | Visible trigger plus compact bottom-sheet menu | Reuse existing Radix menu primitives and their responsive treatment. Test nested menus and long content. |
| Hover disclosure | Fine-pointer `Popover`; coarse-pointer `Drawer` | Use `useTouchDrawer`; hover cannot be the only touch path. |
| Click/search combobox popover | Keep Popover when touch-usable and viewport-contained; otherwise use `Drawer` | Base the choice on option count, search depth, keyboard behavior, and nearby curated precedent. |
| Temporary sidebar or navigation rail | Inset bottom `Drawer` | Give it a fixed header, `min-h-0` scrolling body, and bottom safe-area padding. |
| Task-workbench header picker | `MobilePillButton` opening `MobilePickerSheet` | Show current context and chevron so the control reads as interactive. Outside the task domain, reuse the interaction pattern without importing a domain-owned component blindly. |
| Multi-column board or long horizontal tabs | One focused column/item plus current-context navigator | Offer previous/next or swipe shortcuts and a drawer containing the full hierarchy. Avoid document-level horizontal scroll. |
| Master/detail or floating preview | Direct navigation or a dedicated full-height mobile surface | Do not squeeze both panes onto one screen or add an unnecessary intermediate action sheet. |
| Dense content viewer such as diff, file, or chat | Dedicated route/layout or full-height drawer | Preserve one clear internal scroll owner and use `100dvh`/`h-dvh`. |
| Persistent desktop toolbar | Compact top/bottom controls; optional safe-area-aware FAB for the primary action | Keep the primary action visible and thumb-reachable. Move secondary actions into an explicit overflow surface. |
| Desktop-only visualization | Mobile-native effective fallback | Derive the fallback at render/selection time; do not persist it over the user's desktop choice. |

Choose based on task frequency and content depth. A frequently revisited primary destination may deserve a route or bottom navigation instead of a drawer. A short, temporary choice should not take over the whole screen.

## Composition and Feel

- Phone breakpoint is below 640px. Use `useResponsiveBreakpoint`; it distinguishes phone, tablet, compact desktop, and full desktop using width plus pointer mode.
- Reuse `@kandev/ui` primitives. `Drawer` is the standard bottom surface; `Sheet` remains useful for tablet/desktop side panels.
- Use inset, rounded card surfaces with restrained borders/backgrounds and shadow, matching nearby components. Prefer existing component classes over copying a frozen class list.
- Give new primary touch actions, standalone icon buttons, and menu rows an actual hitbox of at least 44px in the active dimension (`min-h-11`, `h-11 w-11`). Existing compact chrome such as `MobilePillButton` is a deliberate density exception, not a sizing precedent for unrelated controls; do not claim surrounding spacing enlarges its hitbox. Provide pressed feedback such as the existing short `active:scale-*` treatment where nearby controls use it.
- Keep labels and current context visible. Use truncation for long names, tabular number badges for counts, and accessible names that include enough hierarchy to disambiguate the action.
- Use `100dvh`/`h-dvh`, not `100vh`, for viewport-filling phone layouts. Account for `env(safe-area-inset-*)` on fixed controls and drawer content.
- In flex surfaces, the usual scroll body is `min-h-0 flex-1 overflow-y-auto overscroll-contain`. Keep headers/actions fixed and avoid nested vertical scrollers unless the interaction truly needs them.
- Keep page/document horizontal overflow at zero. A deliberately scrollable code/table region must contain its own overflow without making navigation two-dimensional.
- Let the existing coarse-pointer input rule keep editable controls at 16px or larger so iOS does not zoom on focus.
- Preserve focus movement, Escape/back dismissal, focus return, and semantic button/link behavior. Touch-first does not mean keyboard-inaccessible.

## State and Responsive Boundaries

- Share domain state and mutations between mobile and desktop. Split presentation before duplicating business logic.
- Branch early when the desktop workbench is expensive or structurally unsuitable. Avoid mounting it and hiding it with CSS on phones.
- Treat tablet separately when current routing does. A phone bottom drawer is not automatically the right tablet surface.
- Keep responsive fallbacks effective rather than persisted. Example: render the mobile-supported view while retaining the user's saved desktop view.
- When live data removes the focused item, choose a deterministic visible fallback. Do not leave an empty shell or update state in a render/effect loop.
- When switching context inside an open drawer, keep it open if the next choice is part of the same hierarchy; close it when the user's selection completes the task.

## Current Code Precedents

Inspect these live files before implementing; use `rg` to find their replacements if paths move.

- `apps/web/hooks/use-responsive-breakpoint.ts` — canonical breakpoint and pointer model.
- `apps/web/hooks/use-compact-task-chrome.ts` — `useTouchDrawer` for coarse-pointer alternatives to hover disclosures.
- `apps/web/components/task/task-layout.tsx` — dedicated mobile, tablet, and desktop compositions sharing task data.
- `apps/web/components/kanban/mobile-menu-sheet.tsx` — phone `Drawer` versus wider `Sheet`, inset card, fixed header, safe-area-aware internal scroll.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` — rich task navigation in an inset bottom drawer while wider layouts retain a side sheet.
- `apps/web/components/task/mobile/mobile-picker-sheet.tsx` and `mobile-pill-button.tsx` — reusable bottom picker and discoverable header trigger.
- `apps/web/components/kanban/mobile-column-tabs.tsx` — one focused board step, visible workflow/step context, previous/next shortcuts, and combined hierarchy drawer.
- `apps/web/components/kanban-with-preview.tsx` — direct phone navigation instead of a compressed desktop preview pane.
- `apps/web/components/kanban/mobile-fab.tsx` — safe-area-aware, thumb-reachable primary action.
- `apps/web/app/globals.css` — phone menu containment, 44px menu rows, safe-area utilities, and coarse-pointer input sizing.

The global Radix menu override is a compatibility layer, not a template for more broad selectors. Prefer a scoped class or shared responsive primitive for new behavior, and cover long and nested menus because parent and submenu surfaces are both portaled.

## Anti-Patterns

- stacking every desktop panel vertically and calling it responsive
- rendering a desktop side sheet at `width: 100%` instead of choosing a phone surface
- preserving a wide table, tab strip, board, or toolbar through page-level horizontal scrolling
- adding an intermediate sheet before the card's obvious primary destination
- hiding required actions or relying on hover, right-click, or long press
- custom overlay markup when an existing `Drawer`, picker, dialog, or menu primitive fits
- stacking drawers/popovers for consecutive choices that belong in one navigable surface
- fixed bottom controls that ignore safe areas or on-screen keyboard overlap
- multiple ambiguous scroll owners inside a viewport-height surface
- cloning action/state logic into mobile components
- persisting a mobile fallback over a desktop preference

## Verification Focus

Use the configured `mobile-chrome` Pixel 5 project and assert user outcomes. Add geometry assertions when containment is the behavior under test:

- primary action is visible and completes the same outcome as desktop
- drawer/menu remains inside the viewport and long content scrolls internally
- new primary actions, standalone icon controls, and menu rows meet the 44px hitbox expectation
- nested choices remain usable without horizontal overflow
- document `scrollWidth` does not exceed `clientWidth`
- fixed controls and final rows clear the bottom safe area
- direct navigation, back/dismiss, and focus return behave predictably
- mobile fallback leaves the stored desktop preference unchanged

Also inspect a rendered phone viewport or screenshot. Tests can prove reachability while still missing a cramped hierarchy, awkward sheet height, or desktop-looking composition.
