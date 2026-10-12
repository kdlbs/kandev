---
id: "03-navigation-split"
title: "Persist the navigation and Tasks split"
status: done
wave: 3
depends_on:
  - "02-direct-sidebar-customization"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-007
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-007.5
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 03: Persist the navigation and Tasks split

## Summary

Add a persisted desktop navigation height and expansion state. Resize the divider
above Tasks, with a bottom fade and compact chevron when navigation is clipped.

## Scope

Extend the existing layout model/codec/validation and shared mutation controller.
Bound navigation without changing row sizes or task scrolling. Preserve focus,
keyboard resize, pointer cancellation, coarse targets, revision handling, and
workspace isolation. Phone, Office, Settings, and rail composition remain unchanged.

## Acceptance

1. Saved split height and expansion survive requests/reloads without rewriting
   visibility/order or losing the compressed height during expansion.
2. Clipping exposes the fading edge and 12px chevron strip; expansion reveals
   navigation and collapse restores height. Bounds retain usable Tasks space.
3. Cancelled gestures do not save, errors reconcile, hidden rows cannot trap
   keyboard focus, and phone/Office/rail behavior remains unchanged.

## ASCII UI preview

Use [UI-08](plan.md#ui-08-resized-navigation-and-tasks-split).

```text
Home                           Home
~~~ bottom fade ~~~            Inbox
         v                     Integrations >
------------------                      ^
TASKS                          ------------------
Task A                         TASKS
Task B                         Task A
```

## Verification

```bash
(cd apps/backend && go test ./internal/user/models ./internal/user/service ./internal/user/store)
(cd apps/web && pnpm exec vitest run lib/sidebar/layout-operations.test.ts lib/sidebar/layout-projection.test.ts components/app-sidebar/sidebar-navigation-split.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/sidebar-direct-customization.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-sidebar-direct-customization.spec.ts)
(cd apps/web && pnpm run typecheck)
```

Also run targeted lint, localization, docs/spec validation, and diff gates from the
plan. Browser proof covers actual resize, reload, expanded-state restoration,
fade and chevron geometry, keyboard/cancel paths, and effective viewport clamping.

## Files likely touched

User layout models/validation; client layout codecs; the shared customization
controller; `app-sidebar.tsx`; a focused navigation split component/helper; their
tests; translations, public tutorial, and plan/spec status/results.

## Dependencies

Task 02's shared mutation controller.

## Risks

Measurement loops, clipped focusable controls, overlarge navigation starving Tasks,
and stale resize responses after a workspace change.

## Parallelism

`sequential`

## Inputs

REQ-UI-SIDEBAR-CUSTOMIZATION-007 and the Navigation split design section.

## Results

Complete on 2026-10-06. Implemented and verified in the primary session.
See the [shared implementation and validation record](plan.md#implementation-results-2026-10-06)
for backend, SQLite/PostgreSQL, frontend, desktop/phone browser, build,
localization, lint/type, and documentation evidence. No unresolved blocker.
The user subsequently authorized delivery to existing PR #4239.
