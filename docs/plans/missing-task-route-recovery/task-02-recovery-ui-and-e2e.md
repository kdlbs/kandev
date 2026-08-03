---
id: "02-recovery-ui-and-e2e"
title: "Add unavailable-route recovery UI and E2E"
status: completed
wave: 2
depends_on: ["01-fallback-boot-context"]
plan: "plan.md"
spec: "../../specs/missing-task-route-recovery/spec.md"
---

# Task 02: Add unavailable-route recovery UI and E2E

Give the unavailable task surface a workspace-preserving overview action and
prove that a cold missing-task route keeps desktop sibling navigation and a
phone-safe recovery path.

## Acceptance

- The unavailable state keeps its generic error semantics and exposes an
  accessible overview link scoped to the resolved workspace when one exists.
- A desktop cold load lists and can navigate to a visible sibling task in the
  global sidebar instead of showing **No tasks yet**.
- On Pixel 5, the overview action is touch-sized and viewport-contained, does
  not introduce horizontal document overflow, and opens Home where the sibling
  task is visible.

## Files likely touched

- `apps/web/components/task/task-page-content.tsx`
- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pseudo/common.json`
- `apps/web/src/task-detail-route.test.tsx`
- `apps/web/components/task/task-page-content.test.tsx`
- `apps/web/e2e/tests/task/task-loading-state.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-loading-state.spec.ts`

## Dependencies

- Task 01 must be complete so production-build E2E receives fallback workspace
  snapshots from the Go boot payload.

## Parallelism

`sequential`. This task consumes Task 01 and its desktop/mobile E2E files share
the same production build and task-route behavior.

## Inputs

- Spec sections **Desired behavior** and **Regression scenarios**.
- Plan sections **Unavailable task recovery action**, **Mobile design
  contract**, and **E2E Tests**.
- `apps/web/components/task/mobile/session-mobile-top-bar.tsx` as the existing
  task-overview navigation exemplar.
- `apps/web/e2e/tests/task/task-loading-state-helpers.ts` and current desktop/
  mobile loading specs as fixture and routing patterns.

## TDD sequence

1. Extend the client route unit test and desktop/mobile E2E specs, then run the
   focused tests against the current implementation to capture the expected
   failures.
2. Add the localized overview action and workspace-scoped destination.
3. Run focused frontend checks.
4. Rebuild through the managed E2E runner, pass the desktop regression, then
   reuse the unchanged build for the mobile regression.

## Verification

Run the workspace install once first if `apps/node_modules` is absent:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web exec vitest run src/task-detail-route.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/task/task-loading-state.spec.ts -- --grep 'keeps sibling tasks available'
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-task-loading-state.spec.ts -- --grep 'returns to task overview'
```

Confirm Playwright discovers and runs one intended test for each focused E2E
command; `No tests found` is not passing evidence.

## Risks

- The overview action must use the fallback store workspace rather than the
  missing task ID, or it can navigate Home into a different workspace.
- The mobile assertion must measure the actual link hitbox and document
  overflow after the unavailable state settles; visibility alone would miss
  the touch and containment contract.

## Output contract

Report RED and GREEN evidence, exact files changed, desktop and Pixel 5 user
outcomes, i18n/typecheck results, generated artifact paths, cleanup, remaining
risks, and synchronized task/plan status. Record every exact command and outcome
in `## Results`.

## Results

Implemented the workspace-aware unavailable-task recovery action and localized
the loading/error copy in
`apps/web/components/task/task-page-content.tsx` plus the English and pseudo
common catalogs. Added route-boundary, desktop, and Pixel 5 E2E coverage.

- RED: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-loading-state.spec.ts -- --grep 'returns to task overview'` — 1 failed as expected because `task-unavailable-overview-link` did not exist.
- Client contract: `cd apps && pnpm --filter @kandev/web exec vitest run src/task-detail-route.test.tsx` — 3 passed.
- Frontend checks: `cd apps/web && pnpm run typecheck` — passed; `pnpm run i18n:check` — passed (2423 referenced keys, pseudo in sync); `pnpm run i18n:ratchet` — passed.
- Desktop GREEN: `cd apps/web && pnpm e2e:run tests/task/task-loading-state.spec.ts -- --grep 'keeps sibling tasks available'` — 1 passed.
- Mobile GREEN: `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-task-loading-state.spec.ts -- --grep 'returns to task overview'` — 1 passed.
- Targeted Prettier and ESLint checks passed with no errors or warnings; typecheck and the i18n ratchets are clean.
- `cd apps && pnpm exec prettier --check web/components/task/task-page-content.tsx web/components/task/task-page-content.test.tsx web/src/task-detail-route.test.tsx web/e2e/tests/task/task-loading-state.spec.ts web/e2e/tests/task/mobile-task-loading-state.spec.ts web/src/locales/en/common.json web/src/locales/pseudo/common.json` — passed.
- `cd apps && pnpm --filter @kandev/web exec eslint components/task/task-page-content.tsx components/task/task-page-content.test.tsx src/task-detail-route.test.tsx e2e/tests/task/task-loading-state.spec.ts e2e/tests/task/mobile-task-loading-state.spec.ts` — passed with no errors or warnings.
- The managed E2E runner rebuilt the Go backend and Vite bundle, and cleaned its temporary test-results/blob-report directories after each run; no generated artifacts are part of the change.

The desktop flow keeps sibling rows usable after a cold missing-task route. The
Pixel 5 flow keeps the recovery link in the viewport at a 44px hitbox with no
horizontal overflow and returns to Home scoped to the resolved workspace.
