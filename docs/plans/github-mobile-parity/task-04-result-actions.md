---
id: "04-result-actions"
title: "Touchable result actions"
status: complete
wave: 4
depends_on: ["03-task-views"]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.3
system_design:
  - ../../specs/integrations/system-design/github-dashboard-mobile.md
---
# Touchable result actions

## Summary and scope

Implement the result rows section of the [design](../../specs/integrations/system-design/github-dashboard-mobile.md), including its focused regressions and translations. Own only the files listed below and required adjacent tests.

## Out of scope

Other work-order outcomes and changes to persistence or provider API contracts.

## Acceptance

- The referenced acceptance criteria pass through the real interaction/state boundary.
- Failure paths retain prior state and permit recovery; desktop behavior remains intact.
- Required targeted checks pass and results are recorded.

## ASCII UI preview

### UI-04: Touchable result actions

```text
Wrapped PR/issue title          [Task]
owner/repository#123
[Linked task   Workflow step]
```

See the [combined preview](plan.md#ascii-ui-preview). Geometry and hierarchy are structural; ASCII spacing is illustrative.

## Verification

From the repository root:

```bash
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/github/mobile-issue-list-task-indicator.spec.ts tests/github/mobile-github-results.spec.ts)
```

Hook regressions use Red-Green-Refactor. Run new/changed tests after the final source edit.

## Files likely touched

Paths relative to apps/web: `components/github/my-github/issue-list.tsx`, `components/integrations/change-request-list.tsx`, `task-row-indicator.tsx`, and `integration-start-task-menu.tsx`.

## Dependencies

Task 03-task-views.

## Risks

Shared integration primitives affect other providers; retain fine-pointer density and independent links/actions.

## Parallelism

sequential

## Inputs

[Requirements](../../specs/integrations/requirements/github-dashboard-mobile.md), paired design, existing source and tests named above.

## Results

The baseline issue action measured 24px in Playwright; it now measures at least 44px. PR/issue actions reuse the shared launcher, long titles/repositories wrap, and linked tasks show their step on touch devices. Mobile action/navigation and desktop PR/issue/launcher regressions pass.
