---
id: "03-prove-responsive-launch-previews"
title: "Prove responsive launch previews"
status: done
wave: 3
depends_on:
  - "02-present-launch-preview-controls"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.1
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.2
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.2
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.3
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.6
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.7
system_design:
  - ../../specs/tasks/system-design/task-create-launch-preview.md
---

# Task 03: Prove Responsive Launch Previews

## Summary

Prove the complete launch destination and prompt preview flow in desktop and
mobile production builds. Use a workflow whose configured start and auto-start
steps differ.

## In scope

- Add a desktop task-create scenario to the existing focused spec.
- Add a mobile task-create launch-preview spec.
- Prove prompt preservation, touch size, viewport containment, and no horizontal
  overflow.

## Out of scope

- Broad E2E or regression suites.
- Backend routing tests that already cover `ResolveAutoStartStep`.
- Screenshot or product-video updates.

## Acceptance

- Desktop shows the first positional destination while the description is empty,
  then the actual auto-start destination and correctly composed step prompt after
  a workflow switch and description entry.
- Mobile completes the same edit, preview, and return-to-edit flow.
- The mobile icon is at least 44 CSS pixels, and the dialog causes no document
  horizontal overflow.

## Verification

```bash
cd apps/web
pnpm e2e:run tests/task/create-task.spec.ts -- --grep "launch prompt preview"
pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-launch-preview.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/task/create-task.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-launch-preview.spec.ts`

## Dependencies

- Task 02 supplies the rendered controls and stable test IDs.

## Risks

- The task-create fixture can retain remembered workflow state. The scenario
  must set or clear that state explicitly.
- The mobile project discovers only files whose names start with `mobile-`.

## Parallelism

`sequential`

## Inputs

- The responsive behavior and verification sections in the system design.
- Existing task-create desktop and mobile Playwright fixtures.

## Results

- Added desktop coverage using a workflow whose configured start and
  auto-start destinations differ, including workflow switching and prompt
  token preservation.
- Added the focused `mobile-create-task-launch-preview.spec.ts` coverage with
  touch interactions, geometry, containment, and document overflow assertions.
- `cd apps/web && pnpm e2e:run --host --no-build tests/task/create-task.spec.ts -- --grep "launch prompt preview"` passed (1 test).
- `cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-create-task-launch-preview.spec.ts` passed (1 test).
