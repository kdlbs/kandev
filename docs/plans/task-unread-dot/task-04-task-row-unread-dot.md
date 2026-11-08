---
id: "04-task-row-unread-dot"
title: "Show the unread dot on task rows"
status: done
wave: 2
depends_on: ["01-unread-read-model"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
acceptance_criteria:
  - AC-TASKS-TASK-UNREAD-DOT-001.1
  - AC-TASKS-TASK-UNREAD-DOT-001.3
  - AC-TASKS-TASK-UNREAD-DOT-001.4
  - AC-TASKS-TASK-UNREAD-DOT-001.5
  - AC-TASKS-TASK-UNREAD-DOT-001.13
  - AC-TASKS-TASK-UNREAD-DOT-001.14
  - AC-TASKS-TASK-UNREAD-DOT-001.15
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
---

# Task 04: Show the unread dot on task rows

## Summary

The shared task row renders a labelled unread dot when the summary says unread,
Show unread dot on tasks is on, and the task is not the open one. Opening an
unread row keeps its dot for the configured delay (default 5 seconds, 0 to 60),
without delaying cursor writes. Desktop and mobile use the same row logic.

The settings enter `TaskRow` through required `unreadMessagesEnabled` and
`unreadDotHideDelayMs` props
passed through the complete shared switcher chain:
`TaskSwitcherProps` → `buildTaskRowProps` → `TaskRowBaseProps` →
`task-switcher-tree.tsx` → `TaskRow`. The desktop task-page owner and
`MobileTaskList` supply the values from `userSettings.unreadTaskDot` and
`userSettings.unreadTaskDotHideDelaySeconds`; neither `TaskSwitcher` nor the
shared row reads the store. Update each intermediate type, builder, row caller,
and both surface callers. Update
`task-switcher-row-render-identity.test.tsx` and
`task-switcher-render-stability.test.tsx`; these tests render without
`StateProvider`, so they supply the explicit setting prop instead of gaining a
provider dependency.

- `unread_messages` type; `TaskSwitcherItem.hasUnreadMessages` set by
  `sidebarStatus` and `sheetStatus`.
- Display rule in `task-switcher-row.tsx`; `TaskRow` prop;
  `TaskItem` prop; `TaskUnreadDot` as the last badge in
  `TaskItemLeadingBadges`.
- `task:taskHasUnreadMessages` and updated `settings:unreadShort` copy in
  `en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`, `ja`, `ko`, plus the generated
  pseudo catalog.
- `docs/public/configuration.md` and `docs/public/operations.md` (`/docs-maintainer`).

## Out of scope

- Backend projection (tasks 01 to 03).
- Kanban cards and Quick Chat indicators.

## Acceptance

- The dot shows for unread, unselected rows when enabled. Selecting an unread
  row starts the configured linger; selecting another row ends it early. New
  messages on an already-selected row do not start a linger.
- The setting is passed through `TaskSwitcherProps`, `buildTaskRowProps`,
  `TaskRowBaseProps`, and `task-switcher-tree.tsx` to the shared row; desktop and
  mobile task-list owners provide it. The shared row does not read the store.
- The provider-free row-render and switcher-stability tests supply the explicit
  setting prop and stay in the verification run.
- The dot has `role="img"` and a localized accessible name, and adds no line to
  the row.
- `pnpm run i18n:check` passes with no hardcoded copy and a current pseudo catalog.

## ASCII UI preview

Applies UI-01 from the [plan](plan.md#ui-01-task-row-with-unread-dot)
(`.1`, `.5`, `.13`, `.14`, `.15`).

```text
| o  Add retry to uploader   (*)   |
```

## Verification

```bash
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run components/task)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps && pnpm --filter @kandev/web lint)
```

## Files likely touched

- `apps/web/lib/types/task-status-summary.ts`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-switcher.tsx` (`buildTaskRowProps`)
- `apps/web/components/task/task-switcher-tree.tsx` (`TaskRowBaseProps` forwarding)
- Desktop task-page switcher-props builder and
  `apps/web/components/task/mobile/mobile-task-list.tsx`
- Existing `TaskSwitcher` / `TaskRow` callsites and task component tests that
  supply the new required prop.
- `apps/web/components/task/task-switcher-row.tsx`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/task-item-leading-badges.tsx`
- `apps/web/components/task/task-unread-dot.tsx` and test
- `apps/web/components/task/task-switcher-row-render-identity.test.tsx`
- `apps/web/components/task/task-switcher-render-stability.test.tsx`
- `apps/web/src/locales/*/task.json`, `apps/web/src/locales/*/settings.json`
- `docs/public/configuration.md`, `docs/public/operations.md`

## Dependencies

`01-unread-read-model` provides the summary field. Frontend tests stub the field.

## Risks

- The title truncates slightly earlier when the dot shows. Accepted; existing
  badges already behave this way.
- `isSelected` must receive the route-aware id on desktop; verify the sidebar
  still passes `highlightedTaskId`.

## Parallelism

`parallel-safe` with `03-unread-list-repair` (disjoint backend and frontend files).

## Inputs

- System design: Frontend.
- `TaskAgentErrorIcon` pattern, `task-switcher-row.tsx`, both row builders.

## Results

Done. `unread_messages` maps to `TaskSwitcherItem.hasUnreadMessages` through
`statusSummaryHasUnreadMessages` in both builders. `unreadMessagesEnabled` is a
required prop on `TaskSwitcherProps` and `TaskRowProps`, copied by
`buildTaskRowProps` and supplied by `TaskSessionSidebar` and `MobileTaskList`
from `userSettings.unreadTaskDot`, with the hide delay supplied from
`userSettings.unreadTaskDotHideDelaySeconds`. `shouldShowUnreadDot` in
`task-switcher-row.tsx` is the only display rule; `TaskUnreadDot` renders as the
last leading badge with `role="img"` and `task:taskHasUnreadMessages`. Copy was
updated in seven locales (zh-hk and zh-tw generated by the converter, pseudo
regenerated) and in `docs/public/configuration.md` and
`docs/public/operations.md`. The planned `task-unread-dot.test.tsx` was folded
into `task-switcher-unread-dot.test.tsx`, which asserts the accessible name
through the real row chain; builder mapping tests were not added because they
would only restate the copy. Verified by `vitest components/task` subsets (223
tests), `tsc --noEmit`, `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and
eslint and prettier on the changed files.
