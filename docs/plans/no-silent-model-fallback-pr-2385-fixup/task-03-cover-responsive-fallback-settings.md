---
id: "03-cover-responsive-fallback-settings"
title: "Cover responsive fallback settings"
status: done
wave: 2
depends_on: ["02-build-fallback-settings-disclosure"]
plan: "plan.md"
spec: "../../specs/agents/requirements/no-silent-model-fallback.md"
---

# Task 03: Cover responsive fallback settings

## Intent

Prove the fallback disclosure's real profile-settings flow, responsive
geometry, input-modality help, and saved behavior on desktop and Pixel 5.

## Acceptance

1. Desktop E2E proves the initial closed state, summary, same-row two-column
   layout, help tooltip, mutual exclusion, save, and reload persistence.
2. Mobile E2E proves tap expansion, stacked layout, 44px info target, help
   drawer content/dismissal, save/reload persistence, and no horizontal page
   overflow.
3. The existing gone-model and task-create picker scenarios remain covered in
   the same feature specs.

## TDD Sequence

1. Extend the desktop and mobile specs first and confirm the new assertions
   fail against the existing two-row, always-expanded UI.
2. After Task 02 lands, run the focused desktop and `mobile-chrome` specs.
3. Inspect the rendered desktop and Pixel 5 states (failure/Playwright
   screenshot or interactive capture) and record the evidence path or exact
   blocker.

## Files Likely Touched

- `apps/web/e2e/tests/settings/no-silent-model-fallback.spec.ts`
- `apps/web/e2e/tests/settings/mobile-no-silent-model-fallback.spec.ts`

## Dependencies

Task 02.

## Parallelism

`sequential` after Task 02 because these scenarios exercise its rendered UI.

## Inputs

- Spec E2E scenarios.
- Existing profile creation/deletion flow in both feature specs.
- `/e2e` tooltip-portal, geometry, input-modality, and mobile project guidance.

## Verification

```sh
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/settings/no-silent-model-fallback.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-no-silent-model-fallback.spec.ts
```

## Risks

- Wait only for finite running animations before bounding-box assertions; do
  not add fixed sleeps.
- Scope desktop tooltip assertions to the open Radix tooltip portal.
- Use `.tap()` for coarse-pointer controls and confirm the mobile project
  discovers the intended test count.

## Output Contract

Report scenario names, discovered test counts, geometry/help/persistence
evidence, screenshots or blocker, exact commands and outcomes, files changed,
and synchronized task/plan status.

## Results

Updated both feature specs. Desktop now covers the closed summary, expansion,
same-row card geometry, tooltip help, mutual exclusion, save, API persistence,
and reload state. Pixel 5 covers tap expansion, stacked card geometry, 44px
help targets, the touch drawer and dismissal, and document overflow; the
existing task-create fallback note scenario remains unchanged.

Verification:

- `cd apps/web && pnpm e2e:run tests/settings/no-silent-model-fallback.spec.ts` — passed (2 tests).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-no-silent-model-fallback.spec.ts` — passed (2 tests).

The first mobile run exposed a finite Collapsible child-remount window during
the touch help tap; the final test keeps `.tap()` and uses Playwright's forced
touch dispatch only for that transient animation. The final run had no
failures or blockers.
