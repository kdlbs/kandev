---
created: 2026-09-26
status: done
requirements:
  - REQ-PLATFORM-FEATURE-TOGGLES-001
system_design:
  - ../../specs/platform/system-design/features-slice-state.md
legacy_specs: []
---

# Implementation Plan: Features slice root typing

## Overview

Remove the Features slice's root-composition casts by typing only the Immer
recipe setter it uses. Preserve the existing all-false defaults, SSR bootstrap,
hydration, and `setFeatures` behavior. The code change is complete in PR #3971;
this package documents the existing boundary and its verification.

## Scope

### In scope

- Type `createFeaturesSlice` with a recipe-only setter dependency.
- Compose the Features slice in the root store with `createFeaturesSlice(set)`.
- Preserve the default, bootstrap, hydration, and whole-map update contracts.
- Remove the three Features entries from the root-state-cast baseline.

### Out of scope

- Runtime flag definitions, profile values, backend defaults, or toggle
  behavior.
- Root-store casts owned by other slices or a broader store redesign.

## Technical approach

- `apps/web/lib/state/slices/features/features-slice.ts`: accept only the
  Immer recipe setter over `Draft<FeaturesSlice>`.
- `apps/web/lib/state/store.ts`: call `createFeaturesSlice(set)` without casts.
- `apps/web/lib/state/slices/features/features-slice.test.ts`: cover defaults,
  boot hydration, map updates, and preservation of unrelated root state.
- `config/architecture-lint/frontend_root_state_cast.json`: remove exactly
  occurrences 1, 2, and 3 of the marker
  `...createFeaturesSlice(set as any, get as any, api as any),` for
  `apps/web/lib/state/store.ts`.

## Tests

- `AC-PLATFORM-FEATURE-TOGGLES-001.8`: `apps/web/app/actions/features.test.ts`
  covers boolean normalization and fail-closed missing, invalid, and
  unavailable responses.
- Root composition and state behavior:
  `apps/web/lib/state/slices/features/features-slice.test.ts`,
  `apps/web/lib/state/slices/features/features-contract.test.ts`,
  `apps/web/lib/state/store.test.ts`, and
  `apps/web/lib/state/hydration/hydrator.test.ts`.

This type-only refactor changes no rendered UI behavior, so it adds no E2E flow.

## Work orders

- [x] [Task 01: Type the Features slice setter](task-01-type-features-slice-setter.md)

## Verification results

- Focused feature, action, contract, root-store, and hydration tests passed:
  55 tests across 5 files.
- Web typecheck and lint passed.
- `python3 scripts/lint-architecture.py --all` passed. The total baseline
  decreased from 49 findings to 46; all 3 removals are Features entries.
- `python3 scripts/lint-architecture.test.py` passed 62 tests.
- Specification catalog validation passed; the new design appears in the
  platform catalog. Specification linter tests passed 36 tests, and all
  specification files passed lint.
- The local PR documentation coverage evaluator returned `covered` for the
  changed source/configuration paths and this linked work order.
- `git diff --check` passed.
