---
id: "04-connectivity-warning-e2e"
title: "Connectivity warning E2E"
status: done
wave: 4
depends_on:
  ["01-connection-issue-timing", "02-desktop-warning-surfaces", "03-mobile-warning-surfaces"]
plan: "plan.md"
spec: "../../specs/ui/requirements/ws-connectivity-warning.md"
---

# Task 04: Connectivity warning E2E

## Acceptance

- Production-build desktop E2E proves enabled-bar placement, feature-off sidebar fallback,
  yellow/red details, mutual exclusion, and immediate recovery.
- Pixel 5 E2E proves the feature-off warning is visible from persistent route chrome, reaches a
  44 px Status action, opens a connection-only drawer, returns focus, and introduces no viewport or
  horizontal-overflow regression.
- Existing status-bar ordering/geometry E2E is updated only where the now-hidden healthy connection
  content is material; stable connection identity and saved ordering remain covered.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/layout/ws-connectivity-warning.spec.ts tests/layout/mobile-ws-connectivity-warning.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/layout/ws-connectivity-warning.spec.ts`
- `apps/web/e2e/tests/layout/mobile-ws-connectivity-warning.spec.ts`
- `apps/web/e2e/helpers/connection-state.ts` if a typed store-bridge helper keeps tests focused
- `apps/web/e2e/tests/layout/app-status-bar.spec.ts`
- Any existing page object that already owns the affected Status trigger, only if reuse is clearer
  than local selectors

## Dependencies

Tasks 01–03.

## Parallelism

Sequential. The E2E contract spans both rendered implementations.

## Inputs

- Every spec scenario involving a visible surface.
- Plan: `E2E Tests`.
- E2E guidance: production managed runner, store bridge for derived-state injection, stable
  `data-testid` selectors, and `mobile-chrome` routing through the `mobile-*.spec.ts` filename.

## Risks

- Do not add real ten-second sleeps; unit tests own timing and E2E owns integration/presentation.
- A backend restart used to disable the feature must restore baseline configuration in teardown.
- Rebuild production assets through `pnpm e2e:run`; do not test a stale Vite bundle.

## Output contract

Report scenarios, screenshots/rendered evidence where captured, exact command and result,
blockers/risks, then mark this task `done` and update its checkbox in `plan.md`.
