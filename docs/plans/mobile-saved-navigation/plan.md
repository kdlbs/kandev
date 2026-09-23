---
created: 2026-09-23
status: done
requirements:
  - REQ-UI-MOBILE-MENU-007
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
  - ../../specs/ui/system-design/unified-mobile-navigation.md
legacy_specs: []
---

# Mobile saved navigation

## Overview

Restore the phone menu hierarchy after the first sidebar customization. The user
requested unattended implementation through a PR with seeded screenshots, so this
package records the work within the authorized delivery session.

## Root cause and scope

`useHasSavedSidebarLayout` switches rendering at revision greater than zero.
`AppNavSections` inserts the entire saved layout before `afterPrimary`, moving
workspace tools ahead of Tasks. Built-in sections also reuse `ShortcutSection`'s
icon strip instead of the phone disclosures and omit setup/recovery paths.

Keep the shared drawer, saved layout data, domain availability, task controller,
desktop composition, and custom shortcut actions. Correct phone ordering, reuse
native built-in disclosures, and preserve labelled setup and navigation paths.
No backend, new preference, credential, or task-lifecycle changes.

## Mobile design contract

The hamburger opens the shipped `AppNavSheet` inset drawer for a temporary
navigation choice. Home/quick actions lead into the existing task list; optional
workspace tools follow. Tasks is the primary navigation action. The fixed header,
dynamic viewport bounds, one content scroller, safe-area padding, and 44px touch
targets remain. Desktop shares layout state while retaining its own composition.

## ASCII UI preview

UI-01: phone menu after saving a sidebar toggle, entered from Home or a task.

```text
Before                        After
Menu                     x    Menu                     x
Workspace                     Workspace
Home                          Home
Quick Chat | Quick terminal   Quick Chat | Quick terminal
New Task                      Tasks v                  +
Automations                   [task rows and filters]
Canvases                      Automations >
Integrations                  Canvases >
  [unlabelled icon strip]      Integrations >
Tasks v                  +    [custom groups]
[task rows below fold]         Utilities

Expanded Integrations:
Integrations v
  [GitHub icon] GitHub
  [GitLab icon] GitLab
  Integration settings
```

Header stays fixed; all content shares one scroller. Section ordering and named
links are requirements; spacing is illustrative. Empty integrations show settings.
UI-02: desktop retains saved nodes/groups above Tasks and existing icon shortcuts.

## Tests and E2E

`settings/mobile-sidebar-customization.spec.ts` covers saved Home visibility,
first toggle save, task-before-tools order, labelled integration links, setup,
touch containment, scroll ownership, and task navigation (AC-UI-MOBILE-MENU-007.2,
007.3, 007.4 and AC-UI-SIDEBAR-CUSTOMIZATION-005.5).
Existing mobile menu hierarchy/integration and desktop customization tests cover
default layout, quick actions, launch handoff, and unchanged desktop behavior.

## Work orders

- [x] [Task 01: Restore saved phone navigation](task-01-restore-phone-navigation.md)

## Verification results

Implemented with regression-first Playwright coverage. Saved layouts now compose
Home and quick actions, Tasks, then workspace tools. Built-in disclosures retain
setup and recovery actions; regular task menus use the existing create button.
The legacy Canvas block defers to the saved layout, including hidden Canvases.

Validation: 16 mobile regressions, 3 desktop regressions, and 57 focused unit tests
passed. Typecheck, focused ESLint, localization checks, docs catalog/spec checks,
and harness validation passed. See the work order for exact commands and results.

Seven screenshots use this branch's production build and a disposable mock-backed
workspace with four seeded tasks and GitHub/GitLab integrations. Captures cover
393px dark/light Tasks and Integrations, 360px and 767px phones, and 1280px desktop.
PR publication and exact-head CI/review status are tracked in the PR.

## Risks

- Plugin links must not duplicate or bypass saved visibility.
- Hidden Home must retain quick actions; task selection must retain its controller.
- Mobile section expansion must not introduce another vertical scroll region.
