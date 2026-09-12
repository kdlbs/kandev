---
id: "03-task-views"
title: "Shared task-view entry"
status: complete
wave: 3
depends_on: ["02-view-picker"]
plan: plan.md
requirements:
  - REQ-UI-MOBILE-TASK-VIEWS-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-VIEWS-001.1
  - AC-UI-MOBILE-TASK-VIEWS-001.2
system_design:
  - ../../specs/ui/system-design/mobile-task-view-access.md
---
# Shared task-view entry

## Summary and scope

Implement the components and flow section of the [design](../../specs/ui/system-design/mobile-task-view-access.md), including its focused regressions and translations. Own only the files listed below and required adjacent tests.

## Out of scope

Other work-order outcomes and changes to persistence or provider API contracts.

## Acceptance

- The referenced acceptance criteria pass through the real interaction/state boundary.
- Failure paths retain prior state and permit recovery; desktop behavior remains intact.
- Required targeted checks pass and results are recorded.

## ASCII UI preview

### UI-03: Shared task-view entry

```text
App menu -> [Task views]
Task drawer: [Saved view v] [Filters]
Matching task -> /t/id
Browser Back -> /github
```

See the [combined preview](plan.md#ascii-ui-preview). Geometry and hierarchy are structural; ASCII spacing is illustrative.

## Verification

From the repository root:

```bash
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/github/mobile-task-view-access.spec.ts)
```

Hook regressions use Red-Green-Refactor. Run new/changed tests after the final source edit.

## Files likely touched

Paths relative to apps/web: `components/navigation/use-task-view-navigation.tsx`, `app-nav-sections.tsx`, `app-nav-sheet.tsx`, `components/kanban/mobile-menu-sheet.tsx`, `components/task/mobile/session-task-switcher-sheet.tsx`, and `session-task-switcher-sheet-hooks.ts`.

## Dependencies

Task 02-view-picker.

## Risks

Reuse the existing drawer; preserve task-workbench navigation and avoid mounting task context until requested.

## Parallelism

sequential

## Inputs

[Requirements](../../specs/ui/requirements/mobile-task-view-access.md), paired design, existing source and tests named above.

## Results

Shared navigation, browser Back, focus restoration, empty Kanban workspaces, and existing task-view creation/editing/selection pass in mobile browsers. The surface is lazy until requested, then retains its controller for child dialogs. The rotation regression first reproduced a lost draft, then passed after keeping the requested controller mounted across width changes. All 22 navigation unit regressions and both task-view entry browser tests pass after that fix.
