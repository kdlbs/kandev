---
created: 2026-09-22
status: implemented
requirements:
  - REQ-UI-MOBILE-TASK-CHROME-001
system_design:
  - ../../specs/ui/system-design/mobile-task-chrome.md
legacy_specs: []
---

# Implementation Plan: Mobile Task Plugin Menu

## Overview

Move task-scoped `chat-top-bar` plugin contributions out of the fixed phone task
header and into the shared app menu's existing Plugins section. First add the
responsive slot contract and host composition with component regressions, then
prove the result with the packaged plugin fixture on the managed Pixel 5 project
and capture the two phone states needed for the PR.

## Confirmed root cause

`SessionMobileTopBar` renders `TaskTopBarPluginActions` inside
`MobileTopBarActions`, whose `shrink-0` cluster also owns the hamburger. Every
registered `chat-top-bar` component is opaque and rendered inline, while
`ChatTopBarSlotProps` has no presentation field or phone containment contract.
The supplied 390-CSS-pixel phone screenshot shows two contributions consuming
the middle of the fixed row and reducing the title/branch region to a cramped
remainder. The code path is deterministic even though the document itself does
not overflow.

The closest shipped correction is the listing `main-top-bar` slot: its phone
presentation leaves the persistent header, renders in the shared app menu, and
receives `presentation: "mobile"`. The task slot should follow that boundary
while preserving task and session identity.

## Scope

### In scope

- Remove `chat-top-bar` plugin components from the persistent phone task header.
- Render those components inside the shared phone menu's existing Plugins group.
- Add a public desktop/mobile presentation field without changing existing task,
  workspace, active-session, or session-list context.
- Contain multiple and long contributions in the menu and preserve 44px phone
  controls supplied through host UI primitives.
- Keep tablet and desktop task-top-bar placement unchanged.
- Add component and managed mobile E2E regressions using the real packaged
  plugin fixture.
- Capture a synthetic long-title header and the opened Plugins menu for the PR.

### Out of scope

- Moving first-party approval, archive, port-forwarding, executor, or
  change-request controls.
- Changing plugin registration lifecycle, backend APIs, persistence, or
  permissions.
- Redesigning plugin navigation or the shared menu hierarchy outside the
  combined Plugins section.
- Changing listing `main-top-bar` behavior.

## Technical approach

### Public slot contract

- Export `ChatTopBarSlotProps` from `apps/packages/plugin-sdk/src/index.ts` and
  use it as the canonical host type in
  `apps/web/components/task/task-top-bar-plugin-actions.tsx`.
- Add `presentation: "desktop" | "mobile"`. Default the host component to
  desktop so existing call sites and plugin behavior remain compatible.
- Update `docs/plans/plugins/PLUGIN-API.md`,
  `docs/public/plugins-authoring.md`, and the SDK exact-consumer test with the
  phone placement, containment, and touch contract.

### Phone composition

- Add a reactive registration-presence helper beside
  `TaskTopBarPluginActions`; it prevents the app menu from rendering an empty
  Plugins group when no `chat-top-bar` slot is registered.
- Give `TaskTopBarPluginActions` a mobile wrapper that uses the available width,
  wraps contributions, constrains direct children, and normalizes host buttons
  to a minimum 44px active target. Keep its desktop return path unchanged.
- Remove the plugin slot from `MobileTopBarActions`. Pass the mobile presentation
  through `AppNavSheet` instead, with the same task, workspace, active-session,
  and task-session IDs.
- Thread the optional content through `AppNavSections` into
  `MobilePluginNavSection`. Render it above plugin destination rows under one
  localized Plugins heading, and keep the section absent when it has neither
  contributions nor destinations.
- Preserve the menu's existing single vertical scroll owner, safe-area padding,
  link dismissal, Escape behavior, and focus return. A plugin control owns its
  interaction and does not close the menu merely because it was activated.

### Regression and PR evidence

- Extend the packaged E2E plugin with deterministic `chat-top-bar`
  contributions that expose the received presentation and include compact and
  wider content.
- Extend `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts` with a task
  scenario that creates a synthetic long-title task, verifies the fixed header
  and menu geometry, activates a plugin control, and checks zero document
  horizontal overflow.
- Use `prCapture` in that scenario to write
  `mobile-task-plugin-header.png` and `mobile-task-plugin-menu.png`. The closed
  screenshot shows recovered task hierarchy; the open screenshot shows where
  plugin controls moved. Validate both manifest entries before PR publication.

## ASCII UI preview

### UI-01: Phone task header with plugin contributions

Entry point: `/t/:taskId` below `md`, with two `chat-top-bar` registrations.

Current behavior confirmed by the supplied screenshot and source trace:

