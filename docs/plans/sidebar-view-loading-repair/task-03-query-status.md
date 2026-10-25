---
id: "03-query-status"
title: "Unify sidebar status and recovery"
status: done
wave: 3
depends_on:
  - "02-view-page-reuse"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.6
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.9
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.12
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.14
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.15
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.16
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.17
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.18
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 03: Unify sidebar status and recovery

## Summary

Give desktop and phone one accurate, localized task-query status. Prove the complete
filter, cache, and recovery outcomes through existing isolated browser fixtures.

## In scope

Typed error mapper using `ApiError.body`, shared query-status presenter, removal
of duplicate archive/page errors, and clear cold versus background states.
Keep workspace-context errors distinct and higher priority. Maintain Filters
access for invalid input and Retry for recoverable reads. Six-language copy,
mobile touch/scroll/focus behavior, and public task/API documentation.

## Out of scope

Redesigning view editors, new dialogs, settings migrations, and changes to the
current conversation or task/session launch behavior.

## Acceptance

1. Red first: component integration test `renders one actionable query error`
   proves exactly one visible error for the original double-banner condition.
   Mapper tests cover every known reason, bounds, unknown/old payloads, and
   malicious values without rendering raw server text. Refresh failure keeps rows;
   initial failure never looks empty. Invalid input directs correction through Filters.
2. Existing desktop and phone paging specs include all five flows in the plan:
   seven-repository loading, held-response A-B-A, invalidation/context races,
   validation/network/denial recovery, and preserved chat. Phone tests verify
   drawer/menu status parity, 44px actions, focus/dismissal, and no overflow.
3. All new copy is localized; Traditional Chinese catalogs are generated.
   Public how-to and API reference explain bounded return reuse and input limits.
   Specs, plan statuses, and exact executed results are synchronized at completion.

## ASCII UI preview

