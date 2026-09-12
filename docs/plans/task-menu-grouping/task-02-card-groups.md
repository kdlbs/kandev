---
id: "02-card-groups"
title: "Group card actions"
status: done
wave: 2
depends_on: ["01-task-row-groups"]
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

# Task 02: Group card actions

## Summary

Reorder the shared card entries and the inherited preview/detail menus.
Preserve conditional plugin placement and the archived/unresolved action sets.
Synchronize descriptions of the old menu positions in the plugin contract documentation.

## In scope

- The files and menu variants named below.
- Focused unit and browser coverage for changed order, dividers, and existing action outcomes.
- TDD: establish failing order/group assertions before production edits.

## Out of scope

- New actions, backend changes, plugin registration types, and replacement mobile shells.

## Acceptance

1. Card context/dropdown menus and normal preview/detail menus use the required order and dividers.
2. Reduced tiers and plugin visibility changes produce no empty groups or changes to action eligibility.
3. Browser checks preserve action outcomes and phone reachability. Placement documentation matches the new groups.

## ASCII UI preview

UI-02: Card context/three-dot menu. UI-03: Same sequence in the phone inset scrolling menu.

```text
Priority >
-------------------------
Edit [submenu only for card plugin actions]
-------------------------
Link > / Detach from parent*
-------------------------
Move to > / Send to workflow >
-------------------------
Primary plugin actions*
-------------------------
Archive / Delete
```

Each slash denotes a separate row. Asterisks denote conditional entries.
UI-04 archived detail: plugins, divider, Delete. Without plugins, show Delete alone.
UI-04 unresolved detail: plugins, divider, Archive and Delete together.

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
(cd apps/web && pnpm test components/kanban-card-menu-items.test.tsx lib/kanban/task-actions-menu-entries.test.ts)
(cd apps/web && pnpm exec eslint components/kanban-card-menu-items.tsx lib/kanban/task-actions-menu-entries.ts components/plugins/task-menu-actions.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/card-menu-delete-archive.spec.ts tests/kanban/task-actions-menu-preview.spec.ts tests/kanban/task-actions-menu-detail.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-task-priority.spec.ts)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/task-menu-grouping
```

Run targeted tests and lint for any additional helper extracted during implementation.
Task 01 supplies the workspace dependency installation.
Existing tests remain in place. New tests assert group boundaries, not merely label presence.
Phone tests must tap the opener and a submenu, complete an action, and reach the final removal row.
Use existing confirmation flows and cancel removal when the scenario only checks reachability.
For long menus, assert viewport containment, internal scroll, and no document horizontal overflow.

## Files likely touched

- `apps/web/components/kanban-card-menu-items.tsx`
- `apps/web/lib/kanban/task-actions-menu-entries.ts`
- `apps/web/components/plugins/task-menu-actions.ts`
- `apps/web/components/kanban-card-menu-items.test.tsx`
- `apps/web/lib/kanban/task-actions-menu-entries.test.ts`
- `apps/web/e2e/tests/kanban/card-menu-delete-archive.spec.ts`
- `apps/web/e2e/tests/kanban/task-actions-menu-preview.spec.ts`
- `apps/web/e2e/tests/kanban/task-actions-menu-detail.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-task-priority.spec.ts`
- `docs/plans/plugins/PLUGIN-API.md`
- `apps/packages/plugin-sdk (placement comments only, if present)`
- `apps/web/lib/plugins/types.ts (placement comments only, if present)`

## Dependencies

Task 01. Keep any shared helper compatible with its task-row consumers.

## Risks

The shared builder affects preview/detail automatically. Plugin comments and tests can still describe the old primary-action location.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-menu-grouping.md).
- [System design](../../specs/tasks/system-design/task-menu-grouping.md).
- [Preview/detail contract](../../specs/tasks/requirements/task-actions-menu.md).
- Existing tests beside the named source files and the listed E2E scenarios.
- Scoped web guidance and the mobile-parity, TDD, and E2E skills.

## Results

Implemented shared card grouping and inherited normal preview/detail ordering.
Archived and unresolved tiers now insert separators only between admitted plugin
and removal groups. Primary plugin placement documentation now describes the
movement-to-removal position, while Edit plugin nesting and Link plugin
placement remain unchanged.

Verification on 2026-09-10:

- `pnpm test components/kanban-card-menu-items.test.tsx lib/kanban/task-actions-menu-entries.test.ts`: 3 focused files passed, 50 tests passed with the row suite.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:check`: passed.
- `pnpm e2e:run --no-build --project chromium tests/kanban/card-menu-delete-archive.spec.ts tests/kanban/task-actions-menu-preview.spec.ts tests/kanban/task-actions-menu-detail.spec.ts`: 9 passed.
- `pnpm e2e:run --no-build --project mobile-chrome tests/kanban/mobile-task-priority.spec.ts`: 1 passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
