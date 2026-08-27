---
id: "02-prove-responsive-grouping"
title: "Prove desktop and mobile grouping"
status: done
wave: 2
depends_on:
  - "01-correct-repository-grouping"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-REPOSITORY-GROUPING-001
acceptance_criteria:
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.2
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.3
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.5
  - AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.6
system_design:
  - ../../specs/ui/system-design/sidebar-repository-grouping.md
---

# Task 02: Prove Desktop and Mobile Grouping

## Summary

Add production-build browser coverage for the complete multi-repository group label. Cover the desktop sidebar and the mobile task-switcher drawer.

## In scope

- Seed a second repository in an isolated E2E workspace.
- Create a task with both repositories in a defined order.
- Assert the complete group label and the task location on desktop.
- Assert the same user outcome in the Pixel 5 mobile project.

## Out of scope

- New page objects without reuse value.
- New responsive controls or layout changes.
- Full E2E suite execution.

## Acceptance

- The desktop sidebar shows the complete ordered label and contains the multi-repository task.
- The mobile drawer shows the same label and task.
- Neither surface shows the task in the primary repository-only group.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/task/sidebar-layout.spec.ts -- --grep "multi-repository group"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "multi-repository group"
```

## Files likely touched

- `apps/web/e2e/tests/task/sidebar-layout.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
- `apps/web/e2e/tests/task/sidebar-repository-grouping-helpers.ts`

## Dependencies

Task 01.

## Risks

- The second repository needs a valid local Git repository in the worker temporary directory.
- The mobile command must select the `mobile-chrome` project before the test path.

## Parallelism

`sequential`

## Inputs

- Task 01 result.
- `apps/web/e2e/helpers/api-client.ts` repository and task seed methods.
- Existing sidebar layout and mobile sidebar view tests.

## Results

The production-build desktop regression passed with 1 test. The mobile-chrome regression passed with 1 test using the existing task-switcher drawer. Both assertions found the complete ordered combination label and the task under that group, and confirmed the task was absent from the primary repository-only group.

Verification:

```text
cd apps/web && pnpm e2e:run tests/task/sidebar-layout.spec.ts -- --grep "multi-repository group"  # 1 passed
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "multi-repository group"  # 1 passed
```