UI-02 / UI-03; [full preview](plan.md#ascii-ui-preview).

```text
Desktop                           Phone Tasks drawer
TASKS [View v] [Filters]           Tasks                   [Close]
Tasks could not be refreshed.     [View v]              [Filters]
[Retry]                           Tasks could not be refreshed.
Task A                            [Retry]      (touch >=44px)
Task B                            Task A
                                  Task B
```

One status below view controls; no archived-only duplicate. The same slot shows
actionable invalid-filter copy on rejection, cold loading/error without rows, or
quiet updating status with retained rows. Pagination remains below the list.
Retain existing inset drawer, fixed header, body scroll, safe areas, direct task
navigation, and focus return. Maps to 002.9, .17-.18. Copy is illustrative.

## Verification

From repository root after Tasks 01-02. Managed E2E rebuilds frontend/backend:

```bash
(cd apps/web && pnpm exec vitest run lib/sidebar/sidebar-task-query-error.test.ts components/task/sidebar-task-query-status.test.tsx components/task/sidebar-task-pagination.test.tsx hooks/domains/kanban/use-sidebar-task-page.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/sidebar/sidebar-task-query-error.ts components/task/sidebar-task-query-status.tsx components/task/sidebar-task-pagination.tsx components/task/task-session-sidebar.tsx components/task/mobile/session-task-switcher-sheet.tsx components/task/mobile/session-task-switcher-sheet-read-status.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run tests/task/sidebar-task-pagination.spec.ts -- --project=chromium)
(cd apps/web && pnpm e2e:run tests/task/mobile-sidebar-task-pagination.spec.ts -- --project=mobile-chrome)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Compare rendered desktop and phone states to the preview during these E2E runs;
record screenshots or observed differences. Run the repository documentation
coverage preflight on the final diff. No full suites or additional review phase.

## Files likely touched

- `apps/web/lib/sidebar/sidebar-task-query-error.ts` and `.test.ts` (new)
- `apps/web/components/task/sidebar-task-query-status.tsx` and `.test.tsx` (new)
- `apps/web/components/task/sidebar-task-pagination.tsx` and `.test.tsx`
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet-read-status.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.test.ts`
- `apps/web/e2e/tests/task/sidebar-task-pagination.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-pagination.spec.ts`
- `apps/web/e2e/tests/task/sidebar-task-pagination-fixtures.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/sidebar.json`
- `docs/public/tasks-and-workflows.md` (how-to)
- `docs/public/websocket-api.md` (API reference)
- This plan and work orders, plus affected design/ADR statuses after review

## Dependencies

Tasks 01 and 02.

## Risks

Multiple hidden desktop/phone mounts can duplicate announcements or requests.
Assert within each visible surface and verify shared request identity. Scope
query errors separately from workspace access failures; never hide the latter.

## Parallelism

`sequential`

## Inputs

- [Design: Responsive surfaces](../../specs/ui/system-design/sidebar-archived-filter.md#responsive-surfaces).
- Existing `ApiError`, pagination component tests, phone read-status helper,
  `SessionTaskSwitcherSheet`, and managed E2E fixtures.
- Mobile-parity, E2E, docs-maintainer skills and `docs/i18n.md`.

## Results

Implemented a shared query-status presenter and safe localized validation mapper.
Desktop no longer forwards query errors into the archive banner, and phone uses
the same status beside its existing task tree. Pure markup extraction keeps the
desktop sidebar and phone sheet within the repository file-size limits.

The existing desktop and phone pagination suites each passed their two original
paging scenarios. The added recovery scenario passed on both viewports, proving
seven real repository selections, held-response A-B-A reuse, refresh failure with
one Retry, archive invalidation, invalid-filter correction, access denial clearing
rows, cold failure/retry after reload, no horizontal overflow, and preserved chat.
The phone case checks the 44px action and native drawer dismissal. View selection
uses native phone chips; the desktop uses its dropdown. No product selector or
layout change was needed for the test. Workspace-response races and shared-request
unmount are covered deterministically in the hook tests rather than duplicated
in browser setup. Existing phone app-navigation paging verifies the other mount.

E2E was added after the model/hook behavioral RED gates. The held-response hook
regression reproduces the pre-fix state without rebuilding old cross-layer wiring;
the browser cases then exercise the full fixed stack. Initial browser failures
were test-selector/fixture mistakes (the DnD live status, desktop-only view picker,
API cleanup method, and nonexistent drawer Close button), corrected without
production changes.

Public docs updated: `tasks-and-workflows.md` (how-to) and `websocket-api.md`
(reference). All six locales and generated pseudo copy pass `i18n:check` and the
new-copy ratchet. Targeted unit/event tests, TypeScript, zero-warning ESLint,
Go lint, public-doc validation, catalog/spec/harness validation, documentation
coverage preflight, and `git diff --check` passed. Final hook receipt and browser
capture evidence are recorded in the plan.

Review follow-up adds initial-loading status coverage and removes the redundant
pagination guard. The final expanded frontend/consumer run passes 105 tests.

CI follow-up: the context-menu drag regression compared absolute row coordinates
while the background query indicator disappeared. Both rows shifted upward by
40 px without changing order or entering a drag. Reproduced the exact assertion
with retries disabled on a fresh managed production build. The test now compares
stable task-ID order, retaining the no-drop-zone and no-drag-opacity assertions.
All six subtask drag/drop scenarios pass with one worker and retries disabled;
actual nesting and sibling reorder remain covered. This is a test-contract fix;
no product behavior or public documentation changes are needed.

A post-format verification exposed another stale-coordinate read in the same
spec's invalid-subtask-target scenario: the intended drag never activated, so
the positive-control root drop zone stayed absent. That source handle now uses
the existing `settledBoundingBox` helper before pointer input, as the spec's
shared drag helper already does. This preserves the positive control and the
persisted parent assertion without adding sleeps or raising timeouts.
