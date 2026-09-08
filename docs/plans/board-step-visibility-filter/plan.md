---
spec: docs/specs/ui/requirements/board-step-visibility-filter.md
created: 2026-08-10
status: implemented
---

# Implementation Plan: Per-workflow column visibility (R2 relocation)

## Overview

R1 shipped the behaviour (hidden set, dual filter, persistence, move-target
filtering) on `feature/board-filter-per-wor-p2u`, with the control rendered as a
"Steps" section inside the global Display dropdown. R2 moves the control to the
workflow it configures and adds the one rule that relocation requires.

Nothing in the persisted contract changes. The backend, the store field, the
hydration paths, `filterTasks`, the collapsed `steps` array, and the bulk-move
filter are all already implemented and correct — this plan does not touch them
except where a test needs retargeting.

The work is four steps:

1. Extract the step list into a shared component, mount it on the swimlane
   header, and delete the Display-dropdown section.
2. Retain a lane whose hidden set is non-empty, so a hidden column is always
   recoverable.
3. Give the phone its own home, scoped to the focused workflow.
4. Retarget the E2E spec, add the retention scenario, add the mobile spec.

Steps 1 and 2 are the correctness pair and must land together in review terms —
step 1 alone creates a trapdoor (hide every step → lane drops → control gone).
They are separate tasks only because each has its own RED test.

## Base state — read before starting

PR #2467's head is `2ddba494d` (pushed 2026-08-10), which is **five commits ahead**
of the R1 tip this plan was first drafted against. The head now also carries:

| Commit | Disposition |
|---|---|
| `refactor(kanban): move workflow swimlane selectors out of the component layer` | **Keep** — independent layering hygiene; `selectWorkflowSwimlanes` now lives in `apps/web/lib/kanban/workflow-swimlanes.ts` |
| `feat(kanban): per-workflow collapsible Steps section disclosure` | **Revert** — the density workaround the relocation removes |
| `feat(kanban): add the Steps filter to the phone Display surface` | **Revert** — replaced by the focused-workflow drawer block in task-03 |
| `docs(spec): fold R2 review follow-ups into the step-visibility-filter spec` | **Superseded** — this spec revision replaces it |
| `test(kanban): close Review round-1 test-rigor gaps in Steps filter` | **Triage** — keep assertions that survive the relocation, drop the disclosure ones |

Reverting removes `apps/web/components/kanban/steps-visibility-section.tsx` and
`apps/web/hooks/use-steps-disclosure-overrides.ts` and their tests. Do not port
either into the new component: the override map, `effectiveExpanded`, and the
surface-lifetime rules exist only to make a global N × S list survivable.

## What already exists on the branch (do not rebuild)

- `hiddenWorkflowStepIds` in `apps/web/lib/state/slices/settings/types.ts`, the
  `kanban_hidden_step_ids` wire field end to end, and the boot/REST/WS hydration
  paths.
- `onToggleStepVisibility(workflowId, stepId)` in
  `apps/web/hooks/use-kanban-display-settings.ts`, with normalisation and
  idempotent persistence in `apps/web/hooks/use-user-display-settings.ts`.
- The dual-filter seam in `apps/web/components/kanban/swimlane-container.tsx`
  (`filterTasks` + the `hiddenSet` / `steps` memo in `WorkflowItemContent`).
- The bulk-move filter in `apps/web/components/kanban-board.tsx`.
- `apps/web/e2e/tests/kanban/step-visibility-filter.spec.ts` (chromium), written
  against `display-button`.

## Frontend

### Shared Columns menu

- New `apps/web/components/kanban/columns-menu.tsx`: a `DropdownMenu` built from
  `@kandev/ui/dropdown-menu`'s `DropdownMenuCheckboxItem`, taking
  `{ workflowId, workflowName, steps, hiddenStepIds, onToggle }` and rendering
  the trigger (`columns-menu-<workflowId>`) plus one item per step
  (`columns-menu-step-<stepId>`), sorted by `position` then `id`.
- The component is presentational: it reads no store and owns no state. Both
  homes pass the same props from `useKanbanDisplaySettings` /
  `kanbanMulti.snapshots`.
- Copy through `t()` in the `kanban` namespace: add `"columns"`, add an
  accessible per-lane trigger label naming the workflow, remove `"steps"` and
  `"stepsSectionDescription"` if nothing else references them, and regenerate the
  pseudo-locale catalog.

### Swimlane header home

- `apps/web/components/kanban/swimlane-header.tsx` gains an optional
  `columnsMenu?: ReactNode` slot rendered in the header's trailing grid cell,
  beside the existing collapse and multi-select controls. Keep it a slot rather
  than wiring the store into the header — the header is currently pure props.
- `apps/web/components/kanban/swimlane-section.tsx` passes it through, outside
  the `!isCollapsed` guard so a collapsed lane keeps its trigger.
- `WorkflowItemContent` (`swimlane-container.tsx`) constructs the menu. It
  already holds `wf`, `snapshot.steps` and the hidden set for exactly this
  workflow, so no new selector is needed.

### Display dropdown removal

- Delete `StepsSection` and its call site from
  `apps/web/components/kanban-display-dropdown.tsx`, and drop the now-unused
  props threaded for it. Restore the dropdown to Workflow, Repository, plugin
  filters, Preview panel, List rows.
- `apps/web/components/kanban-display-dropdown.test.tsx` loses its Steps cases;
  the equivalent assertions move to the new component's test.

### Lane retention

