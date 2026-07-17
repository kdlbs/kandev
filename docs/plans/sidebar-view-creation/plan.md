---
spec: docs/specs/ui/sidebar-view-creation.md
created: 2026-07-17
status: completed
---

# Implementation Plan: Direct Sidebar View Creation

## Overview

Add a guarded optimistic `createSidebarView()` state action first, then connect it to desktop and mobile controls through one shared popover/rename controller. Finish with production-build Playwright journeys proving immediate creation, optional rename, persistence, blocked states, and mobile touch parity. Existing user-settings contracts remain unchanged.

---

## Backend

No backend changes are planned. `apps/backend/internal/user/service/service.go::applySidebarViews` already validates the 50-view limit, nonblank names and IDs, and unique IDs; `applySidebarViewState` validates active-view membership. Existing settings storage, boot hydration, WebSocket updates, retries, rollback, and error toast remain the persistence path.

## Frontend

### Instant-create state contract

- In `apps/web/lib/state/slices/ui/sidebar-view-builtins.ts`, keep `DEFAULT_VIEW` canonical and add a factory that creates independent default view data plus a frontend 50-view limit aligned with backend `maxSidebarViews`.
- In `apps/web/lib/state/slices/ui/sidebar-view-actions.ts`, add `createSidebarView()` to `buildSidebarBackendActions`. Reuse `makeId`, `mutateViews`, and `toSidebarSettingsPayload`; guard drafts and the limit, choose the lowest available automatic name, append canonical defaults, and activate the new view.
- Add the action signature to `UISliceActions` in `apps/web/lib/state/slices/ui/types.ts` and forward it through `AppState` in `apps/web/lib/state/store.ts`.
- Keep `saveSidebarDraftAs`, `duplicateSidebarView`, and their persistence behavior unchanged.

### Shared creation and rename flow

- Create `apps/web/components/task/sidebar-filter/use-sidebar-view-popover.ts` to own popover-open state, a monotonic rename-request token, `startNewView`, and disabled reasons for an active draft or the 50-view limit.
- Update `SidebarFilterPopover` in `sidebar-filter-popover.tsx` to accept the rename request, clear stale requests when the popover closes or the active view rolls back, and constrain its current `22rem` width to the viewport.
- Update `ViewHeaderRow` in `view-manager.tsx` to enter rename mode for an external request, prefill from the newly active view, expose an accessible input name, and retain existing autofocus, Enter-save, Escape/Cancel, manual Rename, and `Save as…` behavior. Saving rename leaves the popover open; cancel never deletes the view.

### Desktop and mobile entry points

- In `apps/web/components/app-sidebar/sections/tasks-view-picker.tsx`, follow `WorkspacePickerContent` from `app-sidebar-workspace-picker.tsx`: append `DropdownMenuSeparator` and an `IconPlus` `New view` item with a stable test ID and keyboard-safe `onSelect`.
- In `apps/web/components/task/sidebar-filter/sidebar-filter-bar.tsx`, add a fixed labeled `New view` action outside `SidebarViewChips` so touch users do not need to scroll or hover. Keep interactive hit areas at least 40×40 px without overlapping the chip or gear targets.
- Adjust `apps/web/components/task/sidebar-filter/sidebar-view-chips.tsx` only if needed to preserve swipe scrolling, press-and-hold drag, and compact desktop dimensions after the mobile bar grows.
- Use existing Radix/shadcn surfaces and transitions. Add no animation dependency or broad sidebar restyle.

## Tests

- **What:** canonical creation, lowest-available naming, independent nested data, append/activate semantics, complete settings payload, draft/limit no-ops, queued-write rollback, and store isolation.
  **Files:** `apps/web/lib/state/slices/ui/sidebar-view-actions.test.ts` and `apps/web/lib/state/slices/ui/ui-slice.test.ts`
  **How:** RED/GREEN Zustand unit tests around `createSidebarView()` with mocked `updateUserSettings`.
