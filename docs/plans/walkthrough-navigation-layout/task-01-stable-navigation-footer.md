---
id: "walkthrough-navigation-layout-01"
title: "Stable navigation footer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/walkthrough-navigation-layout.md"
---

# Task 01: Stable navigation footer

## Acceptance

1. The open walkthrough gives its explanation an internal scroll region and
   keeps its Previous/Next footer in a stable final-card position while steps
   change.
2. The desktop floating window remains draggable; the phone bottom sheet keeps
   its navigation visible, reachable, and inside the dynamic viewport.
3. Desktop and `mobile-chrome` E2E coverage prove the navigation geometry with
   step changes and retain the existing walkthrough progression behavior.

## Verification

```sh
(
  set -e
  cd apps/web
  pnpm e2e:run tests/review/walkthrough.spec.ts
  pnpm e2e:run tests/review/mobile-walkthrough.spec.ts -- --project=mobile-chrome
)
```

## Files likely touched

- `apps/web/components/diff/walkthrough-step-card.tsx`
- `apps/web/components/diff/walkthrough-floating-window.tsx`
- `apps/web/e2e/tests/review/walkthrough.spec.ts`
- `apps/web/e2e/tests/review/mobile-walkthrough.spec.ts`

## Dependencies

None.

## Parallelism

Sequential; the regression tests and shared walkthrough components change
together.

## Inputs

- [Stable Walkthrough Navigation spec](../../specs/ui/requirements/walkthrough-navigation-layout.md)
- [Fix plan](plan.md)
- Existing `WalkthroughFloatingWindow` mobile bottom-sheet and desktop drag
  behavior.

## Output contract

Report the failing regression-test evidence, implementation files, focused
desktop/mobile E2E results, visual verification, risks, and updated task/plan
status.

## Completion record

- Desktop and `mobile-chrome` walkthrough E2E passed with stable navigation
  geometry assertions.
- Fresh synthetic desktop and mobile captures confirmed the footer is visible,
  reachable, and does not overlap the viewport edge.
- The remaining risk is browser-specific safe-area behavior; the floating
  surface reserves `safe-area-inset-bottom` for the navigation footer.
