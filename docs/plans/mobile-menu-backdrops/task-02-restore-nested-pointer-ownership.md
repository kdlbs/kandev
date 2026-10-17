---
id: "02-restore-nested-pointer-ownership"
title: "Restore nested drawer menu pointer ownership"
status: done
wave: 2
depends_on: ["01-add-menu-backdrops"]
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-TASK-NAVIGATION-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-NAVIGATION-001.10
  - AC-UI-MOBILE-TASK-NAVIGATION-001.11
system_design:
  - ../../specs/ui/system-design/mobile-menu-backdrops.md
---

# Task 02: Restore Nested Drawer Menu Pointer Ownership

## Scope and contract

Restore usable mobile workspace navigation when a shared dropdown closes inside
a modal task drawer and route navigation disposes both surfaces. Preserve the
existing [nested surface and dismissal design](../../specs/ui/system-design/mobile-menu-backdrops.md)
and [mobile navigation requirements](../../specs/ui/requirements/mobile-task-navigation.md).
No backdrop redesign, backend change, dependency change, global pointer reset,
or relaxation of tests, retries, or authorization is included.

## Cause and implementation

Radix DismissableLayer retains a closed menu during its exit Presence. After
closing, its outside-pointer effect no longer owns restoration, but its layer
membership remains. Disposing the drawer while that menu remains skips the
drawer restoration because two disabled layers are registered. Final menu
disposal then leaves the body's inline pointer style at `none`.

The shared Drawer carries its actual modal mode and content ancestry through
React contexts, including portals. A dropdown inside modal Drawer content
defaults to nonmodal, leaving the enclosing drawer as the outside-pointer
owner. Explicit dropdown modal props and standalone defaults remain intact.
The decorative menu backdrop, task/workspace handlers, and existing Radix
keyboard and focus behavior remain unchanged.

## File map

- `apps/packages/ui/src/drawer.tsx`: provide drawer mode and content ancestry.
- `apps/packages/ui/src/dropdown-menu.tsx`: derive only the nested default mode.
- `apps/packages/ui/src/modal-drawer-context.tsx`: shared scoped contexts.
- `apps/web/components/task/mobile/session-task-switcher-sheet.test.tsx`:
  animation-aware pointer restoration and standalone/modal regression coverage.
- `apps/web/e2e/tests/task/mobile-nested-workspace-navigation.spec.ts`:
  first A-to-B and second B-to-A navigation, keyboard dismissal and focus.

## Verification and results

At baseline `a63fa40c2f27949cf2f9056888236d7478dd2264`, the real Vaul/Radix
component regression retained closed-menu Presence, disposed the navigation
tree, and failed: expected original inline `auto`, actual `none` (1 failed,
8 passed, no skips). The immediate-unmount counterpart passed, exposing the
missing lifecycle seam. The real baseline mobile reconnect scenario also
failed on its first workspace switch with orphaned pointer blocking.

Implementation revision `e3d535971ed65cac6f4abac369bc5a6fc4d7633f` passed:

- `cd apps/web && pnpm exec vitest run components/task/mobile/session-task-switcher-sheet.test.tsx`:
  11 passed, no skips; original empty and `auto` styles, retained exit,
  immediate disposal, standalone blocking, and drawer close/reopen covered.
- Focused `mobile-chrome` nested navigation and reconnect checks: 3 passed,
  no skips, no retries. Both workspace directions, outer modal blocking,
  two-level Escape, opener focus, and task selection passed.
- Affected mobile task switch/archive/edit checks: 3 passed; desktop reconnect:
  1 passed. Desktop discovery needed a local basename-scoped ignore because
  the existing broad mobile-spec ignore matched the worktree ancestor name.
- Web typecheck, focused ESLint, and normal commit hooks passed.
- Independent Review and QA passed that implementation revision. QA exercised
  Pixel 5 Chromium, WebSocket reconnect, outside tap and keyboard containment;
  all ten final screenshots were opened and visually inspected.

Run the managed browser checks sequentially from `apps/web`, with task-owned
`PLAYWRIGHT_BROWSERS_PATH` and an isolated port offset:

```sh
pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome e2e/tests/task/mobile-nested-workspace-navigation.spec.ts e2e/tests/task/mobile-sidebar-shared-task-state.spec.ts -- --grep 'recovers shared coverage after reconnect|nested workspace menu' --retries=0 --trace=on
```

`--no-build` requires verified current task-owned backend and frontend builds.
Use repository synthetic seeders only. Assert original inline pointer style
after navigation, continued outside blocking while the drawer remains open,
and actual destination content, rather than checking only the URL.

## Evidence boundaries

The checks above bind to the recorded implementation revision, not a later
documentation or publication commit. Review and QA must account for any
successor delta. Human acceptance remains separate. The prepared Human QA
runtime retains that implementation revision until a controlled refresh.
Physical iOS, a second LAN client, and full CI were not verified. No live user
data or another task's runtime was used.
