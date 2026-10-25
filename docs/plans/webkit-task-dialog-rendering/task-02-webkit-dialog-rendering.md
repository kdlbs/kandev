---
id: "02-webkit-dialog-rendering"
title: "Apply WebKit-safe task dialog rendering"
status: done
wave: 2
depends_on: ["01-rendering-engine-marker"]
plan: "plan.md"
spec: "../../specs/ui/requirements/webkit-task-dialog-rendering.md"
---

# Task 02: Apply WebKit-Safe Task Dialog Rendering

## Acceptance

- The WebKit Create Task dialog and nested discard confirmation use opacity-only motion, have no
  non-identity transform on their text-bearing surfaces, are centered without stretching, and are
  explicitly stacked above their overlays with the nested confirmation above the task form. The
  nested confirmation uses an explicit `z-52` overlay, `z-53` content layer, and fit-content height
  on narrow WebKit viewports.
- Desktop Chromium keeps the existing scale-and-fade motion, translated centering, dimensions,
  focus behavior, and task-creation controls.
- All engines omit the generic top-right close control while retaining the footer Cancel action.
- The dialog surface has zero top padding while retaining its side and bottom spacing.
- The narrow WebKit path remains full-height, viewport-contained, internally usable, and free of
  document horizontal overflow.

## TDD sequence

1. Add the desktop and mobile E2E regressions, force the document marker to `webkit`, and run the
   focused commands to confirm RED because the dialog still uses the shared transform animation
   and centering.
2. Add the Create Task opt-in attribute and scoped opacity-only WebKit CSS, transform-free
   centering, and explicit stacking.
3. Run the desktop Chromium/WebKit-branch E2E, then the mobile WebKit-branch E2E.
4. Refactor only if the selectors or geometry rules can be made clearer without widening the
   workaround to other dialogs; rerun both focused commands after refactoring.

## Files likely touched

- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/discard-local-changes-dialog.tsx`
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/task/create-task-webkit-rendering.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-webkit-rendering.spec.ts`
- `docs/plans/webkit-task-dialog-rendering/plan.md`
- `docs/plans/webkit-task-dialog-rendering/task-02-webkit-dialog-rendering.md`

## Verification

Run from `apps/web`:

```bash
pnpm e2e:run --host --project chromium -- tests/task/create-task-webkit-rendering.spec.ts --workers=1
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-create-task-webkit-rendering.spec.ts --workers=1
```

The first managed-runner command rebuilds the production Vite assets. The mobile command reuses
that build so both checks exercise the same output.

## Dependencies

Task 01 must establish and test the `data-rendering-engine` root contract.

## Parallelism

`sequential`. The CSS, opt-in attribute, and desktop/mobile geometry tests form one coupled
Red-Green-Refactor cycle.

## Inputs

- Rendering and responsive scenarios in
  `docs/specs/ui/requirements/webkit-task-dialog-rendering.md`.
- Root marker contract from Task 01.
- Shared dialog classes in `apps/packages/ui/src/dialog.tsx`.
- Create Task sizing in `apps/web/components/task-create-dialog.tsx`.
- Existing entry points in `KanbanPage` and `MobileKanbanPage`.
- Existing horizontal-overflow assertion in `apps/web/e2e/helpers/layout-assertions.ts`.

## Risks

- Override the animation name with custom keyframes that contain only opacity; setting the
  animation scale variable to `1` is insufficient because the shared keyframe still writes a
  transform and creates a composited layer.
- Preserve mobile `height: 100%`; use desktop `height: fit-content` only at the existing 640px
  breakpoint so fixed inset centering does not stretch the desktop dialog.
- Do not add `translateZ(0)`, `will-change: transform`, or font-smoothing overrides; those can force
  the WebKit compositing behavior this fix avoids.

## Output contract

Report the observed desktop/mobile RED failures, selectors and geometry applied, files changed,
both final focused command results, the macOS native visual-check status, remaining risks, and
updated task/plan statuses in this conversation.

## Results

- RED: the Chromium harness observed the shared `enter` animation and transform-based centering
  under the forced WebKit marker; the mobile path likewise had the shared motion before the
  opt-in CSS existed.
- GREEN: the desktop Chromium/WebKit-branch spec passes 2 tests and the mobile WebKit-branch spec
  passes 1 test after the opt-in CSS was added.
- Native visual check: not available in this Linux worktree; the selector behavior and geometry
  are covered by the focused Chromium and mobile viewport checks, with a macOS Safari/Tauri check
  remaining for the PR reviewer.
