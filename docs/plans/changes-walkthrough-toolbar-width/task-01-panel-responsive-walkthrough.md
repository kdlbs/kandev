---
id: "01-panel-responsive-walkthrough"
title: "Make the Walkthrough action panel-responsive"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/changes-walkthrough-toolbar-width.md"
---

# Task 01: Make the Walkthrough Action Panel-Responsive

## Acceptance

- A 349px Changes panel renders an accessible, fully contained icon-only
  Walkthrough action, independent of the viewport width.
- A 350px-or-wider Changes panel renders the complete Walkthrough label without
  clipping the action or the branch and Pull controls.
- The Pixel 5 Changes request flow remains usable, contained, and free of
  document-level horizontal overflow.

## TDD sequence

1. Add the desktop 349px/350px geometry regression and the mobile containment
   assertions, then run the focused command and confirm they fail because the
   current label follows viewport width.
2. Add the named panel containers and replace the viewport breakpoint with the
   350px container-query boundary.
3. Run the focused desktop and mobile commands until both scenarios pass.
4. Refactor only if the minimal class changes expose duplication or unclear
   naming; rerun the focused command after any refactor.

## Files likely touched

- `apps/web/components/task/changes-panel.tsx`
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`
- `apps/web/components/task/changes-panel-header.tsx`
- `apps/web/e2e/tests/review/walkthrough.spec.ts`
- `apps/web/e2e/tests/review/mobile-walkthrough.spec.ts`
- `docs/plans/changes-walkthrough-toolbar-width/plan.md`
- `docs/plans/changes-walkthrough-toolbar-width/task-01-panel-responsive-walkthrough.md`

## Verification

Run from `apps/web`:

```bash
pnpm e2e:run --host --project chromium -- tests/review/walkthrough.spec.ts --grep "adapts the Changes walkthrough label to panel width" --workers=1
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/review/mobile-walkthrough.spec.ts --grep "requests a walkthrough from Changes" --workers=1
```

The managed runner rebuilds the production Vite assets before Playwright runs.

## Dependencies

None.

## Parallelism

`sequential`. The production and E2E changes define one coupled responsive
contract and should be completed through one Red-Green-Refactor cycle.

## Inputs

- Spec scenarios in
  `docs/specs/ui/requirements/changes-walkthrough-toolbar-width.md`.
- Frontend and mobile contract in `plan.md`.
- Existing responsive toolbar implementation in
  `apps/web/components/task/changes-panel-header.tsx`.
- Existing Dockview resize helper in `apps/web/e2e/helpers/dockview-resize.ts`.
- Existing request flows in the desktop and mobile walkthrough specs.

## Risks

- Apply the named container to the full panel root so the threshold is not
  shifted by the header's horizontal padding.
- Do not relax `PanelHeaderBarSplit` overflow or right-slot shrink behavior;
  those preserve the branch and Pull actions at narrow widths.

## Output contract

Report the files changed, the observed RED failures, the final focused command
result, any Dockview rounding encountered, remaining risks, and the updated task
and plan statuses in this conversation.
