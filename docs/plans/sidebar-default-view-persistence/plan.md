---
spec: docs/specs/ui/requirements/sidebar-view-creation.md
created: 2026-08-05
status: complete
---

# Implementation Plan: Persist the Default Sidebar View

## Overview

Repair the clean-settings contract at its backend source of truth. The backend
will provide the canonical `All tasks` view and matching active ID whenever
stored user settings omit sidebar-view fields, while preserving explicit stored
values and the existing active-view validation. A focused HTTP integration test
and the existing sidebar browser suite will prove that the first filter edit no
longer returns HTTP 400.

## Confirmed root cause

`internal/user/store.defaultUserSettings` currently supplies an empty
`SidebarViews` slice and no active ID, while the frontend independently starts
with `view-all-tasks`. The first draft-only PATCH sends
`sidebar_active_view_id="view-all-tasks"` without `sidebar_views`, so
`applySidebarViewState` validates the ID against the backend's empty list and
rejects the request. Commit `b9eb6ae05` removed the legacy client migration
that previously populated the backend, exposing this mismatch on clean data.

This repair follows
[ADR 0041](../../decisions/0041-backend-owned-portable-user-settings.md): the
backend owns the effective value of omitted portable settings. It does not
reintroduce browser migration or relax referential validation.

---

## Backend

### Canonical sidebar defaults

- In `apps/backend/internal/user/store/sqlite.go`, add a fresh-value helper for
  the canonical sidebar view: ID `view-all-tasks`, name `All tasks`, empty
  filters, `state`/`asc` sort, `repository` grouping, and empty collapsed
  groups.
- Use fresh slices on each call so one request cannot mutate another user's
  defaults through shared backing arrays.
- Set both `SidebarViews` and `SidebarActiveViewID` in
  `defaultUserSettings`.
- Change the temporary JSON scan fields for `sidebar_views` and
  `sidebar_active_view_id` to preserve field presence. Overlay them only when
  they are present, so settings JSON such as `{}` or
  `{"workspace_id":"..."}` retains the backend default while explicit stored
  values remain authoritative.
- When a write explicitly replaces `sidebar_views` without an active-view
  field, preserve the referential invariant: an empty replacement resolves to
  the canonical default instead of persisting an active ID that has no view.
- Keep `applySidebarViewState` unchanged: non-default or stale active IDs must
  still be rejected unless they exist in the effective saved-view list.

### HTTP integration path

- Add `apps/backend/internal/user/handlers/handlers_test.go` using an in-memory
  SQLite repository, the real service/controller, and a Gin test router.
- Start from the repository-created default user whose settings JSON is `{}`.
- Assert `GET /api/v1/user/settings` returns the canonical view and matching
  active ID.
- PATCH only `sidebar_active_view_id` plus `sidebar_draft`, matching the current
  frontend edit request, and assert HTTP 200 plus durable read-back. This test
  must return HTTP 400 before the production change for the diagnosed reason.

---

## Frontend

No production frontend change is planned. The existing client PATCH shape and
optimistic behavior remain the consumer contract being verified.

### E2E fixture and scenario

- In `apps/web/e2e/fixtures/test-base.ts`, reset sidebar settings to the same
  canonical view and active ID rather than an impossible empty-view UI state.
  Reuse one fixture constant for both reset paths.
- In `apps/web/e2e/tests/task/sidebar-filter.spec.ts`, extend the new-user
  default-view scenario to edit the canonical view before any create/save-as
  action. Assert the draft reaches user settings, no `Sidebar views` error toast
  appears, and the view remains active after reload.
- Desktop and mobile use the same store action and backend route; this repair
  changes no layout, interaction target, or responsive composition. Existing
  mobile sidebar-view coverage remains applicable.

---

## Tests

- **What:** omitted sidebar fields resolve to a fresh canonical view and
  matching active ID, including when unrelated settings are present.
  **File:** `apps/backend/internal/user/store/sqlite_test.go`.
  **How:** table-driven `scanUserSettings` unit test covering `{}` and an
  unrelated-field JSON object, plus an explicit stored-view preservation case.
