---
created: 2026-09-21
status: complete
requirements:
  - REQ-UI-TASK-NAVIGATION-001
system_design:
  - ../../specs/ui/system-design/task-navigation.md
legacy_specs: []
---

# Implementation Plan: Task link navigation

## Overview

Fix dependency navigation and prevent recurrence across first-party task links.
The shared primitives, first-party migration, enforcement, and desktop/mobile
proof are complete across the three sequential work orders.

## Evidence and assumptions

Confirmed: the user requests a fix plan and central task-link construction.
Verified: DependencyRow uses a native anchor to `/tasks/${entry.id}`. Both
Blocks and Blocked by share the row, on desktop and touch. Existing component
and E2E tests check visibility but never activate the dependency link.
Verified: linkToTask already builds `/t/:id`; AppLink already handles guarded
SPA navigation. Thus URL construction and click transport need separate guards.
Source tracing establishes the cause; a browser reproduction has not been run.
No material product choice remains unresolved. The reusable navigation contract
belongs to UI; dependency state remains owned by Tasks.

## Scope

### In scope

- Shared TaskLink, compatible URL-builder extension, committed-navigation callback.
- Dependency popover/drawer fix, touch targets, regression coverage.
- Migration of first-party hand-built task-detail URLs and known raw task anchors.
- ESLint enforcement, wiring coverage, and scoped engineering guidance.

### Out of scope

Backend routes/APIs, Office destination semantics, external/plugin links, user
content rewriting, a generic route catalog, and layout redesign.

## Technical approach

Follow [the design](../../specs/ui/system-design/task-navigation.md).
Use linkToTask as URL authority and AppLink as click/guard authority; TaskLink
combines them without another routing implementation. Preserve session, layout,
canvas query context, explicit new-tab behavior, and compatibility routes.

## ASCII UI preview

UI-01: Dependency navigation, expanded state (AC .1, .4, .5, .6).

```text
Desktop: [blocks 2] -> Popover: [Task B | REVIEW] -> Task B workbench
Phone:   [blocks 2] -> Drawer:
                      Dependencies        [Close]
                      [Task B | REVIEW]   (44px+ tap row)
                      [Task C | CREATED]  (list scrolls)
                      -> Task B workbench; drawer closes
Before: selecting a row loads a new document through /tasks/:id.
After:  selecting a row commits SPA navigation to /t/:id.
```

Control order, touch targets, and successful-navigation dismissal are required.
Spacing is illustrative. Retain existing localized labels and status displays.
Cancelled navigation keeps the disclosure; modified clicks keep the current page.

## Tests

| Acceptance | Evidence |
| --- | --- |
| .1, .3, .4 | task-link.test.tsx, app-link.test.tsx, task-dependency-chip.test.tsx |
| .2 | links.test.ts and migrated caller tests |
| .1, .2 prevention | no-task-link-bypass.test.ts and task-link-wiring.test.ts |
| .1, .4, .5, .6 | Browser scenarios below |

## E2E tests

Add the desktop test `dependency links preserve the desktop task workbench` to
`e2e/tests/task/task-dependencies.spec.ts` (chromium) and the mobile test
`mobile dependency links preserve the task workbench and touch layout` in
`e2e/tests/task/mobile-task-dependency-navigation.spec.ts` (mobile-chrome).
They exercise Blocks and Blocked by, target identity, canonical href, disclosure
closure, document sentinel survival, no main-frame document request, and Back.
The phone test uses the real drawer and measures 44px targets and viewport
containment.

## Work orders

- [x] [Task 01: Shared task links and dependency fix](task-01-shared-task-links.md)
- [x] [Task 02: Migrate and enforce task URL construction](task-02-enforce-task-links.md)
- [x] [Task 03: Desktop and touch navigation proof](task-03-browser-regressions.md)

## Verification results

Implementation completed on 2026-09-21:

- Task 01 added `TaskLink`, successful-navigation callbacks, canonical dependency
  links, guarded disclosure dismissal, and touch-sized dependency rows.
- Task 02 migrated first-party task destinations, added bounded ESLint
  enforcement, added real-config wiring coverage, and recorded the migration
  inventory.
- Task 03 added desktop and mobile no-reload regressions. The mobile layout now
  forwards the task status identity needed by sessionless dependency tasks.
- Focused Vitest checks passed: 6 files and 51 tests for the routing, dependency,
  and rule contracts.
- Broad related Vitest checks passed: 437 files and 4,254 tests.
- `pnpm run typecheck` and `pnpm run lint` passed.
- Chromium dependency E2E passed: 6 tests.
- Mobile dependency E2E passed: 1 test.
- Specification validation, specification lint, and `git diff --check` passed.

Planning validation passed on 2026-09-21:

- `python3 scripts/list-docs.py validate`: 294 decisions and 1067 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/task-link-navigation`: passed.
- Catalog discovery includes the new requirement and design. Git status includes all three new work orders and the inventory.

## Risks

- Changing query construction must preserve duplicate parameters and avoid double encoding.
- Closing a disclosure before a dirty-navigation guard commits would lose context.
- Static rules cannot resolve arbitrary runtime strings; test bounded detection honestly.
- Compatibility route recognition and API URLs must not become false positives.
