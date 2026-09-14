---
id: "01-task-row-groups"
title: "Group task-row actions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MENU-GROUPING-001
acceptance_criteria:
  - AC-TASKS-MENU-GROUPING-001.1
  - AC-TASKS-MENU-GROUPING-001.2
  - AC-TASKS-MENU-GROUPING-001.3
  - AC-TASKS-MENU-GROUPING-001.4
  - AC-TASKS-MENU-GROUPING-001.5
  - AC-TASKS-MENU-GROUPING-001.6
  - AC-TASKS-MENU-GROUPING-001.7
  - AC-TASKS-MENU-GROUPING-001.8
system_design:
  - ../../specs/tasks/system-design/task-menu-grouping.md
---

# Task 01: Group task-row actions

## Summary

Reorder single-task and bulk task-row menus. Keep groups free of empty or duplicate dividers.
Preserve action eligibility, touch interaction, selection clearing, and confirmations.

## In scope

- The files and menu variants named below.
- Focused unit and browser coverage for changed order, dividers, and existing action outcomes.
- TDD: establish failing order/group assertions before production edits.

## Out of scope

- New actions, backend changes, plugin registration types, and replacement mobile shells.

## Acceptance

1. Normal, subtask, archived, and bulk menu variants have the specified groups with no empty dividers.
2. Existing handlers, disabled states, selection behavior, and drag guards pass their regression tests.
3. Desktop and phone tests prove ordering and an action outcome, including scrolling to removal actions.

## ASCII UI preview

UI-01: Task-row menu. UI-03: Same sequence in the phone inset scrolling menu.

```text
Pin / Color > / Priority >
-------------------------
Edit / Rename / Duplicate (disabled)
-------------------------
Create subtask / Nest under > / Link > / Detach*
-------------------------
Move to > / Send to workflow >
-------------------------
Primary plugin actions*
-------------------------
Archive / Delete
```

Each slash denotes a separate action row. Asterisks denote conditional entries.
UI-04 bulk order is Pin, movement, removal. Existing archived-row eligibility remains intact.

The [full previews](plan.md#ascii-ui-preview) define UI-01 through UI-04.
Criteria AC-TASKS-MENU-GROUPING-001.1 through .8 apply to the relevant variants.
The phone entry point is the existing visible ellipsis.
Below 640px, the existing inset menu scrolls internally and preserves safe-area spacing.
At wider widths, existing anchored placement remains. Touch rows retain 44px targets.
No menu heading or fixed removal footer is added. Compare rendered desktop and phone screenshots with these previews.

## Verification

Run from the repository root. Managed E2E commands rebuild the application.
Do not overlap desktop and mobile runs. Record discovered tests and final results.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test components/task/task-switcher-context-menu.test.tsx)
(cd apps/web && pnpm exec eslint components/task/task-switcher-context-menu.tsx components/task/task-switcher-action-items.tsx components/task/task-switcher-plugin-menu-items.tsx components/task/task-switcher-link-menu.tsx components/task/task-move-context-menu.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-menu-grouping.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/task-menu-grouping
```

Run targeted tests and lint for any additional helper extracted during implementation.
The workspace install is required before the first package command in this worktree.
Existing tests remain in place. New tests assert group boundaries, not merely label presence.
Phone tests must tap the opener and a submenu, complete an action, and reach the final removal row.
Use existing confirmation flows and cancel removal when the scenario only checks reachability.
For long menus, assert viewport containment, internal scroll, and no document horizontal overflow.

## Files likely touched

- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-switcher-action-items.tsx`
- `apps/web/components/task/task-switcher-plugin-menu-items.tsx`
- `apps/web/components/task/task-switcher-link-menu.tsx`
- `apps/web/components/task/task-move-context-menu.tsx`
- `apps/web/components/task/task-switcher-context-menu.test.tsx`
- `apps/web/e2e/tests/task/task-menu-grouping.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`

## Dependencies

None.

## Risks

JSX children can return null after group composition. Existing action components can insert their own separators.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-menu-grouping.md).
- [System design](../../specs/tasks/system-design/task-menu-grouping.md).
- [Preview/detail contract](../../specs/tasks/requirements/task-actions-menu.md).
- Existing tests beside the named source files and the listed E2E scenarios.
- Scoped web guidance and the mobile-parity, TDD, and E2E skills.

## Results

Implemented the grouped single-task and bulk task-row composition with one
conditional divider between nonempty groups. Existing action eligibility,
selection clearing, drag guards, archive confirmation, touch sizing, and
mobile scrolling remain intact. Primary plugin actions are rendered as a
separate group before removal.

Verification on 2026-09-10:

- `pnpm test components/task/task-switcher-context-menu.test.tsx ...`: 3 files passed, 50 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:check`: passed.
- `pnpm e2e:run --no-build --project chromium tests/task/task-menu-grouping.spec.ts`: 1 passed after the managed runner build.
- `pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts`: 17 passed.
- Fresh desktop and phone task-row screenshots were captured, inspected, compressed, and mapped in the PR asset manifest.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
