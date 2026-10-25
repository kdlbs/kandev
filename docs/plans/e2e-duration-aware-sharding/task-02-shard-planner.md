---
id: "02-shard-planner"
title: "Generate duration-aware shard manifests"
status: completed
wave: 2
depends_on:
  - "01-timing-profile"
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 02: Generate duration-aware shard manifests

## Acceptance

- The planner creates deterministic fourteen-shard normal and six-shard
  container manifests using weighted project/file units and stable
  longest-processing-time bin packing.
- Manifest validation rejects missing, duplicated, out-of-cohort, or
  unaccounted catalog units and records predicted cost, unknown count, warm
  count, profile source, and fallback mode.
- The planner identifies indivisible files that dominate a shard and leaves a
  tested seam for later `file:line` splitting without enabling it by default.

## Verification

```sh
cd apps
pnpm --filter @kandev/web test -- e2e/scripts/plan-shards.test.ts
pnpm --filter @kandev/web typecheck
```

Tests must cover stable tie-breaking, repeated planning with the same inputs,
empty/unknown catalogs, project aggregation, changed-file multipliers, exact
coverage, overlap rejection, and malformed manifest versions.

## Files likely touched

- `apps/web/e2e/scripts/plan-shards.ts`
- `apps/web/e2e/scripts/plan-shards.test.ts`
- `apps/web/e2e/scripts/e2e-timings.ts` (only for shared types or fallback
  resolution)

## Dependencies

Depends on Task 01's versioned profile schema, stable test identity, and
classification/fallback contract.

## Parallelism

Sequential after Task 01. CI wiring must consume the manifest shape created
here rather than reimplementing assignment logic in YAML or shell.

## Inputs

- The current normal and container project definitions.
- The profile/catalog types and fallback rules from Task 01.
- The shard counts currently defined in `.github/workflows/e2e-tests.yml`.

## Output contract

Report the manifest schema, bin-packing rule, coverage validation behavior,
sample predicted distributions, heavy-file diagnostics, files changed, and
exact targeted test result. Include a small checked-in fixture input in the
unit tests, not a generated CI manifest.

## Implementation result

Implemented deterministic LPT planning and strict manifest validation. The
local dry-run produced 14 normal and 6 container manifests with complete
catalog coverage; the containers cohort reported the Docker launch file as a
dominant indivisible unit at 240 predicted seconds and 63.2% of its shard.
The focused planner tests passed as part of the 15-test tooling run.
Review remediation replaced source-regex counting with Playwright's discovered
project/test catalog. The planner now matches all 2,103 discovered tests (1,989
normal and 114 container tests after the new cleanup regression cases).