- **What:** the first draft PATCH succeeds through the real HTTP → controller →
  service → SQLite path without sending `sidebar_views`.
  **File:** `apps/backend/internal/user/handlers/handlers_test.go`.
  **How:** Gin/`httptest` integration test with the repository-created clean
  user; assert GET defaults, PATCH 200, and GET read-back.
- **What:** unmatched active IDs remain rejected.
  **File:** `apps/backend/internal/user/service/service_test.go`.
  **How:** retain and run the existing `TestApplySidebarViewState` regression
  coverage; no validation weakening is expected.
- **What:** an explicit empty `sidebar_views` replacement without an active ID
  retains a coherent canonical view.
  **File:** `apps/backend/internal/user/service/service_test.go` and
  `apps/backend/internal/user/handlers/handlers_test.go`.
  **How:** assert service normalization and real HTTP read-back before the
  first draft-only edit.

## E2E Tests

- **Scenario:** **GIVEN** a new user's canonical `All tasks` view, **WHEN** the
  first filter is edited, **THEN** the draft persists without a settings error
  and survives reload.
  **File:** `apps/web/e2e/tests/task/sidebar-filter.spec.ts`.
  **What to verify:** canonical view selection, persisted
  `sidebar_active_view_id`/`sidebar_draft`, absence of the error toast, and
  active-view continuity after reload.

---

## Verification Results

- `cd apps/backend && go test -tags fts5 -run 'TestScanUserSettingsSidebarDefaults|TestScanUserSettingsPreservesExplicitEmptySidebarSettings|TestHTTPUpdateSidebarDraftFromCleanSettings|TestApplySidebarViews|TestApplySidebarViewState' ./internal/user/store ./internal/user/handlers ./internal/user/service -v` — 23 tests passed across 3 packages.
- `cd apps/backend && go test -tags fts5 ./internal/user/...` — 227 tests passed across 6 packages.
- `cd apps && pnpm --filter @kandev/web run typecheck` — passed with no TypeScript errors.
- `cd apps && pnpm --filter @kandev/web test -- lib/state/slices/ui/ui-slice.test.ts -t "syncs filter sort and group drafts"` — 1 passed, 41 skipped.
- `cd apps && pnpm --filter @kandev/web lint -- e2e/fixtures/test-base.ts e2e/helpers/api-client.ts e2e/tests/task/sidebar-filter.spec.ts` — passed.
- `cd apps/web && pnpm e2e:run --host --project chromium tests/task/sidebar-filter.spec.ts -- --grep "default 'All tasks' view"` — 1 Playwright test passed in 5.3s against the rebuilt backend; the managed runner exited cleanly and left no E2E backend/browser processes.
- `git diff --check` — passed.

- PR #2293 ([implementation head `cdc4f3737408d0aeb9c24b7076d98db2b21342b3`](https://github.com/kdlbs/kandev/pull/2293)) — all 40 reported checks passed after the review fixup; subsequent documentation-only commits are pushed with no failures or unresolved review threads observed while their fresh checks run.

---

## Implementation Waves And Parallel Candidates

Sequential execution in the primary conversation:

- [x] [Task 01: Persist the canonical sidebar default](task-01-persist-canonical-sidebar-default.md) — done

This is one vertical fix spanning a shared persistence contract and its browser
regression. It is not parallel-safe and does not authorize subagents.

## Risks and boundaries

- The backend and frontend currently duplicate the canonical view identity and
  shape. The tests pin their agreement; introducing a generated/shared contract
  is out of scope for this repair.
- `All tasks` is existing persisted domain copy. This fix does not change
  localization behavior or rename existing views.
- Explicit non-empty sidebar arrays remain explicit values under ADR 0041;
  empty replacements without an active ID normalize to the canonical view so
  the backend cannot persist an invalid active/view pair.
- No schema migration, endpoint shape change, feature flag, or settings-data
  rewrite is required.
