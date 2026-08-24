---
spec: docs/specs/ui/requirements/task-listing-display-preferences.md
created: 2026-07-25
status: done
---

# Implementation Plan: Task Listing Display Preferences

## Overview

First correct workflow-filter resolution and introduce a device-local view
preference with a compatibility fallback from the existing portable Kanban
mode. In parallel, extend backend user settings with the portable richer-row
boolean. Then wire that setting through frontend state and render optional
Kanban-like metadata in list rows before proving the integrated desktop and
mobile flows.

## Root cause

`resolveDesiredWorkflowId` in
`apps/web/lib/kanban/resolve-workflow.ts` treats a single visible workflow as a
reason to replace an explicit `null` selection with that workflow ID.
`KanbanBoard.useWorkflowSelection` runs the resolver after the display dropdown
stores **All Workflows** as `null`, so Kanban immediately selects the sole
workflow again. List does not run that effect, which is why the same selector
appears stable there.

## Backend

### Portable richer-row user setting

- `apps/backend/internal/user/models/models.go`: add
  `TasksListShowDetails bool` to `UserSettings`.
- `apps/backend/internal/user/dto/dto.go`: expose
  `tasks_list_show_details` in read and PATCH DTOs and map it in
  `FromUserSettings`.
- `apps/backend/internal/user/service/service.go`: apply the optional boolean
  patch and include it in `user.settings.updated` events.
- `apps/backend/internal/user/store/sqlite.go`: serialize and deserialize the
  boolean in the existing JSON-backed user settings record. No SQL migration is
  required.
- `apps/backend/internal/backendapp/boot_state_routes.go`: include
  `tasksListShowDetails` in the SPA boot payload.

### Backend tests

- `apps/backend/internal/user/dto/dto_test.go`: prove read mapping and PATCH
  omission/false semantics.
- `apps/backend/internal/user/service/service_test.go`: prove applying and
  publishing both true and false values without changing omitted values.
- Add or update the closest `apps/backend/internal/user/store/*_test.go` and
  `apps/backend/internal/backendapp/*_test.go` coverage if the existing
  round-trip/boot-payload suites enumerate every field.

## Frontend

### Workflow-filter correction

- `apps/web/lib/kanban/resolve-workflow.ts`: stop interpreting a single visible
  workflow as stronger than an explicit empty filter.
- `apps/web/lib/kanban/resolve-workflow.test.ts`: replace the one-workflow
  auto-selection expectation with a regression that preserves `null`; retain
  valid active/saved workflow fallback coverage.
- `apps/web/app/page.tsx`, `apps/web/app/tasks/page.tsx`, and
  `apps/web/src/spa-routes.tsx`: update stale comments only where needed; all
  call sites continue using the shared resolver.

### Device-local task-listing view

- Add `apps/web/lib/task-listing/view-preference.ts` and
  `view-preference.test.ts` with the versioned localStorage key, strict enum
  parsing, legacy `kanbanViewMode` fallback, mobile effective-view resolution,
  and same-document change notification.
- Add `apps/web/hooks/use-task-listing-view.ts` to expose the stored preference,
  synchronize the in-memory Kanban/Pipeline mode used by the board, and perform
  route changes without backend writes.
- `apps/web/app/page-client.tsx`: when Home mounts, restore List by navigating to
  `/tasks`; otherwise synchronize the saved Kanban/Pipeline mode before the
  board becomes authoritative.
- `apps/web/app/tasks/tasks-page-client.tsx`: record List when `/tasks` is
  explicitly shown.
- `apps/web/components/kanban/kanban-header.tsx` and
  `apps/web/components/kanban/mobile-menu-sheet.tsx`: use the device-local hook
  for selected state and view changes. Phone renders the effective Kanban
  fallback for a saved Pipeline preference without writing over Pipeline.
- `apps/web/hooks/use-kanban-display-settings.ts` and
  `apps/web/hooks/use-user-display-settings.ts`: stop view-toggle interactions
  from PATCHing `kanban_view_mode`; retain the legacy backend field only as a
  first-use compatibility input.

### Portable richer-row frontend setting

- `apps/web/lib/types/http-user-settings.ts`,
  `apps/web/lib/types/backend.ts`, `apps/web/lib/state/slices/settings/types.ts`,
  `apps/web/lib/state/slices/settings/settings-slice.ts`, and
  `apps/web/lib/ssr/user-settings.ts`: map `tasks_list_show_details` to
  `tasksListShowDetails`, defaulting to `false`.
- `apps/web/lib/ssr/user-settings.test.ts` and the closest settings-slice tests:
  prove true mapping and the false default.
- `apps/web/hooks/use-user-display-settings.ts` and
  `apps/web/hooks/use-kanban-display-settings.ts`: carry, optimistically update,
  and PATCH the richer-row preference without dropping unrelated settings.
- `apps/web/components/kanban-display-dropdown.tsx`: accept the current page and
  show a self-documenting **Show task details** checkbox only for List.
- `apps/web/components/kanban/mobile-menu-sheet.tsx`: show the same checkbox
  only for List in the existing phone drawer/tablet sheet.

