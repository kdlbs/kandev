---
id: "02-pinned-shortcuts"
title: "Add bounded pinned-task shortcuts"
status: done
wave: 2
depends_on: ["01-shared-navigation"]
plan: plan.md
requirements:
  - REQ-UI-MOBILE-MENU-003
acceptance_criteria:
  - AC-UI-MOBILE-MENU-003.1
  - AC-UI-MOBILE-MENU-003.2
  - AC-UI-MOBILE-MENU-003.3
system_design:
  - ../../specs/ui/system-design/unified-mobile-navigation.md
---

# Task 02: Add bounded pinned-task shortcuts

## Summary

Add up to three workspace-authorized pinned tasks to the shared phone menu.
Reuse existing pin order and task data, preserve route history, and keep app
navigation usable when task reads fail.

## In scope

- Pure eligible-pin projection and lazy menu content with current workspace
  read state, localized loading/error/retry, and no-pin omission.
- Shortcuts use Task 01's surface-aware selection callback; Task views remains
  available for saved filters and full task-list access.
- Unit and E2E coverage for isolation, read races/errors, cap/order, pin updates,
  history, and containment; update public navigation instructions.

## Out of scope

Task row redesign, pin mutation controls, new storage or APIs, Office task pins,
new recency tracking, new navigation ownership, and publication.

## Acceptance

- Projection excludes every ineligible candidate before applying the three-row
  cap, preserves pin order, and never writes preferences.
- Workspace pending/denied/error states cannot expose foreign or stale task
  names; global navigation and the existing Task views path remain usable.
- Shortcut history and focus match the originating surface; focused checks and
  rendered preview comparison pass.

## ASCII UI preview

### UI-04: Phone pins and unavailable data

```text
Global destinations
Pinned tasks                 [Loading tasks...]
  Fix checkout               or [Could not load tasks] [Retry]
  Update docs
  Review onboarding
Workspace actions / Utilities
```

See [UI-04 and UI-01](plan.md#ascii-ui-preview). Maps to -003.1 through .3.
The menu retains one scroller. Empty pins omit the section; denial reveals no
cached titles. Desktop/tablet stay as UI-05 with no new shortcut section.

## Verification

From repository root, after Task 01's dependency bootstrap. Use TDD for the
projection and recovery behavior. The files below are created in this task.

```bash
(cd apps/web && pnpm exec vitest run components/navigation/mobile-pinned-task-items.test.ts components/navigation/mobile-pinned-tasks.test.tsx components/navigation/app-nav-sheet.test.tsx)
(cd apps/web && pnpm exec eslint components/navigation/mobile-pinned-task-items.ts components/navigation/mobile-pinned-tasks.tsx components/navigation/app-nav-sheet.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-navigation-pins.spec.ts tests/layout/mobile-unified-navigation.spec.ts tests/github/mobile-task-view-access.spec.ts tests/layout/mobile-sidebar-read-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Cover more than three pins, duplicate/missing/deleted/archived/foreign IDs,
workspace switching with delayed responses, denial and retry, unpinning while
open, current-task shortcut selection, and non-task browser Back. Inspect the
loaded/empty/error phone screenshots against UI-04 and verify safe-area and
single-scroll containment with a long task title.

## Implementation files

- `apps/web/components/navigation/mobile-pinned-task-items.ts`,
  `mobile-pinned-tasks.tsx`, `mobile-task-shortcuts.tsx`, and their tests.
- `apps/web/components/navigation/app-nav-sheet.tsx`, `app-nav-sections.tsx`
  and callback types introduced by Task 01.
- `apps/web/e2e/tests/layout/mobile-navigation-pins.spec.ts`, relevant locales,
  `docs/public/sessions-and-review.md`, and this package's result/status fields.

## Dependencies

Task 01 supplies the shared shell and origin-aware selection contract.

## Risks

Cached task presence is not authorization. Share workspace read freshness and
identity checks instead of introducing a parallel task fetch or inferred scope.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/ui/requirements/unified-mobile-navigation.md),
  [design](../../specs/ui/system-design/unified-mobile-navigation.md).
- `useSidebarTaskPrefs`, `useWorkspaceSidebarTasks`, existing task-switcher
  workspace read recovery, and Task 01's callbacks.

## Results

Implemented. The [plan verification results](plan.md#verification-results) record
exact commands and outcomes: 109 focused unit tests pass, and all 242 affected
mobile scenarios have passing evidence across the broad run and focused reruns. Both compact-desktop browser checks
also pass.
Typecheck, targeted ESLint, translation completeness, and i18n ratchet pass.
Phone screenshots were inspected at 393px and 767px, including menu/options
containment and pin loading/error/denial states. Task creation survives rotation;
workspace metadata refreshes preserve the workbench and cached chat scroll state.
No backend contract or preference storage was added.
