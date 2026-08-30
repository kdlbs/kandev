---
id: "03-editor-and-proof"
title: "Build target-mode editor and proof"
status: completed
wave: 3
depends_on:
  - "02-dispatch-target-lifecycles"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-TARGETS-001
  - REQ-OFFICE-AUTOMATION-TARGETS-002
  - REQ-OFFICE-AUTOMATION-TARGETS-003
system_design:
  - ../../specs/office/system-design/automation-target-modes.md
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-TARGETS-001.1
  - AC-OFFICE-AUTOMATION-TARGETS-001.3
  - AC-OFFICE-AUTOMATION-TARGETS-001.4
  - AC-OFFICE-AUTOMATION-TARGETS-002.1
  - AC-OFFICE-AUTOMATION-TARGETS-003.1
  - AC-OFFICE-AUTOMATION-TARGETS-003.2
  - AC-OFFICE-AUTOMATION-TARGETS-003.3
  - AC-OFFICE-AUTOMATION-TARGETS-003.4
---

# Task 03: Build target-mode editor and proof

## Summary

Expose target and repository choices in the automation editor, allow available
executor profiles for repository-free runs, and make visible normal-task rules
clear and accessible on desktop and mobile. Prove the two modes and toolbar
geometry with focused component and Playwright coverage.

## In scope

- Automation frontend types, payloads, form state, selectors, repository mode,
  validation, and translations in all supported locales.
- Accessible target and continuity descriptions, mobile scroll/overflow and
  touch geometry.
- Export and New Automation equal-height toolbar actions.
- Desktop and mobile E2E flows for hidden scratch and visible workflow tasks.
- Public automation documentation and feature-status wording.

## Out of scope

- Backend schema or dispatch changes.
- New mobile navigation surfaces or a second confirmation drawer.

## Acceptance

- The editor can save a hidden no-repository/no-workflow automation with
  scratch execution and explains that its per-run tasks stay out of Kanban and
  the sidebar.
- The editor can save a workflow-backed normal-task automation, select either
  continuity policy, and clearly explains that its tasks are visible.
- Desktop and mobile tests prove target selection, visible task creation,
  reusable visible continuation, equal toolbar heights, 44px controls, and no
  horizontal overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/automations components/runs
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/automations-settings.spec.ts tests/automations-run-detail.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/mobile-automations-scroll.spec.ts tests/mobile-automation-detail.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/lib/types/automation.ts`
- `apps/web/components/automations/automation-payload.ts`
- `apps/web/components/automations/automation-editor.tsx`
- `apps/web/components/automations/config-section.tsx`
- `apps/web/components/automations/automation-editor-sections.tsx`
- `apps/web/components/automations/automation-repository-selection.ts`
- `apps/web/components/automations/*.test.*`
- `apps/web/src/locales/*/automations.json`
- `apps/web/e2e/tests/automations-settings.spec.ts`
- `apps/web/e2e/tests/mobile-automations-scroll.spec.ts`
- `docs/public/automation-and-mcp.md`
- `docs/public/feature-status.md`

## Dependencies

Task 02.

## Risks

- Conditional form controls can leave stale workflow, repository, or executor
  values in the payload when the target changes.
- The phone editor must keep one scroll owner while adding explanatory copy.

## Parallelism

`sequential`

## Inputs

- Tasks 01 and 02.
- `docs/specs/office/requirements/automation-target-modes.md`.
- `docs/specs/office/system-design/automation-target-modes.md`.
- `apps/web/components/automations` and existing mobile automation tests.

## Results

- Added target and repository mode controls, conditional workflow validation,
  scratch execution, accessible hidden/visible destination
  descriptions, all-locale translations, and equal-height toolbar actions.
- Kept the complete shared transcript mounted and focused the selected turn;
  replies remain visible while viewing a run.
- Added hidden no-repository and visible workflow-task E2E coverage.
- Verification: 313 focused frontend tests, typecheck, i18n checks, 27 desktop
  E2E tests, 10 mobile E2E tests, and public documentation validation passed.
