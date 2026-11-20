---
created: 2026-10-21
status: implemented
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-006
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
legacy_specs: []
---

# Implementation Plan: Sidebar Dialog Event Isolation

## Overview

Keep task-prompt gestures inside the create-task dialog. Deliver one sequential
work order that restricts sidebar customization activation to its DOM region.
UI owns this reusable sidebar interaction contract. Task creation and settings
persistence retain their existing owners.

## Evidence and root cause

`SidebarLayoutNavigation` wraps `AppSidebarNewTaskItem` in `SidebarCustomizeMenu`.
The item owns `RoutedNewTaskDialog`, which renders `TaskCreateDialog` through a
portal. React sends portal events through the sidebar's component ancestry.
The installed Radix trigger opens on `contextmenu` and starts a 700 ms timer on
touch or pen `pointerdown`. It does not check DOM containment.

A temporary component reproduction used the real `SidebarCustomizeMenu` and
real Radix primitive with a portaled contenteditable. Only unrelated settings
and routing hooks were mocked. Three diagnostic assertions passed:

- A context event in the portaled prompt opens Sidebar settings.
- A touch held beyond 700 ms in that prompt opens Sidebar settings.
- A short touch with matching pointer release keeps the menu closed.

The prompt was explicitly outside the trigger's DOM subtree. Source inspection
also shows that global phone menu CSS presents the context menu as a sheet.
This confirms an event-ownership defect. It does not establish that every short
tap triggers the user's symptom. Real-device keyboard and release timing remain
unverified. Subsequent Chromium phone emulation reproduced the sheet with a
native CDP held-touch sequence. Desktop right-click reproduced the same leak. The temporary reproduction is removed after diagnosis.

## Scope

Own the sidebar activation boundary, regression tests, and specification links.
Exclude task lifecycle, settings persistence, global context-menu changes,
editor rewrites, visual restyling, and publication.

## Technical approach

Implement the local ref-forwarding trigger described in the owning design.
Filter injected `onPointerDown` and `onContextMenu` handlers by DOM containment
before calling them. Keep browser defaults for events from portals. Preserve
ref forwarding, other props, legitimate activation, and timer cleanup.
Do not use `preventDefault()` as a portal filter: it can suppress editor focus
or native selection. Do not modify all dialogs or every shared context menu.

## UI behavior preview

No control order or geometry changes. On phones, the path remains Menu, New Task,
then the focused creation form. Prompt taps focus the editor. Held touches can
select text. Sidebar settings stays closed. Customize sidebar still opens its
own inset drawer from the navigation menu.

On desktop, New Task opens the existing centered dialog. Prompt context gestures
remain in the editor. Right-clicking the actual navigation region still opens
Sidebar settings.

The closest shipped exemplars are `TaskCreateDialog` and
`MobileSidebarCustomization`. Preserve the form's current scroll owner and the
customization drawer's internal scroller, safe-area clearance, and 44px targets.
The existing full-screen phone form supports the primary editing task. The
inset customization drawer supports temporary settings choices. State and
business logic remain shared. No new copy, motion, or layout is required.
These interaction outcomes map to AC-UI-SIDEBAR-CUSTOMIZATION-006.9.

## Tests

Add `components/app-sidebar/sidebar-customize-menu.test.tsx` with the real menu
primitive and a portal. First assert the corrected outcome and observe failure
on the current source. Cover context events, held touch, short tap, cancelled
touch, normal sidebar activation, and preservation of default behavior.
Positive sidebar activation maps to AC-UI-SIDEBAR-CUSTOMIZATION-006.1.
Portal isolation maps to AC-UI-SIDEBAR-CUSTOMIZATION-006.9.

## E2E tests

Extend `e2e/tests/settings/mobile-sidebar-direct-customization.spec.ts` and
`e2e/tests/settings/sidebar-direct-customization.spec.ts`.
Open New Task through the real navigation entry. Tap and type in the real prompt.
Exercise a held touch and a context event. Keep the draft and editor available,
with `sidebar-customize-menu` absent. Dismiss and reopen the form, then verify
normal customization remains reachable. Use a native touch sequence where
supported, with a documented dwell as gesture stimulus. Report device-emulation
limits. Do not present synthetic context events as native text-selection proof.
The existing positive customization flow covers AC-UI-SIDEBAR-CUSTOMIZATION-006.7.

## Work orders

- [x] [Task 01: Isolate sidebar activation](task-01-isolate-sidebar-activation.md)

## Verification results

Implementation completed on 2026-10-21. `SidebarCustomizeTrigger` filters the
Radix-injected activation handlers before they receive events from portals.
Browser defaults and pointer cleanup handlers remain intact.

- Component red: three expected failures for context events, held touch, and
  held pen. Seven existing-behavior assertions passed.
- Component green: `pnpm exec vitest run components/app-sidebar/sidebar-customize-menu.test.tsx`
  passed all 10 tests after the final test edits.
- Mobile browser red: the native CDP touch hold opened Sidebar settings in the
  original built assets. Log: `/tmp/kandev-run.e2e.pvuFVbFc.log`.
- Desktop browser red: a real right-click in the prompt opened Sidebar settings
  in the original built assets. Log: `/tmp/kandev-run.e2e.VWDwfStn.log`.
- Mobile browser green: both tests in `mobile-sidebar-direct-customization.spec.ts`
  passed after the managed runner rebuilt the current source.
  Log: `/tmp/kandev-run.e2e.0bYNqiGZ.log`.
- Desktop browser green: all seven tests in `sidebar-direct-customization.spec.ts`
  passed after a managed rebuild. Log: `/tmp/kandev-run.e2e.k20W5ifU.log`.
- Scoped ESLint and Prettier checks passed without warnings.
- `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed.
- Specification catalog validation, specification lint, documentation coverage
  preflight, and `git diff --check` passed. The specification linter's own 36
  tests also passed during planning.

The exact product commands are in the work order. Host verification needed a
loopback bind and installed Nix library paths. The default typecheck exhausted
Node's 4 GiB heap, so the successful run used 8 GiB.

Desktop discovery required a temporary config because this worktree's absolute
path contains `mobile-`. The existing exclusion matched that directory name.
The temporary config narrowed only the desktop mobile exclusion to
`/\/mobile-[^/]+\.spec\.ts$/`, retaining all other exclusions and the one-worker
budget. It was removed after verification. No runner or global configuration
changes are included.

Phone evidence uses Chromium's Pixel 5 emulation and a native CDP touch sequence.
Physical phone keyboards and iOS selection behavior were not tested. The managed
runner removed its isolated backend data and processes. The temporary library
path file and test config were removed.

Public documentation does not change for this repair. It restores the existing
editing interaction without new copy or settings.

## Risks

Radix injects handlers through `asChild`. A handler on the wrong layer can leave
its timer active. A default-cancelling guard can break text editing. The browser
regressions must exercise actual dialog composition in addition to component tests.

## Related delivery records

The completed sidebar-customization, sidebar-presentation-preferences,
mobile-saved-navigation, and flagged-sidebar-entries packages retain their
historical results. This package owns the additional event-isolation coverage.
