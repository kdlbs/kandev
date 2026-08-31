---
id: "01-restrict-changes-auto-focus"
title: "Restrict Changes auto-focus"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LAYOUT-PROFILES-001
acceptance_criteria:
  - AC-UI-TASK-LAYOUT-PROFILES-001.11
system_design:
  - ../../specs/ui/system-design/task-layout-profiles.md
---

# Task 01: Restrict Changes Auto-focus

## Summary

Restrict automatic Changes activation to the Default top-right group with exactly Files and Changes. Preserve the active tab in all other layouts and group compositions.

## In scope

- Add direct unit coverage for the shared activation guard.
- Track the active built-in or custom profile identity through layout application and environment-scoped restores.
- Preserve the effective default's profile identity through fresh builds and Reset Layout, including reserved built-in overrides.
- Require `RIGHT_TOP_GROUP` and exact `files` and `changes` membership.
- Replace the arbitrary non-Agent-group E2E expectation with the VS Code complaint scenario.
- Preserve Default-layout activation and inactive-task pending behavior.

## Out of scope

- Change the marker and fingerprint logic.
- Change backend layout-profile persistence.
- Change mobile or tablet behavior.

## Acceptance

- A meaningful update activates Changes in the Default Files and Changes group.
- A `vscode | files | changes` group keeps VS Code active after the update.
- A Files and Changes group outside `RIGHT_TOP_GROUP` keeps its active tab.
- A copied Default custom profile keeps its active tab even when its group IDs and tabs match the built-in Default.
- A reserved customized Default remains the built-in Default identity when used for a fresh build or Reset Layout; arbitrary custom defaults remain custom.

## Verification

```bash
pnpm exec vitest run components/task/changes-panel-focus.test.ts
pnpm run typecheck
pnpm test
pnpm run lint
pnpm e2e:run tests/layout/changes-panel-focus.spec.ts -- --grep "VS Code group"
python3 ../../scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/web/components/task/changes-panel-focus.ts apps/web/components/task/changes-panel-focus.test.ts apps/web/e2e/tests/layout/changes-panel-focus.spec.ts docs/specs docs/plans
```

Run the first five commands from `apps/web`. Run the spec test linter from `apps/web` and the all-files spec linter plus the diff check from the repository root. Run `pnpm install --frozen-lockfile` from `apps/` first when dependencies are absent.

## Files likely touched

- `apps/web/components/task/changes-panel-focus.ts`
- `apps/web/components/task/changes-panel-focus.test.ts`
- `apps/web/e2e/tests/layout/changes-panel-focus.spec.ts`
- `apps/web/lib/layout/layout-profiles.ts`
- `apps/web/lib/layout/layout-profiles.test.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-env-switch-action.test.ts`
- `apps/web/lib/state/dockview-preset-persistence.test.ts`
- `apps/web/components/task/dockview-desktop-layout.tsx`

## Dependencies

None.

## Risks

- The E2E stimulus has no completion event for a forbidden activation. Use the existing bounded negative-assertion dwell.

## Parallelism

`sequential`

## Inputs

- `AC-UI-TASK-LAYOUT-PROFILES-001.11`
- The Changes-attention control flow in the task-layout system design.
- Commit `f100fc97a5`, which expanded activation from the Default group to every non-Agent group.

## Results

- `pnpm exec vitest run components/task/changes-panel-focus.test.ts`: passed, 20 tests before the fixup and 20 tests after the fixup.
- `pnpm exec vitest run lib/local-storage.test.ts lib/state/dockview-store.test.ts lib/state/dockview-preset-persistence.test.ts components/task/changes-panel-focus.test.ts`: passed, 92 tests.
- `pnpm run typecheck`: passed.
- `pnpm test`: passed, 1,677 test files and 14,381 tests (4 skipped).
- `pnpm run lint`: passed.
- `pnpm e2e:run tests/layout/changes-panel-focus.spec.ts -- --grep "VS Code group"`: passed, 1 test.
- The complete `changes-panel-focus.spec.ts` suite passed, 6 tests, and produced a fresh desktop PR capture.
- `python3 ../../scripts/lint-spec-files.test.py`: passed, 20 tests.
- `python3 scripts/lint-spec-files.py --all`: passed from the repository root.
- `git diff --check`: passed.
