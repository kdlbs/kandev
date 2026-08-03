---
id: "02-sidebar-edit-e2e"
title: "Prove sidebar edit parity"
status: done
wave: 2
depends_on: ["01-shared-sidebar-edit-flow"]
plan: "plan.md"
spec: "../../specs/tasks/sidebar-task-edit.md"
---

# Task 02: Prove sidebar edit parity

## Acceptance

- Desktop Playwright coverage edits a non-active sidebar task, verifies persistence and updated sidebar copy, and proves the current route/task did not change.
- Phone Playwright coverage opens Edit from the visible task-actions menu, proves the drawer-to-dialog handoff and started-task locks, saves, and sees the updated row after reopening the drawer.
- Tablet coverage cancels Edit and returns to the still-open task-switcher sheet; the expanded phone menu remains viewport-contained, internally scrollable, and touch-sized with Edit included.

## Verification

Use TDD: add the new scenarios first and observe their missing-Edit failure before implementing or adjusting selectors. The managed runner rebuilds the production Vite bundle and backend fixtures before running both owning projects.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/sidebar-layout.spec.ts -- --workers=1)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/sidebar-layout.spec.ts tests/task/mobile-sidebar-task-actions.spec.ts -- --workers=1)
git diff --check
```

Confirm Playwright discovers tests in both `chromium` and `mobile-chrome`, and record the final test counts plus any screenshot or trace paths.

## Files likely touched

- `apps/web/e2e/tests/task/sidebar-layout.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`

## Dependencies

- Task 01: the shared Edit menu contract, edit controller, and responsive host wiring must exist.

## Parallelism

Sequential after Task 01. These tests consume Task 01's menu item, dialog wiring, and responsive presentation behavior.

## Inputs

- Every scenario in `docs/specs/tasks/sidebar-task-edit.md`.
- Plan `E2E Tests` and `Mobile design contract` sections.
- Existing patterns: `SessionPage.sidebar`, the visible **Task actions** button, managed `prCapture` evidence, API seeding/polling, and the current mobile menu geometry assertions.

## Risks

- Scope locators to the visible sidebar/drawer because task switchers can remain mounted in more than one responsive surface.
- Dropdown/context menus can detach during WebSocket updates; retry the open-select sequence using the established sidebar helper pattern if required rather than increasing timeouts.
- The E2E runner serves the production build; do not use `--no-build` after frontend changes.

## Output contract

Report RED and GREEN evidence, exact discovered/passed test counts, production-build evidence, screenshots/traces, cleanup, blockers, and remaining risks. Reconcile the likely-file list with the actual diff, set this task to `done`, and synchronize its checkbox plus verification results in `plan.md`.

## Results

- RED evidence: the new desktop and phone scenarios initially failed because the Edit menu item was not wired; the first desktop editor attempt also exposed the existing split Start/Update control for a pending task, so the scenario now intentionally uses a started task, matching the existing editor contract. The first phone seed helper did not persist descriptions, so that scenario now uses the API create helper to verify the locked prompt value.
- GREEN evidence: the managed Chromium sidebar suite passed 9 tests, including the context-menu and persistence scenarios. The managed mobile-chrome sidebar/action suite passed 10 tests, including phone drawer dismissal and tablet sheet retention. The final PR capture runs independently passed 2 Chromium and 2 mobile-chrome tests.
- `cd apps/web && pnpm run build` passed before the managed E2E runs. Four fresh screenshots (desktop menu/editor, phone editor, tablet editor) were inspected, compressed with `pngquant`, and validated against the manifest; the asset directory is ignored and is not part of the source diff.
- `git diff --check` passed. No blockers remain.
