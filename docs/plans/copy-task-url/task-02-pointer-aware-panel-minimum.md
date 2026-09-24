---
id: "02-pointer-aware-panel-minimum"
title: "Pointer-aware preview panel minimum and indicator floor"
status: todo
wave: 2
depends_on:
  - "01-preview-header-copy-url"
plan: "plan.md"
system_design:
  - "../../specs/ui/system-design/kanban-preview-workflow-step-navigation.md"
requirements:
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002
acceptance_criteria:
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.17
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.1
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.2
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.7
---

# Task 02: Pointer-aware preview panel minimum and indicator floor

## Summary

Task 01's copy-task-link control is the third fixed-width control in the
preview header. At the old 300px panel minimum it leaves 14px for the step
indicator at a coarse pointer (below the 44px hit area of AC-001.17) and 54px
at a fine pointer (below a two-digit count's 67.4px). This work order derives
the minimum panel width from the header budget per pointer mode (320px fine,
380px coarse) and stops the indicator from shrinking below its content floor.
The derivation is in the system design's
[Header layout](../../specs/ui/system-design/kanban-preview-workflow-step-navigation.md#header-layout)
section.

## Scope

- `PREVIEW_PANEL.MIN_WIDTH_PX` 300 -> 320; new `COARSE_MIN_WIDTH_PX: 380`.
- One pure rendered-width helper next to the constants:
  `max(chosen, isFinePointer ? MIN_WIDTH_PX : COARSE_MIN_WIDTH_PX)`.
- `KanbanWithPreview` reads `isFinePointer` from `useResponsiveBreakpoint` and
  uses the rendered width for `useKanbanLayout`, both layouts' `width` style,
  and the resize handler's start width. The resize handler clamps to the
  rendered minimum. No effect writes the rendered width back to state.
- `useKanbanPreview` keeps clamping the chosen width to `MIN_WIDTH_PX` (now
  320) on restore and in `updatePreviewWidth`.
- The preview's indicator wrapper in `task-preview-panel.tsx` enforces the
  indicator floor (for example `[&>button]:min-w-min`); the shared
  `CompactWorkflowTrigger` defaults stay unchanged.

## Exclusions

- The task top bar's presentation, the 500px default, and the 95vw maximum.
- Touch resizing (the handle stays mouse-only) and the task actions menu
  trigger's own hit area.
- Workflows of 100 or more steps. Their three-digit count overflows the
  indicator onto the control cluster at the minimum width, and its floor wins
  over the half-share cap, so AC-002.2 and the cap of AC-002.4 are not
  guaranteed for them (requirements Out of scope).

## ASCII UI preview

`UI-02: Preview header at the minimum panel width`, from the kanban board with
a task previewed, full control cluster, two-digit count. Excerpt of the plan's
[ASCII UI preview](plan.md#ascii-ui-preview). Structure is required; spacing is
illustrative.

```text
Fine pointer, panel 320px (was 300px):
| Fix login redi... | * Rev.. 12/15 | ... [copy] [open] [x] |
  title >= 88px      indicator floor   control cluster 120px

Coarse pointer, panel 380px (was 300px):
| Fix login redi... | * Rev.. 12/15 v | ...  [copy] [open] [x] |
  title >= 88px      indicator >= 44x44  control cluster 160px
```

## Acceptance

1. At a fine pointer the panel renders at no less than 320px and at a coarse
   pointer at no less than 380px, from the same stored chosen width, flipping
   live with the pointer mode and never overwriting storage (AC-002.7).
2. At each minimum, with a 10+ step workflow, the task on step 10 or later
   (a two-digit count on both sides), a long step name, and the task
   actions trigger present, the header is one row, the title is at least
   88px, every cluster control is inside the panel, the indicator box contains
   its marker, count (and coarse cue), and at a coarse pointer the indicator is
   at least 44x44 (AC-001.17, AC-002.1 to AC-002.4). The indicator's width is
   at most half the title-and-indicator group's width plus 1px (the AC-002.4
   cap): about 91px coarse, where the cap binds, and about 74px fine, where the
   title floor binds first.
3. The E2E coverage in the system design's Test strategy exists: the existing
   fine-pointer containment test extended, and a new coarse-pointer test on
   `coarseDesktopTestPage` (1280x900, `hasTouch`). Both tests assert the panel
   rendered inline (no floating backdrop, and the panel does not overlap the
   board) before asserting the budget.

## Verification

- `cd apps/web && pnpm exec vitest run lib/settings/preview-panel-width.test.ts hooks/use-kanban-preview.test.ts components/kanban-with-preview.rendered.test.tsx components/task-preview-panel.test.tsx components/task-preview-panel-step-indicator.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run lint`
- `cd apps/web && pnpm e2e:run --host --no-strict -- e2e/tests/kanban/preview-workflow-step-navigation.spec.ts --project chromium`

Test file names for the new helper and hook tests are suggestions; keep them
beside their sources.

## Files likely touched

- `apps/web/lib/settings/constants.ts`
- `apps/web/lib/settings/preview-panel-width.ts` and `.test.ts` (new helper)
- `apps/web/hooks/use-kanban-preview.ts` and a new `use-kanban-preview.test.ts`
- `apps/web/components/kanban-with-preview.tsx`
- `apps/web/components/kanban-with-preview.rendered.test.tsx` (already mocks
  `useResponsiveBreakpoint`)
- `apps/web/components/task-preview-panel.tsx` (indicator wrapper class)
- `apps/web/e2e/tests/kanban/preview-workflow-step-navigation.spec.ts`

## Risks

- The coarse test must render inline to prove the binding layout.
  `coarseDesktopTestPage` leaves a board container of about 1024px, so a 380px
  panel stays inline; `tabletTestPage` (900x900) would float it and is not
  used. If the inline assertion fails, fix the setup, not the assertion.
- Playwright cannot switch pointer media mid-test, so the live flip is proven
  only by the component test.
- `min-w-min` must outrank the trigger's own `min-w-0` from the wrapper
  selector; verify with a computed-style assertion, not by class presence.
