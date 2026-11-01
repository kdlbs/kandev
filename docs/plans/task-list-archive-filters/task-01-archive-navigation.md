---
id: "01-archive-navigation"
title: "Add archive navigation and query integrity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-001
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-002
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-003
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-004
acceptance_criteria:
  - AC-TASKS-LIST-ARCHIVE-FILTERS-001.1
  - AC-TASKS-LIST-ARCHIVE-FILTERS-001.2
  - AC-TASKS-LIST-ARCHIVE-FILTERS-001.3
  - AC-TASKS-LIST-ARCHIVE-FILTERS-001.4
  - AC-TASKS-LIST-ARCHIVE-FILTERS-001.5
  - AC-TASKS-LIST-ARCHIVE-FILTERS-002.1
  - AC-TASKS-LIST-ARCHIVE-FILTERS-002.2
  - AC-TASKS-LIST-ARCHIVE-FILTERS-002.3
  - AC-TASKS-LIST-ARCHIVE-FILTERS-002.5
  - AC-TASKS-LIST-ARCHIVE-FILTERS-003.5
  - AC-TASKS-LIST-ARCHIVE-FILTERS-004.1
  - AC-TASKS-LIST-ARCHIVE-FILTERS-004.2
  - AC-TASKS-LIST-ARCHIVE-FILTERS-004.3
  - AC-TASKS-LIST-ARCHIVE-FILTERS-004.5
system_design:
  - ../../specs/tasks/system-design/task-list-archive-filters.md
---

# Task 01: Add archive navigation and query integrity

## Summary

Deliver a global archive destination with explicit archive modes on desktop
and phone. Establish the shared query model, route restoration, and response
guards before reorganizing the remaining filters.

## In scope

- Global command, encoded link, localization, and disabled unresolved-workspace state.
- Archive-mode selector above rows on both viewports. Remove old archive
  checkboxes and migrate affected callers and tests.
- URL query state, explicit All scope overrides, existing manual portable
  saves, atomic page resets, workspace transitions, and current-query API mapping.
- SPA boot eligibility and response identity guards. Reject mismatched rows
  and totals on initial, query-only, delayed, and cross-workspace reads.
- Effective query values for existing List filter surfaces until Task 02 moves them.

## Out of scope

The complete desktop filter toolbar, phone Filters drawer, applied indicators,
new result-state copy, last-page recovery, and public documentation updates
belong to Task 02. No backend API, boot payload schema, lifecycle, or plugin changes.

## Acceptance

1. The global command opens the current workspace archive with explicit All
   scope and no saved-filter writes. Existing Archive task behavior remains valid.
2. Active, Archived, and All use the existing server flags, totals, and paging.
   Query restoration and manual edits follow the specified persistence rules.
3. Incompatible boot rows and late responses never become current results.
   Desktop and phone archive flows pass, including accessible selector targets.

## UI preview

