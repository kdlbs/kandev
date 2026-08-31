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
  - AC-UI-TASK-LAYOUT-PROFILES-001.9
system_design:
  - ../../specs/ui/system-design/task-layout-profiles.md
---

# Task 01: Restrict Changes Auto-focus

## Summary

Restrict automatic Changes activation to the Default top-right group with exactly Files and Changes. Preserve the active tab in all other layouts and group compositions.

## In scope

- Add direct unit coverage for the shared activation guard.
- Require `RIGHT_TOP_GROUP` and exact `files` and `changes` membership.
- Replace the arbitrary non-Agent-group E2E expectation with the VS Code complaint scenario.
- Preserve Default-layout activation and inactive-task pending behavior.

## Out of scope

- Change the marker and fingerprint logic.
- Change Dockview layout construction or persistence.
- Change mobile or tablet behavior.

## Acceptance

- A meaningful update activates Changes in the Default Files and Changes group.
- A `vscode | files | changes` group keeps VS Code active after the update.
- A Files and Changes group outside `RIGHT_TOP_GROUP` keeps its active tab.

## Verification

```bash
pnpm exec vitest run components/task/changes-panel-focus.test.ts
pnpm run typecheck
pnpm e2e:run tests/layout/changes-panel-focus.spec.ts -- --grep "VS Code group"
python3 ../../scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/web/components/task/changes-panel-focus.ts apps/web/components/task/changes-panel-focus.test.ts apps/web/e2e/tests/layout/changes-panel-focus.spec.ts docs/specs docs/plans
```

Run the first three commands from `apps/web`. Run the spec test linter from `apps/web` and the all-files spec linter plus the diff check from the repository root. Run `pnpm install --frozen-lockfile` from `apps/` first when dependencies are absent.

## Files likely touched

- `apps/web/components/task/changes-panel-focus.ts`
- `apps/web/components/task/changes-panel-focus.test.ts`
- `apps/web/e2e/tests/layout/changes-panel-focus.spec.ts`

## Dependencies

None.

## Risks

- The E2E stimulus has no completion event for a forbidden activation. Use the existing bounded negative-assertion dwell.

## Parallelism

`sequential`

## Inputs

- `AC-UI-TASK-LAYOUT-PROFILES-001.9`
- The Changes-attention control flow in the task-layout system design.
- Commit `f100fc97a5`, which expanded activation from the Default group to every non-Agent group.

## Results

- `pnpm exec vitest run components/task/changes-panel-focus.test.ts`: passed, 19 tests.
- `pnpm run typecheck`: passed.
- `pnpm e2e:run tests/layout/changes-panel-focus.spec.ts -- --grep "VS Code group"`: passed, 1 test.
- The complete `changes-panel-focus.spec.ts` suite passed, 6 tests, and produced a fresh desktop PR capture.
- `python3 ../../scripts/lint-spec-files.test.py`: passed, 20 tests.
- `python3 scripts/lint-spec-files.py --all`: passed from the repository root.
- `git diff --check`: passed.
