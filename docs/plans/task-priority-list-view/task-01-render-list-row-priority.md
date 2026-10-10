---
id: "01-render-list-row-priority"
title: "Render priority in task list rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PRIORITY-VISIBILITY-006
acceptance_criteria:
  - AC-TASKS-PRIORITY-VISIBILITY-006.1
  - AC-TASKS-PRIORITY-VISIBILITY-006.2
  - AC-TASKS-PRIORITY-VISIBILITY-006.3
  - AC-TASKS-PRIORITY-VISIBILITY-006.4
  - AC-TASKS-PRIORITY-VISIBILITY-006.5
system_design:
  - ../../specs/tasks/system-design/task-priority-visibility.md
---

# Task 01: Render Priority in Task List Rows

## Summary

Render the shared `TaskPriorityIndicator` on the `/tasks` row title line, after
the title, for compact and detailed rows. See [plan.md](plan.md).

## In scope

- `PrimaryTaskLine` renders `TaskPriorityIndicator` with test ID
  `tasks-list-row-priority` directly after the title.
- Unit tests in `rich-task-list-row.test.tsx`.
- Desktop and mobile Playwright assertions.

## Out of scope

- Priority menus, sorting, grouping or filtering in the task list.
- Backend, contract, locale or token changes.

## Acceptance

- Critical, high and low rows show the indicator after the title in compact and
  detailed displays; medium, empty and unknown values show nothing.
- The first `/tasks` render shows the indicator on desktop and phone with no
  document horizontal overflow on the phone.

## ASCII UI preview

`UI-01: Task list row` (excerpt; full previews in [plan.md](plan.md)).

```text
| (o) Fix login redirect loop  [!]  [PR #12]              2h ago  [a] [x]  |
| (o) Update onboarding copy                              3h ago  [a] [x]  |
```

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run app/tasks/rich-task-list-row.test.tsx app/tasks/tasks-list-view.test.tsx components/task/task-priority-indicator.test.tsx --reporter=dot
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/task/task-list.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-listing-display.spec.ts
git diff --check
```

## Files likely touched

- `apps/web/app/tasks/rich-task-list-row.tsx`
- `apps/web/app/tasks/rich-task-list-row.test.tsx`
- `apps/web/e2e/tests/task/task-list.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts`

## Dependencies

None. `TaskPriorityIndicator` and the HTTP `Task.priority` field already exist.

## Risks

- Narrow rows lose title width; the indicator must stay `shrink-0` while the
  title truncates.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-PRIORITY-VISIBILITY-006` and the task list section of the system design.
- Existing `TaskPriorityIndicator` and its tests.

## Results

- `PrimaryTaskLine` renders `TaskPriorityIndicator` (`tasks-list-row-priority`)
  after the title, so compact and detailed rows share it.
- Unit tests cover compact and detailed rows, high and low, order before the
  archived badge, and the medium, empty, unknown and absent fallback.
- Desktop E2E seeds critical and medium tasks; mobile E2E asserts the indicator
  on a critical rich row alongside the existing no-overflow check.

- Review remediation corrected the system design to identify
  `routeData.tasksPage.tasks` in the Go boot payload as the initial task source.
  The SPA passes it to `TasksPageClient`; this is not server-rendered row HTML.

- Hosted CI remediation makes the existing workflow profile lifecycle test wait
  for a new completed turn on the matching primary session in
  `WAITING_FOR_INPUT` before tab-label checks, using the pre-move profile A turn
  IDs to exclude its prior turn. The return step seeds `/e2e:simple-message`
  so the existing-session empty-prompt suppression does not make the fixture idle.
  A no-return-turn mutation passed the old wait
  and failed the repaired readiness assertion. No production behavior or
  assertion timeout changed. All four reuse/new and complete/park combinations
  passed with zero retries.
