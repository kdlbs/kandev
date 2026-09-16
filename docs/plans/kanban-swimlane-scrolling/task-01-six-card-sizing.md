---
id: "01-six-card-sizing"
title: "Size swimlanes for six task rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-002
acceptance_criteria:
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

# Task 01: Size swimlanes for six task rows

## Summary

Replace the 400px compact cap with measured initial six-row sizing. Preserve sparse, single-workflow, phone, and drag behavior.

## In scope

Prefix measurement, column chrome reports, shared height calculation, memo comparisons, height-helper updates, and desktop/tablet/phone regression evidence.

## Out of scope

Backend changes, new preferences, column-width changes, other listing views, publication, and delegation.

## Acceptance

- Six initial cards fit without clipping at the top of a dense lane. Sparse lanes use their content height and existing floor.
- Scrolling later cards does not resize the lane. WIP dividers, metadata updates, and preview widths produce correct prefix measurements.
- Existing mode transitions, drag freeze, virtualized mounts, and phone navigation pass the specified focused checks.

## ASCII UI preview

UI-01 and UI-03: sizing excerpt from the [full preview](plan.md#ascii-ui-preview).

```text
Desktop                         Phone
[Workflow A / columns]          [Workflow / Step v]
[1] [1]                        [card]
[2] [2]                        ...
[3]                            full-height list
[4]                            [Create task]
[5]
[6]                            no six-card cap on phone
[Workflow B / columns]
[1]                            sparse lane fits content
```

Maps to 002.1-002.7. UI-04 empty and collapsed states stay as shown in the full preview.

## Verification

Run from the repository root. The managed E2E runner rebuilds production assets.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-compact-swimlane-height.test.ts hooks/domains/kanban/use-column-natural-height.test.ts components/kanban/virtualized-column-task-list.render-stability.test.tsx components/kanban/swimlane-kanban-content.render-stability.test.tsx components/kanban/swimlane-container.render-stability.test.tsx)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-compact-swimlane-height.ts hooks/domains/kanban/use-column-natural-height.ts components/kanban/virtualized-column-task-list.tsx components/kanban-column.tsx components/kanban/swimlane-kanban-content.tsx e2e/tests/kanban/swimlane-height.spec.ts e2e/tests/kanban/swimlane-height-helpers.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/swimlane-height.spec.ts tests/kanban/virtualized-column-spacing.spec.ts tests/kanban/large-column-virtualization.spec.ts tests/kanban/tablet-large-column-virtualization.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/kanban/mobile-large-column-virtualization.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record behavioral RED before production edits, final GREEN results, and rendered screenshots.
Inspect the plan scenario matrix as part of these runs. Add any extracted helper test to this command block before completion.

## Files likely touched

- `apps/web/components/kanban/virtualized-column-task-list.tsx`
- `apps/web/components/kanban/virtualized-column-task-list.render-stability.test.tsx`
- `apps/web/components/kanban-column.tsx`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/hooks/domains/kanban/use-compact-swimlane-height.ts`
- `apps/web/hooks/domains/kanban/use-compact-swimlane-height.test.ts`
- `apps/web/hooks/domains/kanban/use-column-natural-height.ts`
- `apps/web/hooks/domains/kanban/use-column-natural-height.test.ts`
- `apps/web/e2e/tests/kanban/swimlane-height.spec.ts`
- `apps/web/e2e/tests/kanban/swimlane-height-helpers.ts`
- `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`

## Dependencies

None. Read the existing height and virtualization suites before adding the RED case.

## Risks

Stale prefix measurements, double-counted row gaps, or unconstrained boxes can cause layout loops and excess mounts.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/adaptive-kanban.md).
- [System design](../../specs/ui/system-design/adaptive-kanban.md).
- [Scenario matrix and companion records](plan.md#e2e-tests).
- `apps/web/AGENTS.md`, mobile-parity, TDD, and E2E skills.

## Results

Implemented and verified on 2026-09-16. The compact lane now sizes from the
first six logical task rows, preserves the 12.5rem sparse floor, and retains
the full virtualizer total for later-card scrolling. WIP dividers and measured
row sizes are included once, estimates cover sparse measurement entries, and
scrolling later cards does not resize the lane.

The RED tests covered the old 400px cap, the unimplemented prefix callback,
and stale offscreen prefix measurements. The GREEN unit/component suites
passed, including the prefix helper, estimate fallback, bounded invalidation,
returned-row remeasurement, WIP-divider row, render stability, height-hook,
and spacing cases. The Chromium height suite passed 6 tests, including a
six-card mixed-height geometry assertion, and the mobile Kanban matrix passed
29 tests. Typecheck, i18n, full lint, formatting, documentation validation,
and whitespace validation also passed.
