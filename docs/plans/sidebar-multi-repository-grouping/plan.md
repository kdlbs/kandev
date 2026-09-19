---
created: 2026-08-27
status: implemented
requirements:
  - REQ-UI-SIDEBAR-REPOSITORY-GROUPING-001
system_design:
  - ../../specs/ui/system-design/sidebar-repository-grouping.md
legacy_specs: []
---

# Implementation Plan: Sidebar Multi-Repository Grouping

## Overview

Project every task repository into the shared sidebar item. Then group multi-repository tasks by their complete ordered repository combination.

First, add the shared data correction and focused unit tests. Then add desktop and mobile browser evidence against a production build.

## Confirmed root cause

`buildSidebarItem` and `toSheetItem` project only the primary `repositoryId` into `repositoryPath`. Neither projector fills `TaskSwitcherItem.repositories`.

`applyGroup` therefore treats a real multi-repository task as a single-repository task. It places the task in the primary repository group.

The existing grouping unit test creates `TaskSwitcherItem.repositories` directly. This test does not pass through either production projector.

The regression test is `groups a projected multi-repository task by every repository slug`. Before the correction, it receives only the primary slug.

## Scope

### In scope

- Project ordered canonical repository slugs for desktop and mobile sidebar items.
- Keep repository links in both item projections for incomplete-metadata detection.
- Give each complete repository combination a stable key and concatenated label.
- Keep tasks visible while repository metadata resolves.
- Add focused unit tests and desktop/mobile Playwright coverage.

### Out of scope

- Repository filter semantics.
- Backend task or repository payload changes.
- Saved-view schema changes.
- Sidebar group-header layout changes.
- Public documentation changes.

## Technical approach

### Sidebar projection

Add one shared helper for ordered repository slug resolution. Use it in `buildSidebarItem` and `toSheetItem`.

The helper accepts task repository links and a repository-slug map. It sorts by `position`, removes duplicate repository IDs, and omits unresolved slugs.

Both projectors set `repositories` from this helper. Both projectors keep `repositoryLinks` for the full attachment count.

Repository filters continue to read the primary `repositoryPath` compatibility value. The complete slug array changes repository grouping only.

### Repository grouping

Update the repository extractor in `apply-view.ts`. Use a named combination when all repository links resolve to two or more slugs.

Build the label with comma-space separators. Build the key with a fixed prefix and `JSON.stringify(repositories)`, including repeated canonical slugs when distinct links resolve to the same slug.

Keep `__multi__` only as the incomplete-metadata fallback. Update repository-group sorting to recognize named combination keys.

## Mobile design contract

- **Outcome and entry:** Desktop uses the Tasks sidebar. Phones use the existing task-switcher drawer.
- **Exemplar:** Keep `SessionTaskSwitcherSheet` and its shared `TaskSwitcher` list.
- **Hierarchy and presentation:** The existing group heading shows the complete repository combination. No new surface is added.
- **Interaction:** The existing group heading remains the collapse and expand control.
- **Scroll and geometry:** Existing desktop and mobile scroll owners remain unchanged.
- **Shared state:** Both surfaces use the same group identity and label rules.
- **Mobile proof:** A Pixel 5 Playwright test shows the full group heading and its task.

## Tests

- `task-session-sidebar-item.test.ts` proves that desktop projection keeps all ordered slugs.
- `session-task-switcher-sheet-item.test.ts` proves the same mobile projection.
- `sidebar-task-repositories.test.ts` proves order, duplicate removal, and unresolved metadata behavior.
- `apply-view.test.ts` proves complete combination keys, labels, separation, and fallback behavior.
- `apply-view-labels.test.ts` keeps the generic label only for incomplete metadata.

## E2E tests

- `sidebar-layout.spec.ts` covers `AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.2`, `.3`, and `.5` on desktop.
- `mobile-sidebar-views.spec.ts` covers `AC-UI-SIDEBAR-REPOSITORY-GROUPING-001.6` on mobile.
- Focused unit tests cover single-repository compatibility, combination identity, and incomplete metadata recovery.

## Work orders

- [x] [Task 01: Correct repository projection and grouping](task-01-correct-repository-grouping.md)
- [x] [Task 02: Prove desktop and mobile grouping](task-02-prove-responsive-grouping.md)

## Verification results

- `cd apps && pnpm --filter @kandev/web exec vitest run lib/sidebar/sidebar-task-repositories.test.ts components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-item.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts lib/sidebar/apply-view.test.ts lib/sidebar/apply-view-labels.test.ts`: passed, 6 files and 111 tests.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm run lint`: passed with zero warnings.
- `cd apps/web && pnpm run i18n:check`: passed.
- `cd apps/web && pnpm e2e:run tests/task/sidebar-layout.spec.ts -- --grep "multi-repository group"`: passed, 1 desktop test.
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "multi-repository group"`: passed, 1 mobile test.

## Risks

- Repository names can contain the display separator. The internal key must not use the display label as its encoding.
- Desktop and mobile have separate item projectors. Both must implement the same contract.
- Repository hydration can be incomplete. The task must not fall back to its primary repository group.
- A new combination key means that the old generic collapsed state does not collapse named combination groups.
