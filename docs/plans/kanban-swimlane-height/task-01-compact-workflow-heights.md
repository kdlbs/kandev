---
id: "01-compact-workflow-heights"
title: "Compact workflow heights"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
  - REQ-UI-ADAPTIVE-KANBAN-002
acceptance_criteria:
  - AC-UI-ADAPTIVE-KANBAN-001.4
  - AC-UI-ADAPTIVE-KANBAN-001.5
  - AC-UI-ADAPTIVE-KANBAN-001.6
  - AC-UI-ADAPTIVE-KANBAN-001.7
  - AC-UI-ADAPTIVE-KANBAN-001.9
  - AC-UI-ADAPTIVE-KANBAN-001.10
  - AC-UI-ADAPTIVE-KANBAN-002.1
  - AC-UI-ADAPTIVE-KANBAN-002.2
  - AC-UI-ADAPTIVE-KANBAN-002.3
  - AC-UI-ADAPTIVE-KANBAN-002.4
  - AC-UI-ADAPTIVE-KANBAN-002.5
  - AC-UI-ADAPTIVE-KANBAN-002.6
  - AC-UI-ADAPTIVE-KANBAN-002.7
system_design:
  - ../../specs/ui/system-design/adaptive-kanban.md
---

# Task 01: Compact workflow heights

## Summary

Replace repeated full-board heights with bounded content heights for multiple visible workflows.
Preserve the definite viewport required by virtualized columns.

## In scope

- Add the browser geometry regression before production changes.
- Implement compact-mode selection, natural-height reports, and local measurement cleanup.
- Preserve full-height single-workflow and phone behavior.
- Cover collapse, empty recovery, filters, previews, tablet, and virtualized task access.

## Out of scope

Backend contracts, new settings, dependencies, Pipeline changes, and publication.

## Acceptance

- The sparse two-workflow browser assertion fails on the original full-height behavior and passes after the correction.
- All compact sizing and mode-transition scenarios satisfy 002.1-002.6, including the mixed dense/sparse case and retained-empty controls.
- Focused interaction and virtualization suites pass, with no stale measurements, document overflow, or saved-preference changes.

## ASCII UI preview

UI-01 and UI-02 excerpt from the [full preview](plan.md#ascii-ui-preview):

```text
Desktop                        Phone
[Workflow A / Columns]         [Workflow / Step v]
[Todo] [Plan] [Done]            [card]
[card]                        | focused column scroll
[Workflow B / Columns]         | fills available height
[Todo] [Review] [Done]          [Create task]
[card]
```

Multiple desktop/tablet lanes remain compact even when another visible lane is collapsed (002.1-002.3).
Phone retains the existing navigator and full-height focused column (002.6).

## Implementation sequence

1. Install workspace dependencies if absent.
2. Add `swimlane-height.spec.ts` with the two sparse workflows at 1440 by 900.
3. Run the targeted RED command. Record the second-header/card bounding boxes and outer scroll position.
4. Implement the sizing path in the paired design.
5. Add the remaining scenario matrix from the plan.
6. Run the targeted checks and inspect desktop, tablet, and phone captures.
7. Record results and promote the paired design only after all work passes.

Use `testPage` before seeding settings. Use disposable workflows and restore baseline settings in cleanup.
Scope selectors by workflow and step IDs. Add stable lane selectors only when existing selectors cannot identify the geometry.
Compare bounding boxes and scroll reachability. Visibility assertions alone do not prove compact sizing.
Wait for finite animations and stable measurements, without fixed sleeps.

## Verification

Run from the repository root. Each command starts from an independent directory context.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/swimlane-height.spec.ts)
```

The first browser run must fail for excessive lane height before production changes.
After implementation, run:

```bash
(cd apps/web && pnpm test -- --run components/kanban/swimlane-container.render-stability.test.tsx components/kanban/swimlane-kanban-content.render-stability.test.tsx components/kanban/virtualized-column-task-list.render-stability.test.tsx)
(cd apps/web && pnpm test -- --run hooks/domains/kanban/use-compact-swimlane-height.test.ts hooks/domains/kanban/use-column-natural-height.test.ts)
(cd apps/web && pnpm exec eslint components/kanban/swimlane-container.tsx components/kanban/swimlane-section.tsx components/kanban/swimlane-kanban-content.tsx components/kanban/adaptive-desktop-kanban.tsx components/kanban/virtualized-column-task-list.tsx components/kanban-column.tsx lib/kanban/view-registry.ts e2e/tests/kanban/swimlane-height.spec.ts e2e/tests/kanban/mobile-kanban.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-compact-swimlane-height.ts hooks/domains/kanban/use-column-natural-height.ts hooks/domains/kanban/use-shared-kanban-layout-props.ts hooks/domains/kanban/use-compact-swimlane-height.test.ts hooks/domains/kanban/use-column-natural-height.test.ts e2e/tests/kanban/swimlane-height-helpers.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/swimlane-height.spec.ts tests/kanban/large-column-virtualization.spec.ts tests/kanban/tablet-large-column-virtualization.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/auto-hide-empty-columns.spec.ts tests/kanban/wip-overflow-queue.spec.ts tests/workflow/workflow-sorting.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/kanban/mobile-large-column-virtualization.spec.ts)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use managed builds for current assets. Run suites sequentially with the runner's resource limits.
If helper extraction adds a unit suite, add its exact path to this block and record its result.

