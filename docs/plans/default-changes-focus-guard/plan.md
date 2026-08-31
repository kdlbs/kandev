---
created: 2026-08-31
status: done
requirements:
  - REQ-UI-TASK-LAYOUT-PROFILES-001
system_design:
  - ../../specs/ui/system-design/task-layout-profiles.md
legacy_specs: []
---

# Implementation Plan: Restrict Changes Auto-focus

## Overview

Restrict automatic Changes activation to the Default layout's Files and Changes group. The implementation and its regression tests form one sequential work order.

## Scope

### In scope

- Apply one shared eligibility rule to active-task and returning-task updates.
- Track the active built-in or custom profile identity across preset application, restores, task switches, and reloads.
- Require the stable Default top-right group.
- Require exactly the Files and Changes tabs in that group.
- Preserve the current tab in VS Code, Plan, Preview, compact, and custom group compositions.

### Out of scope

- Change Git-status detection, fingerprinting, or inactive-task pending state.
- Change backend layout-profile persistence or panel placement.
- Change mobile or tablet task layouts.

## Technical approach

### Changes activation guard

Update `activateChangesPanel` in `apps/web/components/task/changes-panel-focus.ts`. Require the active profile to be the built-in Default, use `RIGHT_TOP_GROUP` as the layout identity, and permit activation only when the live group contains exactly `files` and `changes`. Persist the active profile identity with each environment-scoped Dockview layout so copied custom profiles remain ineligible after restores and task switches. Carry the effective default's identity alongside its layout when seeding the store, so a reserved customized Default remains built-in Default during fresh builds and Reset Layout while arbitrary custom defaults remain custom.

Keep both activation callers on this shared helper. `ChangesTab` uses it for active-task count increases. `useChangesPanelAutoFocus` uses it after inactive-task updates.

Keep an ineligible pending update available for a later eligible layout. Do not change the pending-state cleanup rules.

### Regression coverage

Add direct unit coverage for the activation guard in `apps/web/components/task/changes-panel-focus.test.ts`.

Replace the E2E expectation that activates Changes in any non-Agent group. The new scenario selects the VS Code preset, leaves VS Code active, and creates another Git update. The test proves that the `vscode | files | changes` group does not lose focus.

Add store and profile-identity coverage for a reserved customized Default used as the effective default during fresh build and Reset Layout, plus the existing env-profile read/write paths.

## Tests

- `AC-UI-TASK-LAYOUT-PROFILES-001.11`: Unit cases cover the eligible Default group, a non-Default group, and a Default group with an extra VS Code tab.
- `AC-UI-TASK-LAYOUT-PROFILES-001.11`: Unit and store-integration cases cover a copied Default custom profile, built-in override identity, and per-environment profile persistence.
- `AC-UI-TASK-LAYOUT-PROFILES-001.11`: Existing state tests preserve reload baselining and inactive-task attention.
- `AC-UI-TASK-LAYOUT-PROFILES-001.11`: A reserved customized Default keeps built-in identity through a fresh build and Reset Layout; arbitrary saved profile IDs remain custom.

## E2E tests

- `AC-UI-TASK-LAYOUT-PROFILES-001.11`: Update `apps/web/e2e/tests/layout/changes-panel-focus.spec.ts` with the reported VS Code group scenario.
- Existing Default-layout scenarios continue to prove eligible activation for active and returning tasks.
- No mobile E2E case is required. Mobile and tablet task layouts do not mount `DockviewDesktopLayout` or this focus hook.

## Work orders

- [x] [Task 01: Restrict Changes auto-focus](task-01-restrict-changes-auto-focus.md) *(done)*

## Verification results

- `pnpm exec vitest run components/task/changes-panel-focus.test.ts`: passed, 20 tests before the fixup and 20 tests after the fixup.
- `pnpm exec vitest run lib/local-storage.test.ts lib/state/dockview-store.test.ts lib/state/dockview-preset-persistence.test.ts components/task/changes-panel-focus.test.ts`: passed, 92 tests.
- `pnpm run typecheck`: passed.
- `pnpm test`: passed, 1,677 test files and 14,381 tests (4 skipped).
- `pnpm run lint`: passed.
- `pnpm e2e:run tests/layout/changes-panel-focus.spec.ts -- --grep "VS Code group"`: passed.
- The complete `changes-panel-focus.spec.ts` suite passed, 6 tests, with the desktop PR capture enabled.
- `python3 ../../scripts/lint-spec-files.test.py`: passed, 20 tests.
- `python3 scripts/lint-spec-files.py --all`: passed when run from the repository root.
- `git diff --check`: passed.

## Risks

- A loose panel-membership check can allow another editor tab to lose focus.
- A guard that uses the current fallback group can misidentify a non-Default layout as the Default layout.
- Removing pending state for an ineligible layout can lose later attention after a switch to the Default layout.
