---
id: "01-restore-custom-default-proportions"
title: "Restore custom-default proportions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LAYOUT-PROFILES-001
acceptance_criteria:
  - AC-UI-TASK-LAYOUT-PROFILES-001.9
  - AC-UI-TASK-LAYOUT-PROFILES-001.10
system_design:
  - ../../specs/ui/system-design/task-layout-profiles.md
---

# Task 01: Restore Custom-Default Proportions

## Summary

Make fresh desktop tasks and Reset Layout use the geometry from the effective custom default. Preserve the responsive behavior of built-in presets and named intents.

## In scope

- Add a failing store regression for custom-default proportions.
- Resolve saved custom-default widths for the measured workbench.
- Store and apply the same pinned-width map.
- Pass the resolved widths through the fast environment-switch path.
- Extend the existing desktop layout-profile E2E scenario.

## Out of scope

- Backend or saved-layout format changes.
- Mobile and tablet task composition.
- Per-environment manual sash-width behavior.

## Acceptance

- A fresh desktop task uses the proportions from the effective custom default.
- Reset Layout applies the same saved proportions.
- A fast switch into an unsaved environment applies the same saved proportions.
- Built-in presets and named intents continue to use responsive default widths.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/state/dockview-preset-persistence.test.ts lib/state/dockview-store.test.ts
cd apps/web && pnpm e2e:run tests/settings/layout-profiles.spec.ts -- --grep "fresh tasks use the no-terminal default while existing tasks wait for Reset Layout"
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-env-switch.ts`
- `apps/web/lib/state/dockview-preset-persistence.test.ts`
- `apps/web/lib/state/dockview-env-switch-pinned.test.ts`
- `apps/web/e2e/tests/settings/layout-profiles.spec.ts`
- `docs/plans/custom-default-layout-proportions/plan.md`
- `docs/plans/custom-default-layout-proportions/task-01-restore-custom-default-proportions.md`

## Dependencies

None.

## Risks

- Intent-based layouts can use the wrong saved widths if the source check is too broad.
- Post-layout enforcement can overwrite the result if `pinnedWidths` stays empty.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/task-layout-profiles.md`
- `docs/specs/ui/system-design/task-layout-profiles.md`
- `docs/decisions/0041-backend-owned-portable-user-settings.md`
- Existing custom-layout width resolution in `apps/web/lib/state/dockview-store.ts`.
- Existing default-layout coverage in `apps/web/lib/state/dockview-preset-persistence.test.ts`.

## Results

- Updated `performBuildDefault` to resolve scaled pinned widths when the
  effective default comes from a saved custom profile.
- Passed the same scaled widths into fast environment switches when the target
  environment has no task-specific saved layout.
- Stored and applied the same width map. Named built-in intents keep their
  responsive preset widths.
- Added store and fast-switch regressions for the scaled custom default and the
  named-intent safeguard.
- Extended the existing layout-profile E2E test with a 75/25 split assertion
  for fresh tasks and Reset Layout.
- Updated the public layout-profile guidance and the durable requirements and
  system design.
- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/dockview-preset-persistence.test.ts lib/state/dockview-store.test.ts` passed (44 tests).
- `cd apps/web && pnpm e2e:run tests/settings/layout-profiles.spec.ts -- --grep "fresh tasks use the no-terminal default while existing tasks wait for Reset Layout"` passed (1 test).
- `cd apps/web && pnpm run typecheck` passed.
