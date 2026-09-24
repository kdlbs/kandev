---
created: 2026-09-23
status: implemented
requirements:
  - REQ-UI-MOBILE-MENU-007
  - REQ-UI-MOBILE-TASK-CHROME-001
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
  - ../../specs/ui/system-design/mobile-task-chrome.md
legacy_specs: []
---

# Implementation Plan: Coherent Mobile Plugin Menu

## Overview

Group the existing phone plugin controls in one place. The current menu renders
workspace widgets, system metrics, Automations, and then more plugin widgets.
Preserve every slot's context and actions while removing that fragmented hierarchy.
The user requested autonomous delivery through a PR with screenshots.

## Scope

Phone app-menu composition, plugin containment, focused regressions, and PR
screenshots. Desktop/tablet layout, plugin APIs, resource preferences, and
plugin implementations remain outside this change.

## Technical approach

`AppNavSheet` supplies the phone workspace context to `MobilePluginNavSection`
through `AppNavSections`. That section composes main-top-bar and sidebar workspace
actions with task actions and existing plugin destinations. When workspace and
task contributions coexist, use localized context labels to explain their scope.
Gate empty groups on registrations, including saved sidebar layouts that already
own plugin destinations. Keep canvases in the workspace section. Move fallback
`StatusSurfaceMetrics` after Integrations and before Utilities, preserving its
existing preference and subscription behavior. Normalize phone host buttons to
44px without forcing square dimensions onto labeled controls.

## ASCII UI preview

### UI-01: Phone app navigation, plugins installed

```text
Before: Tasks > | workspace widgets | Metrics | Automations > | Plugins

After:
  Menu                                              x   (fixed)
  Workspace picker
  Home
  [Quick Chat] [Quick terminal]
  Tasks >                                           +
  Automations >
  Plugins
    Workspace
    [workspace actions and status, wrapping]
    Task
    [task actions and status, wrapping]
    [plugin destinations]
  Integrations >
  [System metrics: CPU / Memory / Disk]
  Utilities
```

One safe-area-aware internal scroller; no additional surface. Context labels
appear only when both contexts exist. With no plugin actions or destinations,
the entire Plugins section is absent. Metrics remain opt-in and are only in
this menu when the app status bar is disabled. Spacing is illustrative.

UI-01D desktop/tablet: existing sidebar, inline plugin toolbar controls, and
resource/status surfaces retain their composition. The phone inset Drawer is
structurally absent there.

## Tests

- `app-nav-sheet.test.tsx`: all phone contributions share one section, context
  remains correct, and empty sections disappear.
- Existing plugin-section and main-top-bar unit tests preserve destinations,
  desktop context, and mobile presentation.

## E2E tests

`mobile-plugin-topbar.spec.ts` covers the real packaged plugin on listing and
task routes, grouped placement, task/workspace context, actions, 44px targets,
resource order, one scroller, containment, dismissal, and screenshots.
`mobile-resource-metrics-display.spec.ts` preserves preferences and the
393/320/767/768px boundary. `mobile-menu-hierarchy.spec.ts` retains quick-action,
disclosure, navigation, and heading-alignment checks after the section-gap change. These map to AC-UI-MOBILE-MENU-007.2/.4 and
AC-UI-MOBILE-TASK-CHROME-001.6/.7.

## Work orders

- [x] [Task 01: Group phone plugin controls](task-01-group-phone-plugins.md)

## Verification results

- RED: navigation unit and managed mobile browser assertions reproduced workspace
  contributions outside Plugins before production changes.
- GREEN: 44 focused Vitest tests passed; web typecheck, targeted ESLint (zero
  warnings), Prettier, and the i18n ratchet passed.
- Final rendered checks: 15 mobile-chrome scenarios passed across plugin menus,
  resource preferences, and menu hierarchy after a fresh `pnpm run build:e2e`.
  The final managed run used `--host --no-build` with that build and the already
  rebuilt backend/plugin package. Phone coverage includes 320/393/767px and the
  existing 768px desktop boundary, both resource locations, Portuguese labels,
  quick actions, plugin activation, and focus return.
- Four fresh screenshots were captured with isolated synthetic data. Dark/light
  task views, the listing menu, and the no-plugin resource view are in the
  ignored PR-asset manifest. The phone drawer is absent on desktop; no unrelated
  desktop screenshot is required.
- Public-doc validation: 62 tests and 47 pages passed. Specification catalog and
  lint passed (300 decisions, 1,131 specifications); diff checks passed.
- PR review added direct coverage of the status-bar preference and semantic
  Plugins-region queries. CI exposed a mobile autopilot assertion that polled
  a transient state; it now checks the durable resumed turn, matching desktop.
  The affected tests and nearby CI ordering passed with one worker, retries
  disabled, and CPU affinity restricted to two cores. An intermittent
  history-recovery failure remains open; its evidence and an unsuccessful
  fixture experiment are handed to PR #3890 for combined verification.

## Risks

Plugins own their markup. Preserve opaque controls and separate workspace/task
contexts rather than deduplicating contributions by appearance. Saved sidebar
layouts already own destinations and must not gain duplicate links.
