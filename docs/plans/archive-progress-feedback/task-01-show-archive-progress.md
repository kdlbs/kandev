---
id: "01-show-archive-progress"
title: "Show archive progress feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-004
acceptance_criteria:
  - AC-TASKS-REMOVAL-NAVIGATION-004.1
  - AC-TASKS-REMOVAL-NAVIGATION-004.2
  - AC-TASKS-REMOVAL-NAVIGATION-004.3
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
---

# Task 01: Show archive progress feedback

## Summary

Wrap user-initiated archive requests in one localized, non-expiring loading
toast and route the remaining `/tasks` archive action through the shared hook.
Prove the pending, success, failure, desktop, and phone states, then capture the
two pending views for the pull request.

## In scope

- Shared archive progress-toast lifecycle and concurrent-request coalescing.
- `/tasks` listing adoption of the shared archive action.
- Shipped locale catalogs.
- Unit and focused desktop/mobile Playwright coverage.
- PR asset capture for both viewports.

## Out of scope

- Changes to archive transport, confirmation, cascade, navigation, recovery,
  persistence, or permissions.
- Loading feedback for delete, unarchive, or programmatic archive callers.
- New toast or mobile navigation primitives.

## Acceptance

- A single or bulk archive shows one localized loading toast before its request
  settles; concurrent tasks in the same bulk action do not create a toast pile.
- The toast remains present without a timeout while pending, then is dismissed
  on resolve or reject while existing terminal feedback remains unchanged.
- Desktop and phone tests prove the visible spinner, polite announcement,
  bottom-right viewport placement, cancellation behavior, cleanup, and PR
  screenshots.

## ASCII UI preview

### UI-01: Desktop archive pending

Full preview: [plan.md#ui-01-desktop-archive-pending](plan.md#ui-01-desktop-archive-pending).
Applies to `AC-TASKS-REMOVAL-NAVIGATION-004.1` through `.3`.

```text
                                         +-------------------------------+
                                         | (spinner) Archiving in progress|
                                         +-------------------------------+
                                          fixed bottom-right; non-blocking
```

### UI-02: Phone archive pending

Full preview: [plan.md#ui-02-phone-archive-pending](plan.md#ui-02-phone-archive-pending).
Applies to `AC-TASKS-REMOVAL-NAVIGATION-004.1` through `.3`.

```text
+------------------------------+
| +--------------------------+ |
| | (spinner) Archiving in   | |
| |           progress       | |
| +--------------------------+ |
+------------------------------+
  viewport-contained above status bar
```

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/use-task-actions.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run tests/task/sidebar-immediate-archive.spec.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-immediate-archive.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public docs/specs docs/plans apps/web
```

## Files likely touched

- `apps/web/hooks/use-task-actions.ts`
- `apps/web/hooks/use-task-actions.test.ts`
- `apps/web/app/tasks/tasks-page-client.tsx`
- `apps/web/src/locales/en/tasks.json`
- `apps/web/src/locales/pseudo/tasks.json`
- `apps/web/src/locales/pt-pt/tasks.json`
- `apps/web/src/locales/zh-cn/tasks.json`
- `apps/web/src/locales/zh-hk/tasks.json`
- `apps/web/src/locales/zh-tw/tasks.json`
- `apps/web/e2e/tests/task/sidebar-immediate-archive-helpers.ts`
- `apps/web/e2e/tests/task/sidebar-immediate-archive.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts`
- `docs/public/tasks-and-workflows.md`

## Dependencies

None. The existing task-removal coordinator, toast loading variant, deferred
archive test helper, and PR asset fixture are the foundations.

## Risks

- Hook instances are surface-local, so the counter must coalesce only requests
  started by one operation without suppressing a separate concurrent action.
- Toast cleanup must run for both fulfilled and rejected archive promises.
- Phone capture must occur before reopening the task picker so the screenshot
  shows the global toast in the normal task surface.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-REMOVAL-NAVIGATION-004` and the archive progress section of the
  task-removal system design.
- Existing `useTaskActions`, `ToastProvider`, archive-removal tests, and
  `PrAssetCapture` patterns.
- The existing `ToastProvider` phone stack is the nearest mobile exemplar; its
  status-bar-aware fixed placement and polite live region remain unchanged.

## Results

- Added a reference-counted loading-toast lifecycle to the shared archive
  action and routed the tasks listing through it.
- Added complete locale coverage and documented the visible pending state in
  the public task lifecycle guide.
- Added unit and desktop/phone Playwright coverage for cancellation, pending,
  success, failure, accessibility, placement, and concurrent archives.
- Captured and reviewed fresh desktop and phone screenshots for the pull
  request.
