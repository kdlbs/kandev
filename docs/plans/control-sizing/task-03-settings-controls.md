---
id: "03-settings-controls"
title: "Settings control sweep"
status: complete
wave: 3
depends_on: ["02-task-controls"]
plan: "plan.md"
requirements:
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-CONTROL-SIZING-001.1
  - AC-UI-CONTROL-SIZING-001.2
  - AC-UI-CONTROL-SIZING-001.3
  - AC-UI-CONTROL-SIZING-001.4
  - AC-UI-CONTROL-SIZING-001.5
  - AC-UI-CONTROL-SIZING-001.6
  - AC-UI-CONTROL-SIZING-001.7
  - AC-UI-CONTROL-SIZING-001.8
  - AC-UI-CONTROL-SIZING-001.9
  - AC-UI-CONTROL-SIZING-001.10
system_design:
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 03: Settings control sweep

## Summary

Align ordinary settings fields, selectors, and actions across pages. Remove local desktop height overrides and preserve complete input/action groups.

## In scope

- Disposition all Settings surfaces candidates in sweep-inventory.md and all additional settings helper callers.
- Normalize Layouts New/Duplicate, workspace heading and placement pickers, and workflow section actions.
- Repair repository secrets, dynamic policy fields, storage actions/fields/confirmations, message queue fields, and executor page actions.
- Update desktop typography coverage from a lower bound to the actual size contract.
- Add settings/control-sizing.spec.ts and its mobile pair. Reuse existing Layouts and storage fixture patterns.

## Out of scope

- Feature state, callbacks, permission changes, and persistence.
- Unrelated theme, typography, or navigation redesign.

## Acceptance

- Equivalent settings action/field groups use 28px desktop height, including attached clear or reveal controls.
- The settings inventory has a disposition for every candidate and no unexplained desktop 32/36/40/44px ordinary controls.
- Phone and tablet paths retain touch targets, label readability, save coordination, and field values.

## Regression first

Assert 28px desktop dimensions for Layouts New/Duplicate and a repository secret input/select pair. Confirm the current 32px and 44px overrides fail.

Use TDD for new helper logic and browser regressions.
Run the new regression before production edits and record the expected failure.
Rerun the same check after the correction.

## Verification

Run from the repository root. The first package command requires installed workspace dependencies.
Run desktop and mobile checks sequentially.
The mobile command reuses the immediately preceding unchanged production build.

```bash
pnpm --dir apps/web exec vitest run components/settings/settings-typography.test.ts
pnpm --dir apps/web e2e:run --project chromium tests/settings/control-sizing.spec.ts tests/settings/settings-typography.spec.ts tests/settings/layout-profiles.spec.ts
pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/settings/mobile-control-sizing.spec.ts tests/settings/mobile-settings-typography.spec.ts tests/settings/mobile-layout-profiles.spec.ts
pnpm --dir apps/web run typecheck
```

Before completion, run ESLint on the exact changed TypeScript files from `apps/web`.
Record the expanded file list and command in Results.
Do not substitute class-string checks for browser geometry.

## Files likely touched

- `apps/web/components/settings/layouts/layout-profile-list.tsx`
- `apps/web/components/settings/layouts/layout-settings.tsx`
- `apps/web/components/settings/workspaces/workspace-settings-shell.tsx`
- `apps/web/components/settings/workflow-section-actions.tsx`
- `apps/web/components/settings/repository-secret-bindings.tsx`
- `apps/web/components/settings/system/storage/storage-action-button.tsx`
- `apps/web/components/settings/system/storage/storage-policy-fields.tsx`
- `apps/web/components/settings/system/storage/storage-adoption-field.tsx`
- `apps/web/components/settings/system/storage/storage-confirmation-dialogs.tsx`
- `apps/web/e2e/tests/settings/settings-typography.spec.ts`
- `apps/web/e2e/tests/settings/control-sizing.spec.ts`
- `apps/web/e2e/tests/settings/mobile-control-sizing.spec.ts`

The [inventory](sweep-inventory.md) section **Settings surfaces** defines the remaining file scope.

## Dependencies

02-task-controls

## Risks

Storage and credential controls have attached icons and disabled tooltip wrappers. Shrinking only the field leaves oversized hit areas.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/control-sizing.md)
- [System design](../../specs/ui/system-design/control-sizing.md)
- [Sweep inventory](sweep-inventory.md)
- Existing Start Task, settings typography, and mobile geometry tests.

## Results

Completed 2026-09-10.

- Red: the settings geometry regression measured Layouts actions at 32px and a
  repository secret control at 44px instead of the 28px desktop standard.
- Green: settings control sizing, typography, and layout-profile coverage passed
  with 10 desktop and 8 mobile tests; settings unit coverage also passed.
- Settings helpers now own complete standard dimensions, including attached
  field actions, while mobile and coarse-pointer paths retain 44px targets.
- The final typecheck and exact changed-file ESLint passed with 0 errors; the
  completed inventory records the settings-surface dispositions.
