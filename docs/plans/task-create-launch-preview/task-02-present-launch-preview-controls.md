---
id: "02-present-launch-preview-controls"
title: "Present launch preview controls"
status: done
wave: 2
depends_on:
  - "01-resolve-launch-preview-data"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.1
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.3
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.1
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.2
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.5
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.6
system_design:
  - ../../specs/tasks/system-design/task-create-launch-preview.md
---

# Task 02: Present Launch Preview Controls

## Summary

Show the launch destination in the workflow selector. Add the inline prompt
preview toggle, localization, component coverage, and public task guidance.

## In scope

- Render the muted destination step beside the selected workflow.
- Add the preview icon after **Enhance prompt with AI**.
- Toggle between the unchanged editor and a read-only composed preview.
- Add accessible names, pressed state, and coarse-pointer sizing.
- Update all locale catalogs and the public task creation guide.

## Out of scope

- Launch behavior, API contracts, or persisted state.
- Other prompt composers.
- Playwright coverage.

## Acceptance

- The selector and prompt editor render the shared launch-preview model without
  adding a second routing rule.
- Toggling the preview preserves the exact task prompt. Losing the preview model
  returns the editor to edit mode.
- Localized, accessible controls work at desktop and coarse-pointer sizes.

## Verification

```bash
cd apps/web
pnpm test -- --run components/task-create-dialog-selectors.test.tsx components/task-create-dialog-form-body.test.tsx components/workflow-selector-row.test.tsx components/task-create-dialog.test.tsx
pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/task-create-dialog-selectors.tsx components/task-create-dialog-selectors.test.tsx components/task-create-dialog-form-body.tsx components/task-create-dialog-form-body.test.tsx components/workflow-selector-row.tsx components/workflow-selector-row.test.tsx
pnpm run i18n:check
pnpm run i18n:ratchet
cd ../..
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/web/components/task-create-dialog-selectors.tsx`
- `apps/web/components/task-create-dialog-selectors.test.tsx`
- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/task-create-dialog-form-body.test.tsx`
- `apps/web/components/workflow-selector-row.tsx`
- `apps/web/components/workflow-selector-row.test.tsx`
- `apps/web/components/task-create-dialog.test.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/zh-hk/task.json`
- `apps/web/src/locales/zh-tw/task.json`
- `docs/public/tasks-and-workflows.md`

## Dependencies

- Task 01 supplies the launch-preview model and composition helper.

## Risks

- The large shared composer can exceed lint limits if preview presentation is
  not extracted into a focused component.
- Long prompt content needs bounded wrapping without adding document overflow.

## Parallelism

`sequential`

## Inputs

- The prompt editor and workflow selector sections in the system design.
- Existing task-create composer toolbar and selector component tests.
- The public **Create a task** procedure.

## Results

- Added the muted launch destination to the workflow selector and an inline
  preview toggle after **Enhance prompt with AI**.
- Added the read-only wrapping preview surface, localized accessible labels,
  pressed state, and 44-pixel coarse-pointer sizing.
- Added all five locale catalog entries, regenerated the pseudo catalog, and
  updated the public task creation guide.
- Wrapped the disabled preview button in the shared focusable tooltip-trigger
  pattern so its explanation remains reachable by keyboard and pointer.
- Focused component tests passed (43 tests), typecheck passed, and the focused
  ESLint command passed with zero warnings.
- `cd apps/web && pnpm run i18n:check` passed; `pnpm run i18n:ratchet` passed.
- `node --test scripts/validate-public-docs.test.mjs` passed (61 tests) and
  `node scripts/validate-public-docs.mjs` validated 45 pages.
- `git diff --check` passed.