## Files likely touched

- `apps/web/components/kanban/swimlane-container.tsx`
- `apps/web/components/kanban/swimlane-section.tsx`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/components/kanban/adaptive-desktop-kanban.tsx`
- `apps/web/components/kanban/virtualized-column-task-list.tsx`
- `apps/web/components/kanban-column.tsx`
- `apps/web/lib/kanban/view-registry.ts`
- `apps/web/e2e/tests/kanban/swimlane-height.spec.ts` (new)
- `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`
- This plan, work order, and paired system design for completion results.

## Dependencies

None. Existing virtualization and adaptive width packages are complete.

## Risks

Natural-height reports must exclude allocated viewport height. New callback props must not bypass the memo comparator or trigger unrelated lane renders.
Long columns must remain scrollable after a lane shrinks, expands, or returns from collapse.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/adaptive-kanban.md), especially REQ-UI-ADAPTIVE-KANBAN-002.
- [System design](../../specs/ui/system-design/adaptive-kanban.md), workflow allocation and height measurement.
- Existing `workflow-filter.spec.ts` for two-workflow setup and cleanup.
- Existing large-column suites and `tabletTestPage` for virtualization and coarse-pointer evidence.
- Existing render-isolation suites for stable lane and card updates.

## Results

Implementation and final browser verification are complete (2026-09-11).

- The original browser test failed because the second card extended below the board at outer scroll position zero.
- The same geometry assertion passed after compact sizing replaced repeated full-board heights.
- Fourteen hook and render-stability tests pass. They cover height aggregation, drag freeze, observer cleanup, and late reports from hidden columns.
- Typecheck, targeted ESLint, formatting, and the i18n ratchet pass.
- The managed runner rebuilt the backend, Vite assets, and fixture plugin after the final source change.
- The final mobile command in Verification passed all 29 tests. The final Chromium matrix passed all 22 tests.
- The Chromium matrix used `--no-build` to reuse the same freshly built assets after the mobile run. Both runs used one worker and zero retries.
- Specification validation passed all 36 linter tests, full specification lint, and the final diff check.
- Desktop and tablet captures match UI-01. The phone capture matches UI-02, with one focused column and its existing navigator.
- Public guides do not specify lane dimensions. No public documentation change is needed.

Intermediate test corrections included an ambiguous collapse selector, short-card fixture sizing, the phone-only navigation helper, and response cleanup.
One earlier browser run failed during shared fixture setup with a workspace-update 404, before rendering the board.
The affected drag tests then passed without retries. No backend code changed.

The final unit regression also rejected late reports after a column was hidden or compact mode was disabled.
The first version failed both cases. The completed hook ignores reports outside the current visible-column scope.

No acceptance criteria remain open. No commit, publication, or additional task was created.
