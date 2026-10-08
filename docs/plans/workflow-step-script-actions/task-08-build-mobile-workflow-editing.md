---
id: 08-build-mobile-workflow-editing
title: Build mobile workflow editing
status: superseded
wave: 7
depends_on:
  - 07-build-lifecycle-action-recipes
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-006
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-006.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.6
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.7
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.8
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.9
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 08: Build mobile workflow editing

## Summary

Compose the shared workflow draft as a phone-native vertical journey with
dedicated step and action editor screens.

## In scope

- Vertical step cards with summaries, issues, destinations, and explicit
  selection.
- Full-height step and action states with predictable browser Back behavior.
- Temporary inset action-choice drawer and explicit move up/down controls.
- Safe areas, one vertical scroll owner, 44-pixel targets, read-only parity,
  dirty-route behavior, and mobile localization.

## Out of scope

- A compressed desktop canvas, required drag gestures, and mobile-only domain
  mutations.

## Acceptance

1. Mobile authors can inspect and edit every Agent, Automation, and Policies
   capability through journey, step, and focused action screens.
2. Navigation and reordering retain one route-local draft and integrate with
   Save changes, issue targeting, read-only state, and dirty-route confirmation.
3. Tested phone viewports have one vertical scroll owner per screen, no
   document-level horizontal overflow, safe fixed actions, and touch targets of
   at least 44 by 44 CSS pixels.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-editor/mobile
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/workflow/mobile-workflow-editor.spec.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check
git diff --check
```

## Files likely touched

- `apps/web/components/settings/workflow-editor/mobile/`
- `apps/web/components/task/mobile/mobile-picker-sheet.tsx` only if a generic
  action-choice capability is missing.
- `apps/web/e2e/tests/workflow/mobile-workflow-editor.spec.ts`
- `apps/web/e2e/pages/workflow-settings-page.ts`
- Workflow locale catalogs and mobile component tests.

## Dependencies

- Task 07 supplies stable lifecycle rows and focused action editors.

## Risks

- Nested drawers would recreate the dense desktop hierarchy and break Back.
- Fixed save/navigation surfaces can cover the final field without safe-area
  and content padding.
- Sharing layout instead of state would make mobile parity brittle.

## Parallelism

`parallel-safe` with Task 09. This task owns workflow editor mobile composition
and mobile workflow E2E; Task 09 owns task chat rendering.

## Inputs

- Shared view model, action catalog, route draft, and desktop editor controls.
- `MobilePickerSheet`, kanban mobile sheets, and existing mobile workflow E2E.

## Results

Implemented the mobile vertical journey, full-height step and action states,
temporary action picker drawer, safe-area layout, touch-sized controls, and
shared draft mutations. The `mobile-chrome` editor E2E passes. Task 12
supersedes this navigation model with the existing inline workflow card.
