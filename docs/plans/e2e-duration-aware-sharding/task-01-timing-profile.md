---
id: "01-timing-profile"
title: "Build the rolling timing profile"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 01: Build the rolling timing profile

## Acceptance

- Merged Playwright blob results produce stable project/file/full-title keys
  and exclude failures, timeouts, and retry attempts from baseline samples.
- A successful `main` profile merges a bounded recent sample history and
  exposes p50/p75, source file hash, source run, and source commit metadata.
- New, changed, stale, and unavailable-profile cases select explicit fallback
  estimates and are reported to the planner instead of being silently treated
  as current timings.

## Verification

```sh
cd apps
pnpm --filter @kandev/web test -- e2e/scripts/e2e-timings.test.ts
pnpm --filter @kandev/web typecheck
```

The unit suite must cover duplicate results, retry-only passes, missing timing
data, changed source hashes, bounded history compaction, and quantile
calculation.

## Files likely touched

- `apps/web/e2e/scripts/e2e-timings.ts`
- `apps/web/e2e/scripts/e2e-timings.test.ts`
- `apps/web/e2e/scripts/plan-shards.ts` (shared profile/catalog types only if
  needed)

## Dependencies

None. Use the Playwright blob result shape already emitted by CI and standard
Node/TypeScript dependencies already present in `apps/web`.

## Parallelism

Independent of the fixture repair tasks. The planner task depends on the
profile schema and classification contract from this task.

## Inputs

- The timing profile and fallback contracts in the linked spec and ADR.
- Current Playwright blob reports from `.github/workflows/e2e-tests.yml`.
- Existing `apps/web/e2e/playwright.config.ts` project matching rules.

## Output contract

Report the profile schema, accepted-sample rule, history bound, p50/p75
implementation, fallback classifications, files changed, and exact targeted
test result. Keep the output format versioned so a malformed or old artifact is
rejected by the planner.

## Implementation result

Implemented `e2e-timings.ts` with versioned profiles, bounded 20-sample
histories, p50/p75 quantiles, stable project/file/title keys, source hashing,
and malformed-blob tolerance. The focused timing/planner/retry suite passed
26 tests across 5 files; web typecheck also passed.
