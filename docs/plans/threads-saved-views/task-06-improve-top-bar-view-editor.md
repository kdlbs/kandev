---
id: "06-improve-top-bar-view-editor"
title: "Improve the Threads top-bar view editor"
status: done
wave: 6
depends_on:
  - "05-prove-thread-view-behavior"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-003
  - REQ-UI-THREADS-SAVED-VIEWS-004
acceptance_criteria:
  - AC-UI-THREADS-SAVED-VIEWS-003.12
  - AC-UI-THREADS-SAVED-VIEWS-003.13
  - AC-UI-THREADS-SAVED-VIEWS-004.11
  - AC-UI-THREADS-SAVED-VIEWS-004.12
  - AC-UI-THREADS-SAVED-VIEWS-004.13
system_design:
  - ../../specs/ui/system-design/threads-saved-views.md
---

# Task 06: Improve the Threads Top-Bar View Editor

## Request checklist

- [x] Add a visible description to each Threads sort option.
- [x] Explain the attention order and every other sort order.
- [x] Show the live task-state icon in each task-picker row.
- [x] Show the current workflow-step label in each task-picker row.
- [x] Show the shared pull-request icon, status color, and disclosure.
- [x] Set five as the default maximum column count.
- [x] Keep another valid limit and no limit available to the user.
- [x] Give the desktop editor popover a stronger border and elevation.
- [x] Preserve the existing mobile drawer and 44-pixel touch targets.
- [x] Add focused unit and browser evidence for the changed behavior.

## Mobile design note

The phone entry point remains the active-view button in the header. The
existing inset drawer remains the only editor surface and the only scroll
owner. The task-picker rows reuse the desktop data and shared status icons.

## Verification

The implementation passed these focused checks:

```bash
(cd apps/backend && go test ./internal/user/store ./internal/user/service)
(cd apps && pnpm --filter @kandev/web test -- --run components/threads lib/threads lib/state/slices/ui)
(cd apps/web && pnpm run i18n:check && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings=0 components/threads lib/threads)
(cd apps/web && pnpm run build:e2e)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium tests/task/threads-view.spec.ts --grep "explains sorts and shows live task details")
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome tests/task/mobile-threads-view.spec.ts --grep "switches and edits saved views")
python3 scripts/lint-spec-files.py --all
```

## Result

Threads sort choices now explain their behavior. The task picker shows live
task state, workflow step, and pull-request status through the shared task
components. New views use a five-column limit, and saved unlimited views stay
unlimited. The desktop editor has a stronger border and elevation. The mobile
editor keeps its existing drawer and touch geometry.

The backend checks passed 310 tests. The focused web checks passed 325 tests.
Typecheck, ESLint, localization, the E2E build, specification lint, and both
browser checks also passed.
