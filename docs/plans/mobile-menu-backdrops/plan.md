---
created: 2026-09-10
status: complete
requirements:
  - REQ-UI-MOBILE-TASK-NAVIGATION-001
system_design:
  - ../../specs/ui/system-design/mobile-menu-backdrops.md
legacy_specs: []
---

# Implementation Plan: Mobile Menu Backdrops

## Overview

Give every shared dropdown/context bottom sheet the background dimming and
blur already used by mobile drawers. One work order adds focused browser
regressions, applies shared root markers and CSS, and verifies nested menus
and desktop compatibility.

The requirements are amended in
[Mobile Task Navigation](../../specs/ui/requirements/mobile-task-navigation.md),
criteria 001.9 through 001.12. The owning contract remains UI presentation;
adjacent task and workspace specifications own state and actions. Existing
criterion 001.3 covers menu geometry but did not require background blur.

## Confirmed cause and audit

Source trace at `796bf5853`:

1. `KanbanCardMenu` in `apps/web/components/kanban-card-content.tsx` renders
   `DropdownMenuContent` for the visible **More options** action.
2. `apps/web/app/globals.css` converts dropdown/context root and submenu
   positioners into bottom sheets below 640px. It sets geometry, scrolling,
   and touch targets but creates no backdrop.
3. Both shared menu primitives render content without an overlay. In contrast,
   `DrawerContent` and `SheetContent` include overlays with supported blur.
4. The existing test `renders kanban card dropdown as a mobile bottom sheet`
   checks bounds and row height but never checks backdrop rendering.

The source audit identifies these affected entry points. Rows describe paths
through the shared primitives, not a claim that every path was browser-tested.

| Surface | Source | Backdrop finding |
| --- | --- | --- |
| Kanban card More options | `components/kanban-card-content.tsx` | Root dropdown has no backdrop |
| Task-list and phone task-switcher Task actions | `components/task/task-item-menu-button.tsx`, `components/task/task-switcher-context-menu.tsx` | Explicit button opens the shared context menu |
| Task options such as Move to, Link, and Priority | `components/kanban-card-menu-items.tsx`, `components/task/task-switcher-context-menu.tsx` | Nested choices need to retain the root backdrop |
| Phone session options and handoff choices | `components/task/mobile/mobile-sessions-section.tsx` | Dropdown inside a phone picker |
| Home workspace selector | `components/kanban/mobile-menu-sheet.tsx`, `components/app-sidebar/app-sidebar-workspace-picker.tsx` | Non-modal dropdown inside an existing Drawer |
| File-tree touch actions | `components/task/file-browser-parts.tsx` | Explicit touch dropdown uses the same root |
| Topbar breadcrumb and action overflow | `components/page-topbar.tsx` | Non-task dropdowns share the missing treatment |
| Quick Chat add-tab menu | `components/quick-chat/quick-tab-add-menu.tsx` | Menu inherits the same phone presentation |
| Integration start-task choices | `components/integrations/integration-start-task-menu.tsx` | Shared dropdown inherits the fix wherever rendered below 640px |
| Home navigation, board navigator, task/session picker, status drawers | `components/kanban/mobile-menu-sheet.tsx`, `components/task/mobile/mobile-picker-sheet.tsx`, `components/app-status-bar/app-status-drawer.tsx` | Existing Drawer overlay already specifies blur |
| Wider Sheet surfaces and AlertDialog | `apps/packages/ui/src/sheet.tsx`, `apps/packages/ui/src/alert-dialog.tsx` | Existing overlay already specifies blur |
| Ordinary Dialog and mobile Quick Chat surface | `apps/packages/ui/src/dialog.tsx`, `components/quick-chat/quick-chat-modal.tsx` | Separate contracts; no generic blur change |

Component paths without a prefix in this table are relative to `apps/web/`.
The mobile Kanban card disables its separate long-press context wrapper;
the reported trigger is the dropdown, not `KanbanCardContextMenu`.

## Scope

### In scope

- Shared root-menu marker and a viewport-covering backdrop below 640px.
- Root/submenu lifecycle, enclosing Drawer behavior, and non-modal compatibility.
- Browser regression coverage and rendered evidence for the existing action flows.

### Out of scope

