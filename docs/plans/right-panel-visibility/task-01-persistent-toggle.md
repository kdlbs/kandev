---
id: "01-persistent-toggle"
title: "Add a persistent right-panel toggle"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-RIGHT-PANEL-VISIBILITY-001
acceptance_criteria:
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.1
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.2
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.3
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.4
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.5
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.6
  - AC-UI-RIGHT-PANEL-VISIBILITY-001.7
system_design:
  - ../../specs/ui/system-design/right-panel-visibility.md
---

# Task 01: Add a persistent right-panel toggle

## Summary

Expose right-panel visibility through a persistent task-header control.
Wire both existing layout stores, preserve independent navigation, and prove the result on desktop, tablet, and phone.

## In scope

Implement the control, adapter, tablet conditional layout, compact reopening, and mixed-center protection from the design.
Remove the old disappearing control and duplicate document-mode right control on these task pages.
Add localized copy in all supported catalogs. Generate Traditional Chinese and pseudo catalogs through repository scripts.
Add the unit and browser scenarios in the plan's test matrix.
Update the public task-workspace instructions after implementation.

## Out of scope

No backend, release flag, new storage, breakpoint change, custom-panel snapshots, or Office-specific surface change.

## Acceptance

- UI-01 follows the active layout, survives hiding, preserves center content, and keeps touch/keyboard access.
- UI-02 retains existing phone navigation and wider-layout preferences.
- All listed regression checks pass, and the plan records actual results and rendered comparisons.

## ASCII UI preview

### UI-01: Desktop and tablet task header

Entry: open a task. The task header stays fixed; panel bodies own scrolling.

```text
BEFORE: current Dockview
+----------------------------------------------------------+
| Task title                         [Layouts] [Editor]     |
+---------------------------------+------------------------+
| Chat                            | Files       [Hide >]   |
|                                 | Terminal               |
+---------------------------------+------------------------+
After Hide: the right column AND its Hide button disappear.

AFTER: right panels visible
+----------------------------------------------------------+
| Task title                    [R >] [Layouts] [Editor]    |
+---------------------------------+------------------------+
| Chat                            | Files / Changes        |
|                                 +------------------------+
|                                 | Terminal               |
+---------------------------------+------------------------+
[R >] = Hide right panels

AFTER: right panels hidden
+----------------------------------------------------------+
| Task title                    [< R] [Layouts] [Editor]    |
+----------------------------------------------------------+
| Chat now uses the released width                         |
|                                                          |
+----------------------------------------------------------+
[< R] = Show right panels; same position, one tap to restore.
```

The existing left-sidebar toggle remains independent, outside this cropped region.
The coarse-pointer tablet fallback uses the same header control and hides Files plus Terminal.
Its Chat/Plan/Changes tabs remain in the left content surface.
Compact desktop starts with its existing single-group default; explicit Show adds a right column.
During initialization or restoration, the same control is disabled and keeps its position.

### UI-02: Phone task navigation

Entry: task below 768 CSS pixels. Retain the shipped full-screen panel composition.

```text
+------------------------------------+
| Task context                       |
+------------------------------------+
| Chat OR Files OR Terminal          |
| One active content surface         |
|                                    |
+------------------------------------+
| Chat | Plan | Changes | Files | >_  |
+------------------------------------+
```

The bottom row is an excerpt of existing navigation; other existing destinations remain available.
Select Files or Terminal, then Chat to return. There is no right-sidebar toggle on phones.
Keep existing safe areas and content scrolling; do not overwrite wider-layout preferences.

Required structure: persistent control order, independent sidebars, reclaimed width, and separate phone navigation.
Spacing and ASCII glyphs are illustrative. Use existing tokens and localized labels.
UI-01 maps to AC-UI-RIGHT-PANEL-VISIBILITY-001.1 through .5 and .7; UI-02 maps to .6.

See the [full package preview and scenario matrix](plan.md#ascii-ui-preview).

## TDD sequence

1. Add the tablet `right: false` regression and run it against current rendering.
2. Add the compact reopen and mixed-center store regressions; record behavioral failures before changes.
3. Implement the adapter, control, and layout fixes with their component tests.
4. Add browser coverage, inspect rendered views, and run the exact checks below.
5. Update public instructions, work-order results, and plan status.

## Verification

```bash
# From the repository root. Install once if this worktree lacks dependencies.
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/use-task-right-panels-toggle.test.ts components/task/task-right-panels-toggle.test.tsx components/task/mobile/session-tablet-layout.test.tsx lib/state/dockview-preset-persistence.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/use-task-right-panels-toggle.ts hooks/use-task-right-panels-toggle.test.ts components/task/task-right-panels-toggle.tsx components/task/task-right-panels-toggle.test.tsx components/task/task-top-bar.tsx components/task/dockview-header-actions.tsx components/task/document/document-controls.tsx components/task/mobile/session-tablet-layout.tsx components/task/mobile/session-tablet-layout.test.tsx lib/state/dockview-store.ts lib/state/dockview-preset-persistence.test.ts e2e/tests/layout/right-panel-visibility.spec.ts e2e/tests/layout/mobile-right-panel-visibility.spec.ts --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/right-panel-visibility.spec.ts tests/layout/pane-persistence-tablet.spec.ts tests/layout/compact-desktop-responsive.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-right-panel-visibility.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

New:

- `apps/web/components/task/task-right-panels-toggle.tsx`
- `apps/web/components/task/task-right-panels-toggle.test.tsx`
- `apps/web/hooks/use-task-right-panels-toggle.ts`
- `apps/web/hooks/use-task-right-panels-toggle.test.ts`
- `apps/web/components/task/mobile/session-tablet-layout.test.tsx`
- `apps/web/e2e/tests/layout/right-panel-visibility.spec.ts`
- `apps/web/e2e/tests/layout/mobile-right-panel-visibility.spec.ts`

Existing:

- `apps/web/components/task/task-top-bar.tsx`
- `apps/web/components/task/dockview-header-actions.tsx`
- `apps/web/components/task/document/document-controls.tsx`
- `apps/web/components/task/mobile/session-tablet-layout.tsx`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-preset-persistence.test.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`
- `docs/public/tasks-and-workflows.md`
- This work order and `plan.md` for status/results.

## Dependencies

None. Execute this vertical slice sequentially.

## Risks

See the [plan risks](plan.md#risks). Do not replace the layout stores or bypass restoration guards.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/right-panel-visibility.md), all seven criteria.
- [System design](../../specs/ui/system-design/right-panel-visibility.md), all sections.
- `apps/web/components/task/task-page-inner.tsx` and `hooks/use-session-layout-state.ts` for active-session routing.
- `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx` for phone composition.
- Existing `pane-persistence-tablet.spec.ts` and `compact-desktop-responsive.spec.ts` for fixtures and responsive assertions.
- `apps/web/AGENTS.md`, `/mobile-parity`, `/tdd`, and `/e2e` instructions.

## Results

Pending. No production or permanent test changes were made during package creation.
