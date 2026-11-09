---
id: "03-graph-dialog-renderer"
title: "Add the graph dialog and renderer"
status: done
wave: 3
depends_on:
  - "02-graph-model-layout"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-001
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-002
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-003
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-004
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-005
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-006
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-007
  - REQ-TASKS-TASK-RELATIONSHIP-GRAPH-008
acceptance_criteria:
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.4
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.5
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.7
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-002.8
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-003.5
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-004.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-004.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-005.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-005.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-005.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-005.4
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-005.5
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-005.6
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.4
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-006.5
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-007.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-007.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-008.1
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-008.2
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-008.3
  - AC-TASKS-TASK-RELATIONSHIP-GRAPH-008.4
system_design:
  - ../../specs/tasks/system-design/task-relationship-graph.md
---

# Task 03: Add the graph dialog and renderer

## Summary

Ship the user-facing graph: the xyflow renderer behind
`TaskGraphRendererProps`, the node card, the lazy dialog and phone sheet, the
entry control, the chip action, themes, i18n, and E2E coverage.

## In scope

- Add `@xyflow/react` to `apps/web` after user approval, and the
  `no-restricted-imports` rule outside `xyflow-task-graph-renderer.tsx`.
- `TaskGraphRendererProps`, the xyflow renderer, and `TaskGraphNodeCard`.
- `TaskGraphDialog` with lazy `TaskGraphContent`, view selector, legend, and
  loading, error, empty, no-order, truncated, and disabled-mode states.
- `TaskGraphButton` in the chat status row and passthrough toolbar, plus the
  "View graph" action in `TaskDependencyChip`.
- Token-based light and dark styling of xyflow.
- Touch: 44 px targets, long-press emphasis, no minimap, no horizontal
  overflow.
- `taskGraph.*` copy in all seven catalogs plus pseudo.
- Desktop and mobile Playwright specs.
- Promote the requirement to `active`, the design to `current`, and the plan
  to `implemented` after all checks pass.

## Out of scope

- Edge or parent editing in the graph.
- Office surfaces.

## ASCII UI preview

See the full previews in [plan.md](plan.md#ascii-ui-preview). The structure
below is binding; spacing is illustrative.

UI-01: Entry control

```text
 ... [⛓ 2 dependencies ▾] [⧉ Graph]        chip popover footer: [View graph →]
```

UI-02: Subtasks view (desktop)

```text
│ Task graph · Ship billing      [ Subtasks | Chain ]                     [×] │
│  ┌────────────┐   ┌────────────┐   ┌────────────┐                           │
│  │1 Schema    │──▶│2 API       │──▶│3 UI        │                           │
│  │✓ Completed │   │This task   │   │🔒 Blocked  │                 [+][-][⤢] │
│ ── Resolved   - - Pending   ━━ Failed   ┆ External                          │
```

UI-04: Phone sheet

```text
│ [×]  Task graph             │
│ [  Subtasks  |   Chain   ]  │  ← 44 px
│ ┌─────────┐   ┌─────────┐   │
│ │1 Schema │──▶│2 API    │─▶ │  ← canvas owns pan/pinch
│                   [+][-][⤢] │  ← 44 px
│ ── Resolved - - Pending ━━ Failed │
```

## Acceptance

- The dialog opens from both entry points, shows both views with step numbers,
  state, edge styles, legend, and focus, and navigates on node activation.
- The renderer is the only importer of `@xyflow/react`; nodes are not draggable
  or connectable and edges are not deletable; light and dark themes use app
  tokens.
- Desktop and mobile E2E specs pass, including live update, Escape close,
  44 px targets, and no horizontal overflow.

## TDD sequence

1. Failing `TaskGraphButton` visibility tests.
2. Failing node card and legend tests for labels, icons, and the focus ring.
3. Failing dialog tests with a stub renderer for each state and the selector.
4. Failing renderer tests for props, activation, emphasis, and color mode.
5. Failing chip test for the "View graph" action.
6. Failing desktop and mobile E2E specs.
7. Implement, then refactor with tests green.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/task-graph components/task/task-dependency-chip.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run --project chromium tests/task/task-graph.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-graph.spec.ts
make fmt
make typecheck test lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task-graph/*.tsx` and tests
- `apps/web/components/task/chat/chat-status-bar.tsx`
- `apps/web/components/task/passthrough-toolbar.tsx`
- `apps/web/components/task/task-dependency-chip.tsx`
- `apps/web/src/locales/*/tasks.json`
- `apps/web/eslint.i18n.options.mjs`
- `apps/web/eslint.config.mjs`, `apps/web/package.json`, `apps/pnpm-lock.yaml`
- `apps/web/e2e/tests/task/task-graph.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-graph.spec.ts`
- `apps/web/AGENTS.md` if a convention changes

## Dependencies

Task 02.

## Risks

- xyflow requires its stylesheet; import it in the lazy content module only.
- jsdom lacks `ResizeObserver` and layout; renderer unit tests need a stub or
  should be limited to props mapping, with E2E covering geometry.
- Long press must cancel on pointer move so it does not block panning.

## Parallelism

`sequential`

## Results

Implemented the xyflow renderer, node card, legend, mode selector, lazy dialog,
`TaskGraphButton`, `useTaskGraphAvailable`, and the chip "View graph" action,
with copy in seven catalogs plus pseudo. Added `@xyflow/react` 12.12.

E2E found three issues that unit tests could not: xyflow disables pointer
events on non-selectable nodes (fixed with a per-node `pointerEvents: "all"`
style), Tailwind arbitrary variants turn `_` into spaces (control sizing moved
to a `[&>button]` class on `Controls`), and xyflow's unlayered stylesheet beats
layered utilities (the touch size is `!important`). Radix restores focus only
to a `DialogTrigger`, so the button is the trigger and the chip passes
`returnFocusTo`.

`useTaskDependencies` moved from the chip component into
`hooks/domains/task/use-task-dependencies.ts` so the availability hook does not
import a component module (tests that stub the chip broke otherwise). The full
web suite also showed that the availability fallback adds one
`/subtask-count` read on task pages with no known relations; the
plan-implementation fixture transport now answers it.

Validation passed:

- `pnpm exec vitest run components/task-graph components/task/task-dependency-chip` and the availability hook (27 tests)
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`
- `pnpm e2e:run --docker --project chromium -- tests/task/task-graph.spec.ts tests/task/task-dependencies.spec.ts tests/task/create-task-dependency-selector.spec.ts` (13 passed)
- `pnpm e2e:run --docker --project mobile-chrome -- tests/task/mobile-task-graph.spec.ts tests/task/mobile-task-dependency-navigation.spec.ts tests/task/mobile-create-task-dependency-selector.spec.ts` (4 passed)
