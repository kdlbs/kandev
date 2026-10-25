---
id: "02-consolidate-frontend-contract"
title: "Consolidate frontend feature contract"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/feature-toggles.md"
---

# Task 02: Consolidate frontend feature contract

- **Acceptance:** Frontend feature names, `FeatureName`, `FeatureFlags`, and
  all-false defaults derive from one declaration; the Zustand slice does not
  repeat each key.
- **Acceptance:** SSR normalization and `useFeature()` continue to treat missing,
  malformed, or unreachable backend values as false.
- **Acceptance:** Focused tests derive their complete feature fixtures instead
  of maintaining a second hardcoded list.
- **Verification:** Red first with the single-declaration/fail-closed tests, then
  run `cd apps && pnpm install --frozen-lockfile`; run `cd apps && pnpm --filter
  @kandev/web exec vitest run lib/state/slices/features/features-slice.test.ts
  app/actions/features.test.ts`; run `cd apps/web && pnpm run typecheck`. Run
  `cd apps && pnpm --filter @kandev/web lint`.
- **Files likely touched:**
  - `apps/web/lib/state/slices/features/types.ts`
  - `apps/web/lib/state/slices/features/features-slice.ts`
  - `apps/web/lib/state/slices/features/features-slice.test.ts`
  - `apps/web/app/actions/features.test.ts`
- **Dependencies:** None.
- **Parallelism:** `parallel-safe` with Task 01 only; files are disjoint and
  there is no shared generated contract, schema, lockfile, or package config.
- **Inputs:** Feature Toggles spec `What`, `Failure modes`, and `Scenarios`;
  `apps/web/AGENTS.md`; existing `getFeatureFlagsAction` and `useFeature`.
- **Output contract:** Report changed files, exact commands and counts,
  blockers/risks, rendered/mobile impact (`None; normalization only` expected),
  external side effects (`None` expected), then update this task and `plan.md`
  statuses/results in the same conversation.

## Results

- Added `defaultFeatureFlags` as the single all-off declaration and derived
  `FeatureName`/`FeatureFlags` from its keys.
- Updated the Zustand slice and server action to consume that declaration;
  malformed, missing, and unreachable values remain fail-closed.
- Updated slice/action tests to derive complete fixtures from the declaration.
- Review remediation added a repository contract test that extracts backend
  `FeaturesConfig` JSON tags and compares them exactly with frontend defaults.
- Review verification: the contract test and full web Vitest suite passed;
  web typecheck and lint passed.
- Verification: `pnpm install --frozen-lockfile` passed; focused Vitest passed
  (2 files, 8 tests); `pnpm run typecheck` passed; `pnpm run lint` passed.
- Rendered/mobile impact: none; state/type normalization only.
- External side effects: none.
