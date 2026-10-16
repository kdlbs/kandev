---
created: 2026-10-09
status: implemented
requirements:
  - REQ-TASKS-PRIORITY-VISIBILITY-006
system_design:
  - ../../specs/tasks/system-design/task-priority-visibility.md
legacy_specs: []
---

# Implementation Plan: Task Priority in the Task List View

## Overview

Show the existing non-medium priority indicator on each row of the `/tasks`
task list view. The HTTP task already carries `priority`, and the board and task
switcher already render the shared `TaskPriorityIndicator`, so this is a
frontend-only display change on the shared row title line.

## Scope

### In scope

- Render `TaskPriorityIndicator` in `PrimaryTaskLine`, after the title and before
  change-request and archived badges, for compact and detailed rows.
- Unit coverage for indicator presence, medium and invalid fallback, and order.
- Desktop and mobile Playwright coverage for first render and no horizontal overflow.

### Out of scope

- A priority menu or control in the task list view.
- Sorting, grouping or filtering the task list by priority.
- New WebSocket subscriptions for the task list, backend changes, new copy or tokens.

## Technical approach

- In `apps/web/app/tasks/rich-task-list-row.tsx`, import
  `TaskPriorityIndicator` from `@/components/task/task-priority-indicator` and
  render `<TaskPriorityIndicator priority={task.priority} testId="tasks-list-row-priority" />`
  directly after the `tasks-list-row-title` span. `PrimaryTaskLine` is shared by
  the compact row and `RichTaskContent`, so both displays get the indicator from
  one change.
- The indicator is `shrink-0` and the title is `min-w-0 truncate`, so a long
  title truncates before the indicator is pushed out of view.
- The indicator sits inside the row button; it has no click handler, so a click
  on it still opens the task, as for the title.

## ASCII UI preview

`UI-01: Task list row` (entry: `/tasks`, compact display, desktop). Structural
requirement: the indicator directly follows the title; spacing is illustrative.

```text
+--------------------------------------------------------------------------+
| (o) Fix login redirect loop  [!]  [PR #12]              2h ago  [a] [x]  |
| (o) Update onboarding copy                              3h ago  [a] [x]  |
|     (o) Translate copy  [v]                             3h ago  [a] [x]  |
| (o) Old migration cleanup  [^]  (Archived)              5d ago  [u] [x]  |
+--------------------------------------------------------------------------+
  [!] critical  [^] high  [v] low   medium: no indicator
```

`UI-02: Task list row, detailed display` (entry: `/tasks` with task details on).

```text
+--------------------------------------------------------------------------+
| (o) Fix login redirect loop  [!]  [PR #12]              2h ago  [a] [x]  |
|     [E2E Repo] [2 sessions]                                              |
|     Users bounce between /login and /home after SSO...                   |
+--------------------------------------------------------------------------+
```

`UI-03: Phone` (entry: `/tasks` on a phone). Composition is shared with desktop;
the relative time is already hidden below `sm`. The title truncates and the
indicator stays visible; the page has no horizontal overflow.

```text
+-------------------------------------+
| (o) Fix login redirect l… [!] [a][x]|
| (o) Update onboarding copy    [a][x]|
+-------------------------------------+
```

Mapping: UI-01 and UI-02 cover `AC-TASKS-PRIORITY-VISIBILITY-006.1` to `.3`;
UI-03 covers `.4` and `.5`.

## Tests

- `apps/web/app/tasks/rich-task-list-row.test.tsx` covers
  `AC-TASKS-PRIORITY-VISIBILITY-006.1` to `.3`: critical, high and low render
  `tasks-list-row-priority` after the title with a localized accessible name in
  compact and detailed displays; medium, empty and unknown values render none
  and no raw token.

## E2E tests

- `apps/web/e2e/tests/task/task-list.spec.ts` covers
  `AC-TASKS-PRIORITY-VISIBILITY-006.1`, `.2` and `.4`: seed a `critical` and a
  `medium` task, open `/tasks`, and assert the indicator only on the critical row
  on first render.
- `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts` covers `.4` and
  `.5` in `mobile-chrome`: the critical row shows the indicator and the document
  has no horizontal overflow.

## Work orders

- [x] [Task 01: Render priority in task list rows](task-01-render-list-row-priority.md)

## Verification results

Review remediation corrected the first-render design description to name the Go
route boot payload and expanded the requirements overview to name all priority
display surfaces. Production behavior is unchanged.

Hosted CI exposed a workflow lifecycle test race: a newly primary session can
still be starting when its tab displays the profile name. The lifecycle test
now captures profile A's prior turn IDs before the return move and waits for
a new completed turn on the matching primary session in `WAITING_FOR_INPUT`
before checking its model-derived tab title. The return step explicitly seeds
`/e2e:simple-message`; an empty prompt intentionally does not start another turn
on an existing session. Reusing a parked session's old
waiting state cannot satisfy this wait. It retains the existing timeout and all
profile, session-count, primary-star, model-label and reload assertions.
The original wait passed a no-return-turn mutation; the repaired wait rejected
that same mutation at the readiness assertion. Four lifecycle combinations
passed locally with zero retries after the correction.

- Focused Vitest: 40 tests passed across 3 files.
- Web typecheck, ESLint (`--max-warnings 0`), Prettier, `i18n:check`,
  `i18n:ratchet` and `e2e:sleep-ratchet` passed.
- `tests/task/task-list.spec.ts` (chromium): 8 passed.
- `tests/task/mobile-task-listing-display.spec.ts` (mobile-chrome): 2 passed.
- `git diff --check` passed.

## Risks

- Dense rows already carry state, change-request, archived and plugin metadata;
  the extra indicator reduces title width on narrow phones.

## Open questions

- None. Medium priority stays visually silent, matching the board and task switcher.
