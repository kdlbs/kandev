---
spec: docs/specs/ui/requirements/sidebar-task-row-presentation.md
created: 2026-08-23
status: complete
---

# Implementation Plan: Sidebar Task Row Presentation

## Overview

Add one task-row presentation object to each saved sidebar view and its draft. Normalize the object
at the frontend boundary, preserve it through the backend-owned user-settings path, and apply the
effective value to every sidebar task row.

Add a compact, collapsed **Task row** editor to the existing view settings. Reuse the installed
`@dnd-kit` packages for pointer, touch, and keyboard ordering. Keep the desktop popover and use a
bottom drawer for the phone and tablet editor.

Refine the provider-neutral change-request status summary in parallel. Give its rows shared label,
icon, and value columns without changing provider status logic.

Related specification: [PR task status summary](../../specs/ui/requirements/pr-task-status-summary.md).

## Confirmed Product Decisions

- Presentation is saved per sidebar view, not globally.
- The current row layout is the migration and new-view default.
- The **Task row** section starts collapsed on every editor open. Its open state is transient.
- The details master toggle hides all under-title metadata, including plugin metadata and queue or
  debug indicators.
- Detail order covers relative time, repository, and pull request number. Hidden fields keep their
  place in the order.
- The right side supports Git changes, relative time, pull request status, and nothing.
- A trailing relative time suppresses the duplicate details value without changing the saved field.
- A trailing pull request status moves the existing provider indicator. It does not duplicate it.
- Repository remains suppressed when the active view groups by repository.
- The mobile editor is an inset bottom drawer with one scroll body, safe-area padding, and 44 pixel
  controls.
- Mobile task rows keep their native primary tap behavior. The compact status indicator stays
  passive on coarse pointers.

## Data Contract and Persistence

### Backend

- Add a nullable task-row presentation object to `models.SidebarView` and
  `models.SidebarViewDraft`.
- Use explicit nested JSON fields for `details_enabled`, `detail_order`, `visible_details`, and
  `trailing`.
- Preserve the nested object through user-settings request and response DTOs, SQLite JSON storage,
  optimistic updates, and boot-state camel-case mapping.
- Set the canonical default object on backend-created default views. Keep a missing object valid for
  legacy saved data.
- Retain the existing service validation for view IDs, names, active-view references, and the
  maximum view count. The frontend owns enum normalization because older clients and future fields
  must render safely.

### Frontend

- Define the domain and API types in the sidebar-view type and HTTP contracts.
- Add one pure normalizer for saved and draft presentation data. It removes unknown and duplicate
  fields, appends missing known fields, limits visibility to known fields, and applies current
  defaults when the object is absent or invalid.
- Extend API wire mapping, boot hydration, migration, built-in views, direct view creation,
  duplication, save-as, overwrite, discard, and draft equality.
- Extend `useEffectiveSidebarView` so a draft previews presentation changes immediately.
- Send complete normalized presentation objects to the backend. Do not add local storage.

## Task Row Rendering

- Pass the effective presentation through `TaskSwitcher` and its row plumbing to `TaskItem`.
- Extract pure helpers that resolve visible detail fields, time semantics, group suppression, and
  the trailing item. Test these helpers before changing markup.
- Render configurable detail fields in saved order. Append non-configurable queue and debug
  indicators after configurable fields when details are enabled.
- Keep plugin `TaskRowMetadata` within the details area so the master toggle produces a true compact
  row.
- Let title badges render all current non-change-request badges. Move only the change-request
  contribution indicator when the right-side choice requests it.
- Generalize the current trailing Git-change slot. Passive Git changes or time can yield to the row
  menu. Interactive provider status and the row menu remain separately focusable.
- Do not reserve trailing space when the selected value is absent.

## View Settings UI

- Extract shared editor content from `SidebarFilterPopover` so desktop and mobile can use the same
  draft and save actions.
- Keep desktop in the anchored, viewport-contained popover.
- Render the phone and tablet editor in an inset bottom drawer. The drawer has one scroll owner,
  bottom safe-area padding, escape and outside-close behavior, and focus restoration.
- Add a collapsed **Task row** section after **Group by**. Use local component state for expansion.
- Derive the compact summary from details-enabled state, visible detail count, and trailing choice.
- Use sortable rows with a visible handle, visibility switch, keyboard sensor, and accessible drag
  announcements. Touch handles and other mobile controls are at least 44 by 44 CSS pixels.
- Explain the relative-time meaning and the dynamic **Shown on right** state in localized copy.
- Add every new key to English, Portuguese, Simplified Chinese, Hong Kong Chinese, and Taiwan
  Chinese catalogs. Generate the Traditional Chinese pair with the repository command.

## Change-Request Summary Alignment

- Refactor `ChangeRequestTaskStatusSummary` so all available rows in one entry share a
  `max-content`, fixed-icon, and flexible-value grid.
- Align secondary detail to the value column.
- Preserve current test IDs, provider presentation data, semantic tones, icons, wrapping,
  multi-entry separators, tooltip state, and viewport containment.
- Exercise the shared component through GitHub, GitLab, and registered-provider unit coverage.

## Tests

- **What:** defaulting, malformed-data normalization, wire round trips, draft equality, save,
  save-as, discard, duplicate, and optimistic rollback.
  **Files:** `apps/web/lib/state/slices/ui/sidebar-view-wire.test.ts`,
  `ui-slice-migration.test.ts`, and `sidebar-view-actions.test.ts`.
  **How:** use old, valid, future-field, duplicate-field, and invalid enum fixtures.