### Rich list rows

- `apps/web/app/tasks/tasks-page-client.tsx`: read the new setting, fetch
  workspace PR summaries while details are enabled, and pass the option into
  the list.
- `apps/web/app/tasks/tasks-list-view.tsx`: keep the compact row unchanged when
  disabled; when enabled, render a taller but still single-column row hierarchy
  with repository slug chips, truncated description, `PRTaskIcon`, session
  count, parent context, and review-attention state. Extract a focused sibling
  component/helper if needed to stay within file/function limits.
- Reuse `repositorySlug`, shared PR status UI, existing badges, and existing
  task navigation/action handlers. Missing metadata renders nothing.

## Mobile design contract

- **Desktop outcome:** existing topbar toggles restore the device's last
  Kanban/Pipeline/List view; List's display dropdown controls optional rich
  rows.
- **Mobile entry point:** the shipped topbar menu opens
  `MobileMenuSheet`; its existing View controls and Display Options section own
  both choices.
- **Nearest shipped exemplars:** `mobile-menu-sheet.tsx` supplies the
  safe-area-aware inset drawer and internal scroll owner;
  `mobile-task-list-search.spec.ts` supplies the compact list interaction;
  Kanban card content supplies metadata hierarchy.
- **Hierarchy and primary action:** task state and title remain first, repo/PR
  context second, description third, attention badges last. The row body opens
  the task; archive/delete remain explicit secondary controls.
- **Presentation:** temporary settings remain in the existing inset bottom
  drawer. Rich task data stays inline in the primary List surface rather than
  opening another overlay.
- **Geometry:** the drawer retains its `100dvh`, safe-area padding, and internal
  scroll. The task-list `<main>` remains the only page-content scroll owner;
  metadata truncates/wraps within the row and must not create document
  horizontal overflow.
- **Shared versus responsive logic:** preference, filtering, metadata
  resolution, and row actions are shared. Only spacing, truncation, and the
  Pipeline effective fallback vary by viewport.
- **Mobile proof:** Playwright selects List from the drawer, reloads Home to
  prove local restoration, enables details, verifies rich metadata and row
  navigation, and asserts no horizontal document overflow.

## Tests

- **One-workflow All Workflows:** unit regression in
  `resolve-workflow.test.ts`; browser regression in
  `e2e/tests/kanban/workflow-filter.spec.ts`.
- **Device preference parsing/fallback:** table-driven Vitest cases in
  `view-preference.test.ts` for valid, missing, malformed, legacy, and mobile
  Pipeline values.
- **Portable details setting:** Go DTO/service/store tests plus frontend SSR and
  settings-store mapping tests.
- **Rich metadata:** pure helper tests for repository/attention derivation where
  logic is extracted; rendered behavior is covered through Playwright rather
  than new React component tests.

## E2E Tests

- `apps/web/e2e/tests/kanban/workflow-filter.spec.ts`: with only the seeded
  workflow, choose **All Workflows**, close/reopen display options, reload, and
  verify it remains selected.
- `apps/web/e2e/tests/task/task-listing-view-preferences.spec.ts`: select
  Pipeline and List through desktop controls, reload Home, and verify the last
  device-local view; enable rich rows and verify repo, description, PR, and
  portable-setting reload behavior.
- `apps/web/e2e/tests/task/mobile-task-listing-display.spec.ts`: use the phone
  drawer to choose List, reopen Home to prove restoration, enable rich rows,
  assert metadata/row navigation, and assert no horizontal overflow. Also
  preseed Pipeline and prove the phone fallback does not overwrite it.

## Implementation waves

Wave 1 (parallel):

- [x] [Task 01 — Device-local view and workflow fix](task-01-device-view-and-workflow-fix.md)
- [x] [Task 02 — Portable richer-row setting contract](task-02-portable-rich-row-setting.md)

Wave 2:

- [x] [Task 03 — Rich list-row UI](task-03-rich-list-row-ui.md)

Wave 3:

- [x] [Task 04 — Desktop and mobile integration coverage](task-04-integration-e2e.md)

## Verification

- `make -C apps/backend fmt`
- `cd apps/backend && go test ./internal/user/... ./internal/backendapp/...`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/kanban/resolve-workflow.test.ts lib/task-listing/view-preference.test.ts lib/ssr/user-settings.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`
- `cd apps/web && pnpm e2e:run tests/kanban/workflow-filter.spec.ts tests/task/task-listing-view-preferences.spec.ts tests/task/mobile-task-listing-display.spec.ts`
- After the implementation commit, delegate final change-aware `verify` with
  the non-bypassed hook receipt.

## Risks

- Home restoration must avoid a `/` ↔ `/tasks` redirect loop and must not
  hijack task-detail or explicitly opened external routes.
- Hydrated backend `kanban_view_mode` can briefly disagree with the local
  preference; the local preference must become authoritative before the board
  chooses Pipeline/Kanban.
- User-settings payload builders carry many unrelated fields; the new boolean
  must not be dropped by optimistic updates or WS fallback writes.
- PR summary fetching should be enabled only when rich rows need it and must
  reuse the existing workspace-level cache/store.
