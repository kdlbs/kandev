---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-MOBILE-001
---
# GitHub dashboard mobile design

## Boundaries and mapping

`REQ-INTEGRATIONS-GITHUB-MOBILE-001` maps to query recovery (AC .1), picker composition (AC .2/.5), result rows (AC .3), and pagination (AC .4). The existing GitHub workspace settings and portable user settings remain the persistence boundary. [Shared task-view access](../../ui/system-design/mobile-task-view-access.md) owns the separate app-navigation entry.

## Query recovery

`useSavedPresets` exposes load status and retry along with existing mutations. Preserve generation/cancellation guards and pending mutation queues. Unavailable workspace data must not be written as an empty collection. Portable snapshot hydration must not overwrite a newer local mutation. Retry the settings read explicitly; query-result Refresh remains a different action.

Associate workspace data and load status with the workspace ID so the first render after navigation cannot expose the previous workspace's snapshot or enable writes against it. Cached portable queries remain available alongside loading/error feedback on both desktop and phone.

`useSavedPresetActions.onConfirmSave` reports whether persistence succeeded in the originating scope. `SavePresetDialog` awaits the outcome, prevents duplicate submission, retains values on failure, and closes only on success. Scope changes discard the old form. Existing error toasts remain localized.

A save completion must belong to the same navigation generation, including A-to-B-to-A navigation. Keep the Save action's accessible name stable while pending and announce progress in one localized status region.

## Picker composition

`GitHubPageClient` keeps one shared query state; `useResponsiveBreakpoint` selects phone presentation below 768px. Extract a domain-owned mobile Views drawer, keeping `PresetsScopeBar` for desktop. The page toolbar supplies the labeled current-view opener. Kind selection applies existing defaults while leaving the drawer open. Final query selection closes it. Default/delete remain separate row actions.

Reuse the geometry of `components/kanban/mobile-menu-sheet.tsx`: inset bottom Drawer, fixed header/kind controls, one min-h-0 body scroller, dynamic maximum height, and a safe-area footer containing Save. Restore focus to the actual opener except when moving into Save; do not stack open overlays.

Open the save form after the Views drawer or desktop saved-query menu completes its close/focus callback. Cancel pending opening work on unmount and retain the mounted initiating trigger for focus return when the form closes.

The mobile Views trigger is a full-width, 44px-high, single-line button. Truncate its visible current-view label while retaining the full accessible name and the complete name inside the drawer. `IntegrationListToolbar` separates mobile secondary information into a row after the query input: Results plus a tabular count on the left, updated time and a touch-sized Refresh on the right. The timestamp can truncate within the remaining width. Desktop retains its inline title/count and refresh group. This shared toolbar composition also applies to GitLab and host-plugin consumers without changing their query or pagination contracts.

The result-count translation owns pluralization and word order; its numeric span retains tabular styling. Loading uses separate localized copy, without a stale count.

## Result rows

Use `ChangeRequestRow` and `IntegrationStartTaskMenu` for both PRs and issues. Phone titles/repository metadata wrap; independent links and task controls retain their semantics. Shared linked-task indicators use conditional touch sizing and inline step context on touch devices; desktop retains compact tooltip presentation.

## Pagination

Keep the numbered desktop footer. Phone pagination uses Previous, a page selector labeled with current/total page, and Next. `MobileResultsPagination` owns a controlled domain-local `Drawer` using the shared UI primitives. Its header and Done action stay fixed above a single scroll body capped at 80dvh with bottom safe-area clearance. Opening focuses and scrolls to the selected page; `aria-current="page"` and a check mark identify it. Each 44px page button dismisses the drawer and only calls `onPageChange` for a different page. Escape, dismissal, and selection return focus to the trigger. Preserve the 1000-result ceiling and the existing `useGitHubSearch` page reset. The result list owns its separate scrolling and the footer clears bottom safe areas.

If a refreshed total no longer includes the current page, opening the chooser uses the drawer's default focus fallback (Done) instead of suppressing focus without a target. Choosing an available page still navigates exactly once.

## Verification

Use hook tests for deferred load/save errors, retries, successful empty collections, and stale workspace responses. Mobile Playwright exercises drawer hierarchy, save failure recovery, long names, task actions, many-page navigation, focus, and geometry. Desktop tests retain saved-query/default and result-action coverage.
