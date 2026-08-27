---
id: "01-columns-menu-on-swimlane-header"
title: "Move the step list to a per-lane Columns menu"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/board-step-visibility-filter.md"
---

# Task 01: Move the step list to a per-lane Columns menu

## Acceptance

- The two superseded commits on the PR head are reverted —
  `feat(kanban): per-workflow collapsible Steps section disclosure` and
  `feat(kanban): add the Steps filter to the phone Display surface` — removing
  `steps-visibility-section.tsx`, `use-steps-disclosure-overrides.ts`, and their
  tests. The selector-relocation commit is kept.
- A shared presentational `ColumnsMenu` renders one workflow's steps as
  `DropdownMenuCheckboxItem`s, ordered by `position` then `id`, with trigger
  `columns-menu-<workflowId>` and items `columns-menu-step-<stepId>`; checked
  state derives from that workflow's hidden set and toggling calls
  `onToggleStepVisibility(workflowId, stepId)`.
- Every rendered swimlane header carries the menu on desktop and tablet, in both
  All-Workflows and single-workflow views, and keeps the trigger while the lane
  is collapsed.
- `kanban-display-dropdown.tsx` no longer renders a Steps section, and no
  `steps-filter-*` testid remains anywhere in `apps/web/`.

## Verification

```
cd apps/web && pnpm vitest run components/kanban/columns-menu.test.tsx components/kanban-display-dropdown.test.tsx && pnpm run typecheck && pnpm run i18n:check
```

The `ColumnsMenu` ordering and checked-state tests must fail first (no such
component), then pass. The dropdown test must fail on its existing Steps
assertions before they are removed.

## Files likely touched

- `apps/web/components/kanban/steps-visibility-section.tsx` (deleted by revert)
- `apps/web/hooks/use-steps-disclosure-overrides.ts` (deleted by revert)
- `apps/web/components/kanban/columns-menu.tsx` (new)
- `apps/web/components/kanban/columns-menu.test.tsx` (new)
- `apps/web/components/kanban/swimlane-header.tsx`
- `apps/web/components/kanban/swimlane-section.tsx`
- `apps/web/components/kanban/swimlane-container.tsx` (`WorkflowItemContent`)
- `apps/web/components/kanban-display-dropdown.tsx`
- `apps/web/components/kanban-display-dropdown.test.tsx`
- `apps/web/src/locales/en/kanban.json` + pseudo-locale catalog

## Dependencies

None. Task 02 depends on this one only for review coherence, not for code.

## Risks

- `swimlane-header.tsx` is currently pure props. Take the menu as a
  `ReactNode` slot rather than wiring the store into it.
- Deleting the `"steps"` / `"stepsSectionDescription"` keys without grepping for
  other references fails `i18n:check`.

## Inputs

- Spec sections `Control surface` (desktop and tablet), `Menu contents &
  selectors`, `Ordering & determinism`, `i18n`
- Plan sections `Shared Columns menu`, `Swimlane header home`, `Display dropdown
  removal`
- Existing `StepsSection` in `kanban-display-dropdown.tsx` as the behaviour to
  port, not to copy verbatim — it groups by workflow and the new component
  must not

## Output contract

Report the RED failures, the new component's props, the removed testids, files
changed, exact targeted test results, and any reference to `"steps"` keys found
elsewhere. Mark this task `done` and tick its plan checkbox.
