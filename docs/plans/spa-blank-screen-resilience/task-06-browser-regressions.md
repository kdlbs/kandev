---
id: "06-browser-regressions"
title: "Prove production desktop and mobile resilience"
status: done
wave: 3
depends_on:
  [
    "01-failure-containment",
    "02-settings-route-determinism",
    "03-mobile-selector-stability",
    "04-preload-recovery",
    "05-self-update-reload",
  ]
plan: "plan.md"
spec: "../../specs/ui/requirements/mobile-task-navigation.md"
decision: "../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md"
---

# Task 06: Prove Production Desktop and Mobile Resilience

## Acceptance

- Production Playwright delays the first Settings chunk and sees accessible
  loading instead of an empty route on desktop and `mobile-chrome`.
- Aborting the first fingerprinted Settings chunk triggers exactly one document
  reload and then renders Settings.
- Persistently aborting that chunk triggers one automatic reload, then visible
  route recovery with no reload loop.
- On mobile, the recovery action is at least 44px high and produces no
  horizontal overflow.
- Reusing the existing optional route-hydration failure pattern, a
  multi-repository task with delayed/failed repository or session enrichment
  opens its mobile repository picker without `pageerror`, update-depth loop, or
  blank root. Task 03's real-store component test owns exact snapshot identity.
- The service update browser test proves no reload occurs before the target
  version and a real document reload occurs after confirmation.
- Browser interception targets the built Settings chunk, not the entry bundle,
  and all tests fail on relevant console/page errors.

## Files likely touched

- `apps/web/e2e/tests/layout/spa-resilience.spec.ts` (new)
- `apps/web/e2e/tests/layout/mobile-spa-resilience.spec.ts` (new)
- `apps/web/e2e/tests/layout/spa-resilience-helpers.ts` (new if shared setup warrants it)
- `apps/web/e2e/tests/system/updates-page.spec.ts`
- Existing E2E page objects/helpers only if a shared interaction is warranted

## Verification

```bash
cd apps/web
pnpm e2e:run --project chromium -- tests/layout/spa-resilience.spec.ts tests/system/updates-page.spec.ts --workers=1
pnpm e2e:run --project mobile-chrome -- tests/layout/mobile-spa-resilience.spec.ts --workers=1
```

## Output contract

Report RED/GREEN production-build evidence for each desktop/mobile scenario,
exact commands, test counts, artifacts for failures, files changed, and any
remaining flake risk. The primary session updates this task and `plan.md` after
accepting the result.