- **What:** backend JSON preservation and boot-state mapping.
  **Files:** `apps/backend/internal/user/store/sqlite_test.go`,
  `service/service_test.go`, `handlers/handlers_test.go`, and
  `internal/backendapp/boot_state_routes_test.go`.
  **How:** persist an explicit disabled/details order and trailing choice, reload it, and compare
  both REST and camel-case boot shapes. Include a legacy missing object.
- **What:** field order, group suppression, relative-time deduplication, details hiding, provider
  icon movement, missing-data collapse, and menu coexistence.
  **Files:** `apps/web/components/task/task-item.test.tsx`,
  `task-item-repository.test.tsx`, and a focused presentation helper test.
  **How:** render table-driven presentation and task-data combinations.
- **What:** collapsed editor state, draft patches, summaries, reorder input, and responsive surface
  choice.
  **Files:** focused tests beside `apps/web/components/task/sidebar-filter/`.
  **How:** render with the shared state provider and mock breakpoint, drag-end, and keyboard reorder
  events.
- **What:** shared status-value and detail alignment.
  **Files:** provider status-summary render tests.
  **How:** render label lengths and wrapped detail, then assert the shared grid contract and
  unchanged provider text.

## E2E Tests

- **Scenario:** Configure, preview, save, discard, save as, and reload a desktop task-row layout.
  **File:** `apps/web/e2e/tests/task/sidebar-filter.spec.ts`.
  **What to verify:** the section starts collapsed, its open state does not mark the view dirty,
  field order and visibility preview immediately, and saved views retain independent layouts.
- **Scenario:** Use the editor and reordered task row on a phone viewport.
  **File:** `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`.
  **What to verify:** the bottom drawer, 44 pixel controls, touch reorder, internal scroll,
  safe-area clearance, primary row tap, reload persistence, and no horizontal overflow.
- **Scenario:** Put pull request status on the right and open its summary beside the row menu.
  **File:** `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`.
  **What to verify:** one status indicator, separate keyboard targets for indicator and menu,
  aligned status-value left edges, aligned merge-queue detail, and viewport containment.
- **Scenario:** Keep the compact provider indicator passive on mobile.
  **File:** `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`.
  **What to verify:** tapping the row still navigates and no compact status control steals the tap.

## Documentation Impact

This design changes no CLI command, public API, install flow, or public configuration key. The
internal specs and plan are sufficient at this checkpoint. During implementation, search public
task and view documentation for screenshots or labels that show the old view editor. Update only
the affected page if one exists.

## Risks

- A non-null Go zero value can confuse a legacy missing object with an explicit disabled setting.
  Use a nullable nested object at the persistence boundary and normalize it in the frontend.
- Moving a provider indicator can remove the only hover or focus trigger. Keep one mounted instance
  and move its presentation location rather than rendering a second copy.
- The task menu currently overlays passive Git changes. Reusing that overlay for an interactive
  status would block its trigger. Give the status and menu separate hit targets.
- Nested drag contexts exist in the mobile sidebar. Keep field sorting inside the editor drawer and
  stop its events from reaching task or saved-view sorting contexts.
- Longer translations can widen labels and summaries. Use content-sized grid tracks with a flexible
  value column and test narrow viewports and pseudo-locale output.
- Hiding the whole details area also hides plugin metadata and queue indicators. The master-toggle
  test must cover those values so future components do not escape the contract.

## Verification Results

- Backend targeted coverage passed: `go test -tags fts5 ./internal/user/... ./internal/backendapp`
  reported 808 passing tests across 7 packages.
- Frontend persistence coverage passed: 4 files, 61 tests. Task-row rendering coverage passed: 5
  files, 78 tests. Settings coverage passed: 4 files, 12 tests. Shared provider-summary coverage
  passed: 4 files, 31 tests.
- Frontend typecheck, full workspace lint, changed-file ESLint, `i18n:check`, `i18n:ratchet`, and
  `git diff --check` passed. `make -C apps/backend test` passed the full Go suite.
- Desktop task-row E2E passed: 1 test in 6.1 seconds. Mobile task-row E2E passed: 1 test in 10.2
  seconds. PR summary E2E passed: 1 test in 3.2 seconds. The mobile status-summary regression
  passed: 1 test in 4.7 seconds. Final capture runs also passed the desktop and mobile task-row
  scenarios.
- The repository `i18n:zh-hant` wrapper remains blocked by its pre-existing `agents` namespace
  residual warning. The scoped task conversion completed successfully for both Traditional Chinese
  locales, with zero task residual warnings.

## Implementation Waves and Parallel Candidates

Wave 1:

- [x] [Task 01: Persist task-row presentation](task-01-persist-task-row-presentation.md)
- [x] [Task 04: Align change-request summary rows](task-04-align-change-request-summary-rows.md)

Wave 2:

- [x] [Task 02: Render configured task rows](task-02-render-configured-task-rows.md)
- [x] [Task 03: Build responsive task-row settings](task-03-build-responsive-task-row-settings.md)

Wave 3:

- [x] [Task 05: Prove task-row presentation end to end](task-05-prove-task-row-presentation.md)

Tasks 01 and 04 are independent. Tasks 02 and 03 both depend on Task 01 and can proceed in
parallel. Task 05 integrates all four production tasks. These labels document dependency only and
do not authorize delegated implementation.
