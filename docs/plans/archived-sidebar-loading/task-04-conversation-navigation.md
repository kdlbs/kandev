---
id: "04-conversation-navigation"
title: "Restore archived conversation navigation"
status: done
wave: 4
depends_on:
  - "03-sidebar-pagination"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-001
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.6
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.9
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.10
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.11
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.12
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.13
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 04: Restore archived conversation navigation

## Summary

Replace archived history-only selection with real SPA route loading.
Read existing conversations without agent preparation or browser refresh.

## In scope

- Wire real router navigation for archived selections from every sidebar entry point.
- Scope route loading state and initial data to the route task before rendering.
- Resolve existing remembered/primary/fallback sessions with validated task ownership.
- Guard ensure while archive state is unknown or archived, including zero-session retries.
- Keep bounded message hydration, explicit loading/error/retry, and read-only empty states.
- Add desktop and phone E2E with distinct archived conversations and delayed responses.
- Update public archive navigation instructions after passing implementation evidence.

## Out of scope

Dockview deserialization internals, session creation behavior for known active tasks,
archive lifecycle changes, and message-history pagination redesign.

## Acceptance

1. Active-to-archived and archived-to-archived navigation show the correct saved conversation without reload, including cold caches and absent environment mappings.
2. Unknown/archived tasks never call ensure, prepare, launch, or resume; a real empty result differs from a failed read.
3. Rapid selection and picker dismissal reject late data, while existing active-task navigation regressions still pass.

## TDD evidence

Replace the archived test that expects session loading to be skipped with a route-level
assertion that real SPA navigation loads the selected task's conversation.
Add `TaskDetailRoute` tests where A's initialData survives into B's first render.
Assert A's session cannot become B's fallback before the effect resets route state.
Test remembered session ownership, missing primary, multiple sessions, and sessionless archives.
Extend ensure tests with unknown archive state and archived zero-session success/failure.
E2E starts with distinct persisted messages in A and B and a selected archived view.
Assert B's message after a click, then C's message after another click, with no reload.
Observe launch endpoints and require zero calls throughout these archived flows.
Test a delayed failed message read and Retry without a misleading empty-chat prompt.

## Risks

Route remounting can mask stale fallback state unless the first render is tested.
Do not fix the defect by launching a replacement session or swallowing load errors.
Uncorrelated layout exceptions remain outside this work order.

## ASCII UI preview

UI-03, excerpt from [the full preview](plan.md#ascii-ui-preview).

```text
Task B [Archived]
Loading conversation...   -> B's saved messages
                         -> Read failed. [Retry]
                         -> No saved conversation.
```

Desktop keeps the conversation in its workbench. Phone closes the picker and focuses
Task B's detail heading. The existing Unarchive action remains available.
No frame can show Task A's messages under Task B's heading.
Maps to AC-UI-SIDEBAR-ARCHIVED-FILTER-001.6 and .9 through .13.

## Verification

Run from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
(cd apps/web && pnpm exec vitest run components/task/task-select-helpers.test.ts components/task/task-select-helpers-archived.test.ts components/task/task-select-races.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts components/task/mobile/session-task-switcher-sheet-archived-selection.test.ts src/task-detail-route.test.tsx components/task/task-page-content.test.tsx components/task/task-page-content-helpers.test.ts hooks/domains/session/use-ensure-task-session.test.ts hooks/domains/session/use-session-messages.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium -- e2e/tests/task/archived-task-navigation.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- e2e/tests/task/mobile-archived-task-navigation.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/task-select-helpers.ts`
- `apps/web/components/task/task-select-helpers.test.ts`
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet-selection.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.test.ts`
- `apps/web/src/task-detail-route.tsx`
- `apps/web/src/task-detail-route.test.tsx`
- `apps/web/components/task/task-page-content.tsx`
- `apps/web/components/task/task-page-content-helpers.ts`
- `apps/web/lib/ssr/session-page-state.ts`
- `apps/web/hooks/domains/session/use-ensure-task-session.ts`
- `apps/web/hooks/domains/session/use-ensure-task-session.test.ts`
- `apps/web/e2e/tests/task/archived-task-navigation.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-archived-task-navigation.spec.ts (new)`
- `docs/public/tasks-and-workflows.md`

New files named in this work order are planned; existing parent directories are present.

## Dependencies

03-sidebar-pagination.

## Parallelism

`sequential`

## Inputs

- [Shared size-based pagination](../../specs/ui/requirements/sidebar-task-pagination.md).

- [Requirements](../../specs/ui/requirements/sidebar-archived-filter.md).
- [System design](../../specs/ui/system-design/sidebar-archived-filter.md).
- [Plan and evidence](plan.md).

## Results

Archived selection now uses SPA task navigation, loads the selected task's existing conversation,
and guards session ensure while archive state is unknown or archived. Unit coverage checks route
identity, loading/error/empty states, and ensure behavior. Desktop and phone E2E navigate from an
active task between two archived conversations, verify distinct messages without document reload,
reload the final task directly, and assert that no session launch request occurs.

Passed: focused selection, route, session, and ensure tests in the 263-test web run; desktop
archived-navigation E2E (1 test); phone archived-navigation E2E (1 test); web typecheck;
`pnpm run i18n:check && pnpm run i18n:ratchet`; public-doc validation; and `git diff --check`.
