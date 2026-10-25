---
id: "02-view-picker"
title: "Mobile Views picker"
status: complete
wave: 2
depends_on: ["01-query-recovery"]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.2
  - AC-INTEGRATIONS-GITHUB-MOBILE-001.5
system_design:
  - ../../specs/integrations/system-design/github-dashboard-mobile.md
---
# Mobile Views picker

## Summary and scope

Implement the picker composition section of the [design](../../specs/integrations/system-design/github-dashboard-mobile.md), including its focused regressions and translations. Own only the files listed below and required adjacent tests.

## Out of scope

Other work-order outcomes and changes to persistence or provider API contracts.

## Acceptance

- The referenced acceptance criteria pass through the real interaction/state boundary.
- Failure paths retain prior state and permit recovery; desktop behavior remains intact.
- Required targeted checks pass and results are recorded.

## ASCII UI preview

### UI-02: Mobile Views picker

```text
GitHub
[Views: Review requested v]
Drawer: Views                  [Done]
[Pull requests] [Issues]
  Inbox / Created / Saved (scroll)
[Save current query] (fixed footer)
```

See the [combined preview](plan.md#ascii-ui-preview). Geometry and hierarchy are structural; ASCII spacing is illustrative.

## Verification

From the repository root:

```bash
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/github/mobile-github-sidebar.spec.ts)
```

Hook regressions use Red-Green-Refactor. Run new/changed tests after the final source edit.

## Files likely touched

Paths relative to apps/web: `app/github/github-page-client.tsx`, `components/github/my-github/mobile-views-picker.tsx`, `presets-sidebar.tsx`, the integration toolbar title slot, and `e2e/pages/mobile-github-page.ts`.

## Dependencies

Task 01-query-recovery.

## Risks

Keep kind switches open, retain explicit selection/default behavior, update every mobile test using the old closing behavior.

## Parallelism

sequential

## Inputs

[Requirements](../../specs/integrations/requirements/github-dashboard-mobile.md), paired design, existing source and tests named above.

## Results

Named Views control opens an inset bottom drawer. Kind switching stays open, final selection/Done closes, focus returns, and a 20-view collection scrolls beneath fixed header/footer controls at 320px. Eight GitHub query/recovery browser scenarios pass; desktop scope-bar scenarios also pass.

Open PR revalidation on 2026-09-11 exposed a drawer/save focus race. A slowed-close regression first failed because the form opened before the drawer released focus. Opening from the drawer's close/focus callback fixes the overlap, and the existing focus-return helper restores the initiating control after Cancel. All seven scenarios in `mobile-github-sidebar.spec.ts` pass after this remediation, including saved-repository selection, failed-save retry, and the new focus regression. Final delivery commands are recorded in the plan.
