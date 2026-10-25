---
id: "03-phone-columns-home"
title: "Render the Columns menu in the phone drawer for the focused workflow"
status: done
wave: 2
depends_on: ["01-columns-menu-on-swimlane-header"]
plan: "plan.md"
spec: "../../specs/ui/requirements/board-step-visibility-filter.md"
---

# Task 03: Render the Columns menu in the phone drawer for the focused workflow

## Acceptance

- On a phone viewport with `currentPage === "kanban"`, `MobileDisplayOptions`
  renders the same `ColumnsMenu` for **exactly** the board navigator's focused
  workflow, positioned after the Repository field and before the Preview-panel
  field.
- Exactly one home renders the control at any breakpoint: the drawer copy is
  absent off mobile (where `TabletHeader` renders lane headers alongside
  `MobileMenuSheet`), and the control is absent entirely when
  `currentPage !== "kanban"`.
- Interactive rows measure ≥ 44 CSS px, long step titles truncate to one line
  with the full title in `title`, and the drawer adds no new scroll container or
  height cap.

## Verification

```
cd apps/web && pnpm vitest run components/kanban/mobile-menu-sheet.test.tsx components/kanban/columns-menu.test.tsx && pnpm run typecheck && pnpm run i18n:ratchet
```

The focused-workflow scoping and the not-rendered-off-mobile case must fail
first, then pass.

## Files likely touched

- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/components/kanban/mobile-menu-sheet.test.tsx` (or the existing
  mobile display-options test file)
- `apps/web/components/kanban/mobile-menu-styles.ts` (tokens only, if needed)
- `apps/web/components/kanban/columns-menu.tsx` (row-geometry class only)

## Dependencies

Task 01 — this reuses the component it creates. Do not fork a phone-only
implementation.

## Risks

- The focused workflow id has more than one plausible source. Read the one
  `getRenderedWorkflows` consumes, or the drawer will configure a workflow the
  user is not looking at.
- Copying the drawer's Preview-panel row (`flex h-10`, 40px) instead of its
  List-rows field (`flex min-h-11`, 44px) silently fails the touch-target
  requirement. The List-rows field is gated behind `currentPage === "tasks"`, so
  it is not on screen next to the new block — read it in the file.

## Inputs

- Spec sections `Control surface` (phone, and `Exactly one home at a time`),
  `Nil / empty / error / defaults / boundary` (`currentPage === "tasks"`), and
  the `(R2)` phone scenario
- Plan section `Phone home`
- `apps/web/AGENTS.md` on `useResponsiveBreakpoint` and the 768px boundary;
  `/mobile-parity` for what parity requires here

## Output contract

Report the RED failures, the focused-workflow source chosen and why, files
changed, exact targeted test result, and confirmation that no breakpoint renders
both homes. Mark this task `done` and tick its plan checkbox.