- `getVisibleWorkflows` (`swimlane-container.tsx:389`) gains a retention check:
  a workflow is kept when its live hidden set is non-empty, even with zero
  filtered tasks. "Live" means intersected with the snapshot's step ids, so a
  stale id never retains a lane.
- The function currently takes `getFilteredTasks`; give it the hidden map and
  snapshots (or a prepared `hasLiveHiddenSteps(workflowId)` predicate) rather
  than reaching into the store from a pure helper.

### Phone home

- `apps/web/components/kanban/mobile-menu-sheet.tsx` → `MobileDisplayOptions`
  renders the same component for the focused workflow only, after Repository and
  before Preview panel, gated on `currentPage === "kanban"`.
- The focused workflow id is the one the board navigator already tracks
  (`getRenderedWorkflows` / `focusedWorkflowId`); read it from the same source
  rather than introducing a second notion of focus.
- Row geometry copies the drawer's List-rows field (`flex min-h-11`), not the
  Preview-panel row (`flex h-10`).

## Testing

### Vitest

- `apps/web/components/kanban/columns-menu.test.tsx` — ordering (`position` then
  `id`), checked state from the hidden set, `onToggle` payload, zero-step
  workflow renders a trigger with no items.
- `apps/web/components/kanban/swimlane-container.test.ts` — the retention rule:
  kept with a live hidden id and zero tasks, dropped with an empty hidden set and
  zero tasks, not retained by a stale id alone.
- `apps/web/components/kanban-display-dropdown.test.tsx` — no Steps section, and
  no `steps-filter-*` testid survives anywhere.
- Existing `use-user-display-settings.test.ts` and the bulk-move coverage stay as
  they are; R2 does not change what they assert.

### E2E

- Retarget `apps/web/e2e/tests/kanban/step-visibility-filter.spec.ts` from
  `display-button` to `columns-menu-<workflowId>` and from
  `steps-filter-step-<stepId>` to `columns-menu-step-<stepId>`. Its two existing
  scenarios (per-workflow isolation, persistence across reload) keep their
  assertions.
- Add the lane-retention scenario to that spec — it is the invariant the
  relocation rests on, and it is the one case R1's own comments called out as
  removing the swimlane entirely.
- New `apps/web/e2e/tests/kanban/mobile-step-visibility-filter.spec.ts`
  (mobile-chrome), per the `mobile-*.spec.ts` convention in
  `apps/web/e2e/README.md`: open the drawer, toggle a step of the focused
  workflow, assert the phone board result, assert ≥ 44px row height and no
  horizontal overflow.

## Verification

```
cd apps/web && pnpm vitest run components/kanban/columns-menu.test.tsx components/kanban/swimlane-container.test.ts components/kanban-display-dropdown.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:raw --project=chromium --grep "step visibility"
cd apps/web && pnpm e2e:raw --project=mobile-chrome --grep "step visibility"
make -C apps/backend test lint
```

The backend command is a no-change guard: R2 touches no Go file, and it should
stay green without any edit.

## Risks

- **`getVisibleWorkflows` is on the hot render path** and currently pure over
  `(filter, workflows, getFilteredTasks, showEmptyBoard)`. Threading the hidden
  map in must not turn it into a store reader or it becomes untestable in
  isolation. Pass a prepared predicate.
- **The retention rule changes existing board behaviour** for one case that used
  to drop a lane. Two E2E comments in the current spec file document the old
  behaviour and will read as stale — update them rather than leaving a
  contradiction in the tests.
- **The phone's focused-workflow id** has more than one plausible source. Picking
  a different one from the navigator's would let the drawer configure a workflow
  the user is not looking at. Read it from `getRenderedWorkflows`' input.
- **i18n key removal** — deleting `"steps"` without checking every reference will
  fail `i18n:check`. Grep before deleting.

## Deviations from this plan

- **`getVisibleWorkflows` was replaced, not amended.** It moved to
  `lib/kanban/workflow-swimlanes.ts` as `selectVisibleWorkflows`, joining the
  selectors the layering commit put there, and takes `hasTasks` /
  `hasLiveHiddenSteps` predicates instead of `getFilteredTasks`. Adding a new
  exported pure selector to the component module would have contradicted the
  commit directly below this work on the PR.
- **The phone's focused workflow needed shared state.** It lived in local
  `useState` inside `kanban-board.tsx`, unreachable from the drawer, so
  `mobileKanban.focusedWorkflowId` was added to the ui slice and
  `SwimlaneContainer` publishes to it. Deriving focus a second way in the drawer
  would have let it configure a workflow the user was not looking at.
- **`ColumnsMenu` gained a `touchTargets` prop.** Content is identical on both
  homes; the flag only relaxes menu density so phone rows clear 44px without
  making desktop menu rows 44px tall.
- **`mobile-step-visibility-filter.spec.ts` already existed** (added by the
  commit this revision reverts) and was rewritten rather than created. Its
  disclosure-only scenarios are gone; a focused-workflow isolation scenario
  replaces them.
- **AC-08's desktop E2E scenario was inverted, not just re-selected.** It
  asserted the workflow disappears when every step is hidden — the exact
  behaviour the retention rule reverses.

## Tasks

- [x] task-01 — Shared Columns menu on the swimlane header; remove the dropdown section
- [x] task-02 — Retain lanes with a non-empty live hidden set
- [x] task-03 — Phone home in the mobile menu drawer
- [x] task-04 — E2E retarget, retention scenario, mobile spec