- **What:** external rename request, automatic-name prefill, focus/accessibility, Enter-save, Escape/Cancel retention, stale-request cleanup, and manual-rename regression.
  **Files:** `apps/web/components/task/sidebar-filter/view-manager.test.tsx` and a focused test beside `use-sidebar-view-popover.ts`.
  **How:** Testing Library component/hook tests with mocked store actions.
- **What:** separated desktop menu item, one create per selection, event-driven popover handoff, draft/limit reasons, and a fixed mobile action.
  **Files:** `apps/web/e2e/tests/task/sidebar-filter.spec.ts`, `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`, and existing `apps/web/components/app-sidebar/sections/tasks-section.test.tsx`
  **How:** production-build Playwright covers real Radix focus/layering and responsive layout; the existing section test guards the picker mount.

## E2E Tests

- **Scenario:** create from desktop with no prior edit, observe immediate active canonical defaults and focused prefilled rename, then rename and reload.
  **File:** `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
  **What to verify:** selection before rename, no clone of active state, persisted renamed view, and popover remaining open.
- **Scenario:** cancel/close rename, create repeatedly, and attempt creation with a dirty draft or 50 views.
  **File:** `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
  **What to verify:** automatic-name retention/sequence and disabled reasons without draft loss.
- **Scenario:** create through the mobile task-switcher sheet, optionally rename/customize, reopen after reload, and inspect narrow layout.
  **File:** `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
  **What to verify:** touch-reachable fixed action, persistence, chip swipe behavior, minimum hit area, and no document-level horizontal overflow.
- Extend `apps/web/e2e/pages/sidebar-filter-popover.ts` with direct-create, automatic-name, and rename helpers; scope mobile locators to the sheet because the hidden desktop sidebar remains mounted.

## Implementation Waves

Wave 1:

- [x] [task-01-instant-create-state](task-01-instant-create-state.md) — done

Wave 2:

- [x] [task-02-sidebar-new-view-ui](task-02-sidebar-new-view-ui.md) — done; depends on task 01

Wave 3:

- [x] [task-03-sidebar-view-e2e](task-03-sidebar-view-e2e.md) — done; depends on task 02

Frontend files overlap and the E2E surface requires integrated behavior, so waves remain sequential.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- \
  lib/state/slices/ui/sidebar-view-actions.test.ts \
  components/task/sidebar-filter/use-sidebar-view-popover.test.tsx \
  components/task/sidebar-filter/view-manager.test.tsx

cd apps/web
pnpm build
pnpm e2e:run --host --no-build --project chromium -- tests/task/sidebar-filter.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-sidebar-views.spec.ts

cd ../..
make fmt
make typecheck test lint
```

### Result

- Production build passed.
- Focused desktop Playwright passed 18/18; focused mobile Playwright passed 5/5.
- Web unit suite passed 5,113 tests across 632 files; backend retry, script tests, typecheck, and all linters passed.
- The repository-wide CLI suite retains an unrelated baseline failure: `.github/workflows/notify-docs.yml` pins pnpm `10.29.3`, while `apps/package.json` pins `9.15.9`. Neither file is changed by this plan.

## Risks and considerations

- The single sidebar draft slot makes creation while dirty destructive; both controller and store action must guard it.
- Never reuse nested references from `DEFAULT_VIEW` when creating a view.
- Create and immediate rename enqueue two ordered full-array settings writes; reuse the existing queue rather than adding timing delays.
- A create rollback can occur while rename UI is focused; the controller must clear stale intent when the created active ID disappears.
- Frontend limit metadata can drift from backend `maxSidebarViews`; backend stays authoritative and failure rollback remains required.
- Radix dropdown-to-popover focus transfer must be event-driven, not timeout-based.
- Hidden desktop and visible mobile controls share test IDs; mobile Playwright locators must stay sheet-scoped.
- Larger touch targets must not overlap or break `TouchSensor` swipe/hold behavior.
