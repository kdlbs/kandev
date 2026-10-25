---
id: "01-type-features-slice-setter"
title: "Type the Features slice setter"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-FEATURE-TOGGLES-001
acceptance_criteria:
  - AC-PLATFORM-FEATURE-TOGGLES-001.8
system_design:
  - ../../specs/platform/system-design/features-slice-state.md
---

# Task 01: Type the Features slice setter

## Summary

Type the Features slice creator with the single Immer recipe-setter dependency
used by its implementation. Keep the existing flag defaults, SSR bootstrap,
hydration, and full-map update behavior unchanged.

## In scope

- Remove root-store casts from the Features slice invocation.
- Keep standalone slice composition and real root-store composition typed.
- Verify `setFeatures` updates feature values and preserves unrelated root state.
- Remove exactly the three Features root-state-cast baseline occurrences.
- Document the existing Features state projection and delivery evidence.

## Out of scope

- Feature-toggle behavior, backend configuration, profile defaults, and runtime
  flag resolution.
- System slice/query ownership, other slice casts, or global store
  reorganization.

## Acceptance

- `createFeaturesSlice` accepts only a recipe callback over
  `Draft<FeaturesSlice>`; its type exposes no replacement flag or whole-slice
  replacement overload.
- Root composition calls `createFeaturesSlice(set)` without casts, and the
  standalone slice test composes without assertions or `any`.
- Existing all-false defaults, server-provided boot values, fail-closed
  normalization, and whole-map `setFeatures` updates remain intact. A root-store
  regression checks that auth and workspace state retain their identity across
  a feature update.
- The baseline removes exactly these entries: path
  `apps/web/lib/state/store.ts`, escape `as any`, marker
  `...createFeaturesSlice(set as any, get as any, api as any),`, occurrences
  1, 2, and 3.

## Verification

```bash
(cd apps/web && pnpm exec vitest run app/actions/features.test.ts lib/state/slices/features/features-slice.test.ts lib/state/slices/features/features-contract.test.ts lib/state/store.test.ts lib/state/hydration/hydrator.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
python3 scripts/lint-architecture.py --all
python3 scripts/lint-architecture.test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/lib/state/slices/features/features-slice.ts`
- `apps/web/lib/state/slices/features/features-slice.test.ts`
- `apps/web/lib/state/store.ts`
- `config/architecture-lint/frontend_root_state_cast.json`
- `docs/specs/platform/README.md`
- `docs/specs/platform/system-design/features-slice-state.md`
- `docs/plans/features-slice-root-typing/plan.md`
- `docs/plans/features-slice-root-typing/task-01-type-features-slice-setter.md`

## Dependencies

None.

## Risks

None. The change narrows a local creator dependency and preserves the existing
state and feature-gate contracts.

## Parallelism

`sequential`

## Inputs

- [Feature toggles requirement](../../specs/platform/requirements/feature-toggles.md)
- [Frontend feature state design](../../specs/platform/system-design/features-slice-state.md)
- [Runtime feature flags ADR](../../decisions/0007-runtime-feature-flags.md)
- [Release toggle gating ADR](../../decisions/2026-08-01-release-toggle-gating-contract.md)
- Features slice and root-store source/tests in the files above.

## Results

- Focused action, feature slice, contract, root-store, and hydration checks:
  55 tests passed across 5 files.
- Web typecheck and lint passed.
- Architecture scan passed; 3 Features findings were removed from a 49-entry
  baseline. The architecture-lint suite passed all 62 tests.
- `python3 scripts/list-docs.py validate` passed (309 decisions and 1184
  specifications validated).
- `python3 scripts/lint-spec-files.test.py` passed 36 tests, and
  `python3 scripts/lint-spec-files.py --all` passed.
- The new system design appears in the platform spec catalog. The local PR
  documentation coverage evaluator returned `covered`, accepting this work
  order for the three triggering source/configuration paths.
- `git diff --check` passed.
