---
id: "03-health-issues"
title: "Contain health issue cards"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/dialog-content-containment.md"
design: "../../specs/ui/system-design/dialog-content-containment.md"
---

# Task 03: Contain Health Issue Cards

## Outcome

Many System Health issues scroll inside their dialog, and every issue Fix action
remains reachable and touch-safe on desktop and phone.

## In scope

- Add focused component coverage for rendering and unchanged navigation
  callbacks.
- Add desktop and mobile-Chrome browser regressions by intercepting the system
  health response with many actual `HealthIssue` shapes.
- Make the issue-card list the one scroll body and add stable layout selectors.
- Give phone Fix actions a 44-pixel minimum height while retaining compact
  desktop density.

## Exclusions

- Do not change health polling, issue models, labels, ordering, or fix URLs.
- Do not add a persistent footer because Fix actions are item-local.
- Do not change the shared Dialog primitive.

## Applicable requirements

- `REQ-UI-DIALOG-CONTENT-CONTAINMENT-001`
- `AC-UI-DIALOG-CONTENT-CONTAINMENT-001.1`, `.2`, `.4` through `.9`

## Implementation acceptance

1. Many issue cards create one body scroll range while the outer dialog, title,
   count, and close control remain inside both viewports.
2. The final issue and its Fix action are reachable; tapping Fix keeps the
   existing close-then-navigate behavior.
3. Short issue lists retain compact height, phone Fix actions meet 44 pixels,
   and the document has no horizontal overflow.

## TDD sequence

1. Add long health fixtures, stable selectors, and desktop/phone cases.
2. RED: record the current oversized dialog and unreachable final Fix action.
3. GREEN: apply the bounded header/body composition and responsive action
   sizing.
4. Run focused component, E2E, type, lint, i18n, and diff checks.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/system-health/health-indicator.test.tsx)
(cd apps/web && pnpm e2e:run --host -- tests/system-health-dialog.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/mobile-system-health-dialog.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/system-health/health-indicator.tsx components/system-health/health-indicator.test.tsx e2e/tests/system-health-dialog.spec.ts e2e/tests/mobile-system-health-dialog.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/web/components/system-health/health-indicator.tsx`
- `apps/web/components/system-health/health-indicator.test.tsx`
- `apps/web/e2e/tests/system-health-dialog.spec.ts`
- `apps/web/e2e/tests/mobile-system-health-dialog.spec.ts`
- `docs/plans/dialog-content-containment/plan.md`
- `docs/plans/dialog-content-containment/task-03-health-issues.md`

## Dependencies

None.

## Parallelism

`sequential`. This work order owns one responsive RED-GREEN cycle.

## Output contract

Record RED/GREEN geometry, final Fix reachability, routing evidence, focused
results, files changed, cleanup, blockers, and residual risks.

## Results

- RED: The long health response made the issue dialog content-sized, leaving
  the final Fix action outside the viewport.
- GREEN: Issue cards now occupy one capped scrolling body. Each Fix action
  remains inside its card, receives a 44-pixel minimum phone height, and keeps
  the existing close-then-navigate behavior.
- The focused component test and desktop/mobile geometry cases passed,
  including final Fix reachability, hit testing, URL navigation, and phone
  document-overflow checks.
- Health polling, issue data, ordering, labels, and fix URLs remain unchanged.
