---
id: "01-refine-launch-step-help-control"
title: "Refine the launch-step help control"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.1
system_design:
  - ../../specs/tasks/system-design/task-create-launch-preview.md
---

# Task 01: Refine the Launch-Step Help Control

## Summary

Replace the task-create launch destination's generic information glyph and
visible prefix with a directional arrow help control followed by the resolved
step name. Preserve accessible explanatory behavior across pointer, keyboard,
and touch input.

## In scope

- Render `IconArrowBigRightLines` in the existing launch-step help button.
- Render only the destination step name beside the button.
- Preserve localized tooltip content and accessible naming, and use the
  standard drawer and responsive target sizing on coarse pointers.
- Remove the unused visible-label translations and update public guidance.
- Update focused component and desktop/mobile browser coverage.

## Out of scope

- Launch routing, workflow selection, task submission, and prompt-preview
  behavior.
- New responsive composition or navigation.

## Acceptance

- The selected workflow is followed by the muted directional arrow button and
  resolved destination step name, with no visible **Start step:** prefix.
- Hovering or focusing the arrow shows the localized launch-destination
  explanation; activating it on a coarse pointer opens the same explanation in
  a drawer.
- The button retains its localized accessible name, and its coarse-pointer hit
  area remains at least 44 CSS pixels without enlarging fine-pointer desktop
  density.

## Verification

```bash
cd apps/web
pnpm test -- --run components/workflow-selector-row.test.tsx
pnpm exec eslint --max-warnings 0 components/workflow-selector-row.tsx components/workflow-selector-row.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run tests/task/create-task.spec.ts -- --grep "launch prompt preview"
pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-launch-preview.spec.ts
cd ../..
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/web/components/workflow-selector-row.tsx`
- `apps/web/components/workflow-selector-row.test.tsx`
- `apps/web/e2e/tests/task/create-task.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-launch-preview.spec.ts`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/zh-hk/task.json`
- `apps/web/src/locales/zh-tw/task.json`
- `apps/web/src/locales/pseudo/task.json`
- `docs/public/tasks-and-workflows.md`

## Dependencies

None.

## Risks

- The arrow can look like navigation if spacing or emphasis separates it from
  the destination label.
- Coarse-pointer detection must route the disclosure to the standard touch
  drawer while fine pointers retain hover and keyboard focus behavior.

## Parallelism

`sequential`

## Inputs

- Launch destination disclosure in the task-create launch-preview requirement.
- Workflow selector and responsive behavior in the paired system design.
- Existing selector component and launch-preview Playwright coverage.

## Results

- Replaced the generic information glyph with `IconArrowBigRightLines` and
  rendered the resolved workflow step name without a visible prefix.
- Preserved localized accessible naming and explanatory tooltip behavior for
  pointer hover and keyboard focus; mobile coverage proves the touch drawer and
  its 44-pixel coarse-pointer trigger.
- Removed the unused visible-label key from all locale catalogs and updated the
  public task-creation how-to guide.
- The focused component test passed (3 tests), targeted desktop and mobile E2E
  each passed (1 test), and all static, i18n, documentation, specification, and
  diff checks listed in the plan passed.
