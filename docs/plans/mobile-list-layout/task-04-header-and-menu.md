---
id: "04-header-and-menu"
title: "Sharp phone header and consistent menu geometry"
status: in_progress
wave: 4
depends_on:
  - "03-controls-and-editor"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-LIST-VIEWS-003
  - REQ-UI-MOBILE-LIST-VIEWS-004
acceptance_criteria:
  - AC-UI-MOBILE-LIST-VIEWS-003.2
  - AC-UI-MOBILE-LIST-VIEWS-004.1
  - AC-UI-MOBILE-LIST-VIEWS-004.2
  - AC-UI-MOBILE-LIST-VIEWS-004.3
  - AC-UI-MOBILE-LIST-VIEWS-004.4
  - AC-UI-MOBILE-LIST-VIEWS-004.5
  - AC-UI-MOBILE-LIST-VIEWS-004.6
system_design:
  - ../../specs/ui/system-design/mobile-list-saved-views.md
---

# Task 04: Sharp phone header and consistent menu geometry

## Summary

Correct the reported top-edge blur with real-device evidence. Align menu header,
workspace, and sections while keeping the existing compact controls.

## In scope

- Capture the blurred Default state on an isolated iPhone browser/PWA reproduction.
- Diagnose safe-area placement and native top-edge compositing before selecting a correction.
- Apply the smallest supported header fix and add focused rendered regression assertions.
- Add visible top spacing to the fixed menu workspace picker.
- Remove empty listing action wrappers and their retained flex gaps.
- Keep tools and utilities ordered under existing workspace/sidebar visibility rules.
- Start contextual Tasks collapsed from List; retain its task selection and omit editor actions there.
- Preserve one menu scroller, safe-area clearance, dialog lifetime, and focus return.

## Out of scope

Global backdrop changes, a font/color-only claim of blur repair, new navigation
collections, hidden required plugin controls, and larger control heights.

## Acceptance

1. Real iPhone Safari and installed PWA show a sharp Default label on cold load,
   after scrolling, and after options/menu dismissal. Recorded evidence supports the chosen fix.
2. UI-06 matches the menu hierarchy, fixed picker/header, and contained scrolling.
   No saved-view edit entry appears in that List menu.
3. UI-01/UI-02 remain compact and contained through the phone/tablet boundary.
   Existing heights, navigation/controller lifetimes, and overlay dismissal remain intact.

## ASCII UI preview

Full previews: [UI-01](plan.md#ui-01-phone-list-search-closed) and
[UI-06](plan.md#ui-06-app-navigation-menu).

### UI-06: App navigation menu

Entry: hamburger from List. AC: 003.2, 004.3-.6.

```text
+--------------------------------------+
| Menu                             [x] |
|                                      |
| [ Default                        v ] |
+--------------------------------------+
| [ + New Task                       ] |
| Home                                 |
| [ Quick Chat ] [ Terminal ]          |
|                                      |
| Canvases                           > |
| Automations                        > |
| Integrations                       > |
| Eligible plugin tools/status         |
|                                      |
| Tasks                              > |
|                                      |
| Utilities                            |
| Settings                             |
| Status                               |
| Customize sidebar                    |
| Theme                                |
|              safe area               |
+--------------------------------------+
```

Header and workspace picker stay fixed. One scroller owns the remaining menu.
Honor configured visibility and relative tool order. Tasks starts collapsed;
its expanded rows remain selectable. No Edit view action or filter editor appears here.

## Verification

```bash
pnpm --dir apps/web exec vitest run components/kanban/kanban-header-mobile.test.tsx components/navigation/app-nav-sheet.test.tsx components/navigation/mobile-task-navigation-provider.test.tsx components/kanban/mobile-menu-sheet.test.tsx
pnpm --dir apps/web e2e:run --project mobile-chrome tests/kanban/mobile-kanban-topbar.spec.ts tests/layout/mobile-menu-hierarchy.spec.ts tests/layout/mobile-navigation-tasks.spec.ts tests/layout/mobile-menu-backdrops.spec.ts tests/layout/mobile-unified-navigation.spec.ts
git diff --check
```

Also use an isolated runtime through `scripts/dev-isolated --web` for the
real-device check. Follow `/debug` instance ownership and teardown rules.
Record device model, OS/browser, Safari versus installed PWA, viewport/scale,
safe-area geometry, and before/after images. Open those images and inspect the
label itself. Headless styles or screenshots cannot substitute for this evidence.

At the same font/viewport, compare control heights with Task 02's baseline.
Check 393px and 767px phone, 768px coarse tablet, and 1024px fine desktop.
Verify header icon hit-testing, internal scrolling, overlay removal, and focus return.

If a device is unavailable or blur remains, record the exact limitation.
Leave this work order incomplete. Menu fixes can proceed, but cannot close the
combined acceptance while the original blur remains unverified.

## Files likely touched

- `apps/web/components/page-topbar.tsx` only if evidence requires a shared hook/class seam.
- `apps/web/components/kanban/kanban-header-mobile.tsx`.
- `apps/web/components/kanban/mobile-listing-context.tsx`.
- `apps/web/components/kanban/mobile-menu-sheet.tsx` and `mobile-listing-menu-actions.tsx`.
- `apps/web/components/navigation/app-nav-sheet.tsx` and `app-nav-surface.tsx`.
- `apps/web/components/navigation/mobile-task-navigation-provider.tsx`.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` for the menu-only capability.
- `apps/web/app/globals.css` only for a scoped, evidence-backed rule.
- The focused component/E2E files named above.

## Dependencies

Task 03 supplies the final header controls and editor ownership.

## Risks

Native Safari chrome differs from emulation. A global safe-area or position
change can introduce double padding or affect task workbenches. Shared menu
presentation must preserve plugin metrics and task-dialog state.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/mobile-list-saved-views.md), 003.2 and 004.
- [Design](../../specs/ui/system-design/mobile-list-saved-views.md), Menu integration, Geometry and density, Header diagnosis.
- User screenshot crop and investigation notes in the Kandev task plan.
- Existing app navigation and mobile task-picker components.

## Results

In progress on 2026-10-27. The phone menu now has a padded fixed workspace
picker, one scrolling navigation region, a collapsed contextual Tasks section,
and no saved-view editor entry. The menu, topbar, navigation, drawer, focus, and
tablet-boundary scenarios passed in the 62-test mobile E2E run. The 14-test
responsive/desktop run also passed at 393px, 767px, 768px, and 1024px; there is no
horizontal overflow. The captured menu panel begins at y=16 and has an 836px
container at the 393x852 test viewport, with its visible surface inset by the
shared drawer padding.

The user-reported Default label is not blurred in headless screenshots. This
does not prove the reported native Safari/PWA compositing defect is fixed. No
real iPhone is available in this environment, so Safari/PWA cold-load, scroll,
orientation, and drawer-dismissal evidence remains pending. No unsupported CSS
correction was made. Keep this work order in progress.