```text
+--------------------------------------------------+
| < | Long task title... v | [plugin] [status] | = |
|   | branch +changes      |                   |   |
+--------------------------------------------------+
    ^ task identity is squeezed by an unbounded,
      non-shrinking plugin cluster
```

Target persistent header:

```text
+--------------------------------------------------+
| < | Long task title with useful context... v | = |
|   | repo / branch +changes                    |   |
+--------------------------------------------------+
    fixed row; plugin contributions are absent
    first-party contextual controls may remain before the menu
```

Target opened shared menu:

```text
+--------------------------------------------------+
| Menu                                           x |
|--------------------------------------------------|
| Workspace                                        |
| [ Current workspace                         v ]  |
|                                                  |
| Home                    [Quick Chat] [Terminal]  |
| Tasks                                            |
|   ... existing task navigation ...               |
|                                                  |
| Plugins                                          |
|   [ plugin action ] [ session status 23% ]       |
|   [ additional controls wrap inside the menu ]   |
|   [ plugin destination row                    ]  |
|                                                  |
| Integrations                                     |
| Utilities                                        |
+--------------------------------------------------+
    one vertical scroll owner; no horizontal page scroll
```

Structural requirements: the phone header keeps task identity as the flexible
region; the hamburger remains fixed and touch reachable; plugin contributions
share one Plugins section, wrap within its width, and expose 44px controls. The
exact spacing, labels inside plugin-owned components, and number of first-party
header controls remain governed by current design tokens and availability.

Tablet and desktop keep the current inline task top bar:

```text
Task identity | metrics | plugin contributions | status/tools/actions
```

This preview maps to `AC-UI-MOBILE-TASK-CHROME-001.5`, `.6`, and `.7`.

## Tests

- `AC-UI-MOBILE-TASK-CHROME-001.7`:
  `apps/web/components/task/mobile/session-mobile-top-bar.test.tsx` first
  recorded the fixed-header placement failure. The task top-bar tests then
  prove the desktop/mobile context, containment wrapper, and reactive presence
  contract.
- `AC-UI-MOBILE-TASK-CHROME-001.5` and `.7`:
  `apps/web/components/task/mobile/session-mobile-top-bar.test.tsx` proves the
  plugin slot is not a child of the fixed action cluster and is supplied to the
  shared menu only while registrations exist.
- `AC-UI-MOBILE-TASK-CHROME-001.7`:
  `apps/web/components/plugins/mobile-plugin-nav-section.test.tsx` and
  `apps/web/components/navigation/app-nav-sheet.test.tsx` prove the combined
  Plugins group, empty behavior, ordering, and context.
- `AC-UI-MOBILE-TASK-CHROME-001.6` and `.7`:
  `apps/web/lib/plugins/sdk-contract.test.ts` and the plugin SDK typecheck prove
  one public structural contract and unchanged desktop presentation.

## E2E tests

- `AC-UI-MOBILE-TASK-CHROME-001.5`, `.6`, and `.7`:
  `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts` installs the packaged
  fixture, opens a long-title task in `mobile-chrome`, proves the closed header
  retains usable non-overlapping title/menu geometry, opens the app menu, proves
  both plugin contributions are contained and receive the mobile presentation,
  activates one, and asserts no document horizontal overflow. The existing
  listing scenario remains in the same focused run.

## Work orders

- [x] [Task 01: Move task plugins into the phone menu](task-01-move-task-plugins.md) (`done`)

## Verification results

- Installed the locked workspace dependencies.
- Focused Vitest suite: 49 tests passed across the task top bar, phone header,
  shared Plugins section, navigation sheet, and public SDK contract.
- Plugin SDK and web TypeScript checks passed; targeted ESLint completed with
  zero warnings.
- Managed `mobile-chrome` Playwright run: 2 tests passed against the packaged
  fixture, including long-title geometry, context, 44px control size,
  activation, focus return, and horizontal-overflow assertions.
- PR capture manifest contains the validated synthetic header and menu PNGs.
- Public-doc validation passed 62 tests and 47 published pages; specification
  indexing and lint passed 294 decisions and 1,075 specifications.

## Risks

- Plugin components can render arbitrary markup. Host width constraints protect
  the menu, while the public presentation field lets cooperative plugins choose
  a clearer phone layout; the host cannot make opaque plugin copy meaningful.
- Moving the phone instance into the drawer changes when a plugin component is
  visibly mounted. The contract must avoid promising background polling or local
  disclosure state while the menu is closed.
- Extending the shared fixture affects every E2E test that installs it. Keep the
  new registrations inert outside task detail and run the existing listing
  scenario with the new task scenario.
- The user-provided screenshot contains live task data. PR captures must use the
  synthetic fixture task instead of publishing that attachment.