- Changes to task actions, permissions, state, routes, or data persistence.
- Menu redesign, new drawers, changing the 640px menu breakpoint, or widening
  blur to dialogs, popovers, Select, and editor toolbars.
- General UI cleanup, new copy, translations, package changes, or feature flags.

## Technical approach

Implement the [design](../../specs/ui/system-design/mobile-menu-backdrops.md)
with a shared `mobile-menu-root` class on `DropdownMenuContent` and
`ContextMenuContent`, plus a scoped positioner `::before` rule inside the
existing phone media query. Reuse the Drawer backdrop utilities. Keep Radix
state and interaction ownership intact; submenu content receives no marker.
Reset the positioner's inline `will-change: transform` hint alongside its
transform so the fixed backdrop's containing block is the viewport.

Document the resulting shared rule in the responsive-surface section of
`apps/web/AGENTS.md`. Public docs need no update: this package records a visual
repair and changes no workflow, label, or public instruction.

## Tests

This is CSS and component markup, with no new state or business logic.
Browser regressions are the primary evidence. Do not add source-text assertions
or a parallel state machine solely to test backdrop presence.

## E2E tests

| Test file and scenario | Criteria |
| --- | --- |
| New `e2e/tests/layout/mobile-menu-backdrops.spec.ts`: **Kanban task options blur the background and dismiss cleanly** | 001.3, 001.9, 001.11 |
| Same file: **task submenus share one backdrop and preserve the task drawer** | 001.3, 001.10, 001.11 |
| Same file: **workspace menu preserves non-modal drawer interaction** | 001.9, 001.11, 001.12 |
| New `e2e/tests/layout/menu-backdrops.spec.ts`: **menu backdrop follows the 640px boundary while open** | 001.11, 001.12 |
| Same file: **desktop menus keep anchored actions without a backdrop** | 001.12 |

Use `mobile-chrome` for the mobile file and `chromium` for boundary/desktop
coverage. Reuse `MobileKanbanPage`, `SessionPage`, and fixture seed APIs.
Place genuinely shared computed-style assertions in
`e2e/helpers/menu-backdrop.ts`. Existing mobile Kanban and sidebar tests provide
nearby interaction patterns and selected compatibility checks.

## Work orders

- [x] [Task 01: Add shared mobile menu backdrops](task-01-add-menu-backdrops.md) (done)

Run the work order sequentially after the explicit implementation request.
There is no delegation or publication step in this package.

## Verification results

- Implementation completed after the user's explicit implementation request.
  The change is two shared root markers and phone-only CSS; no action handler,
  dependency, additional portal, or menu state changed.
- Red: the Kanban browser regression failed before production changes because
  the root positioner's `::before` content was `none` rather than generated.
- Browser inspection also identified Radix's `will-change: transform` hint as
  a containing-block constraint. Clearing it changed backdrop coverage from
  the menu's 377 x 289px bounds to the full 393 x 851px viewport.
- Green: all five new browser scenarios and all four selected compatibility
  scenarios passed. Typecheck, targeted ESLint, and Prettier checks passed.
- One intermediate Kanban run reported a missing backdrop. A diagnostic run,
  five sequential traced repetitions, and the final full mobile file passed;
  the intermittent result was not reproduced again. See the work order for
  execution details and the local desktop-discovery workaround.
- Visual evidence: inspected light/dark Kanban menus and a nested task menu
  in Chromium. The background is blurred and foreground text stays sharp.
  WebKit and physical-device rendering were not tested.
- The runtime and browser were isolated from the user's application and data.
  No public documentation or localization changes were required.
- Specification validation: all files passed; all 30 specification-linter tests
  passed. `git diff --check` passed.

## Risks

- A backdrop inside scrolling menu content would be clipped; keep it on the
  positioning wrapper and verify long menus.
- A transformed or incorrectly stacked ancestor can change viewport coverage;
  inspect computed styles and screenshot the actual portaled hierarchy.
- Adding markers to submenus would compound dimming and blur.
- A pointer-active overlay could break the non-modal Home workspace picker or
  cause different outside-dismiss behavior.
- Drawer/menu nesting must leave the parent interactive after dismissal and
  must not let a tap on an action begin a task drag.
- The configured phone browser is Chromium; actual WebKit rendering remains a
  compatibility consideration beyond that project's evidence.