Use [the full previews](plan.md#ui-preview). Structured rows follow the active
task instruction to use Kandev rich output rather than ASCII sketches.

**UI-01: Command panel, archive navigation result**

| Region | Content |
| --- | --- |
| Search | Existing command-panel search |
| Navigation result | Archive icon, Go to archived tasks |
| Unresolved workspace | Same command disabled |

**UI-02 / UI-03: Shared archive-mode row**

| Desktop | Phone |
| --- | --- |
| Active / **Archived** / All above task rows | Same choices in one full-width row above task rows |
| Ordinary 28px controls | At least 44px active targets |

This work order adds the archive row. Task 02 supplies the remaining final
toolbar and drawer composition. Coverage: `001.1` through `.4`, `002.1`, `003.5`.

## Verification

Run each command from the repository root. Install workspace dependencies once
if this is a fresh worktree, as described in the plan. Capture a behavioral RED
before production edits. Managed E2E commands build fresh production assets.

```bash
(cd apps/web && pnpm exec vitest run lib/tasks/tasks-list-query.test.ts hooks/use-tasks-list-query-state.test.tsx app/tasks/tasks-page-client.query.test.tsx app/tasks/tasks-page-fetch-policy.test.ts app/tasks/tasks-list-view.test.tsx src/spa-routes.tasks-preferences.test.tsx components/global-commands.test.tsx components/homepage-commands.test.tsx lib/api/domains/kanban-api.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=12288 pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/tasks/tasks-list-query.ts hooks/use-tasks-list-query-state.ts hooks/use-tasks-list-data.ts app/tasks/tasks-page-client.tsx app/tasks/tasks-page-content.tsx app/tasks/tasks-list-controls.tsx app/tasks/tasks-list-archive-modes.tsx app/tasks/tasks-list-view.tsx app/tasks/tasks-page-fetch-policy.ts src/spa-routes.tsx components/global-commands.tsx components/kanban/kanban-header.tsx components/kanban/kanban-header-mobile.tsx components/kanban-display-dropdown.tsx components/kanban/mobile-menu-task-list-options.tsx lib/links.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-list-archive-navigation.spec.ts tests/task/command-palette-archive.spec.ts tests/task/archive-freezes-task-state.spec.ts tests/task/task-list-filters.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-list-archive-navigation.spec.ts tests/task/mobile-task-list-search.spec.ts)
git diff --check
```

Confirm each E2E command discovers the intended tests. Retain behavioral
coverage when replacing removed checkbox selectors. Inspect archive selection
and row navigation on both viewports. Record screenshots or artifact paths.
The touch archive entry uses existing View options to select List, followed
by the visible Archived control. For palette coverage, follow the existing
phone test pattern: Control+K, then tap the command. Do not invent a touch opener.

## Files likely touched

- New `apps/web/lib/tasks/tasks-list-query.ts` and `.test.ts`.
- New `apps/web/hooks/use-tasks-list-query-state.ts` and `.test.tsx`, plus
  `use-tasks-list-data.ts`. The page query test exercises the data hook.
- `apps/web/lib/links.ts`, `apps/web/components/global-commands.tsx`, and command tests.
- `apps/web/app/tasks/tasks-page-client.tsx`, `tasks-page-content.tsx`,
  `tasks-list-controls.tsx`, `tasks-list-view.tsx`, `tasks-page-fetch-policy.ts`,
  existing tests, and new `tasks-page-client.query.test.tsx`.
- New `apps/web/app/tasks/tasks-list-archive-modes.tsx`.
- `apps/web/src/spa-routes.tsx` and `spa-routes.tasks-preferences.test.tsx`.
- Shared List filter callers in `components/kanban/kanban-header.tsx`,
  `kanban-header-mobile.tsx`, `components/kanban-display-dropdown.tsx`, and
  `components/kanban/mobile-menu-task-list-options.tsx`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/{common,tasks,kanban}.json`
  as needed. Translate source locales, generate Traditional Chinese and pseudo.
- New desktop/phone archive navigation specs and the existing browser specs
  listed in Verification. Search all E2E callers for removed archive selectors.

## Dependencies

None. Read the package assumptions before implementation.

## Risks

Saved defaults, global active workflow, and route overrides are different inputs.
Use the effective query everywhere in List without saving automatic overrides.
The SPA can retain its initial boot object across navigation. Do not treat
presence of boot data as proof that its task rows match the current query.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-list-archive-filters.md).
- [System design](../../specs/tasks/system-design/task-list-archive-filters.md),
  especially Command registration through Request ownership.
- Root and web `AGENTS.md`, `/tdd`, `/e2e`, `/mobile-parity`, and `better-ui`.
- Existing `onlyArchived` API test, global command tests, archive action E2E,
  fetch-policy tests, and SPA grouping-link tests.

## Results

Complete on 2026-10-20.

- Behavioral RED covered missing archive navigation, query mappings and response
  ownership. Workspace-loss and Home-navigation regressions also failed before
  their guards were added. `/tmp/kandev-archive-loss-red.log` records those two failures.
- The combined focused unit run passed 99 tests in 13 suites. It includes every
  unit file named above. Typecheck, changed-file lint with zero warnings, and
  localization checks passed.
- All archive navigation, archive-action compatibility, and mode/history browser
  cases passed on the final production assets. Desktop uses Ctrl+K and Enter.
  Phone uses existing View options, direct mode taps, and the palette command.
- Opening the archive retained saved scope, accepted all workflows/repositories,
  and opened archived detail without creating a session. Existing conversation
  navigation specs remain unchanged; List navigation uses separate new files.
- The [plan results](plan.md#verification-results) record exact browser evidence,
  shared validation counts, environment corrections, and screenshot paths.

The query/data boundary is implemented in `use-tasks-list-query-state.ts` and
`use-tasks-list-data.ts`. Shared Home navigation stays authoritative; List
normalization cannot replace a committed route outside Tasks.

PR review remediation adds URL workspace resolution, bootstrap cancellation,
translated route recovery, and explicit metadata readiness. Unit regressions
failed before the fix. Desktop and phone browser cases now cover cross-workspace
history and recovery. See [PR review remediation](plan.md#pr-review-remediation).

The readiness flags are independent for workflows and repositories. An unused
filter's failed metadata read cannot hold the current query in loading. Cached
repository data does not prove that a pending fresh read completed. These
regressions failed before the refinement in unit tests and a real browser.

List controls mount after the route metadata reads settle. Controlled delayed
workflow reads prove that desktop and phone workspace navigation show loading
before the correct filtered rows.

CI remediation restores the workflow resolved for the current workspace as
List's ordinary default. A component regression verifies the task request and
unchanged portable settings. The full desktop startup suite verifies that a
workspace change followed by List displays the second workspace's task.
See [CI remediation](plan.md#ci-remediation) for current validation results.
