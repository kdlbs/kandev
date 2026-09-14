---
id: "04-application-controls"
title: "Application control sweep"
status: complete
wave: 4
depends_on: ["03-settings-controls"]
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

# Task 04: Application control sweep

## Summary

Normalize remaining ordinary application controls and close the sweep inventory. Cover integrations, Office, navigation, and shared wrappers with representative rendered evidence.

## In scope

- Disposition all Other application surfaces and Office surfaces candidates in sweep-inventory.md.
- Inspect integration toolbars, saved-query dialogs, GitHub/GitLab actions, provider credentials, and application status alerts.
- Inspect Tasks list controls, Office fields, sidebar search, generic selectors, and shared confirmation actions.
- Extend layout/control-sizing.spec.ts and its mobile pair across the changed shared families.
- Repeat raw-control, helper, CSS, size-prop, inline-style, padding-only, and caller searches. Add every newly found defect to the inventory before closure.

## Out of scope

- Feature state, callbacks, permission changes, and persistence.
- Unrelated theme, typography, or navigation redesign.

## Acceptance

- Every candidate has a source-backed disposition or a completed change with relevant rendered evidence.
- Representative integration, Office, and navigation controls satisfy desktop sizes and touch minimums.
- The final inventory lists retained exceptions and any blocked checks without claiming a complete fix for unresolved defects.

## Regression first

Add a desktop regression for an application status alert or provider credential field with unconditional h-11. Confirm that the current control fails the standard-height assertion.

Use TDD for new helper logic and browser regressions.
Run the new regression before production edits and record the expected failure.
Rerun the same check after the correction.

## Verification

Run from the repository root. The first package command requires installed workspace dependencies.
Run desktop and mobile checks sequentially.
The mobile command reuses the immediately preceding unchanged production build.

```bash
pnpm --dir apps/web e2e:run --project chromium tests/layout/control-sizing.spec.ts
pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/layout/mobile-control-sizing.spec.ts
pnpm --dir apps/web run typecheck
git diff --check
```

Before completion, run ESLint on the exact changed TypeScript files from `apps/web`.
Record the expanded file list and command in Results.
Do not substitute class-string checks for browser geometry.

## Files likely touched

- `apps/web/components/integrations/integration-list-toolbar.tsx`
- `apps/web/components/integrations/integration-save-query-dialog.tsx`
- `apps/web/components/github/github-pat-form.tsx`
- `apps/web/components/github/pr-merge-button.tsx`
- `apps/web/components/app-status-bar/backend-reload-required-alert.tsx`
- `apps/web/components/app-status-bar/agent-runtime-unavailable-alert.tsx`
- `apps/web/components/app-sidebar/sections/settings/settings-search.tsx`
- `apps/web/app/tasks/tasks-list-controls.tsx`
- `apps/web/app/tasks/tasks-pagination.tsx`
- `apps/web/app/office/projects/[id]/project-header.tsx`
- `apps/web/e2e/tests/layout/control-sizing.spec.ts`
- `apps/web/e2e/tests/layout/mobile-control-sizing.spec.ts`
- `docs/plans/control-sizing/sweep-inventory.md`

The [inventory](sweep-inventory.md) section **Other application surfaces and Office surfaces** defines the remaining file scope.

## Dependencies

03-settings-controls

## Risks

Provider and Office feature gates need isolated seed data. Shared integration components can affect several providers. Search hits are not automatically defects.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/control-sizing.md)
- [System design](../../specs/ui/system-design/control-sizing.md)
- [Sweep inventory](sweep-inventory.md)
- Existing Start Task, settings typography, and mobile geometry tests.

## Results

Completed 2026-09-10.

- Green: representative application geometry passed with 3 desktop and 2 mobile
  layout tests after migrating integration, Office, navigation, status, provider,
  automation, and shared-wrapper controls.
- Follow-up helper, size-prop, and padding-only searches added 36 implementation
  files to the inventory. Every candidate now has a changed, conforming,
  touch-only, content-sized, or documented-exception disposition.
- Final harness/specification validation and `git diff --check` passed. Exact
  changed-file ESLint covered 200 `apps/web` TypeScript files with 0 errors and
  7 non-blocking warnings.
