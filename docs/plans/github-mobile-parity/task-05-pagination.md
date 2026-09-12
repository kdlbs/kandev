---
id: "05-pagination"
title: "Compact phone pagination"
status: complete
wave: 5
depends_on: ["04-result-actions"]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.4
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.5
system_design:
  - ../../specs/integrations/system-design/github-dashboard-mobile.md
---
# Compact phone pagination

## Summary and scope

Implement the pagination section of the [design](../../specs/integrations/system-design/github-dashboard-mobile.md), including its focused regressions and translations. Own only the files listed below and required adjacent tests.

## Out of scope

Other work-order outcomes and changes to persistence or provider API contracts.

## Acceptance

- The referenced acceptance criteria pass through the real interaction/state boundary.
- Failure paths retain prior state and permit recovery; desktop behavior remains intact.
- Required targeted checks pass and results are recorded.

## ASCII UI preview

### UI-05: Compact phone pagination

```text
Results (scroll)
101-125 of 1000+
[Previous] [Page 5 of 40 v] [Next]
```

See the [combined preview](plan.md#ascii-ui-preview). Geometry and hierarchy are structural; ASCII spacing is illustrative.

## Verification

From the repository root:

```bash
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/github/mobile-github-results.spec.ts)
```

Hook regressions use Red-Green-Refactor. Run new/changed tests after the final source edit.

## Files likely touched

Paths relative to apps/web: `components/github/my-github/results-pagination.tsx` and its unit/browser regressions.

## Dependencies

Task 04-result-actions.

## Risks

Keep direct page selection and the search cap; test page 5 of 40 and widths on both sides of 768px.

## Parallelism

sequential

## Inputs

[Requirements](../../specs/integrations/requirements/github-dashboard-mobile.md), paired design, existing source and tests named above.

## Results

Three pagination unit tests pass. A phone browser scenario seeds 1,050 PRs, navigates directly to pages 5 and 40, verifies disabled previous/next limits, checks no overflow at 320px, and confirms the 767/768px presentation boundary. The existing 1,000-result limit and search state remain unchanged.
