---
id: "02-overflow-cues"
title: "Add accessible overflow cues"
status: done
wave: 2
depends_on: ["01-six-card-sizing"]
plan: "plan.md"
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-003
acceptance_criteria:
  - AC-UI-ADAPTIVE-KANBAN-003.1
  - AC-UI-ADAPTIVE-KANBAN-003.2
  - AC-UI-ADAPTIVE-KANBAN-003.3
  - AC-UI-ADAPTIVE-KANBAN-003.4
  - AC-UI-ADAPTIVE-KANBAN-003.5
  - AC-UI-ADAPTIVE-KANBAN-003.6
  - AC-UI-ADAPTIVE-KANBAN-003.7
  - AC-UI-ADAPTIVE-KANBAN-003.8
  - AC-UI-ADAPTIVE-KANBAN-003.9
system_design:
  - ../../specs/ui/system-design/adaptive-kanban.md
---

# Task 02: Add accessible overflow cues

## Summary

Show directional overflow fades and reveal native scrollbars during interaction. Preserve native scrolling and every existing card action.

## In scope

Local overflow hook, stationary fades, scoped scrollbar CSS, keyboard access, coarse-pointer fallback, motion/contrast modes, boundary scroll chaining, rendered evidence, and documentation impact review.

## Out of scope

Backend changes, new preferences, column-width changes, other listing views, publication, and delegation.

## Acceptance

- Fade and thumb states follow actual overflow at all boundaries and do not shift column or header geometry.
- Keyboard and touch users can reach the last task. Wheel input at column boundaries moves the outer workflow container without moving siblings.
- Drag, selection, WIP, responsive, accessibility, and theme checks pass. Package status reflects actual results.

## ASCII UI preview

UI-02 and UI-03: cue excerpt from the [full preview](plan.md#ascii-ui-preview).

```text
Desktop column / hover          Phone / touch
[fixed header]                  [Workflow / Step v]
..top fade..    | thumb         [card]
[middle cards] |               ...
..bottom fade..|               ..bottom fade..
                               [Create task / safe area]
Lane: left/right fades only where columns remain offscreen.
```

Maps to 003.1-003.9. UI-01 defines the unchanged swimlane hierarchy.
UI-04 defines empty and tablet states. Fades never receive pointer events.

## Verification

Run from the repository root. The managed E2E runner rebuilds production assets.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-kanban-overflow.test.ts components/kanban/adaptive-desktop-kanban.test.tsx components/kanban/virtualized-column-task-list.render-stability.test.tsx)
(cd apps/web && pnpm exec eslint hooks/domains/kanban/use-kanban-overflow.ts hooks/domains/kanban/use-kanban-overflow.test.ts hooks/domains/kanban/use-desktop-kanban-pan.ts components/kanban/adaptive-desktop-kanban.tsx components/kanban/virtualized-column-task-list.tsx components/kanban/swimlane-kanban-content.tsx e2e/tests/kanban/swimlane-scroll-affordances.spec.ts e2e/tests/kanban/mobile-kanban.spec.ts e2e/tests/kanban/mobile-kanban-navigation.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/swimlane-scroll-affordances.spec.ts tests/kanban/swimlane-height.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/kanban-reorder.spec.ts tests/kanban/auto-hide-empty-columns.spec.ts tests/kanban/wip-overflow-queue.spec.ts tests/kanban/task-multi-select.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/kanban/mobile-kanban-navigation.spec.ts tests/kanban/mobile-auto-hide-empty-columns.spec.ts tests/kanban/mobile-kanban-reorder.spec.ts tests/kanban/mobile-large-column-virtualization.spec.ts)
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

- `apps/web/hooks/domains/kanban/use-kanban-overflow.ts (new)`
- `apps/web/hooks/domains/kanban/use-kanban-overflow.test.ts (new)`
- `apps/web/hooks/domains/kanban/use-desktop-kanban-pan.ts (new)`
- `apps/web/components/kanban/adaptive-desktop-kanban.tsx`
- `apps/web/components/kanban/adaptive-desktop-kanban.test.tsx`
- `apps/web/components/kanban/virtualized-column-task-list.tsx`
- `apps/web/components/kanban/virtualized-column-task-list.render-stability.test.tsx`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/kanban/swimlane-scroll-affordances.spec.ts (new)`
- `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-kanban-navigation.spec.ts (new)`
- `apps/web/src/locales/*/kanban.json (only new accessible label frames)`
- `docs/specs/ui/requirements/adaptive-kanban.md`
- `docs/specs/ui/system-design/adaptive-kanban.md`

## Dependencies

Task 01 must pass before this task starts. Its settled sizing supplies the viewport geometry for overflow checks.

## Risks

Native scrollbar rendering differs by engine. Ancestor overflow and drag-reserve space can produce misleading edge states.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/adaptive-kanban.md).
- [System design](../../specs/ui/system-design/adaptive-kanban.md).
- [Scenario matrix and companion records](plan.md#e2e-tests).
- `apps/web/AGENTS.md`, mobile-parity, TDD, and E2E skills.

## Results

Implemented and verified on 2026-09-16. Kanban-owned overflow state now drives
top, bottom, left, and right fades; native scrollbars reveal on hover, focus, or
active scrolling without changing geometry; and the existing drag-only
horizontal scrollbar suppression remains intact. Scroll regions keep native
vertical chaining, translated accessible names, touch scrolling, reduced-motion
behavior, and forced-color fallbacks.

The RED tests covered the missing overflow state and cues, incorrect offset
dimension boundaries, and drag-reserve content extent. The GREEN overflow hook
and integration suites passed, including client-dimension tolerance and
real-content extent coverage. The Chromium affordance suite passed 5 tests,
including tablet full-strip cues and forced colors, fitting-board reserve
exclusion, and an overflowing board at its last real column. Neighboring
desktop Kanban suites
passed 23 tests, and the mobile Kanban regression matrix passed 29 tests,
including final-card navigation and document-width containment. Backend
build/plugin packaging, the E2E build, typecheck, i18n, full lint, formatting,
documentation validation, and whitespace checks passed. E2E screenshots were
captured during the desktop and mobile runs.

Chromium and mobile-Chromium were available for this verification. Firefox and
WebKit were not run in this environment.
